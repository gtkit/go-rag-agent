package graph

import "testing"

func TestBuildPromptMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		history             []string
		evidence            string
		query               string
		wantCount           int
		wantFirstContent    string
		wantHistory0Content string
		wantHistory1Content string
		wantEvidenceContent string
		wantLastContent     string
	}{
		{
			name:                "keeps system history evidence query order",
			history:             []string{"what is rag?", "what are chunks?"},
			evidence:            "retrieved evidence block",
			query:               "how does it work?",
			wantCount:           5,
			wantFirstContent:    "Answer with retrieved evidence first. If evidence is insufficient, say so explicitly.",
			wantHistory0Content: "what is rag?",
			wantHistory1Content: "what are chunks?",
			wantEvidenceContent: "Retrieved evidence:\nretrieved evidence block",
			wantLastContent:     "how does it work?",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildPromptMessages(tt.history, tt.evidence, tt.query)
			if len(got) != tt.wantCount {
				t.Fatalf("buildPromptMessages() len = %d, want %d", len(got), tt.wantCount)
			}
			if got[0].Content != tt.wantFirstContent {
				t.Fatalf("message[0] = %q, want %q", got[0].Content, tt.wantFirstContent)
			}
			if got[1].Content != tt.wantHistory0Content {
				t.Fatalf("message[1] = %q, want %q", got[1].Content, tt.wantHistory0Content)
			}
			if got[2].Content != tt.wantHistory1Content {
				t.Fatalf("message[2] = %q, want %q", got[2].Content, tt.wantHistory1Content)
			}
			if got[3].Content != tt.wantEvidenceContent {
				t.Fatalf("message[3] = %q, want %q", got[3].Content, tt.wantEvidenceContent)
			}
			if got[4].Content != tt.wantLastContent {
				t.Fatalf("message[4] = %q, want %q", got[4].Content, tt.wantLastContent)
			}
		})
	}
}
