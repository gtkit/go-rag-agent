package rag

import (
	"strings"
	"unicode"
)

func RewriteFollowUp(query string, history []string) string {
	normalizedQuery := normalizeQueryText(query)
	if normalizedQuery == "" {
		return ""
	}

	if !looksReferential(normalizedQuery) {
		return normalizedQuery
	}

	for i := len(history) - 1; i >= 0; i-- {
		item := normalizeQueryText(history[i])
		if item == "" {
			continue
		}
		if !isConcreteHistoryItem(item) {
			continue
		}
		return item + " " + normalizedQuery
	}
	return normalizedQuery
}

func normalizeQueryText(input string) string {
	fields := strings.Fields(strings.TrimSpace(input))
	return strings.Join(fields, " ")
}

func looksReferential(query string) bool {
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	for i, word := range words {
		switch word {
		case "it", "they", "them", "he", "she", "him", "her", "former", "latter":
			return true
		case "this", "that", "these", "those":
			if isDemonstrativePronounUsage(words, i) {
				return true
			}
		}
	}
	return false
}

func isDemonstrativePronounUsage(words []string, idx int) bool {
	if idx < 0 || idx >= len(words) {
		return false
	}
	if idx == len(words)-1 {
		return true
	}

	next := words[idx+1]
	pronounFollowers := map[string]struct{}{
		"about": {}, "is": {}, "are": {}, "was": {}, "were": {}, "am": {}, "be": {}, "been": {}, "being": {},
		"do": {}, "does": {}, "did": {}, "can": {}, "could": {}, "will": {}, "would": {}, "should": {},
		"may": {}, "might": {}, "must": {}, "has": {}, "have": {}, "had": {}, "one": {}, "ones": {},
		"to": {}, "for": {}, "of": {}, "with": {}, "in": {}, "on": {}, "at": {}, "as": {},
	}
	_, ok := pronounFollowers[next]
	return ok
}

func isConcreteHistoryItem(item string) bool {
	lowerItem := strings.ToLower(item)
	if looksReferential(lowerItem) {
		return false
	}
	ambiguousPhrases := map[string]struct{}{
		"tell me more": {},
		"go on":        {},
		"continue":     {},
		"more details": {},
	}
	if _, ok := ambiguousPhrases[lowerItem]; ok {
		return false
	}
	words := strings.FieldsFunc(lowerItem, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	if len(words) == 0 {
		return false
	}
	nonInformative := map[string]struct{}{
		"tell": {}, "me": {}, "more": {}, "go": {}, "on": {}, "continue": {}, "details": {},
		"what": {}, "about": {}, "please": {}, "the": {}, "a": {}, "an": {},
	}
	for _, word := range words {
		if _, ok := nonInformative[word]; !ok {
			return true
		}
	}
	return false
}
