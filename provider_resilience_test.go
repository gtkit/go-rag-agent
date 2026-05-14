package ragagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gtkit/go-rag-agent/internal/llm"
	"github.com/gtkit/go-rag-agent/internal/websearch"
)

type flakyChatModel struct {
	generateCalls int
	streamCalls   int
	failTimes     int
	streamFail    bool
	answer        string
	info          map[string]any
}

func (m *flakyChatModel) Generate(context.Context, []llm.Message) (llm.Message, error) {
	m.generateCalls++
	if m.generateCalls <= m.failTimes {
		return llm.Message{}, errors.New("429 rate limit exceeded")
	}
	return llm.Message{
		Role:           llm.RoleAssistant,
		Content:        m.answer,
		GenerationInfo: m.info,
	}, nil
}

func (m *flakyChatModel) Stream(_ context.Context, _ []llm.Message, emit func(string) error) error {
	m.streamCalls++
	if m.streamFail {
		if err := emit("partial"); err != nil {
			return err
		}
		return errors.New("503 provider unavailable")
	}
	return emit(m.answer)
}

type sequencedChatModel struct {
	generateCalls int
	answer        string
	errs          []error
}

func (m *sequencedChatModel) Generate(context.Context, []llm.Message) (llm.Message, error) {
	call := m.generateCalls
	m.generateCalls++
	if call < len(m.errs) && m.errs[call] != nil {
		return llm.Message{}, m.errs[call]
	}
	return llm.Message{
		Role:    llm.RoleAssistant,
		Content: m.answer,
	}, nil
}

func (m *sequencedChatModel) Stream(_ context.Context, _ []llm.Message, emit func(string) error) error {
	return emit(m.answer)
}

type sequencedEmbedder struct {
	calls int
	errs  []error
}

func (e *sequencedEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	call := e.calls
	e.calls++
	if call < len(e.errs) && e.errs[call] != nil {
		return nil, e.errs[call]
	}
	rows := make([][]float32, 0, len(texts))
	for range texts {
		rows = append(rows, []float32{1})
	}
	return rows, nil
}

type sequencedSearcher struct {
	calls   int
	errs    []error
	results []websearch.Result
}

func (s *sequencedSearcher) Search(_ context.Context, query string) ([]websearch.Result, error) {
	call := s.calls
	s.calls++
	if call < len(s.errs) && s.errs[call] != nil {
		return nil, s.errs[call]
	}
	return s.results, nil
}

func TestProviderGovernanceConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     ProviderGovernanceConfig
		wantErr error
	}{
		{
			name:    "defaults normalize",
			cfg:     ProviderGovernanceConfig{},
			wantErr: nil,
		},
		{
			name: "invalid retry attempts",
			cfg: ProviderGovernanceConfig{
				RetryMaxAttempts: -1,
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "local rate limit defaults burst",
			cfg: ProviderGovernanceConfig{
				RateLimit: ProviderRateLimitConfig{
					RequestsPerSecond: 10,
					Burst:             0,
				},
			},
			wantErr: nil,
		},
		{
			name: "invalid circuit breaker threshold",
			cfg: ProviderGovernanceConfig{
				CircuitBreaker: ProviderCircuitBreakerConfig{
					FailureThreshold: -1,
				},
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "invalid circuit breaker open timeout",
			cfg: ProviderGovernanceConfig{
				CircuitBreaker: ProviderCircuitBreakerConfig{
					FailureThreshold: 1,
					OpenTimeout:      -1,
				},
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := tc.cfg.normalized()
			err := Config{
				ChatModel:          "gpt-4o-mini",
				ChatBaseURL:        "https://api.example.com/v1",
				ChatAPIKey:         "key",
				EmbeddingModel:     "text-embedding-3-small",
				ProviderGovernance: cfg,
			}.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, tc.wantErr)
			}
		})
	}
}

