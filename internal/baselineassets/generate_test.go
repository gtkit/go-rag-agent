package baselineassets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	ragagent "github.com/gtkit/go-rag-agent"
)

func TestGenerateSDKPositioningBaseline(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		wantPassedCases      int
		wantCitationSource   string
		wantCitationChunkID  string
		wantRetrievalHit     int
		wantCitationCount    int
		wantDuration         int64
		wantTraceQuery       string
		wantTraceSuccess     bool
		wantTraceCost        float64
		wantTraceToolCalls   int
		wantTraceProviderNum int
	}{
		{
			name:                 "writes deterministic eval and trace baseline files",
			wantPassedCases:      1,
			wantCitationSource:   "testdata/baseline/overview.md",
			wantCitationChunkID:  "testdata/baseline/overview.md:0",
			wantRetrievalHit:     1,
			wantCitationCount:    1,
			wantDuration:         0,
			wantTraceQuery:       "summarize the positioning baseline",
			wantTraceSuccess:     true,
			wantTraceCost:        0,
			wantTraceToolCalls:   1,
			wantTraceProviderNum: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			outDir := filepath.Join(t.TempDir(), "baseline")
			if err := GenerateSDKPositioningBaseline(t.Context(), outDir); err != nil {
				t.Fatalf("GenerateSDKPositioningBaseline() error = %v", err)
			}

			report, err := ragagent.ReadEvalReportJSON(filepath.Join(outDir, "eval-report.json"))
			if err != nil {
				t.Fatalf("ReadEvalReportJSON() error = %v", err)
			}
			if got := report.Summary.PassedCases; got != tt.wantPassedCases {
				t.Fatalf("report.Summary.PassedCases = %d, want %d", got, tt.wantPassedCases)
			}
			if len(report.Results) != 1 {
				t.Fatalf("len(report.Results) = %d, want 1", len(report.Results))
			}
			if len(report.Results[0].Answer.Citations) != tt.wantCitationCount {
				t.Fatalf("len(report.Results[0].Answer.Citations) = %d, want %d", len(report.Results[0].Answer.Citations), tt.wantCitationCount)
			}
			if got := report.Results[0].Answer.Citations[0].SourcePath; got != tt.wantCitationSource {
				t.Fatalf("report.Results[0].Answer.Citations[0].SourcePath = %q, want %q", got, tt.wantCitationSource)
			}
			if got := report.Results[0].Answer.Citations[0].ChunkID; got != tt.wantCitationChunkID {
				t.Fatalf("report.Results[0].Answer.Citations[0].ChunkID = %q, want %q", got, tt.wantCitationChunkID)
			}

			var summary ragagent.ExecutionTraceSummary
			dataPath := filepath.Join(outDir, "trace-summary.json")
			data, err := os.ReadFile(dataPath)
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", dataPath, err)
			}
			if err := json.Unmarshal(data, &summary); err != nil {
				t.Fatalf("json.Unmarshal(trace summary) error = %v", err)
			}
			if got := summary.Query; got != tt.wantTraceQuery {
				t.Fatalf("summary.Query = %q, want %q", got, tt.wantTraceQuery)
			}
			if got := summary.Success; got != tt.wantTraceSuccess {
				t.Fatalf("summary.Success = %v, want %v", got, tt.wantTraceSuccess)
			}
			if got := summary.RetrievalHitCount; got != tt.wantRetrievalHit {
				t.Fatalf("summary.RetrievalHitCount = %d, want %d", got, tt.wantRetrievalHit)
			}
			if got := summary.CitationCount; got != tt.wantCitationCount {
				t.Fatalf("summary.CitationCount = %d, want %d", got, tt.wantCitationCount)
			}
			if got := summary.Duration.Nanoseconds(); got != tt.wantDuration {
				t.Fatalf("summary.Duration = %d, want %d", got, tt.wantDuration)
			}
			if got := summary.TotalEstimatedCostUSD; got != tt.wantTraceCost {
				t.Fatalf("summary.TotalEstimatedCostUSD = %v, want %v", got, tt.wantTraceCost)
			}
			if got := summary.ToolCallCount; got != tt.wantTraceToolCalls {
				t.Fatalf("summary.ToolCallCount = %d, want %d", got, tt.wantTraceToolCalls)
			}
			if got := summary.ProviderCallCount; got != tt.wantTraceProviderNum {
				t.Fatalf("summary.ProviderCallCount = %d, want %d", got, tt.wantTraceProviderNum)
			}
		})
	}
}
