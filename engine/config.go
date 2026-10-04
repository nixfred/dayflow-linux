package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Category is a user-definable bucket for activity classification.
// The description is shown to the vision model so it can map screenshots
// to the right bucket.
type Category struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color,omitempty"` // optional hex color for UI
}

// defaultSiteName is the OpenRouter X-Title app attribution.
const defaultSiteName = "dayflow-linux"

type Config struct {
	Provider             string     `json:"provider"` // openrouter, local, custom, mcp
	OpenRouterAPIKey     string     `json:"openrouter_api_key"`
	Model                string     `json:"model"`
	APIBaseURL           string     `json:"api_base_url"`        // OpenAI-compatible endpoint; empty = OpenRouter
	RequestTimeoutSec    int        `json:"request_timeout_sec"` // per-attempt provider timeout; 0 = 180s default
	CaptureIntervalSec   int        `json:"capture_interval_sec"`
	BlockMinutes         int        `json:"block_minutes"`
	FramesPerBlock       int        `json:"frames_per_block"`
	JPEGQuality          int        `json:"jpeg_quality"`
	FrameMaxDim          int        `json:"frame_max_dim"` // bound stored frames' longer edge (px); 0 = keep native size; min nonzero = minFrameMaxDim
	KeepFrames           bool       `json:"keep_frames"`
	RetentionDays        int        `json:"retention_days"`
	IgnoreApps           []string   `json:"ignore_apps"`     // hyprctl window classes, case-insensitive
	CaptureCommand       string     `json:"capture_command"` // override; default auto-detect grim
	Output               string     `json:"output"`          // grim -o <output>; empty = all outputs; "auto" = focused monitor per tick
	SiteName             string     `json:"site_name"`       // OpenRouter X-Title
	MaxStorageMB         int        `json:"max_storage_mb"`  // legacy: cap on the whole data dir; 0 = off (new installs use the split caps below)
	MaxFramesMB          int        `json:"max_frames_mb"`   // cap on frames + quarantine dirs; 0 = unlimited
	MaxDBMB              int        `json:"max_db_mb"`       // cap on the journal database (blocks, events, calls); 0 = unlimited
	AutoPauseLocked      bool       `json:"auto_pause_locked"`
	FilterInappropriate  bool       `json:"filter_inappropriate"` // redact adult/explicit content
	Debug                bool       `json:"debug"`                // verbose engine log to debug.log
	Categories           []Category `json:"categories"`
	ClassificationPrompt string     `json:"classification_prompt"` // extra instructions for the vision model
	JevClassification    bool       `json:"jev_classification"`    // use Jev for category/productive (default true)
	AgentRecaps          bool       `json:"agent_recaps"`          // generate agent-session recaps (default false — opt-in; recaps send bounded scrubbed transcript excerpts to the chat provider + decisions endpoint)
	AgentRecapBatch      bool       `json:"agent_recap_batch"`     // submit uncached recaps as one OpenRouter batch (~50% off, async) instead of inline calls; requires an OpenRouter-routed provider for agent_recap
	ClassificationModel  string     `json:"classification_model"`  // Jev model slug; default typesafe/jev-1.13
	Providers            []Provider `json:"providers,omitempty"`   // multi-provider list; empty = migrated from legacy keys
	Routing              Routing    `json:"routing,omitempty"`
	// Pricing maps a model slug to USD per 1M tokens (prompt+completion
	// combined). `dayflow usage` renders dollar estimates only when set;
	// rates are never fetched. Set via `config patch` — nested maps merge,
	// so one patch can add a model without restating the others.
	Pricing       map[string]float64 `json:"pricing,omitempty"`
	PanelExpanded bool               `json:"panel_expanded"` // remember the panel Expand/Shrink toggle
	DisableJudges bool               `json:"-"`              // runtime-only: read-only MCP sessions must not egress

	// Knowledge sync — opt-in push of distilled atoms to a Kurultai brain.
	// Off by default: enabling it is consent to journal text leaving the
	// machine. All endpoint details are user config — no personal
	// infrastructure is baked into the binary.
	KnowledgeSync      bool   `json:"knowledge_sync"`
	KnowledgeTransport string `json:"knowledge_transport,omitempty"`  // "http" (default) | "ssh"
	KnowledgeURL       string `json:"knowledge_url,omitempty"`        // brain base URL — required for http
	KnowledgeSSHHost   string `json:"knowledge_ssh_host,omitempty"`   // required for ssh
	KnowledgeContainer string `json:"knowledge_container,omitempty"`  // required for ssh
	KnowledgePort      int    `json:"knowledge_port,omitempty"`       // default 8421 (ssh only)
	KnowledgeSecretRef string `json:"knowledge_secret_ref,omitempty"` // required: "omaseal://service/account" or literal

	Notifications NotificationConfig `json:"notifications"` // desktop notifications: master switch + per-class gates
}