func TestResilientChatModelCircuitBreakerFastFailsAndRecovers(t *testing.T) {
	t.Parallel()

	model := &sequencedChatModel{
		answer: "answer",
		errs:   []error{errors.New("503 provider unavailable")},
	}
	governance := ProviderGovernanceConfig{
		RetryMaxAttempts: 1,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		CircuitBreaker: ProviderCircuitBreakerConfig{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		},
	}
	governors := newProviderGovernors(governance, nil)
	wrapped := newResilientChatModel(model, governance, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	_, err := wrapped.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if err == nil {
		t.Fatal("Generate() error = nil, want non-nil")
	}
	if model.generateCalls != 1 {
		t.Fatalf("generate calls after first failure = %d, want 1", model.generateCalls)
	}

	_, err = wrapped.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if !errors.Is(err, ErrProviderCircuitOpen) {
		t.Fatalf("Generate() error = %v, want errors.Is(..., %v)", err, ErrProviderCircuitOpen)
	}
	if model.generateCalls != 1 {
		t.Fatalf("generate calls after fast fail = %d, want 1", model.generateCalls)
	}

	time.Sleep(15 * time.Millisecond)

	msg, err := wrapped.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("Generate() after open timeout error = %v", err)
	}
	if msg.Content != "answer" {
		t.Fatalf("Generate() content = %q, want %q", msg.Content, "answer")
	}
	if model.generateCalls != 2 {
		t.Fatalf("generate calls after recovery = %d, want 2", model.generateCalls)
	}
}

func TestResilientProviderGovernorSharedAcrossChatAndEmbedder(t *testing.T) {
	t.Parallel()

	embedder := &sequencedEmbedder{
		errs: []error{errors.New("503 provider unavailable")},
	}
	chat := &sequencedChatModel{answer: "answer"}
	governance := ProviderGovernanceConfig{
		RetryMaxAttempts: 1,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		CircuitBreaker: ProviderCircuitBreakerConfig{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Second,
			HalfOpenMaxCalls: 1,
		},
	}
	governors := newProviderGovernors(governance, nil)
	wrappedEmbedder := newResilientEmbedder(embedder, governance, governors.forProvider("openai"), "openai", "text-embedding-3-small")
	wrappedChat := newResilientChatModel(chat, governance, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	if _, err := wrappedEmbedder.EmbedTexts(context.Background(), []string{"hello"}); err == nil {
		t.Fatal("EmbedTexts() error = nil, want non-nil")
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls = %d, want 1", embedder.calls)
	}
	sharedGovernor := governors.forProvider("openai")
	if sharedGovernor.circuitBreaker == nil {
		t.Fatal("shared governor circuit breaker = nil")
	}
	if sharedGovernor != governors.forProvider("openai") {
		t.Fatal("provider governors should return shared governor instance")
	}
	if sharedGovernor.circuitBreaker.state != providerCircuitStateOpen {
		t.Fatalf("shared governor circuit state = %q, want %q", sharedGovernor.circuitBreaker.state, providerCircuitStateOpen)
	}

	_, err := wrappedChat.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if !errors.Is(err, ErrProviderCircuitOpen) {
		t.Fatalf("Generate() error = %v, want errors.Is(..., %v)", err, ErrProviderCircuitOpen)
	}
	if chat.generateCalls != 0 {
		t.Fatalf("chat generate calls = %d, want 0", chat.generateCalls)
	}
}

func TestResilientSearcherRetriesAndTraces(t *testing.T) {
	t.Parallel()

	searcher := &sequencedSearcher{
		errs:    []error{errors.New("503 temporarily unavailable")},
		results: []websearch.Result{{Title: "Example", URL: "https://example.com", Content: "snippet"}},
	}
	var traces []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
		traces = append(traces, call)
	})
	governance := ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		Pricing: ProviderPricingConfig{
			WebSearchPerCallUSD: 0.02,
		},
	}
	governors := newProviderGovernors(governance, nil)
	wrapped := newResilientSearcher(searcher, governance, governors.forProvider("tavily"), "tavily")

	results, err := wrapped.Search(ctx, "rag")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(results))
	}
	if searcher.calls != 2 {
		t.Fatalf("Search() calls = %d, want 2", searcher.calls)
	}
	if len(traces) != 1 {
		t.Fatalf("provider traces len = %d, want 1", len(traces))
	}
	if traces[0].Operation != "web_search" || traces[0].Attempts != 2 || traces[0].EstimatedCostUSD != 0.02 {
		t.Fatalf("provider trace = %+v, want web_search attempts=2 cost=0.02", traces[0])
	}
}

func TestProviderErrorClassificationAndRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		provider  string
		wantClass string
		wantRetry bool
	}{
		{name: "nil", err: nil, wantClass: "", wantRetry: false},
		{name: "circuit", err: ErrProviderCircuitOpen, wantClass: providerErrorClassCircuit, wantRetry: false},
		{name: "canceled", err: context.Canceled, wantClass: "canceled", wantRetry: false},
		{name: "deadline", err: context.DeadlineExceeded, wantClass: providerErrorClassTransient, wantRetry: true},
		{name: "rate limit text", err: errors.New("429 too many requests"), wantClass: providerErrorClassRateLimit, wantRetry: true},
		{name: "auth text", err: errors.New("401 invalid api key"), wantClass: providerErrorClassAuth, wantRetry: false},
		{name: "transient text", err: errors.New("connection reset by peer"), wantClass: providerErrorClassTransient, wantRetry: true},
		{name: "permanent text", err: errors.New("400 invalid request"), wantClass: providerErrorClassPermanent, wantRetry: false},
		{name: "unknown text", err: errors.New("provider exploded"), wantClass: providerErrorClassUnknown, wantRetry: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotClass := classifyProviderError(tt.err, tt.provider)
			if gotClass != tt.wantClass {
				t.Fatalf("classifyProviderError() = %q, want %q", gotClass, tt.wantClass)
			}
			if gotRetry := shouldRetryProviderError(gotClass); gotRetry != tt.wantRetry {
				t.Fatalf("shouldRetryProviderError(%q) = %v, want %v", gotClass, gotRetry, tt.wantRetry)
			}
		})
	}
}

func TestProviderUsageAndCostHelpers(t *testing.T) {
	t.Parallel()

	usage := generationInfoUsage(map[string]any{
		"PromptTokens":     int32(10),
		"CompletionTokens": int64(5),
		"TotalTokens":      float64(15),
	})
	if usage.PromptTokens != 10 || usage.CompletionTokens != 5 || usage.TotalTokens != 15 {
		t.Fatalf("generationInfoUsage() = %+v, want 10/5/15", usage)
	}

	cfg := ProviderGovernanceConfig{
		Pricing: ProviderPricingConfig{
			ChatModels: map[string]TokenPricing{
				"chat": {InputUSDPer1K: 0.1, OutputUSDPer1K: 0.2},
			},
			EmbeddingModels: map[string]TokenPricing{
				"embed": {InputUSDPer1K: 0.01},
			},
		},
	}
	if got := estimateChatCost(cfg, "chat", 1000, 500); got != 0.2 {
		t.Fatalf("estimateChatCost() = %v, want 0.2", got)
	}
	if got := estimateEmbeddingCost(cfg, "embed", 1000); got != 0.01 {
		t.Fatalf("estimateEmbeddingCost() = %v, want 0.01", got)
	}
	if got := minDuration(time.Millisecond, 2*time.Millisecond); got != time.Millisecond {
		t.Fatalf("minDuration() = %v, want 1ms", got)
	}
}

