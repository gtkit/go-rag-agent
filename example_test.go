package ragagent_test

import (
	"errors"
	"fmt"

	ragagent "my-gtkit-package/go-rag-agent"
)

func ExampleConfig_Validate() {
	cfg := ragagent.Config{
		ChatBaseURL:    "https://api.openai.example/v1",
		ChatAPIKey:     "placeholder-chat-key",
		EmbeddingModel: "text-embedding-3-small",
	}

	err := cfg.Validate()
	fmt.Println(errors.Is(err, ragagent.ErrInvalidConfig))

	// Output: true
}
