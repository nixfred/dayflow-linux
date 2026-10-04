package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxModelsBody caps the /models catalog read: the 15s client timeout does
// not bound a continuously delivered body, so the read itself must.
const maxModelsBody = 8 << 20 // 8 MiB — the catalog is ~2-3 MB today

// ModelPreset is a recommended vision model for Dayflow.
type ModelPreset struct {
	Name   string `json:"name"`
	Slug   string `json:"slug"`
	Vision bool   `json:"vision"`
	Notes  string `json:"notes"`
}

func modelPresets() []ModelPreset {
	return []ModelPreset{
		{
			Name:   "Gemma 4 31B — default",
			Slug:   "google/gemma-4-31b-it",
			Vision: true,
			Notes:  "Best overall balance on OpenRouter; fast, cheap, and accurate.",
		},
		{
			Name:   "Gemma 3 27B — bigger still small",
			Slug:   "google/gemma-3-27b-it",
			Vision: true,
			Notes:  "More capable than the 12B without being huge. Good for detailed summaries.",
		},
		{
			Name:   "Gemma 3 12B — cheap minimal",
			Slug:   "google/gemma-3-12b-it",
			Vision: true,
			Notes:  "Small, fast, cheapest. Good for everyday work tracking.",
		},
		{
			Name:   "Qwen2.5-VL 7B — local minimal",
			Slug:   "qwen2.5vl:7b",
			Vision: true,
			Notes:  "Local option for Ollama/LM Studio. Pull with: ollama pull qwen2.5vl:7b",
		},
		{
			Name:   "Gemma 3 4B — local tiny",
			Slug:   "gemma3:4b",
			Vision: true,
			Notes:  "Minimal local vision option. Lower accuracy but runs on modest hardware.",
		},
	}
}

func knownVisionModel(slug string) bool {
	for _, p := range modelPresets() {
		if p.Slug == slug {
			return p.Vision
		}
	}
	return false
}

