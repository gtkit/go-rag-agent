package memory

import "sync"

// Turn 表示一次用户与助手的对话往返。
type Turn struct {
	User      string
	Assistant string
}

// History 为单个 Session 保存有界对话历史。
type History struct {
	mu        sync.RWMutex
	maxRounds int
	turns     []Turn
}

// NewHistory 创建一个按最大轮次限制的会话历史。
func NewHistory(maxRounds int) *History {
	return &History{
		maxRounds: maxRounds,
		turns:     make([]Turn, 0, max(maxRounds, 0)),
	}
}

// Append 追加一轮对话，并在超限时裁剪最旧内容。
func (h *History) Append(user, assistant string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.maxRounds <= 0 {
		h.turns = h.turns[:0]
		return
	}

	h.turns = append(h.turns, Turn{
		User:      user,
		Assistant: assistant,
	})
	if len(h.turns) > h.maxRounds {
		start := len(h.turns) - h.maxRounds
		h.turns = append([]Turn(nil), h.turns[start:]...)
	}
}

// Clear 清空当前保存的所有对话轮次。
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.turns = make([]Turn, 0, max(h.maxRounds, 0))
}

// Turns 返回当前历史轮次的拷贝。
func (h *History) Turns() []Turn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]Turn(nil), h.turns...)
}

// LastUserQueries 按存储顺序返回用户侧提问文本。
func (h *History) LastUserQueries() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	queries := make([]string, 0, len(h.turns))
	for _, turn := range h.turns {
		queries = append(queries, turn.User)
	}
	return queries
}
