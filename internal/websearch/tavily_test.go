package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gtkit/httpc"
)

func TestTavilyClientSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		statusCode    int
		responseBody  string
		wantErr       bool
		wantErrSub    string
		wantTitle     string
		wantURL       string
		wantAuth      string
		wantQuery     string
		wantDepth     string
		wantMaxResult float64
	}{
		{
			name:          "success",
			statusCode:    http.StatusOK,
			responseBody:  `{"results":[{"title":"Example","url":"https://example.com","content":"snippet"}]}`,
			wantErr:       false,
			wantTitle:     "Example",
			wantURL:       "https://example.com",
			wantAuth:      "Bearer tvly-test",
			wantQuery:     "what is rag",
			wantDepth:     "basic",
			wantMaxResult: 5,
		},
		{
			name:          "non 2xx returns error",
			statusCode:    http.StatusUnauthorized,
			responseBody:  `{"error":"unauthorized"}`,
			wantErr:       true,
			wantErrSub:    "unexpected status",
			wantAuth:      "Bearer tvly-test",
			wantQuery:     "what is rag",
			wantDepth:     "basic",
			wantMaxResult: 5,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotAuth, gotQuery, gotDepth string
			var gotMaxResults float64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				gotQuery, _ = payload["query"].(string)
				gotDepth, _ = payload["search_depth"].(string)
				gotMaxResults, _ = payload["max_results"].(float64)
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.responseBody))
			}))
			defer srv.Close()

			client := NewTavilyClient(httpc.New(httpc.WithTimeout(5*time.Second)), TavilyConfig{
				BaseURL:     srv.URL,
				APIKey:      "tvly-test",
				MaxResults:  5,
				SearchDepth: "basic",
				Topic:       "general",
			})

			results, err := client.Search(context.Background(), "what is rag")
			if (err != nil) != tc.wantErr {
				t.Fatalf("Search() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErrSub != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErrSub)) {
				t.Fatalf("Search() error = %v, want contains %q", err, tc.wantErrSub)
			}
			if !tc.wantErr {
				if len(results) != 1 {
					t.Fatalf("Search() len = %d, want 1", len(results))
				}
				if results[0].Title != tc.wantTitle || results[0].URL != tc.wantURL {
					t.Fatalf("Search() result = %+v", results[0])
				}
			}
			if gotAuth != tc.wantAuth {
				t.Fatalf("Authorization = %q, want %q", gotAuth, tc.wantAuth)
			}
			if gotQuery != tc.wantQuery {
				t.Fatalf("query = %q, want %q", gotQuery, tc.wantQuery)
			}
			if gotDepth != tc.wantDepth {
				t.Fatalf("search_depth = %q, want %q", gotDepth, tc.wantDepth)
			}
			if gotMaxResults != tc.wantMaxResult {
				t.Fatalf("max_results = %v, want %v", gotMaxResults, tc.wantMaxResult)
			}
		})
	}
}