func TestResilientChatModelTraceIncludesThrottleDelayAndCircuitState(t *testing.T) {
	t.Parallel()

	model := &sequencedChatModel{answer: "answer"}
	var traces []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
		traces = append(traces, call)
	})
	governance := ProviderGovernanceConfig{
		RetryMaxAttempts: 1,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		RateLimit: ProviderRateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             1,
		},
	}
	governors := newProviderGovernors(governance, nil)
	wrapped := newResilientChatModel(model, governance, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	for range 2 {
		if _, err := wrapped.Generate(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hello"}}); err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	}

	if len(traces) != 2 {
		t.Fatalf("provider traces len = %d, want 2", len(traces))
	}
	if traces[1].ThrottleDelay <= 0 {
		t.Fatalf("second trace throttle delay = %v, want > 0", traces[1].ThrottleDelay)
	}
	if traces[1].CircuitState != providerCircuitStateClosed {
		t.Fatalf("second trace circuit state = %q, want %q", traces[1].CircuitState, providerCircuitStateClosed)
	}
}

func TestProviderCircuitBreakerLogsStateTransitions(t *testing.T) {
	t.Parallel()

	logger := &loggerStub{}
	model := &sequencedChatModel{
		answer: "answer",
		errs:   []error{errors.New("503 provider unavailable")},
	}
	governance := ProviderGovernanceConfig{
		RetryMaxAttempts: 1,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		CircuitBreaker: ProviderCircuitBreakerConfig{
			FailureThreshold: 1,
			OpenTimeout:      10 * time.Millisecond,
			HalfOpenMaxCalls: 1,
		},
	}
	governors := newProviderGovernors(governance, logger)
	wrapped := newResilientChatModel(model, governance, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	if _, err := wrapped.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}); err == nil {
		t.Fatal("Generate() error = nil, want non-nil")
	}

	time.Sleep(15 * time.Millisecond)

	if _, err := wrapped.Generate(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}); err != nil {
		t.Fatalf("Generate() after open timeout error = %v", err)
	}

	entries := logger.snapshot()
	if len(entries) < 3 {
		t.Fatalf("logger entries len = %d, want >= 3", len(entries))
	}
	if entries[0].msg != "ragagent provider circuit state changed" || entries[0].kv[5] != providerCircuitStateOpen {
		t.Fatalf("first logger entry = %+v, want open transition", entries[0])
	}
	if entries[1].msg != "ragagent provider circuit state changed" || entries[1].kv[5] != providerCircuitStateHalfOpen {
		t.Fatalf("second logger entry = %+v, want half-open transition", entries[1])
	}
	if entries[2].msg != "ragagent provider circuit state changed" || entries[2].kv[5] != providerCircuitStateClosed {
		t.Fatalf("third logger entry = %+v, want closed transition", entries[2])
	}
}

func TestResilientChatModelRetriesAndAggregatesUsage(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{
		failTimes: 1,
		answer:    "answer",
		info: map[string]any{
			"PromptTokens":     100,
			"CompletionTokens": 50,
			"TotalTokens":      150,
		},
	}
	var traces []ProviderCallTrace
	ctx := withProviderTraceObserver(context.Background(), func(call ProviderCallTrace) {
		traces = append(traces, call)
	})
	governors := newProviderGovernors(ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		Pricing: ProviderPricingConfig{
			ChatModels: map[string]TokenPricing{
				"gpt-4o-mini": {InputUSDPer1K: 0.01, OutputUSDPer1K: 0.02},
			},
		},
	}, nil)
	wrapped := newResilientChatModel(model, ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
		Pricing: ProviderPricingConfig{
			ChatModels: map[string]TokenPricing{
				"gpt-4o-mini": {InputUSDPer1K: 0.01, OutputUSDPer1K: 0.02},
			},
		},
	}, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	msg, err := wrapped.Generate(ctx, []llm.Message{{Role: llm.RoleUser, Content: "hello"}})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "answer" {
		t.Fatalf("Generate() content = %q, want %q", msg.Content, "answer")
	}
	if model.generateCalls != 2 {
		t.Fatalf("generate calls = %d, want 2", model.generateCalls)
	}
	if len(traces) != 1 {
		t.Fatalf("provider traces len = %d, want 1", len(traces))
	}
	if traces[0].Attempts != 2 {
		t.Fatalf("provider trace attempts = %d, want 2", traces[0].Attempts)
	}
	if traces[0].TotalTokens != 150 {
		t.Fatalf("provider trace total tokens = %d, want 150", traces[0].TotalTokens)
	}
	if traces[0].EstimatedCostUSD <= 0 {
		t.Fatalf("provider trace estimated cost = %f, want positive", traces[0].EstimatedCostUSD)
	}
}

