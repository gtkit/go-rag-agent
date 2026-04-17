package llm

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Role 表示聊天消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message 表示聊天模型输入输出消息。
type Message struct {
	Role    Role
	Content string
}

// ChatModel 表示当前项目使用的聊天模型抽象。
type ChatModel interface {
	Generate(ctx context.Context, input []Message) (Message, error)
	Stream(ctx context.Context, input []Message, emit func(string) error) error
}

// ChatConfig 保存聊天模型适配器的构造配置。
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

// Validate 检查聊天模型配置是否完整且有效。
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
