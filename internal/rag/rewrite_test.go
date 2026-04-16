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
			want:    "WHAT is RAG?",
		},
		{
			name:    "referential follow-up prepends most recent history",
			query:   "  what about   it?  ",
			history: []string{"First topic", "   Explain Vector Database   "},
			want:    "Explain Vector Database what about it?",
		},
		{
			name:    "skip ambiguous recent history and use latest concrete one",
			query:   "how does it scale?",
			history: []string{"Explain Vector Database", "what about it?"},
			want:    "Explain Vector Database how does it scale?",
		},
		{
			name:    "leave referential query unchanged when history is ambiguous",
			query:   "what about it?",
			history: []string{"tell me more", "what about that?"},
			want:    "what about it?",
		},
		{
			name:    "demonstrative noun phrase is not treated as follow up",
			query:   "can this library work offline?",
			history: []string{"explain vector database"},
			want:    "can this library work offline?",
		},
		{
			name:    "plural demonstrative noun phrase is not treated as follow up",
			query:   "are these apis stable?",
			history: []string{"explain vector database"},
			want:    "are these apis stable?",
		},
		{
			name:    "demonstrative pronoun style follow up remains referential",
			query:   "what about this?",
			history: []string{"Explain Vector Database"},
			want:    "Explain Vector Database what about this?",
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
