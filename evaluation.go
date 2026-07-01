package ragagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	// ExpectRefusal 为 true 时，本用例以"实际拒答"为通过条件，
	// 用于评测不可回答类问题；此时不再计算 recall/precision/groundedness/answer-match。
	ExpectRefusal bool
}

// EvalResult 描述一条评测用例的结果。
type EvalResult struct {
	Name              string
	Answer            Answer
	RetrievalRecall   float64
	CitationPrecision float64
	Groundedness      float64
	AnswerMatch       bool
	// Refused 标识本用例运行时系统是否拒答。
	Refused bool
	Passed  bool
	Err     error
}

// EvalSummary 汇总一批评测结果。
type EvalSummary struct {
	TotalCases        int
	PassedCases       int
	RetrievalRecall   float64
	CitationPrecision float64
	Groundedness      float64
	AnswerMatchRate   float64
	// RefusalAccuracy 是期望拒答用例中被正确拒答的比例；
	// 当没有期望拒答用例时定为 1.0。
	RefusalAccuracy float64
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

// StructuredEvalSummary 汇总结构化输出评测结果。
type StructuredEvalSummary struct {
	TotalCases                int
	PassedCases               int
	StructuredOutputValidRate float64
}

// EvalReport 表示一次完整评测报告。
type EvalReport struct {
	Summary           EvalSummary
	Results           []EvalResult
	StructuredSummary StructuredEvalSummary
	StructuredResults []StructuredEvalResult
}

// EvalThresholds 定义评测门禁阈值。
type EvalThresholds struct {
	MinRetrievalRecall           float64
	MinCitationPrecision         float64
	MinGroundedness              float64
	MinAnswerMatchRate           float64
	MinStructuredOutputValidRate float64
	MinRefusalAccuracy           float64
	MaxFailures                  int
}

type answerJSON struct {
	Text      string     `json:"text"`
	Citations []Citation `json:"citations,omitempty"`
}

type evalResultJSON struct {
	Name              string     `json:"name"`
	Answer            answerJSON `json:"answer"`
	RetrievalRecall   float64    `json:"retrieval_recall"`
	CitationPrecision float64    `json:"citation_precision"`
	Groundedness      float64    `json:"groundedness"`
	AnswerMatch       bool       `json:"answer_match"`
	Refused           bool       `json:"refused,omitempty"`
	Passed            bool       `json:"passed"`
	Err               string     `json:"err,omitempty"`
}

type structuredResultJSON struct {
	Name                  string `json:"name"`
	RawJSON               string `json:"raw_json"`
	StructuredOutputValid bool   `json:"structured_output_valid"`
	Passed                bool   `json:"passed"`
	Err                   string `json:"err,omitempty"`
}

type evalReportJSON struct {
	Summary           EvalSummary            `json:"summary"`
	Results           []evalResultJSON       `json:"results"`
	StructuredSummary StructuredEvalSummary  `json:"structured_summary"`
	StructuredResults []structuredResultJSON `json:"structured_results"`
}

// isRefusalError 判定一个问答错误是否属于系统拒答（证据不足或工具调用受限）。
func isRefusalError(err error) bool {
	return errors.Is(err, ErrEvidenceInsufficient) || errors.Is(err, ErrToolCallLimitExceeded)
}

// RunEvalSuite 执行一组同步问答评测。
//
// 对于标注 ExpectRefusal 的用例，通过条件为"系统实际拒答"，不计入
// recall/precision/groundedness/answer-match 的均值；其余用例沿用既有判定。
func RunEvalSuite(ctx context.Context, agent *Agent, cases []EvalCase) (EvalSummary, []EvalResult, error) {
	results := make([]EvalResult, 0, len(cases))
	summary := EvalSummary{TotalCases: len(cases)}
	answerableCount := 0
	refusalExpectedCount := 0
	refusalCorrect := 0
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
		result.Refused = isRefusalError(err)

		if c.ExpectRefusal {
			refusalExpectedCount++
			result.Passed = result.Refused
			if result.Passed {
				refusalCorrect++
			}
		} else {
			answerableCount++
			if err == nil {
				result.RetrievalRecall = scoreRecall(answer.Citations, c.WantCitationSources)
				result.CitationPrecision = scorePrecision(answer.Citations, c.WantCitationSources)
				result.Groundedness = scoreGroundedness(answer.Text, c.WantGroundedSubstrings)
				result.AnswerMatch = containsAll(answer.Text, c.WantAnswerContains)
				result.Passed = result.RetrievalRecall >= 1 && result.CitationPrecision >= 1 && result.Groundedness >= 1 && result.AnswerMatch
			}
			summary.RetrievalRecall += result.RetrievalRecall
			summary.CitationPrecision += result.CitationPrecision
			summary.Groundedness += result.Groundedness
			if result.AnswerMatch {
				summary.AnswerMatchRate++
			}
		}

		if result.Passed {
			summary.PassedCases++
		}
		results = append(results, result)
	}
	if answerableCount > 0 {
		n := float64(answerableCount)
		summary.RetrievalRecall /= n
		summary.CitationPrecision /= n
		summary.Groundedness /= n
		summary.AnswerMatchRate /= n
	}
	if refusalExpectedCount > 0 {
		summary.RefusalAccuracy = float64(refusalCorrect) / float64(refusalExpectedCount)
	} else {
		summary.RefusalAccuracy = 1
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

// SummarizeStructuredEvalResults 汇总结构化输出评测结果。
func SummarizeStructuredEvalResults(results []StructuredEvalResult) StructuredEvalSummary {
	summary := StructuredEvalSummary{TotalCases: len(results)}
	for _, result := range results {
		if result.Passed {
			summary.PassedCases++
		}
		if result.StructuredOutputValid {
			summary.StructuredOutputValidRate++
		}
	}
	if len(results) > 0 {
		summary.StructuredOutputValidRate /= float64(len(results))
	}
	return summary
}

// MarshalEvalReportJSON 将评测报告编码为 JSON。
func MarshalEvalReportJSON(report EvalReport) ([]byte, error) {
	payload := newEvalReportJSON(report)
	return json.MarshalIndent(payload, "", "  ")
}

// WriteEvalReportJSON 将评测报告写入指定 JSON 文件。
func WriteEvalReportJSON(path string, report EvalReport) error {
	data, err := MarshalEvalReportJSON(report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir eval report dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write eval report json %q: %w", path, err)
	}
	return nil
}

// ReadEvalReportJSON 读取一个评测报告 JSON 文件。
func ReadEvalReportJSON(path string) (EvalReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalReport{}, fmt.Errorf("read eval report json %q: %w", path, err)
	}
	var payload evalReportJSON
	if err := json.Unmarshal(data, &payload); err != nil {
		return EvalReport{}, fmt.Errorf("unmarshal eval report json %q: %w", path, err)
	}
	return payload.toReport(), nil
}

func newEvalReportJSON(report EvalReport) evalReportJSON {
	payload := evalReportJSON{
		Summary:           report.Summary,
		StructuredSummary: report.StructuredSummary,
		Results:           make([]evalResultJSON, 0, len(report.Results)),
		StructuredResults: make([]structuredResultJSON, 0, len(report.StructuredResults)),
	}
	for _, result := range report.Results {
		payload.Results = append(payload.Results, evalResultJSON{
			Name: result.Name,
			Answer: answerJSON{
				Text:      result.Answer.Text,
				Citations: result.Answer.Citations,
			},
			RetrievalRecall:   result.RetrievalRecall,
			CitationPrecision: result.CitationPrecision,
			Groundedness:      result.Groundedness,
			AnswerMatch:       result.AnswerMatch,
			Refused:           result.Refused,
			Passed:            result.Passed,
			Err:               errString(result.Err),
		})
	}
	for _, result := range report.StructuredResults {
		payload.StructuredResults = append(payload.StructuredResults, structuredResultJSON{
			Name:                  result.Name,
			RawJSON:               result.Answer.RawJSON,
			StructuredOutputValid: result.StructuredOutputValid,
			Passed:                result.Passed,
			Err:                   errString(result.Err),
		})
	}
	return payload
}

func (payload evalReportJSON) toReport() EvalReport {
	report := EvalReport{
		Summary:           payload.Summary,
		StructuredSummary: payload.StructuredSummary,
		Results:           make([]EvalResult, 0, len(payload.Results)),
		StructuredResults: make([]StructuredEvalResult, 0, len(payload.StructuredResults)),
	}
	for _, result := range payload.Results {
		report.Results = append(report.Results, EvalResult{
			Name: result.Name,
			Answer: Answer{
				Text:      result.Answer.Text,
				Citations: slices.Clone(result.Answer.Citations),
			},
			RetrievalRecall:   result.RetrievalRecall,
			CitationPrecision: result.CitationPrecision,
			Groundedness:      result.Groundedness,
			AnswerMatch:       result.AnswerMatch,
			Refused:           result.Refused,
			Passed:            result.Passed,
			Err:               stringToError(result.Err),
		})
	}
	for _, result := range payload.StructuredResults {
		report.StructuredResults = append(report.StructuredResults, StructuredEvalResult{
			Name:                  result.Name,
			Answer:                StructuredAnswer{RawJSON: result.RawJSON},
			StructuredOutputValid: result.StructuredOutputValid,
			Passed:                result.Passed,
			Err:                   stringToError(result.Err),
		})
	}
	return report
}

func stringToError(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return errors.New(value)
}

// CheckEvalThresholds 校验评测摘要是否满足阈值。
func CheckEvalThresholds(report EvalReport, thresholds EvalThresholds) error {
	if thresholds.MaxFailures >= 0 {
		failures := report.Summary.TotalCases - report.Summary.PassedCases
		if failures > thresholds.MaxFailures {
			return fmt.Errorf("eval failures %d exceed max %d", failures, thresholds.MaxFailures)
		}
	}
	if report.Summary.RetrievalRecall < thresholds.MinRetrievalRecall {
		return fmt.Errorf("retrieval recall %.3f is below min %.3f", report.Summary.RetrievalRecall, thresholds.MinRetrievalRecall)
	}
	if report.Summary.CitationPrecision < thresholds.MinCitationPrecision {
		return fmt.Errorf("citation precision %.3f is below min %.3f", report.Summary.CitationPrecision, thresholds.MinCitationPrecision)
	}
	if report.Summary.Groundedness < thresholds.MinGroundedness {
		return fmt.Errorf("groundedness %.3f is below min %.3f", report.Summary.Groundedness, thresholds.MinGroundedness)
	}
	if report.Summary.AnswerMatchRate < thresholds.MinAnswerMatchRate {
		return fmt.Errorf("answer match rate %.3f is below min %.3f", report.Summary.AnswerMatchRate, thresholds.MinAnswerMatchRate)
	}
	if report.StructuredSummary.StructuredOutputValidRate < thresholds.MinStructuredOutputValidRate {
		return fmt.Errorf("structured output valid rate %.3f is below min %.3f", report.StructuredSummary.StructuredOutputValidRate, thresholds.MinStructuredOutputValidRate)
	}
	if report.Summary.RefusalAccuracy < thresholds.MinRefusalAccuracy {
		return fmt.Errorf("refusal accuracy %.3f is below min %.3f", report.Summary.RefusalAccuracy, thresholds.MinRefusalAccuracy)
	}
	return nil
}

// CompareEvalReportWithBaseline 校验当前评测摘要不低于基线。
func CompareEvalReportWithBaseline(report EvalReport, baseline EvalReport) error {
	return CheckEvalThresholds(report, EvalThresholds{
		MinRetrievalRecall:           baseline.Summary.RetrievalRecall,
		MinCitationPrecision:         baseline.Summary.CitationPrecision,
		MinGroundedness:              baseline.Summary.Groundedness,
		MinAnswerMatchRate:           baseline.Summary.AnswerMatchRate,
		MinStructuredOutputValidRate: baseline.StructuredSummary.StructuredOutputValidRate,
		MinRefusalAccuracy:           baseline.Summary.RefusalAccuracy,
		MaxFailures:                  baseline.Summary.TotalCases - baseline.Summary.PassedCases,
	})
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

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
