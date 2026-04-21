package ragagent

import "errors"

var (
	// ErrInvalidConfig indicates the input configuration violates contract rules.
	ErrInvalidConfig = errors.New("ragagent: invalid config")
	// ErrUnsupportedSource indicates the requested knowledge source type is unsupported.
	ErrUnsupportedSource = errors.New("ragagent: unsupported source")
	// ErrAgentClosed indicates the agent has been closed and cannot accept more work.
	ErrAgentClosed = errors.New("ragagent: agent closed")
	// ErrSessionClosed indicates the session has been closed and cannot continue.
	ErrSessionClosed = errors.New("ragagent: session closed")
	// ErrContextAssembly indicates context assembly failed before model execution.
	ErrContextAssembly = errors.New("ragagent: context assembly failed")
	// ErrEvidenceInsufficient indicates retrieved evidence is insufficient to answer.
	ErrEvidenceInsufficient = errors.New("ragagent: evidence insufficient")
	// ErrToolCallLimitExceeded indicates tool execution exceeded configured max calls.
	ErrToolCallLimitExceeded = errors.New("ragagent: tool call limit exceeded")
	// ErrStructuredOutputTarget indicates structured output target is invalid.
	ErrStructuredOutputTarget = errors.New("ragagent: structured output target is invalid")
	// ErrStructuredOutputInvalid indicates model output could not be parsed into structured JSON.
	ErrStructuredOutputInvalid = errors.New("ragagent: structured output is invalid")
	// ErrExecutionBudgetExceeded indicates total execution exceeded configured budget.
	ErrExecutionBudgetExceeded = errors.New("ragagent: execution budget exceeded")
)
