package ragagent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MemoryFailurePolicy controls how memory provider retrieval errors affect a request.
type MemoryFailurePolicy string

const (
	// MemoryFailurePolicyFailClosed returns memory provider retrieval errors to the caller.
	MemoryFailurePolicyFailClosed MemoryFailurePolicy = "fail_closed"
	// MemoryFailurePolicyFailOpen records retrieval errors and continues with empty memory.
	MemoryFailurePolicyFailOpen MemoryFailurePolicy = "fail_open"
)

// MemoryRetrieveRequest describes a memory retrieval request before prompt construction.
type MemoryRetrieveRequest struct {
	SessionID string
	Query     string
	Scope     MemoryScope
	History   []Message
}

// MemoryRetrieveResult is context returned by a MemoryProvider.
type MemoryRetrieveResult struct {
	MemoryText string
	Messages   []Message
}

// MemoryMemorizeRequest describes a memory write after a successful answer.
type MemoryMemorizeRequest struct {
	SessionID string
	Scope     MemoryScope
	User      string
	Assistant string
}

// MemoryProvider retrieves and stores memory around an answer generation lifecycle.
type MemoryProvider interface {
	Retrieve(ctx context.Context, req MemoryRetrieveRequest) (MemoryRetrieveResult, error)
	Memorize(ctx context.Context, req MemoryMemorizeRequest) error
	Close() error
}

// LongTermMemoryProviderConfig configures the LongTermMemoryStore adapter.
type LongTermMemoryProviderConfig struct {
	TopK      int
	Threshold float64
	TTL       time.Duration
	MaxRunes  int
}

type longTermMemoryProvider struct {
	store    LongTermMemoryStore
	embedder Embedder
	cfg      LongTermMemoryProviderConfig
}

// NewLongTermMemoryProvider adapts an existing LongTermMemoryStore into a MemoryProvider.
func NewLongTermMemoryProvider(store LongTermMemoryStore, embedder Embedder, cfg LongTermMemoryProviderConfig) MemoryProvider {
	if cfg.TopK == 0 {
		cfg.TopK = 3
	}
	if cfg.MaxRunes == 0 {
		cfg.MaxRunes = 512
	}
	return &longTermMemoryProvider{store: store, embedder: embedder, cfg: cfg}
}

func (p *longTermMemoryProvider) Retrieve(ctx context.Context, req MemoryRetrieveRequest) (MemoryRetrieveResult, error) {
	if p == nil || p.store == nil || p.embedder == nil || p.cfg.TopK <= 0 {
		return MemoryRetrieveResult{}, nil
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return MemoryRetrieveResult{}, nil
	}
	rows, err := p.embedder.EmbedTexts(ctx, []string{query})
	if err != nil {
		return MemoryRetrieveResult{}, fmt.Errorf("embed memory query: %w", err)
	}
	if len(rows) != 1 || len(rows[0]) == 0 {
		return MemoryRetrieveResult{}, fmt.Errorf("memory query embedding is empty")
	}
	hits, err := p.store.Search(ctx, req.SessionID, rows[0], p.cfg.TopK, float32(p.cfg.Threshold))
	if err != nil {
		return MemoryRetrieveResult{}, fmt.Errorf("search memory: %w", err)
	}
	hits = filterLongTermMemoryHits(hits, req.Scope, time.Now())
	if len(hits) == 0 {
		return MemoryRetrieveResult{}, nil
	}
	var builder strings.Builder
	for i, hit := range hits {
		if i > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("- User: ")
		builder.WriteString(strings.TrimSpace(hit.Memory.User))
		builder.WriteString("\n  Assistant: ")
		builder.WriteString(strings.TrimSpace(hit.Memory.Assistant))
	}
	return MemoryRetrieveResult{MemoryText: builder.String()}, nil
}

func (p *longTermMemoryProvider) Memorize(ctx context.Context, req MemoryMemorizeRequest) error {
	if p == nil || p.store == nil || p.embedder == nil {
		return nil
	}
	compressedUser, compressedAssistant := compressLongTermMemoryTurn(req.User, req.Assistant, p.cfg.MaxRunes)
	content := strings.TrimSpace(compressedUser) + "\n" + strings.TrimSpace(compressedAssistant)
	rows, err := p.embedder.EmbedTexts(ctx, []string{content})
	if err != nil {
		return fmt.Errorf("embed memory record: %w", err)
	}
	if len(rows) != 1 || len(rows[0]) == 0 {
		return fmt.Errorf("memory record embedding is empty")
	}
	now := time.Now()
	expiresAt := time.Time{}
	if p.cfg.TTL > 0 {
		expiresAt = now.Add(p.cfg.TTL)
	}
	return p.store.Store(ctx, []LongTermMemoryRecord{{
		ID:        buildLongTermMemoryID(req.SessionID, req.Scope, compressedUser, compressedAssistant),
		SessionID: req.SessionID,
		UserID:    req.Scope.UserID,
		Tenant:    req.Scope.Tenant,
		User:      compressedUser,
		Assistant: compressedAssistant,
		Embedding: rows[0],
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}})
}

func (p *longTermMemoryProvider) Close() error {
	if p == nil || p.store == nil {
		return nil
	}
	return p.store.Close()
}
