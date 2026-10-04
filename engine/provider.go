package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PromptOverrides lets a provider replace the default prompt templates.
// Empty fields fall back to the built-in templates.
type PromptOverrides struct {
	TitlePrompt    string `json:"title_prompt,omitempty"`
	SummaryPrompt  string `json:"summary_prompt,omitempty"`
	DetailedPrompt string `json:"detailed_prompt,omitempty"`
	ChatPrompt     string `json:"chat_prompt,omitempty"`
}

// Provider is one LLM endpoint dayflow can route tasks to.
// Kind is one of: openrouter, local, custom, gemini, chatgpt, claude, mcp, cli.
type Provider struct {
	ID              string          `json:"id"`
	Name            string          `json:"name,omitempty"`
	Kind            string          `json:"kind"`
	APIBaseURL      string          `json:"api_base_url,omitempty"` // empty = OpenRouter
	APIKey          string          `json:"api_key,omitempty"`
	Model           string          `json:"model,omitempty"`
	Vision          bool            `json:"vision,omitempty"` // can read images
	Chat            bool            `json:"chat,omitempty"`   // can do text/chat tasks
	Enabled         bool            `json:"enabled"`
	PromptOverrides PromptOverrides `json:"prompt_overrides,omitempty"`

	// The fields below configure kind=="cli" providers — a subscription-
	// auth'd agent CLI run as a hardened subprocess (see provider_cli.go).
	Command        string   `json:"command,omitempty"`         // executable name/path, e.g. cursor-agent
	Args           []string `json:"args,omitempty"`            // argv template; {prompt} (only after --) and {file} placeholders
	EnvPassthrough []string `json:"env_passthrough,omitempty"` // extra env var names the child may inherit
	CLITimeoutSec  int      `json:"cli_timeout_sec,omitempty"` // per-invocation timeout; 0 = 180s
	AllowHotPath   bool     `json:"allow_hot_path,omitempty"`  // opt in to vision/summary routing (minutes-scale latency)
	ScratchHome    bool     `json:"scratch_home,omitempty"`    // run with a scratch HOME instead of the real one
}

// Routing decides which provider serves which task.
// TaskProvider keys: vision, summary, detailed, chat, review, standup.
type Routing struct {
	Primary      string            `json:"primary,omitempty"`
	Secondary    string            `json:"secondary,omitempty"`
	TaskProvider map[string]string `json:"task_provider,omitempty"`
}

var providerKinds = []string{"openrouter", "local", "custom", "gemini", "chatgpt", "claude", "mcp", "cli"}

func validProviderKind(k string) bool {
	for _, v := range providerKinds {
		if k == v {
			return true
		}
	}
	return false
}

// migrateLegacyProviders ensures cfg has a usable Providers list and Routing by
// folding the old single-provider keys into a "default" provider when needed.
func migrateLegacyProviders(cfg *Config) {
	for i := range cfg.Providers {
		cfg.Providers[i].APIBaseURL = normalizeAPIBaseURL(cfg.Providers[i].APIBaseURL)
	}
	if len(cfg.Providers) == 0 {
		cfg.Providers = []Provider{{
			ID:         "default",
			Name:       "Default",
			Kind:       cfg.Provider,
			APIBaseURL: cfg.APIBaseURL,
			APIKey:     cfg.OpenRouterAPIKey,
			Model:      cfg.Model,
			Vision:     true,
			Chat:       true,
			Enabled:    true,
		}}
	}
	if cfg.Routing.Primary == "" {
		cfg.Routing.Primary = cfg.Providers[0].ID
	}
}

// effectiveProviders returns the configured providers, or a migrated default
// provider built from the legacy single-provider fields. Used so configs built
// in code (tests, daemon) route the same way loadConfig results do.
func effectiveProviders(cfg Config) []Provider {
	if len(cfg.Providers) > 0 {
		return cfg.Providers
	}
	c := cfg
	migrateLegacyProviders(&c)
	return c.Providers
}

