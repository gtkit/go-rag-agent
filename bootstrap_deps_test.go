package ragagent

// 这是 Task 1.1 的测试期依赖锚点，在真实引用稳定落地前用于保留依赖。
// 注意：components/embedding/openai@latest 当前会解析到上游最新 pseudo-version，
// 因为该模块暂时没有发布带 tag 的稳定版本。
import (
	_ "github.com/cloudwego/eino-ext/components/embedding/openai"
	_ "github.com/cloudwego/eino-ext/components/model/openai"
	_ "github.com/gtkit/json"
	_ "github.com/gtkit/logger"
	_ "github.com/philippgille/chromem-go"
)