// fetchModels lists models from OpenRouter. Needs a key; returns nil on error.
func fetchModels(apiKey string) ([]struct {
	ID           string `json:"id"`
	Architecture struct {
		InputModalities []string `json:"input_modalities"`
	} `json:"architecture"`
	Pricing struct {
		Prompt string `json:"prompt"`
	} `json:"pricing"`
}, error) {
	req, _ := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	setOpenRouterHeaders(req, "")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID           string `json:"id"`
			Architecture struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
			Pricing struct {
				Prompt string `json:"prompt"`
			} `json:"pricing"`
		} `json:"data"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelsBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxModelsBody {
		return nil, fmt.Errorf("models response exceeded %d bytes", maxModelsBody)
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// isVisionModel reports whether the model accepts image input on OpenRouter.
// Returns (vision, reachable): reachable=false when the API couldn't be asked.
// When a custom api_base_url is set, we cannot query OpenRouter's catalog, so
// we trust the user's choice and return (true, false).
func isVisionModel(cfg Config, model string) (bool, bool) {
	if cfg.APIBaseURL != "" {
		return true, false
	}
	if knownVisionModel(model) {
		return true, true
	}
	if cfg.OpenRouterAPIKey == "" {
		return false, false
	}
	models, err := fetchModels(cfg.OpenRouterAPIKey)
	if err != nil {
		return false, false
	}
	for _, m := range models {
		if m.ID == model || strings.TrimPrefix(m.ID, "~") == model {
			for _, mod := range m.Architecture.InputModalities {
				if mod == "image" {
					return true, true
				}
			}
			return false, true // model found but no image modality
		}
	}
	return false, true // not found — treat as non-vision
}

func runSetup() error {
	r := bufio.NewReader(os.Stdin)

	fmt.Println("dayflow setup")
	fmt.Println("=============")
	fmt.Println("Get an API key at https://openrouter.ai/keys (or use a local endpoint like http://localhost:11434/v1 for Ollama)")
	fmt.Print("API endpoint [https://openrouter.ai/api/v1]: ")
	baseURL, _ := r.ReadString('\n')
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || baseURL == "https://openrouter.ai/api/v1" {
		baseURL = ""
	}

	fmt.Print("API key (sk-or-... or blank for local endpoint): ")
	key, _ := r.ReadString('\n')
	key = strings.TrimSpace(key)

	if baseURL == "" {
		if key == "" {
			return fmt.Errorf("no key entered")
		}
		if !strings.HasPrefix(key, "sk-or-") {
			fmt.Println("warning: key doesn't look like an OpenRouter key (expected sk-or-...)")
		}

		fmt.Println("validating key...")
		models, err := fetchModels(key)
		if err != nil {
			return fmt.Errorf("could not reach OpenRouter: %w", err)
		}
		if len(models) == 0 {
			return fmt.Errorf("key rejected or no models returned")
		}
		fmt.Printf("key OK (%d models available)\n\n", len(models))
	} else {
		fmt.Println("using custom endpoint; skipping OpenRouter validation")
	}

	// pick a vision model
	var vision []string
	if baseURL == "" {
		models, _ := fetchModels(key)
		for _, m := range models {
			for _, mod := range m.Architecture.InputModalities {
				if mod == "image" {
					vision = append(vision, m.ID)
					break
				}
			}
		}
		sort.Strings(vision)
	}
	suggested := "google/gemma-4-31b-it"
	if baseURL != "" {
		suggested = "gemma3:4b"
	}
	fmt.Println("Recommended vision models:")
	for _, p := range modelPresets() {
		if baseURL != "" && !strings.Contains(p.Slug, ":") {
			continue // skip OpenRouter-only slugs for local endpoints
		}
		mark := " "
		for _, v := range vision {
			if v == p.Slug {
				mark = "✓"
			}
		}
		fmt.Printf("  %s %-30s  %s\n", mark, p.Slug, p.Notes)
	}
	fmt.Printf("Model [%s]: ", suggested)
	choice, _ := r.ReadString('\n')
	choice = strings.TrimSpace(choice)
	if choice == "" {
		choice = suggested
	}

	// Recaps opt-in (R5): only offered when a local agent transcript store
	// exists — nothing detected means nothing to recap. Only an explicit
	// "y" enables it; empty, EOF, or non-interactive input defaults to no
	// and writes nothing, so a piped/interrupted setup can't flip it on.
	recaps := false
	if stores := detectedStoreNames(); len(stores) > 0 {
		fmt.Printf("\nAgent transcript stores found: %s\n", strings.Join(stores, ", "))
		fmt.Println("Dayflow can write a one-line recap of each coding-agent session. Transcripts")
		fmt.Println("are read locally either way; recaps send a bounded, scrubbed excerpt to")
		fmt.Println("your chat provider and to the decisions endpoint that judges sessions.")
		fmt.Print("Enable agent-session recaps? [y/N]: ")
		recaps = promptYes(r)
	}

	if err := setConfigValue("api_base_url", baseURL); err != nil {
		return err
	}
	if err := setConfigValue("openrouter_api_key", key); err != nil {
		return err
	}
	if err := setConfigValue("model", choice); err != nil {
		return err
	}
	if recaps {
		if err := setConfigValue("agent_recaps", "true"); err != nil {
			return err
		}
	}
	fmt.Printf("\nWrote %s (model=%s)\n", configPath(), choice)
	fmt.Println("Next: dayflow install && systemctl --user enable --now dayflow-capture.service")
	return nil
}

func printModelPresets() {
	b, _ := json.MarshalIndent(modelPresets(), "", "  ")
	fmt.Println(string(b))
}

func listModels(cfg Config) {
	printModelPresets()
	if cfg.APIBaseURL != "" {
		fmt.Println("\ncustom api_base_url set; list local models with your endpoint's /models route")
		return
	}
	models, err := fetchModels(cfg.OpenRouterAPIKey)
	if err != nil {
		fmt.Println("\ncould not fetch models:", err)
		return
	}
	fmt.Println("vision-capable models (image input):")
	for _, m := range models {
		for _, mod := range m.Architecture.InputModalities {
			if mod == "image" {
				fmt.Printf("  %s  in:%s\n", m.ID, m.Pricing.Prompt)
				break
			}
		}
	}
}

// doctorCheck is one doctor result; status is "ok", "warn", "info", or
// "fail". "info" is advisory — it never counts toward the failure total.
type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// collectDoctorChecks runs every check and returns results plus the failure
// count. Rendering (text or JSON) happens in runDoctor.
func collectDoctorChecks(cfg Config, deep bool) ([]doctorCheck, int) {
	var checks []doctorCheck
	fail := 0
	check := func(name string, ok bool, hint string) {
		if ok {
			checks = append(checks, doctorCheck{Name: name, Status: "ok"})
		} else {
			fail++
			checks = append(checks, doctorCheck{Name: name, Status: "fail", Detail: hint})
		}
	}
	warn := func(name, detail string) {
		checks = append(checks, doctorCheck{Name: name, Status: "warn", Detail: detail})
	}

	check("wayland session", os.Getenv("WAYLAND_DISPLAY") != "", "not running under Wayland")
	_, grimErr := exec.LookPath("grim")
	check("grim installed", grimErr == nil || cfg.CaptureCommand != "", "install grim or set capture_command")
	// output:"auto" only resolves on the grim path — a custom capture_command
	// never gets the -o injection, so the setting is silently dead there.
	if cfg.Output == "auto" && cfg.CaptureCommand != "" {
		warn("output auto", "output \"auto\" is ignored when capture_command is set — the custom command controls which monitor is grabbed")
	}
	check("config file", fileExists(configPath()), "run: dayflow setup")
	visionProvider, configured := configuredVisionProvider(cfg)
	check("api reachable", configured,
		"no usable vision provider — configure the routed provider's endpoint/key/model, or a cli command with allow_hot_path")
	// Each enabled cli provider's command must be on PATH — an absent binary
	// fails at call time, which is a doctor's job to catch early.
	for _, p := range effectiveProviders(cfg) {
		if !p.Enabled || p.Kind != "cli" {
			continue
		}
		_, err := exec.LookPath(p.Command)
		check("cli provider "+p.ID+" on PATH", err == nil,
			fmt.Sprintf("command %q not found — fix with: dayflow provider set %s command <name>", p.Command, p.ID))
	}
	// Verify the routed model rather than a stale legacy model/endpoint.
	visionCfg := cfg
	visionCfg.Model = visionProvider.Model
	visionCfg.APIBaseURL = visionProvider.APIBaseURL
	if providerNeedsAuth(visionProvider) {
		visionCfg.OpenRouterAPIKey = resolveProviderKey(visionProvider)
	}
	vis, reachable := isVisionModel(visionCfg, visionProvider.Model)
	if visionProvider.Kind == "cli" {
		checks = append(checks, doctorCheck{Name: "cli vision provider", Status: "info", Detail: "vision support is determined by the configured CLI"})
	} else if !reachable && visionCfg.APIBaseURL == "" {
		warn("model vision support", visionProvider.Model+" — couldn't verify vision support (offline?)")
	} else if visionCfg.APIBaseURL != "" {
		checks = append(checks, doctorCheck{Name: "custom endpoint", Status: "ok",
			Detail: visionCfg.APIBaseURL + " — vision support not verified"})
	} else {
		check("model is vision-capable", vis, visionProvider.Model+" cannot read images: dayflow config set model google/gemma-4-31b-it")
	}
	_, hyErr := exec.LookPath("hyprctl")
	if hyErr != nil && len(cfg.IgnoreApps) > 0 {
		warn("hyprctl", "hyprctl not found — ignore_apps won't work on this compositor")
	}
	// notify-send on PATH is not enough: the capture service runs under a
	// systemd env that only recently gained DBUS_SESSION_BUS_ADDRESS, and a
	// binary that exists still can't reach the user's session bus without it
	// (or GLib's $XDG_RUNTIME_DIR/bus fallback). Pure env/socket probe — a
	// test-send would be a side effect, not a check.
	if notificationBusReachable() {
		checks = append(checks, doctorCheck{Name: "notification bus", Status: "ok"})
	} else {
		warn("notification bus", "DBUS_SESSION_BUS_ADDRESS unset and $XDG_RUNTIME_DIR/bus missing — notifications from dayflow-capture won't reach the desktop daemon")
	}

	if _, err := os.Stat(dbPath()); os.IsNotExist(err) {
		warn("database", "no database yet — capture has not run")
	} else {
		sv, svErr := peekSchemaVersion()
		if svErr != nil {
			check("schema version", false, "could not read schema version: "+svErr.Error())
		} else if sv <= schemaVersion {
			checks = append(checks, doctorCheck{Name: "schema version", Status: "ok",
				Detail: fmt.Sprintf("database v%d, binary v%d", sv, schemaVersion)})
		} else {
			// A newer database than this binary understands can only be
			// fixed by upgrading the engine (or pointing it at the newer db).
			check("schema version", false,
				fmt.Sprintf("database at schema v%d, binary expects v%d — upgrade the engine", sv, schemaVersion))
		}
		if svErr == nil && sv < schemaVersion {
			warn("schema migration", fmt.Sprintf("database was at schema v%d; migrated to v%d — restart dayflow-capture and any dayflow mcp clients", sv, schemaVersion))
		}
		db, err := openDB()
		if err != nil {
			fail++
			checks = append(checks, doctorCheck{Name: "database open/migrate", Status: "fail", Detail: err.Error()})
		} else {
			defer db.Close()
			pragma := "quick_check"
			if deep {
				pragma = "integrity_check"
			}
			var qc string
			if err := db.QueryRow(`PRAGMA ` + pragma).Scan(&qc); err != nil {
				fail++
				checks = append(checks, doctorCheck{Name: "sqlite integrity", Status: "fail", Detail: err.Error()})
			} else {
				check("sqlite integrity", qc == "ok", pragma+": "+qc)
			}
			var fk int
			db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
			check("foreign keys", fk == 1, "foreign_keys pragma is off")
			if orphans, err := orphanFrameFiles(db); err == nil && len(orphans) > 0 {
				warn("orphan frames", fmt.Sprintf("%d file(s) under frames/ have no frames row — run: dayflow reconcile --dry-run", len(orphans)))
			}
		}
	}
	if fi, err := os.Stat(dataDir()); err == nil {
		check("data dir permissions", fi.Mode().Perm()&0o077 == 0,
			fmt.Sprintf("%s is %04o, want 0700", dataDir(), fi.Mode().Perm()))
	}
	if fi, err := os.Stat(configPath()); err == nil {
		check("config permissions", fi.Mode().Perm()&0o077 == 0,
			fmt.Sprintf("%s is %04o, want 0600", configPath(), fi.Mode().Perm()))
	}
	// Agent transcript stores: probe each DB-backed store read-only via
	// openStoreProbe (no temp-dir copy — Devin's sessions.db is multi-GB)
	// and run the adapter's own candidate extraction over a recent window,
	// so upstream schema drift lands in doctor with the same vocabulary
	// `agents --json` reports. Absent stores are info — an uninstalled tool
	// is not drift.
	for _, name := range []string{"opencode", "devin", "cursor"} {
		var present []string
		for _, p := range agentStoreDBs()[name] {
			if _, err := os.Stat(p); err == nil {
				present = append(present, p)
			}
		}
		if len(present) == 0 {
			checks = append(checks, doctorCheck{Name: "agent store " + name, Status: "info",
				Detail: "store absent — source not installed"})
			continue
		}
		if err := probeAgentStore(name, present); err != nil {
			warn("agent store "+name, err.Error())
		} else {
			checks = append(checks, doctorCheck{Name: "agent store " + name, Status: "ok",
				Detail: fmt.Sprintf("%d store(s) readable", len(present))})
		}
	}
	// Binary vs installed plugin manifest: the two upgrade together, so a
	// version gap means a half-applied upgrade. No manifest at all is an
	// engine-only install — info, not a failure.
	if mPath, mVer, err := pluginManifestVersion(); err != nil {
		warn("plugin manifest", mPath+" unreadable: "+err.Error())
	} else if mPath == "" {
		checks = append(checks, doctorCheck{Name: "plugin manifest", Status: "info",
			Detail: "engine-only install (no installed plugin manifest)"})
	} else if mVer != version {
		exe, _ := os.Executable()
		fail++
		checks = append(checks, doctorCheck{Name: "plugin manifest", Status: "fail",
			Detail: fmt.Sprintf("manifest %s at %s vs engine %s (%s) — upgrade both halves", mVer, mPath, version, exe)})
	} else {
		checks = append(checks, doctorCheck{Name: "plugin manifest", Status: "ok", Detail: "v" + mVer})
	}
	return checks, fail
}

// probeAgentStore opens each present store read-only and runs the
// source's own candidate extraction over a recent window. Extraction —
// not a {table→columns} assertion — is the contract: Cursor's two legal
// layouts would false-positive a schema check, and a renamed payload
// field wouldn't trip one.
func probeAgentStore(name string, paths []string) error {
	var errs []string
	for _, p := range paths {
		db, err := openStoreProbe(p)
		if err != nil {
			errs = append(errs, filepath.Base(p)+": "+err.Error())
			continue
		}
		err = probeAgentStoreExtraction(name, db)
		db.Close()
		if err != nil {
			errs = append(errs, filepath.Base(p)+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

// probeTailRows bounds every agent-store probe to a table's newest rows.
// The adapters' windowed extraction is a full-table scan on the big
// stores — Devin's message_nodes is multi-GB with no created_at index,
// Cursor's cursorDiskKV can't use its key index for a LIKE prefix, and
// composerHeaders' value is a per-row blob — so doctor runs the same
// tables/columns over a bounded tail instead of calling them.
const probeTailRows = 500

func probeAgentStoreExtraction(name string, db *sql.DB) error {
	e := time.Now()
	s := e.AddDate(0, 0, -90)
	switch name {
	case "opencode":
		layout := opencodeLayout(db)
		if layout == "" {
			return fmt.Errorf("schema drift: no known opencode layout")
		}
		return probeOpencodeStore(db, layout, s.UnixMilli(), e.UnixMilli())
	case "devin":
		return probeDevinStore(db, s.Unix(), e.Unix())
	case "cursor":
		hasKV := sqliteTableExists(db, "cursorDiskKV")
		switch {
		case sqliteTableExists(db, "composerHeaders"):
			return probeCursorHeaders(db)
		case hasKV:
			return probeCursorKVFallback(db)
		default:
			return fmt.Errorf("schema drift: no composer tables")
		}
	}
	return fmt.Errorf("unknown agent source %q", name)
}

// probeOpencodeStore exercises both opencode extraction reads over a
// rowid tail: the session join (opencodeCandidates' shape) and the
// message-table columns ocMessages reads (LENGTH(data) instead of the
// blob itself). time_created has no guaranteed index, so the unbounded
// window filter could full-scan a large store.
func probeOpencodeStore(db *sql.DB, layout string, sMs, eMs int64) error {
	msgTable := "session_message"
	if layout == "old" {
		msgTable = "message"
	}
	rows, err := db.Query(`SELECT s.id, s.title, s.directory FROM session s
	  WHERE s.id IN (SELECT session_id FROM `+msgTable+`
	    WHERE rowid > (SELECT COALESCE(MAX(rowid),0) FROM `+msgTable+`) - ?
	      AND time_created >= ? AND time_created < ?)
	  ORDER BY s.time_created`, probeTailRows, sMs, eMs)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var title, dir sql.NullString
		if err := rows.Scan(&id, &title, &dir); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	if layout == "old" {
		mq := `SELECT id, session_id, time_created, time_updated,
		  COALESCE(LENGTH(data),0) FROM message
		  WHERE rowid > (SELECT COALESCE(MAX(rowid),0) FROM message) - ?`
		if err := probeScan(db, mq, probeTailRows); err != nil {
			return err
		}
		return probeScan(db, `SELECT message_id, COALESCE(LENGTH(data),0)
		  FROM part
		  WHERE rowid > (SELECT COALESCE(MAX(rowid),0) FROM part) - ?`,
			probeTailRows)
	}
	return probeScan(db, `SELECT id, session_id, type, seq, time_created,
	  time_updated, COALESCE(LENGTH(data),0) FROM session_message
	  WHERE rowid > (SELECT COALESCE(MAX(rowid),0) FROM session_message) - ?`,
		probeTailRows)
}

// probeDevinStore runs devinCandidates' join shape and devinDBMessages'
// column set over the row_id tail — message_nodes has no created_at
// index, so the adapter's windowed IN subquery would full-scan a
// multi-GB table on every doctor run.
func probeDevinStore(db *sql.DB, sSec, eSec int64) error {
	if !sqliteTableExists(db, "sessions") || !sqliteTableExists(db, "message_nodes") {
		return fmt.Errorf("schema drift: devin store missing sessions/message_nodes")
	}
	rows, err := db.Query(`SELECT s.id, s.title, s.working_directory FROM sessions s
	  WHERE s.id IN (SELECT session_id FROM message_nodes
	    WHERE row_id > (SELECT COALESCE(MAX(row_id),0) FROM message_nodes) - ?
	      AND created_at >= ? AND created_at < ?)
	  ORDER BY s.created_at`, probeTailRows, sSec, eSec)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var title, dir sql.NullString
		if err := rows.Scan(&id, &title, &dir); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	// LENGTH() reads the record header, never the chat_message payload —
	// the column is exercised without paying for the blob.
	return probeScan(db, `SELECT node_id, created_at,
	  COALESCE(LENGTH(chat_message),0) FROM message_nodes
	  WHERE row_id > (SELECT COALESCE(MAX(row_id),0) FROM message_nodes) - ?`,
		probeTailRows)
}

// probeCursorHeaders exercises composerHeaders over the rowid tail:
// cheap columns for the tail rows, then the value blob for a few of
// them — the adapter's SELECT includes value for every row, which reads
// every blob in the table.
func probeCursorHeaders(db *sql.DB) error {
	rows, err := db.Query(`SELECT composerId, workspaceId, createdAt, lastUpdatedAt
	  FROM composerHeaders
	  WHERE rowid > (SELECT COALESCE(MAX(rowid),0) FROM composerHeaders) - ?`,
		probeTailRows)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var h cursorHeader
		var ws, created, updated any
		if err := rows.Scan(&h.id, &ws, &created, &updated); err != nil {
			rows.Close()
			return err
		}
		h.workspace = cursorStr(ws)
		h.created = cursorMs(created)
		h.updated = cursorMs(updated)
		if len(ids) < 3 {
			ids = append(ids, h.id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		var v any
		if err := db.QueryRow(`SELECT value FROM composerHeaders
		  WHERE composerId = ?`, id).Scan(&v); err != nil {
			return err
		}
	}
	return nil
}

// probeCursorKVFallback is cursorHeaderFallback's read bounded and made
// index-usable: 'composerData;' is the byte after ':' so the range covers
// exactly the composerData:* keys the LIKE 'composerData:%' pattern
// matched, and the key index keeps it off a full cursorDiskKV scan.
func probeCursorKVFallback(db *sql.DB) error {
	rows, err := db.Query(`SELECT key, value FROM cursorDiskKV
	  WHERE key >= 'composerData:' AND key < 'composerData;'
	  ORDER BY key LIMIT ?`, probeTailRows)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var value any
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		// Undecodable blobs are skipped by the adapter's fallback too —
		// the probe's contract is that the read succeeds, not that every
		// payload parses.
		var d struct {
			ComposerID string `json:"composerId"`
			CreatedAt  any    `json:"createdAt"`
			UpdatedAt  any    `json:"lastUpdatedAt"`
			IsDraft    bool   `json:"isDraft"`
			Name       string `json:"name"`
		}
		if json.Unmarshal(cursorBytes(value), &d) != nil {
			continue
		}
	}
	return rows.Err()
}

// probeScan drains a tail-bounded query's rows into loosely-typed values —
// the probe asserts the read succeeds, so per-column types don't matter.
func probeScan(db *sql.DB, query string, args ...any) error {
	rows, err := db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		dest := make([]any, len(cols))
		for i := range dest {
			dest[i] = new(any)
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
	}
	return rows.Err()
}

func runDoctor(cfg Config, jsonOut bool, deep bool) {
	checks, fail := collectDoctorChecks(cfg, deep)
	_, configured := configuredVisionProvider(cfg)
	if jsonOut {
		out := map[string]interface{}{
			"checks":         checks,
			"failures":       fail,
			"version":        version,
			"schema_version": schemaVersion,
			"configured":     fileExists(configPath()) && configured,
			"data_dir":       dataDir(),
			"config_path":    configPath(),
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	} else {
		for _, c := range checks {
			switch c.Status {
			case "ok":
				if c.Detail != "" {
					fmt.Printf("  ok   %s — %s\n", c.Name, c.Detail)
				} else {
					fmt.Printf("  ok   %s\n", c.Name)
				}
			case "warn":
				fmt.Printf("  warn %s — %s\n", c.Name, c.Detail)
			case "info":
				fmt.Printf("  info %s — %s\n", c.Name, c.Detail)
			default:
				fmt.Printf("  FAIL %s — %s\n", c.Name, c.Detail)
			}
		}
		fmt.Printf("  engine version: %s (schema v%d)\n", version, schemaVersion)
		fmt.Printf("  data: %s\n  config: %s\n", dataDir(), configPath())
	}
	if fail > 0 {
		os.Exit(1)
	}
}

// probeEndpoint reports whether a local inference endpoint answers /models.
func probeEndpoint(base string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(strings.TrimSuffix(base, "/") + "/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// detectResult is the `detect` payload. Agents reports which coding-agent
// transcript stores exist locally — Onboarding.qml gates its recaps
// consent step on this map, and runSetup gates its recaps prompt on it.
type detectResult struct {
	Ollama   bool            `json:"ollama"`
	LMStudio bool            `json:"lmstudio"`
	Agents   map[string]bool `json:"agents"`
	Presets  []ModelPreset   `json:"presets"`
}

func runDetect(jsonOut bool) {
	d := detectResult{
		Ollama:   probeEndpoint("http://localhost:11434/v1"),
		LMStudio: probeEndpoint("http://localhost:1234/v1"),
		Agents:   agentStoresDetected(),
		Presets:  modelPresets(),
	}
	if jsonOut {
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Printf("ollama:   %v\nlmstudio: %v\n", d.Ollama, d.LMStudio)
	if d.Ollama || d.LMStudio {
		fmt.Println("local endpoint found — a local model works without an API key")
	}
	var stores []string
	for name, ok := range d.Agents {
		if ok {
			stores = append(stores, name)
		}
	}
	sort.Strings(stores)
	if len(stores) > 0 {
		fmt.Println("agent stores: " + strings.Join(stores, ", "))
	}
	printModelPresets()
}

// peekSchemaVersion reads the recorded schema version without running
// migrations. Returns 0 for a database that predates schema_migrations; a
// read failure (corrupt file, locked db) is returned as an error so callers
// can report it as what it is instead of "schema v0".
func peekSchemaVersion() (int, error) {
	db, err := sql.Open("sqlite", dbPath()+"?_pragma=query_only(1)")
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil // unversioned database — predates schema_migrations
		}
		return 0, err
	}
	return v, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// notificationBusReachable reports whether a session D-Bus is plausibly
// reachable from this environment: an explicit DBUS_SESSION_BUS_ADDRESS,
// or the socket at $XDG_RUNTIME_DIR/bus that GLib falls back to. Pure
// env/socket inspection — doctor must not send test notifications.
func notificationBusReachable() bool {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return true
	}
	rt := os.Getenv("XDG_RUNTIME_DIR")
	if rt == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(rt, "bus"))
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// pluginManifestPath locates the installed omarchy plugin manifest — the
// engine binary and the plugin upgrade together, so version drift between
// them means a half-applied upgrade.
func pluginManifestPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "omarchy", "plugins", "io.github.duketopceo.dayflow", "manifest.json")
}

// pluginManifestVersion reads the installed plugin manifest's version.
// An absent manifest returns ("", "", nil) — an engine-only install is
// not a failure; other read/parse failures come back as err.
func pluginManifestVersion() (path, ver string, err error) {
	path = pluginManifestPath()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", nil
		}
		return path, "", err
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return path, "", err
	}
	return path, m.Version, nil
}

// agentStoresDetected reports which agent transcript stores exist on this
// machine, keyed by source name. It reuses the adapters' own store-root
// resolution (including their DAYFLOW_*_DIR/DB test overrides) so detection
// can never disagree with what a scan would find.
func agentStoresDetected() map[string]bool {
	opencode := false
	for _, p := range opencodeDBPaths() {
		if fileExists(p) {
			opencode = true
			break
		}
	}
	return map[string]bool{
		"claude":   fileExists(claudeDir()),
		"codex":    fileExists(codexDir()),
		"opencode": opencode,
		"devin":    fileExists(devinDir()),
		"cursor":   fileExists(cursorDBPath()),
	}
}

// detectedStoreNames is the sorted subset of agent sources with a store
// present — the gate behind both the detect payload consumers and the
// setup recaps prompt.
func detectedStoreNames() []string {
	var names []string
	for name, ok := range agentStoresDetected() {
		if ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// promptYes reads one answer line; only an explicit "y"/"yes" is true.
// EOF, empty input, and anything else are a no — the recaps opt-in must
// default off on piped or interrupted stdin.
func promptYes(r *bufio.Reader) bool {
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
