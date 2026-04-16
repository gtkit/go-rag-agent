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
)
