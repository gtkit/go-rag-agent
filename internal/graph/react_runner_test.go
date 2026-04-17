package graph

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/gtkit/go-rag-agent/internal/memory"
)

type fakeChatModel struct {
	answer         string
	generateCalls  int
	lastGenerateIn []*schema.Message
}

func (f *fakeChatModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.generateCalls++
	f.lastGenerateIn = input
	return &schema.Message{
		Role:    schema.Assistant,
		Content: f.answer,
	}, nil
}

func (f *fakeChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close()
	return sr, nil
}

func (f *fakeChatModel) WithTools(_ []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

func TestBuildPromptMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		history      []memory.Turn
		evidence     string
		query        string
		wantRoles    []schema.RoleType
		wantContents []string
	}{
		{
			name: "keeps role-aware history evidence and query order",
			history: []memory.Turn{
				{User: "what is rag?", Assistant: "rag is retrieval-augmented generation"},
				{User: "what are chunks?", Assistant: "chunks are split text units"},
			},
			evidence: "retrieved evidence block",
			query:    "how does it work?",
			wantRoles: []schema.RoleType{
				schema.System,
				schema.User,
				schema.Assistant,
				schema.User,
				schema.Assistant,
				schema.User,
				schema.User,
			},
			wantContents: []string{
				"Answer with retrieved evidence first. If evidence is insufficient, use available tools. Prefer local retrieval before web search. If evidence is still insufficient, say so explicitly.",
				"what is rag?",
				"rag is retrieval-augmented generation",
				"what are chunks?",
				"chunks are split text units",
				"Relevant context:\nretrieved evidence block",
				"how does it work?",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildPromptMessages(tt.history, tt.evidence, tt.query)
			if len(got) != len(tt.wantRoles) {
				t.Fatalf("buildPromptMessages() len = %d, want %d", len(got), len(tt.wantRoles))
			}
			for i := range tt.wantRoles {
				if got[i].Role != tt.wantRoles[i] {
					t.Fatalf("message[%d].Role = %q, want %q", i, got[i].Role, tt.wantRoles[i])
				}
				if got[i].Content != tt.wantContents[i] {
					t.Fatalf("message[%d].Content = %q, want %q", i, got[i].Content, tt.wantContents[i])
				}
			}
		})
	}
}

func TestReactRunnerAskUsesPlainModelWhenEvidenceProvided(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		req          Request
		wantErr      bool
		wantErrSub   string
		wantAnswer   string
		wantGenCalls int
	}{
		{
			name: "uses plain model path when evidence exists",
			req: Request{
				Query:        "final question",
				History:      []memory.Turn{{User: "u1", Assistant: "a1"}},
				EvidenceText: "context block",
			},
			wantErr:      false,
			wantErrSub:   "",
			wantAnswer:   "model-only answer",
			wantGenCalls: 1,
		},
		{
			name: "requires react path when evidence empty",
			req: Request{
				Query:        "final question",
				History:      []memory.Turn{{User: "u1", Assistant: "a1"}},
				EvidenceText: "",
			},
			wantErr:      true,
			wantErrSub:   "react agent is required",
			wantAnswer:   "",
			wantGenCalls: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fm := &fakeChatModel{answer: "model-only answer"}
			runner := &ReactRunner{
				model: fm,
				agent: nil,
			}

			got, err := runner.Ask(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Ask() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErrSub != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErrSub)) {
				t.Fatalf("Ask() error = %v, want contains %q", err, tt.wantErrSub)
			}
			if got != tt.wantAnswer {
				t.Fatalf("Ask() answer = %q, want %q", got, tt.wantAnswer)
			}
			if fm.generateCalls != tt.wantGenCalls {
				t.Fatalf("model Generate calls = %d, want %d", fm.generateCalls, tt.wantGenCalls)
			}
		})
	}
}
