package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderTestAgentTasksUseChatRoutes(t *testing.T) {
	for _, task := range []string{"agent_recap", "agent_briefing"} {
		for _, override := range []bool{false, true} {
			name := task + "/chat fallback"
			if override {
				name = task + "/override"
			}
			t.Run(name, func(t *testing.T) {
				testEnv(t)
				got := ""
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request orRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						return
					}
					got = request.Model
					w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
				}))
				defer srv.Close()
				cfg := defaultConfig()
				cfg.Providers = []Provider{
					{ID: "primary", Kind: "local", APIBaseURL: srv.URL, Model: "primary-model", Enabled: true},
					{ID: "chat", Kind: "local", APIBaseURL: srv.URL, Model: "chat-model", Enabled: true},
					{ID: "agent", Kind: "local", APIBaseURL: srv.URL, Model: "agent-model", Enabled: true},
				}
				cfg.Routing = Routing{Primary: "primary", TaskProvider: map[string]string{"chat": "chat"}}
				want := "chat-model"
				if override {
					cfg.Routing.TaskProvider[task] = "agent"
					want = "agent-model"
				}
				if err := runProvider(cfg, []string{"test", task}, false); err != nil {
					t.Fatal(err)
				}
				if got != want {
					t.Fatalf("model=%q, want %q", got, want)
				}
			})
		}
	}
}
