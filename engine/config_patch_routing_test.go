package main

import (
	"testing"
)

func TestConfigPatchUpdatesActiveProvider(t *testing.T) {
	for _, withProviders := range []bool{false, true} {
		name := "legacy only"
		if withProviders {
			name = "panel round trip"
		}
		t.Run(name, func(t *testing.T) {
			cfg := testEnv(t)
			t.Setenv("PATH", t.TempDir())
			cfg.Providers = []Provider{
				{ID: "primary", Kind: "openrouter", APIKey: "old-key", Model: "old-model", Enabled: true},
				{ID: "other", Kind: "local", APIBaseURL: "http://localhost:1234/v1", Model: "other-model", Enabled: true},
			}
			cfg.Routing = Routing{Primary: "primary"}
			if err := writeConfig(cfg); err != nil {
				t.Fatal(err)
			}
			patch := `{"provider":"local","model":"new-model","api_base_url":"http://localhost:11434","openrouter_api_key":""}`
			if withProviders {
				patch = `{"provider":"local","model":"new-model","api_base_url":"http://localhost:11434","openrouter_api_key":"","providers":[{"id":"primary","kind":"openrouter","api_key":"***redacted***","model":"old-model","enabled":true},{"id":"other","kind":"local","api_base_url":"http://localhost:1234/v1","model":"other-model","enabled":true}]}`
			}
			if err := patchConfig(patch); err != nil {
				t.Fatal(err)
			}
			got, err := loadConfig()
			if err != nil {
				t.Fatal(err)
			}
			p, err := providerForTask(got, "vision")
			if err != nil {
				t.Fatal(err)
			}
			if p.Kind != "local" || p.Model != "new-model" || p.APIBaseURL != "http://localhost:11434/v1" || p.APIKey != "" {
				t.Fatalf("active provider=%+v", p)
			}
			other := findProvider(got, "other")
			if other == nil || other.Model != "other-model" || other.APIBaseURL != "http://localhost:1234/v1" {
				t.Fatalf("other provider changed: %+v", other)
			}
		})
	}
}

func TestConfigPatchPreservesExplicitProviderEdits(t *testing.T) {
	cfg := testEnv(t)
	t.Setenv("PATH", t.TempDir())
	cfg.Model = "legacy-model"
	cfg.Providers = []Provider{{ID: "primary", Kind: "local", APIBaseURL: "http://localhost:11434/v1", Model: "old-model", Enabled: true}}
	cfg.Routing.Primary = "primary"
	if err := writeConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := patchConfig(`{"model":"legacy-change","providers":[{"id":"primary","kind":"local","api_base_url":"http://localhost:11434/v1","model":"explicit-change","enabled":true}]}`); err != nil {
		t.Fatal(err)
	}
	got, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	p, err := providerForTask(got, "vision")
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "explicit-change" {
		t.Fatalf("explicit provider edit lost: %+v", p)
	}
	if err := patchConfig(`{"capture_interval_sec":20,"model":"legacy-change"}`); err != nil {
		t.Fatal(err)
	}
	got, err = loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	p, err = providerForTask(got, "vision")
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "explicit-change" {
		t.Fatalf("unchanged legacy field reset provider: %+v", p)
	}
}
