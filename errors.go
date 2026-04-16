package ragagent

import "errors"

var (
	ErrInvalidConfig        = errors.New("ragagent: invalid config")
	ErrUnsupportedSource    = errors.New("ragagent: unsupported source")
	ErrAgentClosed          = errors.New("ragagent: agent closed")
	ErrSessionClosed        = errors.New("ragagent: session closed")
	ErrContextAssembly      = errors.New("ragagent: context assembly failed")
	ErrEvidenceInsufficient = errors.New("ragagent: evidence insufficient")
)