// providerForTask resolves the provider for a task: TaskProvider override,
// then Primary, then Secondary, then the first enabled provider.
//
// Hot-path guard: a cli provider cannot serve "vision" or "summary" unless
// its config sets allow_hot_path — a minutes-scale subprocess would stall
// the per-block summarize loop. Ineligible cli providers are skipped at every
// resolution step, so a task_provider override pointing at one falls back to
// primary/secondary rather than routing to it.
func providerForTask(cfg Config, task string) (Provider, error) {
	provs := effectiveProviders(cfg)
	eligible := func(p *Provider) bool {
		if p == nil || !p.Enabled {
			return false
		}
		if p.Kind == "cli" && !p.AllowHotPath && (task == "vision" || task == "summary") {
			return false
		}
		return true
	}
	find := func(id string) *Provider {
		for i := range provs {
			if provs[i].ID == id && eligible(&provs[i]) {
				return &provs[i]
			}
		}
		return nil
	}
	if id := cfg.Routing.TaskProvider[task]; id != "" {
		if p := find(id); p != nil {
			return *p, nil
		}
		// The configured route names a provider that exists but is ineligible
		// (disabled, or a cli provider without allow_hot_path on the
		// vision/summary hot path) — warn rather than silently rerouting.
		for i := range provs {
			if provs[i].ID != id {
				continue
			}
			reason := "disabled"
			if provs[i].Enabled {
				reason = "cli provider without allow_hot_path"
			}
			target := "first enabled provider"
			if p := find(cfg.Routing.Primary); p != nil {
				target = "primary " + p.ID
			} else if p := find(cfg.Routing.Secondary); p != nil {
				target = "secondary " + p.ID
			}
			debugf(cfg, "routing: task %q provider %q skipped (%s) — falling back to %s",
				task, id, reason, target)
			break
		}
	}
	if p := find(cfg.Routing.Primary); p != nil {
		return *p, nil
	}
	if p := find(cfg.Routing.Secondary); p != nil {
		return *p, nil
	}
	for i := range provs {
		if eligible(&provs[i]) {
			return provs[i], nil
		}
	}
	err := fmt.Errorf("no enabled provider configured for task %q", task)
	for i := range provs {
		if provs[i].Enabled && provs[i].Kind == "cli" && !provs[i].AllowHotPath {
			err = fmt.Errorf("%w (cli provider %q is ineligible: set allow_hot_path on it to opt in)", err, provs[i].ID)
			break
		}
	}
	return Provider{}, err
}

// configuredVisionProvider checks the actual vision route without making a
// model call. Presence of legacy OpenRouter fields is not provider readiness.
func configuredVisionProvider(cfg Config) (Provider, bool) {
	p, err := providerForTask(cfg, "vision")
	if err != nil || !validProviderKind(p.Kind) {
		return p, false
	}
	if p.Kind == "cli" {
		if p.Command == "" {
			return p, false
		}
		_, err := exec.LookPath(p.Command)
		return p, err == nil
	}
	if strings.TrimSpace(p.Model) == "" {
		return p, false
	}
	if !providerNeedsAuth(p) {
		return p, strings.TrimSpace(p.APIBaseURL) != ""
	}
	return p, resolveProviderKey(p) != ""
}

// providerNeedsAuth reports whether the provider should receive an
// Authorization header. Local and MCP endpoints never get keys; cli
// providers carry no dayflow-held key at all (the subprocess uses its own
// subscription auth).
func providerNeedsAuth(p Provider) bool {
	return p.Kind != "local" && p.Kind != "mcp" && p.Kind != "cli"
}

func providerChatURL(p Provider) string {
	if p.APIBaseURL != "" {
		return strings.TrimSuffix(p.APIBaseURL, "/") + "/chat/completions"
	}
	return openRouterURL
}

func providerUsesOpenRouterHeaders(p Provider) bool {
	return p.APIBaseURL == "" || strings.Contains(p.APIBaseURL, "openrouter.ai")
}

// setOpenRouterHeaders stamps app attribution on a request — every call to
// OpenRouter (chat, decisions, models listing) carries these so the app
// shows up correctly on the OpenRouter activity/leaderboard pages.
func setOpenRouterHeaders(req *http.Request, siteName string) {
	if siteName == "" {
		siteName = defaultSiteName
	}
	req.Header.Set("HTTP-Referer", "https://github.com/duketopceo/dayflow-linux")
	req.Header.Set("X-Title", siteName)
}

// defaultRequestTimeout bounds a single provider attempt. The old hard 120s
// ceiling was hit 58 times in 11 days while successful calls averaged 40s.
const defaultRequestTimeout = 180 * time.Second

const (
	// providerMaxAttempts is one try plus retries for transient faults.
	providerMaxAttempts = 3
	// providerMaxBodyBytes caps a single response body. The old 1 MiB cap
	// truncated long completions mid-JSON, which surfaced as a parse error.
	providerMaxBodyBytes = int64(16 << 20)
)

// providerError records whether a failure is worth retrying. The message is
// byte-identical to what the DB and logs already store.
type providerError struct {
	transient bool
	msg       string
}

