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
	fields := strings.Fields(strings.ToLower(input))
	return strings.Join(fields, " ")
}

func looksReferential(query string) bool {
	referentialTerms := []string{
		"it", "that", "this", "they", "them", "those", "these", "he", "she", "him", "her", "former", "latter",
	}
	words := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	})
	for _, word := range words {
		for _, term := range referentialTerms {
			if word == term {
				return true
			}
		}
	}
	return false
}

func isConcreteHistoryItem(item string) bool {
	if looksReferential(item) {
		return false
	}
	ambiguousPhrases := map[string]struct{}{
		"tell me more": {},
		"go on":        {},
		"continue":     {},
		"more details": {},
	}
	if _, ok := ambiguousPhrases[item]; ok {
		return false
	}
	words := strings.FieldsFunc(item, func(r rune) bool {
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