// normalizeAPIBaseURL trims whitespace and trailing slashes, and appends /v1
// when the URL has no path so that Ollama/LM Studio endpoints work out of the box.
func normalizeAPIBaseURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	u = strings.TrimSuffix(u, "/")
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/v1"
	}
	return parsed.String()
}

// minFrameMaxDim floors frame_max_dim when normalization is enabled — a
// sub-320px long edge destroys the journal's evidentiary value.
const minFrameMaxDim = 320

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "dayflow")
}

func dataDir() string {
	if d := os.Getenv("DAYFLOW_DATA_DIR"); d != "" {
		return d
	}
	d, err := os.UserHomeDir()
	if err != nil {
		d = os.Getenv("HOME")
	}
	return filepath.Join(d, ".local", "share", "dayflow")
}

func framesDir() string  { return filepath.Join(dataDir(), "frames") }
func exportsDir() string { return filepath.Join(dataDir(), "exports") }
func dbPath() string     { return filepath.Join(dataDir(), "dayflow.db") }
func pausePath() string  { return filepath.Join(dataDir(), "PAUSED") }
func configPath() string {
	if p := os.Getenv("DAYFLOW_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(configDir(), "config.json")
}

func defaultConfig() Config {
	return Config{
		Provider:             "openrouter",
		Model:                "google/gemma-4-31b-it",
		CaptureIntervalSec:   10,
		BlockMinutes:         15,
		FramesPerBlock:       30,
		JPEGQuality:          55,
		FrameMaxDim:          1920,
		KeepFrames:           false,
		RetentionDays:        0, // storage caps own eviction; days only prune when set explicitly
		IgnoreApps:           []string{"swaylock", "hyprlock", "waylock", "gtklock", "i3lock", "xscreensaver", "screensaver"},
		SiteName:             defaultSiteName,
		MaxFramesMB:          20480,
		MaxDBMB:              10240,
		AutoPauseLocked:      true,
		FilterInappropriate:  true,
		Categories:           defaultCategories(),
		ClassificationPrompt: defaultClassificationPrompt,
		JevClassification:    true,
		ClassificationModel:  defaultJevModel,
		AgentRecaps:          false,
		Notifications:        defaultNotifications(),
	}
}

const defaultClassificationPrompt = `Browsing and coding can each be work or personal depending on what is visible.
- Work = actively shipping or maintaining projects, job-related tasks, debugging, configuring systems, reading docs to solve a problem, applying for roles, or writing project code.
- Personal = entertainment, social media scrolling, idle chat, adult content, or consumption with no clear goal.
- A personal side project still counts as productive when the user is intentionally building or learning for that project.
- Err on the side of productive when the user is actively creating, debugging, or problem solving.`

func defaultCategories() []Category {
	return []Category{
		{Name: "coding", Description: "Writing, debugging, reviewing, or shipping code, config, scripts, or infrastructure."},
		{Name: "browsing", Description: "General web browsing, reading docs, or searching without a concrete task."},
		{Name: "communication", Description: "Email, chat, calls, video meetings, or messaging."},
		{Name: "writing", Description: "Writing documents, notes, markdown, specs, or long-form text."},
		{Name: "design", Description: "Creating or editing designs, images, video, UI/UX, or 3D assets."},
		{Name: "media", Description: "Watching videos, listening to music, gaming, or other entertainment."},
		{Name: "meetings", Description: "In a meeting, standup, interview, or call."},
		{Name: "system", Description: "OS maintenance, package installs, backups, file management, or sysadmin work."},
		{Name: "idle", Description: "Screen locked, away, or no visible activity."},
		{Name: "personal", Description: "Personal, private, or sensitive activity that is not work-related."},
		{Name: "other", Description: "Anything that does not fit the other buckets."},
	}
}

func loadConfig() (Config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err != nil {
		if !os.IsNotExist(err) {
			return cfg, err
		}
	} else if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if k := os.Getenv("OPENROUTER_API_KEY"); k != "" {
		cfg.OpenRouterAPIKey = k
	}
	if cfg.OpenRouterAPIKey == "" {
		cfg.OpenRouterAPIKey = openRouterKeysFallback()
	}
	if cfg.OpenRouterAPIKey == "" && keyringAvailable() {
		if k, err := keyringGet("openrouter"); err == nil {
			cfg.OpenRouterAPIKey = k
		}
	}
	if cfg.CaptureIntervalSec <= 0 {
		cfg.CaptureIntervalSec = 10
	}
	if cfg.BlockMinutes <= 0 {
		cfg.BlockMinutes = 15
	}
	if cfg.RequestTimeoutSec <= 0 {
		// One provider attempt. The old hard-coded 120s ceiling was exceeded by
		// the gemma-4-31b tail 58 times in 11 days.
		cfg.RequestTimeoutSec = 180
	}
	if cfg.FramesPerBlock <= 0 {
		cfg.FramesPerBlock = 30
	}
	if cfg.JPEGQuality <= 0 || cfg.JPEGQuality > 100 {
		cfg.JPEGQuality = 55
	}
	// frame_max_dim: 0 disables normalization; a tiny positive cap (patches
	// bypass setConfigValue's floor) would destroy journal evidence, so
	// sub-minimum values clamp up and negatives reset to the default.
	if cfg.FrameMaxDim < 0 {
		cfg.FrameMaxDim = defaultConfig().FrameMaxDim
	} else if cfg.FrameMaxDim > 0 && cfg.FrameMaxDim < minFrameMaxDim {
		cfg.FrameMaxDim = minFrameMaxDim
	}
	if cfg.Model == "" {
		cfg.Model = "google/gemma-4-31b-it"
	}
	if cfg.Provider == "" {
		cfg.Provider = "openrouter"
	}
	cfg.APIBaseURL = normalizeAPIBaseURL(cfg.APIBaseURL)
	if cfg.SiteName == "" {
		cfg.SiteName = defaultSiteName
	}
	// Storage caps: a legacy max_storage_mb still bounds the whole data dir,
	// and migrates to the frame cap when no split keys are present. Absent
	// split keys get their own defaults; explicit 0 means unlimited.
	raw := string(b)
	if !strings.Contains(raw, "max_frames_mb") {
		if strings.Contains(raw, "max_storage_mb") {
			cfg.MaxFramesMB = cfg.MaxStorageMB
		} else {
			cfg.MaxFramesMB = 20480
		}
	}
	if !strings.Contains(raw, "max_db_mb") {
		cfg.MaxDBMB = 10240
	}
	if !strings.Contains(string(b), "jev_classification") {
		cfg.JevClassification = true
	}
	// agent_recaps has no absent-key backfill: it is opt-in, so a missing
	// key must decode to false like any other default-off bool.
	// notifications: an absent object keeps the defaultConfig values, but a
	// present object that omits "enabled", or whose classes omit "stall"
	// (a hand edit, or a `config patch` that only set other keys), must not
	// silently disable stall alerts — silence means a capture stall goes
	// unreported. Disabling stall requires an explicit stall:false.
	var nprobe struct {
		Notifications struct {
			Enabled *bool           `json:"enabled"`
			Classes map[string]bool `json:"classes"`
		} `json:"notifications"`
	}
	if json.Unmarshal(b, &nprobe) == nil {
		if nprobe.Notifications.Enabled == nil {
			cfg.Notifications.Enabled = true
		}
		if nprobe.Notifications.Classes == nil {
			cfg.Notifications.Classes = map[string]bool{notifyClassStall: true}
		} else if _, ok := nprobe.Notifications.Classes[notifyClassStall]; !ok {
			if cfg.Notifications.Classes == nil {
				cfg.Notifications.Classes = map[string]bool{}
			}
			cfg.Notifications.Classes[notifyClassStall] = true
		}
	}
	if cfg.ClassificationModel == "" {
		cfg.ClassificationModel = defaultJevModel
	}
	migrateLegacyProviders(&cfg)
	return cfg, nil
}

// openRouterKeysFallback reads ~/.config/openrouter/keys.json if present.
func openRouterKeysFallback() string {
	b, err := os.ReadFile(filepath.Join(configDir(), "..", "openrouter", "keys.json"))
	if err != nil {
		return ""
	}
	var k struct {
		APIKey string `json:"api_key"`
	}
	if json.Unmarshal(b, &k) != nil {
		return ""
	}
	return k.APIKey
}

func writeConfig(cfg Config) error {
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0o600)
}

