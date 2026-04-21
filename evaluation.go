package ragagent

import (
	"context"
	"strings"
)

// EvalCase 定义一次同步问答评测用例。
type EvalCase struct {
	Name                   string
	SessionID              string
	Query                  string
	Options                QueryOptions
	WantCitationSources    []string
	WantAnswerContains     []string
	WantGroundedSubstrings []string
}

// EvalResult 描述一条评测用例的结果。
type EvalResult struct {
	Name              string
	Answer            Answer
	RetrievalRecall   float64
	CitationPrecision float64
	Groundedness      float64
	AnswerMatch       bool
	Passed            bool
	Err               error
}

// EvalSummary 汇总一批评测结果。
type EvalSummary struct {
	TotalCases        int
	PassedCases       int
	RetrievalRecall   float64
	CitationPrecision float64
	Groundedness      float64
	AnswerMatchRate   float64
}

// StructuredEvalCase 定义一次结构化输出评测用例。
type StructuredEvalCase struct {
	Name      string
	SessionID string
	Query     string
	Options   QueryOptions
	NewTarget func() any
	Validate  func(target any) error
}

// StructuredEvalResult 描述一次结构化输出评测结果。
type StructuredEvalResult struct {
	Name                  string
	Answer                StructuredAnswer
	StructuredOutputValid bool
	Passed                bool
	Err                   error
}

// RunEvalSuite 执行一组同步问答评测。
func RunEvalSuite(ctx context.Context, agent *Agent, cases []EvalCase) (EvalSummary, []EvalResult, error) {
	results := make([]EvalResult, 0, len(cases))
	summary := EvalSummary{TotalCases: len(cases)}
	for _, c := range cases {
		result := EvalResult{Name: c.Name}
		if agent == nil {
			result.Err = ErrInvalidConfig
			results = append(results, result)
			continue
		}
		session := agent.GetSession(defaultString(c.SessionID, c.Name))
		answer, err := session.AskWithOptions(ctx, c.Query, c.Options)
		result.Answer = answer
		result.Err = err
		if err == nil {
			result.RetrievalRecall = scoreRecall(answer.Citations, c.WantCitationSources)
			result.CitationPrecision = scorePrecision(answer.Citations, c.WantCitationSources)
			result.Groundedness = scoreGroundedness(answer.Text, c.WantGroundedSubstrings)
			result.AnswerMatch = containsAll(answer.Text, c.WantAnswerContains)
			result.Passed = result.RetrievalRecall >= 1 && result.CitationPrecision >= 1 && result.Groundedness >= 1 && result.AnswerMatch
		}
		if result.Passed {
			summary.PassedCases++
		}
		summary.RetrievalRecall += result.RetrievalRecall
		summary.CitationPrecision += result.CitationPrecision
		summary.Groundedness += result.Groundedness
		if result.AnswerMatch {
			summary.AnswerMatchRate++
		}
		results = append(results, result)
	}
	if len(cases) > 0 {
		n := float64(len(cases))
		summary.RetrievalRecall /= n
		summary.CitationPrecision /= n
		summary.Groundedness /= n
		summary.AnswerMatchRate /= n
	}
	return summary, results, nil
}

// RunStructuredEvalSuite 执行一组结构化输出评测。
func RunStructuredEvalSuite(ctx context.Context, agent *Agent, cases []StructuredEvalCase) ([]StructuredEvalResult, error) {
	results := make([]StructuredEvalResult, 0, len(cases))
	for _, c := range cases {
		result := StructuredEvalResult{Name: c.Name}
		if agent == nil || c.NewTarget == nil {
			result.Err = ErrInvalidConfig
			results = append(results, result)
			continue
		}
		target := c.NewTarget()
		answer, err := agent.GetSession(defaultString(c.SessionID, c.Name)).AskStructuredWithOptions(ctx, c.Query, c.Options, target)
		result.Answer = answer
		result.Err = err
		if err == nil {
			result.StructuredOutputValid = c.Validate == nil || c.Validate(target) == nil
			result.Passed = result.StructuredOutputValid
		}
		results = append(results, result)
	}
	return results, nil
}

func scoreRecall(citations []Citation, wantSources []string) float64 {
	if len(wantSources) == 0 {
		return 1
	}
	matched := 0
	for _, source := range wantSources {
		for _, citation := range citations {
			if citation.SourcePath == source {
				matched++
				break
			}
		}
	}
	return float64(matched) / float64(len(wantSources))
}

func scorePrecision(citations []Citation, wantSources []string) float64 {
	if len(citations) == 0 {
		if len(wantSources) == 0 {
			return 1
		}
		return 0
	}
	if len(wantSources) == 0 {
		return 0
	}
	matched := 0
	for _, citation := range citations {
		for _, source := range wantSources {
			if citation.SourcePath == source {
				matched++
				break
			}
		}
	}
	return float64(matched) / float64(len(citations))
}

func scoreGroundedness(answer string, groundedSubstrings []string) float64 {
	if len(groundedSubstrings) == 0 {
		return 1
	}
	matched := 0
	for _, want := range groundedSubstrings {
		if strings.Contains(answer, want) {
			matched++
		}
	}
	return float64(matched) / float64(len(groundedSubstrings))
}

func containsAll(answer string, wants []string) bool {
	for _, want := range wants {
		if !strings.Contains(answer, want) {
			return false
		}
	}
	return true
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
