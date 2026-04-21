package ragagent

import "time"

// TokenPricing 定义每千 token 的估算价格。
type TokenPricing struct {
	InputUSDPer1K  float64
	OutputUSDPer1K float64
}

// ProviderPricingConfig 定义 provider 使用量成本估算配置。
type ProviderPricingConfig struct {
	ChatModels          map[string]TokenPricing
	EmbeddingModels     map[string]TokenPricing
	WebSearchPerCallUSD float64
}

// ProviderGovernanceConfig 定义 provider 级治理配置。
type ProviderGovernanceConfig struct {
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	RetryMaxDelay    time.Duration
	Pricing          ProviderPricingConfig
}

func (c ProviderGovernanceConfig) normalized() ProviderGovernanceConfig {
	if c.RetryMaxAttempts == 0 {
		c.RetryMaxAttempts = 2
	}
	if c.RetryBaseDelay == 0 {
		c.RetryBaseDelay = 200 * time.Millisecond
	}
	if c.RetryMaxDelay == 0 {
		c.RetryMaxDelay = 2 * time.Second
	}
	return c
}

func (c ProviderGovernanceConfig) chatPrice(model string) TokenPricing {
	return c.Pricing.ChatModels[model]
}

func (c ProviderGovernanceConfig) embeddingPrice(model string) TokenPricing {
	return c.Pricing.EmbeddingModels[model]
}
