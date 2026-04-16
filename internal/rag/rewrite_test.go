package rag

import "testing"

func TestRewriteFollowUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		history []string
		want    string
	}{
		{
			name:    "standalone query normalization",
			query:   "   WHAT   is   RAG?   ",
			history: nil,
			want:    "what is rag?",
		},
		{
			name:    "referential follow-up prepends most recent history",
			query:   "  what about   it?  ",
			history: []string{"First topic", "   Explain vector database   "},
			want:    "explain vector database what about it?",
		},
		{
			name:    "skip ambiguous recent history and use latest concrete one",
			query:   "how does it scale?",
			history: []string{"explain vector database", "what about it?"},
			want:    "explain vector database how does it scale?",
		},
		{
			name:    "leave referential query unchanged when history is ambiguous",
			query:   "what about it?",
			history: []string{"tell me more", "what about that?"},
			want:    "what about it?",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := RewriteFollowUp(tc.query, tc.history)
			if got != tc.want {
				t.Fatalf("RewriteFollowUp() = %q, want %q", got, tc.want)
			}
		})
	}
}
