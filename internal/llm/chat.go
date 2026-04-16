package llm

import (
	"fmt"
	"net/url"
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

func (c ChatConfig) normalized() ChatConfig {
	c.Model = strings.TrimSpace(c.Model)
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.APIKey = strings.TrimSpace(c.APIKey)
	return c
}

// Validate checks whether the chat config is complete and valid.
func (c ChatConfig) Validate() error {
	c = c.normalized()

	if c.Model == "" {
		return fmt.Errorf("chat model is required")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("chat base url is required")
	}
	if _, err := parseAndValidateBaseURL(c.BaseURL); err != nil {
		return fmt.Errorf("chat base url is invalid: %w", err)
	}
	if c.APIKey == "" {
		return fmt.Errorf("chat api key is required")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("chat timeout must be positive")
	}
	return nil
}

func parseAndValidateBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("base url must include scheme and host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base url scheme must be http or https")
	}
	return parsed, nil
}