func TestResilientChatModelStreamDoesNotRetryAfterChunk(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{streamFail: true}
	governors := newProviderGovernors(ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
	}, nil)
	wrapped := newResilientChatModel(model, ProviderGovernanceConfig{
		RetryMaxAttempts: 2,
		RetryBaseDelay:   time.Millisecond,
		RetryMaxDelay:    5 * time.Millisecond,
	}, governors.forProvider("openai"), "openai", "gpt-4o-mini")

	err := wrapped.Stream(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hello"}}, func(string) error { return nil })
	if err == nil {
		t.Fatal("Stream() error = nil, want non-nil")
	}
	if model.streamCalls != 1 {
		t.Fatalf("stream calls = %d, want 1", model.streamCalls)
	}
}

func TestAskExecutionTraceIncludesProviderUsage(t *testing.T) {
	t.Parallel()

	model := &flakyChatModel{
		answer: "answer",
		info: map[string]any{
			"PromptTokens":     100,
			"CompletionTokens": 50,
			"TotalTokens":      150,
		},
	}
	a, err := New(Config{
		ChatModel: "gpt-4o-mini",
		Runtime: RuntimeComponents{
			ChatModel: model,
			Embedder:  stubRuntimeEmbedder{},
		},
		TopK:             1,
		ChunkSize:        32,
		ChunkOverlap:     0,
		MaxHistoryRounds: 8,
		RequestTimeout:   time.Second,
		ProviderGovernance: ProviderGovernanceConfig{
			RetryMaxAttempts: 1,
			RetryBaseDelay:   time.Millisecond,
			RetryMaxDelay:    5 * time.Millisecond,
			Pricing: ProviderPricingConfig{
				ChatModels: map[string]TokenPricing{
					"gpt-4o-mini": {InputUSDPer1K: 0.01, OutputUSDPer1K: 0.02},
				},
			},
		},
		Storage: StorageComponents{
			VectorStore: &injectedVectorStoreStub{
				searchHits: []SearchHit{
					{
						Chunk: ChunkRecord{
							ChunkID:    "doc:0",
							ParentID:   "doc",
							SourcePath: "/tmp/doc.md",
							Title:      "doc",
							Text:       "gateway api exact match",
							StartRune:  0,
							EndRune:    23,
						},
						Score: 0.99,
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() {
		if cerr := a.Close(); cerr != nil {
			t.Fatalf("Close() error = %v", cerr)
		}
	})

	answer, err := a.GetSession("provider-usage").Ask(context.Background(), "gateway api")
	if err != nil {
		t.Fatalf("Ask() error = %v", err)
	}
	if answer.Trace == nil {
		t.Fatal("Answer.Trace = nil")
	}
	if len(answer.Trace.ProviderCalls) == 0 {
		t.Fatal("ProviderCalls len = 0, want > 0")
	}
	call := answer.Trace.ProviderCalls[len(answer.Trace.ProviderCalls)-1]
	if call.TotalTokens != 150 {
		t.Fatalf("ProviderCalls total tokens = %d, want 150", call.TotalTokens)
	}
	if call.EstimatedCostUSD <= 0 {
		t.Fatalf("ProviderCalls estimated cost = %f, want positive", call.EstimatedCostUSD)
	}
}
