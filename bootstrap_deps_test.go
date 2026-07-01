package ragagent

// 这是 Task 1.1 的测试期依赖锚点，在真实引用稳定落地前用于保留依赖。
import (
	_ "github.com/gtkit/go-llm-provider/v2/provider"
	_ "github.com/gtkit/json"
	_ "github.com/philippgille/chromem-go"
)
