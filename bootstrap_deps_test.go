package ragagent

// Test-only bootstrap anchor for Task 1.1 dependencies until concrete usage lands.
// Note: components/embedding/openai@latest currently resolves to the latest upstream
// pseudo-version because that module does not publish a tagged release yet.
import (
	_ "github.com/cloudwego/eino-ext/components/embedding/openai"
	_ "github.com/cloudwego/eino-ext/components/model/openai"
	_ "github.com/gtkit/json"
	_ "github.com/gtkit/logger"
	_ "github.com/philippgille/chromem-go"
)