// patchConfig merges a JSON patch object into the current config. Values with
// the sentinel "***redacted***" are ignored so the panel can safely round-trip
// the API key field without overwriting it.
func patchConfig(patch string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	previous := cfg
	previous.Providers = append([]Provider(nil), cfg.Providers...)
	var patchMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(patch), &patchMap); err != nil {
		return fmt.Errorf("patch must be a JSON object: %w", err)
	}
	if v, ok := patchMap["openrouter_api_key"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil && s == "***redacted***" {
			delete(patchMap, "openrouter_api_key")
		}
	}
	// The panel round-trips providers with masked api_key fields — and strips
	// the sentinel before patching, so a missing/empty api_key must keep the
	// existing key for that id. Otherwise a settings save silently clobbers
	// the real credential.
	if v, ok := patchMap["providers"]; ok {
		var provs []map[string]json.RawMessage
		if json.Unmarshal(v, &provs) == nil {
			existing := effectiveProviders(cfg)
			changed := false
			for _, p := range provs {
				rawKey, hasKey := p["api_key"]
				var key string
				hasReal := hasKey && json.Unmarshal(rawKey, &key) == nil &&
					key != "" && key != "***redacted***"
				if hasReal {
					continue
				}
				var id string
				restored := false
				if json.Unmarshal(p["id"], &id) == nil {
					for _, e := range existing {
						if e.ID == id && e.APIKey != "" {
							p["api_key"], _ = json.Marshal(e.APIKey)
							restored = true
							break
						}
					}
				}
				if !restored {
					// no stored key to preserve — drop the placeholder so a
					// masked/empty value is never persisted
					delete(p, "api_key")
				}
				changed = true
			}
			if changed {
				if fixed, err := json.Marshal(provs); err == nil {
					patchMap["providers"] = fixed
				}
			}
		}
	}
	if v, ok := patchMap["api_base_url"]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			patchMap["api_base_url"] = json.RawMessage(`"` + normalizeAPIBaseURL(s) + `"`)
		}
	}
	baseJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(baseJSON, &merged); err != nil {
		return err
	}
	for k, v := range patchMap {
		merged[k] = v
	}
	mergedJSON, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(mergedJSON, &cfg); err != nil {
		return fmt.Errorf("patched config is invalid: %w", err)
	}
	// A "notifications" patch replaces the whole object — one that omits
	// "enabled", or whose classes omit "stall", must not switch them off by
	// omission (silence means a capture stall goes unreported). Disabling
	// stall requires an explicit stall:false; loadConfig applies the same
	// backfill to hand-edited files.
	if v, ok := patchMap["notifications"]; ok {
		var n struct {
			Enabled *bool           `json:"enabled"`
			Classes map[string]bool `json:"classes"`
		}
		if json.Unmarshal(v, &n) == nil {
			if n.Enabled == nil {
				cfg.Notifications.Enabled = true
			}
			if n.Classes == nil {
				cfg.Notifications.Classes = map[string]bool{notifyClassStall: true}
			} else if _, ok := n.Classes[notifyClassStall]; !ok {
				if cfg.Notifications.Classes == nil {
					cfg.Notifications.Classes = map[string]bool{}
				}
				cfg.Notifications.Classes[notifyClassStall] = true
			}
		}
	}
	if cfg.Model == "" {
		return fmt.Errorf("model is required")
	}
	if cfg.Provider == "" {
		cfg.Provider = "openrouter"
	}
	// Mirror changed legacy fields into the active provider. A panel snapshot
	// can carry an unchanged providers array, so array presence alone cannot
	// suppress this. Explicit edits to a provider field win over legacy fields.
	primary := cfg.Routing.Primary
	if primary == "" && len(cfg.Providers) > 0 {
		primary = cfg.Providers[0].ID
	}
	oldProvider := findProvider(previous, primary)
	newProvider := findProvider(cfg, primary)
	for _, key := range []string{"provider", "model", "api_base_url", "openrouter_api_key"} {
		if _, present := patchMap[key]; !present {
			continue
		}
		changed, explicit := false, false
		switch key {
		case "provider":
			changed = cfg.Provider != previous.Provider
			explicit = oldProvider != nil && newProvider != nil && newProvider.Kind != oldProvider.Kind
		case "model":
			changed = cfg.Model != previous.Model
			explicit = oldProvider != nil && newProvider != nil && newProvider.Model != oldProvider.Model
		case "api_base_url":
			changed = cfg.APIBaseURL != previous.APIBaseURL
			explicit = oldProvider != nil && newProvider != nil && newProvider.APIBaseURL != oldProvider.APIBaseURL
		case "openrouter_api_key":
			changed = cfg.OpenRouterAPIKey != previous.OpenRouterAPIKey
			explicit = oldProvider != nil && newProvider != nil && newProvider.APIKey != oldProvider.APIKey
		}
		// Without an explicit providers patch, changes in cfg can only be legacy
		// edits. With one, respect the provider field when it changed independently.
		if _, hasProviders := patchMap["providers"]; changed && (!hasProviders || !explicit) {
			syncLegacyProvider(&cfg, key)
		}
	}
	migrateLegacyProviders(&cfg)
	return writeConfig(cfg)
}

