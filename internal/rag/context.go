package rag

import (
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidContextLimit = errors.New("rag: invalid max chars")
var ErrNoContextAssembled = errors.New("rag: no chunks assembled")

func AssembleContext(chunks []Chunk, maxChars int) ([]Chunk, string, error) {
	if maxChars <= 0 {
		return nil, "", fmt.Errorf("%w: %d", ErrInvalidContextLimit, maxChars)
	}

	seen := make(map[string]struct{}, len(chunks))
	kept := make([]Chunk, 0, len(chunks))
	var builder strings.Builder
	contextRuneCount := 0

	for _, chunk := range chunks {
		if _, ok := seen[chunk.ChunkID]; ok {
			continue
		}
		seen[chunk.ChunkID] = struct{}{}

		segment := fmt.Sprintf("[%s] %s", chunk.ChunkID, chunk.Text)
		next := segment
		if builder.Len() > 0 {
			next = "\n\n" + segment
		}
		nextRuneCount := len([]rune(next))
		if contextRuneCount+nextRuneCount > maxChars {
			if len(kept) > 0 {
				break
			}
			continue
		}
		builder.WriteString(next)
		contextRuneCount += nextRuneCount
		kept = append(kept, chunk)
	}

	if len(kept) == 0 {
		return nil, "", fmt.Errorf("%w: maxChars=%d", ErrNoContextAssembled, maxChars)
	}
	return kept, builder.String(), nil
}
