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

// ProviderRateLimitConfig 定义 provider 级本地限流配置。
type ProviderRateLimitConfig struct {
	RequestsPerSecond float64
	Burst             int
}

func (c ProviderRateLimitConfig) normalized() ProviderRateLimitConfig {
	if c.RequestsPerSecond > 0 && c.Burst == 0 {
		c.Burst = 1
	}
	return c
}

func (c ProviderRateLimitConfig) enabled() bool {
	return c.RequestsPerSecond > 0
}

// ProviderCircuitBreakerConfig 定义 provider 级断路器配置。
type ProviderCircuitBreakerConfig struct {
	FailureThreshold int
	OpenTimeout      time.Duration
	HalfOpenMaxCalls int
}

func (c ProviderCircuitBreakerConfig) normalized() ProviderCircuitBreakerConfig {
	if c.enabled() && c.HalfOpenMaxCalls == 0 {
		c.HalfOpenMaxCalls = 1
	}
	return c
}

func (c ProviderCircuitBreakerConfig) enabled() bool {
	return c.FailureThreshold > 0 || c.OpenTimeout > 0 || c.HalfOpenMaxCalls > 0
}

// ProviderGovernanceConfig 定义 provider 级治理配置。
type ProviderGovernanceConfig struct {
	RetryMaxAttempts int
	RetryBaseDelay   time.Duration
	RetryMaxDelay    time.Duration
	RateLimit        ProviderRateLimitConfig
	CircuitBreaker   ProviderCircuitBreakerConfig
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
	c.RateLimit = c.RateLimit.normalized()
	c.CircuitBreaker = c.CircuitBreaker.normalized()
	return c
}

func (c ProviderGovernanceConfig) chatPrice(model string) TokenPricing {
	return c.Pricing.ChatModels[model]
}

func (c ProviderGovernanceConfig) embeddingPrice(model string) TokenPricing {
	return c.Pricing.EmbeddingModels[model]
}