func writeDefaultConfig() error {
	cfg := defaultConfig()
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), b, 0o600)
}

// setConfigValue updates one key in config.json. Supported keys:
// provider, model, api_base_url, capture_interval_sec, block_minutes, frames_per_block,
// jpeg_quality, frame_max_dim, keep_frames, retention_days, ignore_apps (comma list),
// openrouter_api_key, output, capture_command, max_storage_mb, max_frames_mb,
// max_db_mb, auto_pause_locked, filter_inappropriate, debug, notifications.enabled,
// notifications (JSON object — deeper keys belong to `config patch`).
func setConfigValue(key, value string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	switch key {
	case "provider":
		p := strings.ToLower(strings.TrimSpace(value))
		if !validProviderKind(p) {
			return fmt.Errorf("provider must be one of: %s", strings.Join(providerKinds, ", "))
		}
		cfg.Provider = p
		if p == "openrouter" {
			cfg.APIBaseURL = ""
		} else if p == "local" && cfg.APIBaseURL == "" {
			cfg.APIBaseURL = "http://localhost:11434/v1"
		}
	case "model":
		cfg.Model = value
	case "api_base_url":
		cfg.APIBaseURL = normalizeAPIBaseURL(value)
	case "openrouter_api_key":
		cfg.OpenRouterAPIKey = value
	case "capture_interval_sec", "block_minutes", "frames_per_block", "jpeg_quality", "frame_max_dim", "retention_days":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		switch key {
		case "capture_interval_sec":
			cfg.CaptureIntervalSec = n
		case "block_minutes":
			cfg.BlockMinutes = n
		case "frames_per_block":
			cfg.FramesPerBlock = n
		case "jpeg_quality":
			cfg.JPEGQuality = n
		case "frame_max_dim":
			if n < 0 {
				return fmt.Errorf("frame_max_dim must be >= 0 (0 disables normalization)")
			}
			if n > 0 && n < minFrameMaxDim {
				fmt.Printf("warning: frame_max_dim %d is too small to be useful — clamped to %d\n", n, minFrameMaxDim)
				n = minFrameMaxDim
			}
			cfg.FrameMaxDim = n
		case "retention_days":
			cfg.RetentionDays = n
		}
	case "keep_frames":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("keep_frames must be true or false")
		}
		cfg.KeepFrames = b
	case "auto_pause_locked":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("auto_pause_locked must be true or false")
		}
		cfg.AutoPauseLocked = b
	case "filter_inappropriate":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("filter_inappropriate must be true or false")
		}
		cfg.FilterInappropriate = b
	case "panel_expanded":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("panel_expanded must be true or false")
		}
		cfg.PanelExpanded = b
	case "debug":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("debug must be true or false")
		}
		cfg.Debug = b
	case "site_name":
		cfg.SiteName = value
	case "classification_prompt":
		cfg.ClassificationPrompt = value
	case "jev_classification":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("jev_classification must be true or false")
		}
		cfg.JevClassification = b
	case "agent_recaps":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("agent_recaps must be true or false")
		}
		cfg.AgentRecaps = b
	case "agent_recap_batch":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("agent_recap_batch must be true or false")
		}
		cfg.AgentRecapBatch = b
	case "notifications.enabled":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("notifications.enabled must be true or false")
		}
		cfg.Notifications.Enabled = b
	case "notifications":
		// Whole-object write; the unmarshal merges over the loaded defaults,
		// so fields the value omits keep their current setting.
		if err := json.Unmarshal([]byte(value), &cfg.Notifications); err != nil {
			return fmt.Errorf("notifications must be a JSON object {enabled, classes, goal_reminder_hour}: %w", err)
		}
	case "classification_model":
		cfg.ClassificationModel = value
	case "categories":
		if err := json.Unmarshal([]byte(value), &cfg.Categories); err != nil {
			return fmt.Errorf("categories must be a JSON array of {name, description, color?}: %w", err)
		}
	case "ignore_apps":
		if value == "" {
			cfg.IgnoreApps = []string{}
		} else {
			cfg.IgnoreApps = strings.Split(value, ",")
			for i := range cfg.IgnoreApps {
				cfg.IgnoreApps[i] = strings.TrimSpace(cfg.IgnoreApps[i])
			}
		}
	case "max_storage_mb", "max_frames_mb", "max_db_mb":
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer", key)
		}
		switch key {
		case "max_storage_mb":
			cfg.MaxStorageMB = n
		case "max_frames_mb":
			cfg.MaxFramesMB = n
		case "max_db_mb":
			cfg.MaxDBMB = n
		}
	case "output":
		// "auto" is valid: on the grim path it resolves to the focused
		// monitor per tick. With capture_command set it is ignored.
		cfg.Output = strings.TrimSpace(value)
	case "capture_command":
		cfg.CaptureCommand = value
	default:
		return fmt.Errorf("unknown config key %q", key)
	}
	syncLegacyProvider(&cfg, key)
	return writeConfig(cfg)
}

// syncLegacyProvider mirrors legacy single-provider keys into the routed
// default provider so `config set` keeps working with multi-provider configs.
func syncLegacyProvider(cfg *Config, key string) {
	var field func(*Provider)
	switch key {
	case "provider":
		field = func(p *Provider) { p.Kind = cfg.Provider; p.APIBaseURL = cfg.APIBaseURL }
	case "model":
		field = func(p *Provider) { p.Model = cfg.Model }
	case "api_base_url":
		field = func(p *Provider) { p.APIBaseURL = cfg.APIBaseURL }
	case "openrouter_api_key":
		field = func(p *Provider) { p.APIKey = cfg.OpenRouterAPIKey }
	default:
		return
	}
	for i := range cfg.Providers {
		if cfg.Providers[i].ID == cfg.Routing.Primary {
			field(&cfg.Providers[i])
			return
		}
	}
	if len(cfg.Providers) > 0 {
		field(&cfg.Providers[0])
	}
}
