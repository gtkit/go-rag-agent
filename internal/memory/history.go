package memory

import "sync"

// Turn stores one user-assistant exchange.
type Turn struct {
	User      string
	Assistant string
}

// History keeps bounded conversation turns for one session.
type History struct {
	mu        sync.RWMutex
	maxRounds int
	turns     []Turn
}

// NewHistory creates bounded session history with maxRounds capacity.
func NewHistory(maxRounds int) *History {
	return &History{
		maxRounds: maxRounds,
		turns:     make([]Turn, 0, max(maxRounds, 0)),
	}
}

// Append adds one turn and trims oldest turns when over capacity.
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

// Clear removes all stored turns.
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.turns = h.turns[:0]
}

// Turns returns a copy of current stored turns.
func (h *History) Turns() []Turn {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]Turn(nil), h.turns...)
}

// LastUserQueries returns user texts in stored order.
func (h *History) LastUserQueries() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	queries := make([]string, 0, len(h.turns))
	for _, turn := range h.turns {
		queries = append(queries, turn.User)
	}
	return queries
}
