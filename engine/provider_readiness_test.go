package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestStatusAndDoctorUseRoutedVisionProvider(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider Provider
		want     bool
	}{
		{"local", Provider{ID: "vision", Kind: "local", APIBaseURL: "http://localhost:11434/v1", Model: "gemma3:4b", Enabled: true}, true},
		{"provider key", Provider{ID: "vision", Kind: "openrouter", APIKey: "test", Model: "google/gemma-4-31b-it", Enabled: true}, true},
		{"missing key", Provider{ID: "vision", Kind: "openrouter", Model: "google/gemma-4-31b-it", Enabled: true}, false},
		{"disabled", Provider{ID: "vision", Kind: "local", APIBaseURL: "http://localhost:11434/v1", Model: "gemma3:4b"}, false},
		{"missing endpoint", Provider{ID: "vision", Kind: "local", Model: "gemma3:4b", Enabled: true}, false},
		{"missing model", Provider{ID: "vision", Kind: "local", APIBaseURL: "http://localhost:11434/v1", Enabled: true}, false},
		{"cli", Provider{ID: "vision", Kind: "cli", Command: "fixture-cli", AllowHotPath: true, Enabled: true}, true},
		{"cli without hot path", Provider{ID: "vision", Kind: "cli", Command: "fixture-cli", Enabled: true}, false},
		{"missing cli", Provider{ID: "vision", Kind: "cli", Command: "missing-cli", AllowHotPath: true, Enabled: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testEnv(t)
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "fixture-cli"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			cfg.OpenRouterAPIKey = ""
			cfg.APIBaseURL = ""
			cfg.Providers = []Provider{tc.provider}
			cfg.Routing = Routing{Primary: "vision"}
			if err := writeConfig(cfg); err != nil {
				t.Fatal(err)
			}
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			old := os.Stdout
			os.Stdout = w
			printStatus(cfg, true)
			os.Stdout = old
			w.Close()
			output, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			var status struct {
				Configured bool `json:"configured"`
			}
			if err := json.Unmarshal(output, &status); err != nil {
				t.Fatal(err)
			}
			if status.Configured != tc.want {
				t.Errorf("status configured=%v, want %v", status.Configured, tc.want)
			}
			checks, _ := collectDoctorChecks(cfg, false)
			check := findCheck(checks, "api reachable")
			if check == nil || (check.Status == "ok") != tc.want {
				t.Errorf("doctor provider check=%+v, want configured=%v", check, tc.want)
			}
		})
	}
}