func (e *providerError) Error() string { return e.msg }

// transientf builds a retryable provider failure.
func transientf(format string, args ...any) error {
	return &providerError{transient: true, msg: fmt.Sprintf(format, args...)}
}

func isTransientProviderErr(err error) bool {
	var pe *providerError
	return errors.As(err, &pe) && pe.transient
}

// providerBackoff returns the wait before the next attempt. It is a variable so
// tests can stub the wait out.
var providerBackoff = func(attempt int) time.Duration {
	if attempt <= 1 {
		return 2 * time.Second
	}
	return 8 * time.Second
}

// callProviderChat posts an OpenAI-compatible chat request to provider p,
// retrying transient failures. A single attempt used to be the whole story:
// on 2026-09-22 that turned 45% of the day's calls into dead blocks, and a
// DNS blip cost a 15-minute capture window permanently.
//
// kind=="cli" dispatches to the hardened subprocess path instead — no HTTP,
// no retries (each attempt is minutes-scale).
func callProviderChat(cfg Config, p Provider, messages []orMessage) (string, int, int, error) {
	if p.Kind == "cli" {
		return callProviderCLIText(cfg, p, messages)
	}
	var lastErr error
	for attempt := 1; attempt <= providerMaxAttempts; attempt++ {
		content, pt, ct, err := providerChatOnce(cfg, p, messages)
		if err == nil {
			return content, pt, ct, nil
		}
		lastErr = err
		if !isTransientProviderErr(err) || attempt == providerMaxAttempts {
			break
		}
		delay := providerBackoff(attempt)
		debugf(cfg, "provider %s attempt %d/%d failed (%v) — retrying in %s",
			p.ID, attempt, providerMaxAttempts, err, delay)
		time.Sleep(delay)
	}
	return "", 0, 0, lastErr
}

// providerChatOnce performs a single request/response exchange.
func providerChatOnce(cfg Config, p Provider, messages []orMessage) (string, int, int, error) {
	apiKey := resolveProviderKey(p)
	if providerNeedsAuth(p) && apiKey == "" {
		// Auth is required for every non-local/MCP provider. The old check also
		// required an empty APIBaseURL, so a custom base URL with no key sent
		// the request without an Authorization header and surfaced as
		// "api 401: Missing Authentication header" instead of a config error.
		return "", 0, 0, fmt.Errorf("no API key for provider %q: set its api_key in %s, OPENROUTER_API_KEY, or `dayflow key set %s`", p.ID, configPath(), p.ID)
	}
	reqBody, _ := json.Marshal(orRequest{Model: p.Model, Messages: messages})
	req, err := http.NewRequest("POST", providerChatURL(p), bytes.NewReader(reqBody))
	if err != nil {
		return "", 0, 0, err
	}
	if providerNeedsAuth(p) && apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	if providerUsesOpenRouterHeaders(p) {
		setOpenRouterHeaders(req, cfg.SiteName)
	}

	timeout := time.Duration(cfg.RequestTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, 0, transientf("api request failed: %v", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, providerMaxBodyBytes+1))
	if readErr != nil {
		return "", 0, 0, transientf("api response read failed: %v", readErr)
	}
	if int64(len(body)) > providerMaxBodyBytes {
		return "", 0, 0, transientf("api response exceeded %d bytes", providerMaxBodyBytes)
	}
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("api %d: %s", resp.StatusCode, truncate(string(body), 300))
		if resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode >= 500 {
			return "", 0, 0, transientf("%s", msg)
		}
		return "", 0, 0, errors.New(msg)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", 0, 0, transientf("api returned an empty response body")
	}
	var or orResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return "", 0, 0, transientf("api response was not valid JSON: %v", err)
	}
	if or.Error != nil {
		return "", 0, 0, fmt.Errorf("api error: %s", or.Error.Message)
	}
	if len(or.Choices) == 0 {
		return "", 0, 0, fmt.Errorf("api returned no choices")
	}
	pt, ct := 0, 0
	if or.Usage != nil {
		pt, ct = or.Usage.PromptTokens, or.Usage.CompletionTokens
	}
	return stripFences(strings.TrimSpace(or.Choices[0].Message.Content)), pt, ct, nil
}

// callProvider resolves the routed provider for task and sends messages.
func callProvider(cfg Config, task string, messages []orMessage) (string, int, int, error) {
	p, err := providerForTask(cfg, task)
	if err != nil {
		return "", 0, 0, err
	}
	return callProviderChat(cfg, p, messages)
}

