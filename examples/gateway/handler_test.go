package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ragagent "github.com/gtkit/go-rag-agent"
)

type stubGatewaySession struct {
	gotQuery string
	answer   ragagent.Answer
	err      error
}

func (s *stubGatewaySession) Ask(_ context.Context, query string) (ragagent.Answer, error) {
	s.gotQuery = query
	return s.answer, s.err
}

type stubGatewayAgent struct {
	session *stubGatewaySession
	gotID   string
}

func (a *stubGatewayAgent) GetSession(id string) gatewaySession {
	a.gotID = id
	return a.session
}

func TestChatCompletionsHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantSession string
		wantQuery   string
		wantContent string
		sessionErr  error
	}{
		{
			name: "routes last user message to session",
			body: `{
				"model":"demo",
				"user":"customer-42",
				"messages":[
					{"role":"system","content":"answer briefly"},
					{"role":"user","content":"what is gateway?"}
				]
			}`,
			wantStatus:  http.StatusOK,
			wantSession: "customer-42",
			wantQuery:   "what is gateway?",
			wantContent: "gateway answer",
		},
		{
			name:        "defaults empty user and model",
			body:        `{"messages":[{"role":"user","content":"hello"}]}`,
			wantStatus:  http.StatusOK,
			wantSession: "default",
			wantQuery:   "hello",
			wantContent: "gateway answer",
		},
		{
			name:       "rejects wrong method",
			body:       `{}`,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "rejects invalid json",
			body:       `{`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "rejects streaming",
			body:       `{"stream":true,"messages":[{"role":"user","content":"hello"}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "rejects empty user message",
			body:       `{"messages":[{"role":"assistant","content":"hello"}]}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns ask error",
			body:       `{"messages":[{"role":"user","content":"hello"}]}`,
			wantStatus: http.StatusInternalServerError,
			sessionErr: errors.New("model failed"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := &stubGatewaySession{answer: ragagent.Answer{Text: "gateway answer"}, err: tc.sessionErr}
			agent := &stubGatewayAgent{session: session}
			handler := NewChatCompletionsHandler(agent)

			method := http.MethodPost
			if tc.name == "rejects wrong method" {
				method = http.MethodGet
			}
			req := httptest.NewRequest(method, "/v1/chat/completions", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				return
			}
			if agent.gotID != tc.wantSession {
				t.Fatalf("session id = %q, want %q", agent.gotID, tc.wantSession)
			}
			if session.gotQuery != tc.wantQuery {
				t.Fatalf("query = %q, want %q", session.gotQuery, tc.wantQuery)
			}
			var response chatCompletionResponse
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("Decode response error = %v", err)
			}
			if len(response.Choices) != 1 || !strings.Contains(response.Choices[0].Message.Content, tc.wantContent) {
				t.Fatalf("response = %+v, want content containing %q", response, tc.wantContent)
			}
		})
	}
}
