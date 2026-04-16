package llm

import (
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
)

// ChatModel represents a tool-capable chat model.
type ChatModel = model.ToolCallingChatModel

// ChatConfig holds construction config for chat model adapters.
type ChatConfig struct {
	Model   string
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Validate checks whether the chat config is complete and valid.
func (c ChatConfig) Validate() error {
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("chat model is required")
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("chat base url is required")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("chat api key is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("chat timeout must be positive")
	}
	return nil
}