// callProviderText is a system+user convenience wrapper around callProvider.
func callProviderText(cfg Config, task, system, user string) (string, int, int, error) {
	return callProvider(cfg, task, []orMessage{
		{Role: "system", Content: []orContent{{Type: "text", Text: system}}},
		{Role: "user", Content: []orContent{{Type: "text", Text: user}}},
	})
}

// findProvider locates a provider by ID regardless of enabled state.
func findProvider(cfg Config, id string) *Provider {
	provs := effectiveProviders(cfg)
	for i := range provs {
		if provs[i].ID == id {
			return &provs[i]
		}
	}
	return nil
}

var providerTasks = []string{"vision", "summary", "detailed", "chat", "review", "standup", "classification"}

func isProviderTask(s string) bool {
	for _, t := range providerTasks {
		if s == t {
			return true
		}
	}
	return false
}

// runProvider implements `dayflow provider list|add|set|remove|test`.
func runProvider(cfg Config, args []string, jsonOut bool) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		if jsonOut {
			masked := effectiveProviders(cfg)
			for i := range masked {
				if masked[i].APIKey != "" {
					masked[i].APIKey = "***redacted***"
				}
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"providers": masked,
				"routing":   cfg.Routing,
			})
		}
		fmt.Println("providers:")
		for _, p := range effectiveProviders(cfg) {
			state := "disabled"
			if p.Enabled {
				state = "enabled"
			}
			key := ""
			if p.APIKey != "" {
				key = "  key=***"
			}
			base := p.APIBaseURL
			if p.Kind == "cli" {
				base = "cmd=" + p.Command
				if p.AllowHotPath {
					base += " hot-path"
				}
			} else if base == "" {
				base = "openrouter"
			}
			fmt.Printf("  %-12s %-10s %-8s model=%s base=%s%s\n", p.ID, p.Kind, state, p.Model, base, key)
		}
		fmt.Printf("routing: primary=%s secondary=%s\n", cfg.Routing.Primary, cfg.Routing.Secondary)
		if len(cfg.Routing.TaskProvider) > 0 {
			tasks := make([]string, 0, len(cfg.Routing.TaskProvider))
			for t := range cfg.Routing.TaskProvider {
				tasks = append(tasks, t)
			}
			sort.Strings(tasks)
			for _, t := range tasks {
				fmt.Printf("  task %-8s -> %s\n", t, cfg.Routing.TaskProvider[t])
			}
		}
		return nil

	case "add":
		if len(args) < 3 {
			return fmt.Errorf("usage: dayflow provider add <id> <kind>")
		}
		id, kind := args[1], strings.ToLower(args[2])
		if !validProviderKind(kind) {
			return fmt.Errorf("kind must be one of: %s", strings.Join(providerKinds, ", "))
		}
		if findProvider(cfg, id) != nil {
			return fmt.Errorf("provider %q already exists", id)
		}
		p := Provider{ID: id, Name: id, Kind: kind, Model: cfg.Model, Enabled: true}
		if kind == "local" {
			p.APIBaseURL = "http://localhost:11434/v1"
		}
		cfg.Providers = append(effectiveProviders(cfg), p)
		if cfg.Routing.Primary == "" {
			cfg.Routing.Primary = id
		}
		if err := writeConfig(cfg); err != nil {
			return err
		}
		fmt.Println("added provider", id)
		if kind == "cli" {
			fmt.Printf("cli provider: set its command next, e.g. `dayflow provider set %s command cursor-agent`\n", id)
			fmt.Println("note: the CLI forwards prompts (and any frames it reads) to its own model backend — a local subprocess does not keep data local")
		}
		return nil

	case "set":
		if len(args) < 4 {
			return fmt.Errorf("usage: dayflow provider set <id> <key> <value>")
		}
		cfg.Providers = effectiveProviders(cfg)
		idx := -1
		for i := range cfg.Providers {
			if cfg.Providers[i].ID == args[1] {
				idx = i
			}
		}
		if idx < 0 {
			return fmt.Errorf("no provider %q", args[1])
		}
		p := &cfg.Providers[idx]
		val := args[3]
		if val == "-" {
			val = strings.TrimSpace(readStdin()) // keeps secrets out of argv
		}
		switch args[2] {
		case "name":
			p.Name = val
		case "kind":
			k := strings.ToLower(val)
			if !validProviderKind(k) {
				return fmt.Errorf("kind must be one of: %s", strings.Join(providerKinds, ", "))
			}
			p.Kind = k
		case "api_base_url":
			p.APIBaseURL = normalizeAPIBaseURL(val)
		case "api_key":
			p.APIKey = val
		case "model":
			p.Model = val
		case "enabled":
			p.Enabled = val == "true" || val == "1" || val == "yes"
		case "vision":
			p.Vision = val == "true" || val == "1" || val == "yes"
		case "chat":
			p.Chat = val == "true" || val == "1" || val == "yes"
		case "title_prompt":
			p.PromptOverrides.TitlePrompt = val
		case "summary_prompt":
			p.PromptOverrides.SummaryPrompt = val
		case "detailed_prompt":
			p.PromptOverrides.DetailedPrompt = val
		case "chat_prompt":
			p.PromptOverrides.ChatPrompt = val
		case "command":
			p.Command = val
			if len(p.Args) == 0 {
				p.Args = cliPresetArgs(val)
			}
		case "args":
			var parsed []string
			if json.Unmarshal([]byte(val), &parsed) != nil {
				// not JSON — accept a comma-separated shorthand
				for _, s := range strings.Split(val, ",") {
					if s = strings.TrimSpace(s); s != "" {
						parsed = append(parsed, s)
					}
				}
			}
			p.Args = parsed
		case "cli_timeout_sec":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return fmt.Errorf("cli_timeout_sec must be a non-negative integer (0 = 180s default)")
			}
			p.CLITimeoutSec = n
		case "allow_hot_path":
			p.AllowHotPath = val == "true" || val == "1" || val == "yes"
		case "scratch_home":
			p.ScratchHome = val == "true" || val == "1" || val == "yes"
		case "env_passthrough":
			var names []string
			for _, s := range strings.Split(val, ",") {
				if s = strings.TrimSpace(s); s != "" {
					names = append(names, s)
				}
			}
			p.EnvPassthrough = names
		default:
			return fmt.Errorf("unknown provider key %q (name, kind, api_base_url, api_key, model, enabled, vision, chat, command, args, cli_timeout_sec, allow_hot_path, scratch_home, env_passthrough, *_prompt)", args[2])
		}
		if p.Kind == "cli" {
			if cliCommandDenied(p.Command) {
				return fmt.Errorf("provider %s: command %q is an interpreter (sh/python/node/...) that would run the prompt as code — pick an agent CLI like cursor-agent or opencode", p.ID, filepath.Base(p.Command))
			}
			if err := validateCLIArgs(p.Args); err != nil {
				return fmt.Errorf("provider %s: %w", p.ID, err)
			}
		}
		if err := writeConfig(cfg); err != nil {
			return err
		}
		fmt.Printf("set %s.%s\n", p.ID, args[2])
		return nil

	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: dayflow provider remove <id>")
		}
		cfg.Providers = effectiveProviders(cfg)
		kept := cfg.Providers[:0]
		found := false
		for _, p := range cfg.Providers {
			if p.ID == args[1] {
				found = true
				continue
			}
			kept = append(kept, p)
		}
		if !found {
			return fmt.Errorf("no provider %q", args[1])
		}
		cfg.Providers = kept
		if cfg.Routing.Primary == args[1] {
			cfg.Routing.Primary = ""
		}
		if cfg.Routing.Secondary == args[1] {
			cfg.Routing.Secondary = ""
		}
		for t, id := range cfg.Routing.TaskProvider {
			if id == args[1] {
				delete(cfg.Routing.TaskProvider, t)
			}
		}
		if err := writeConfig(cfg); err != nil {
			return err
		}
		fmt.Println("removed provider", args[1])
		return nil

	case "test":
		if len(args) < 2 {
			return fmt.Errorf("usage: dayflow provider test <id|task>")
		}
		var p Provider
		if isProviderTask(args[1]) {
			var err error
			p, err = providerForTask(cfg, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("task %s routes to provider %s (%s)\n", args[1], p.ID, p.Kind)
		} else {
			fp := findProvider(cfg, args[1])
			if fp == nil {
				return fmt.Errorf("no provider %q", args[1])
			}
			p = *fp
		}
		text, pt, ct, err := callProviderChat(cfg, p, []orMessage{
			{Role: "user", Content: []orContent{{Type: "text", Text: "Reply with exactly: ok"}}},
		})
		if err != nil {
			return fmt.Errorf("provider %s test failed: %w", p.ID, err)
		}
		fmt.Printf("provider %s OK: %q (tokens %d+%d)\n", p.ID, truncate(text, 80), pt, ct)
		return nil
	}
	return fmt.Errorf("unknown provider subcommand %q (list, add, set, remove, test)", sub)
}
