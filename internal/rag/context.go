package rag

import (
	"errors"
	"fmt"
	"strings"
)

var errInvalidContextLimit = errors.New("invalid max chars")
var errNoAssembledContext = errors.New("no chunks assembled")

func AssembleContext(chunks []Chunk, maxChars int) ([]Chunk, string, error) {
	if maxChars <= 0 {
		return nil, "", fmt.Errorf("%w: %d", errInvalidContextLimit, maxChars)
	}

	seen := make(map[string]struct{}, len(chunks))
	kept := make([]Chunk, 0, len(chunks))
	var builder strings.Builder

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
		if len([]rune(builder.String()+next)) > maxChars {
			if len(kept) > 0 {
				break
			}
			continue
		}
		builder.WriteString(next)
		kept = append(kept, chunk)
	}

	if len(kept) == 0 {
		return nil, "", fmt.Errorf("%w: maxChars=%d", errNoAssembledContext, maxChars)
	}
	return kept, builder.String(), nil
}
