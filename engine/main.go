package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const version = "1.5.0"

// positionalArgs drops --flags but keeps single-dash args — every real
// flag here is long-form, so `-1` is a typo'd positional (a bad date),
// not a flag, and must surface as an error rather than silently
// defaulting to today. An empty arg is skipped (HasPrefix on "" is safe
// but a bare "" is never a positional either).
func positionalArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if a != "" && !strings.HasPrefix(a, "--") {
			out = append(out, a)
		}
	}
	return out
}

// dateArg scans args for a YYYY-MM-DD date, erroring on malformed positional
// args so a typo never silently reports the wrong day.
func dateArg(args []string, def time.Time) (time.Time, error) {
	d := def
	for _, a := range positionalArgs(args) {
		parsed, err := time.ParseInLocation("2006-01-02", a, time.Local)
		if err != nil {
			return d, fmt.Errorf("bad date %q (want YYYY-MM-DD)", a)
		}
		d = parsed
	}
	return d, nil
}

// readStdin reads one line from stdin — used by `config set -`,
// `config patch -`, and `provider set <id> <key> -` so secrets never appear
// in argv. Line-based (not ReadAll) because callers like Quickshell keep the
// pipe open after writing, so waiting for EOF would hang.
func readStdin() string {
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}

func usage() {
	fmt.Fprintf(os.Stderr, `dayflow %s — private, automatic work journal for Wayland/Omarchy

Usage: dayflow <command> [args]

Engine:
  daemon              Run the capture loop in the foreground (for systemd)
  summarize [--now] [--retry]   Summarize all complete pending blocks
                          (--now includes current, --retry resets failed/dead)
  install             Write + enable systemd user units (capture service, summarize timer)
  uninstall           Disable and remove the systemd units

Query:
  today [--json]      Print today's timeline
  timeline [--json] [YYYY-MM-DD]   Print a day's timeline
  day <YYYY-MM-DD> [--json] [--grid]   Timeline, or the daily workflow grid
  status [--json]     Show recording state and counts
  frames [YYYY-MM-DD] [--json]   List captured frames for a day
  agents [YYYY-MM-DD] [--json] [--no-recaps]   Coding-agent sessions + recaps
                          (Claude Code, Codex, OpenCode, Devin, Cursor)
  briefing [YYYY-MM-DD] [--json] [--refresh] Day briefing: sessions grouped into
                          workstreams with condensed turns + status
  ingest [--json] [--reindex]   Index agent-chat turns into the local FTS
                          store (--reindex wipes and rebuilds the index)
  search-agents <q> [--json]   Full-text search indexed agent conversations
  sync [YYYY-MM-DD] [--json]   Push the day's distilled atoms to the Kurultai
                          brain (knowledge_sync in config; opt-in egress)
  ask <question> [--json]   One-shot chat over journal + agent history
  forecast [YYYY-MM-DD] [--json]   Predict a day's category mix from history
                          (default: tomorrow)
  playback [on|off|status] [--json]   Opt-in frame retention for timelapse
                          playback (applies the standard storage cap)
  blocks [--json]     List blocks that failed summarization
  standup [--json]    Generate a standup update from yesterday/today
  standup draft [--date YYYY-MM-DD]   Print the saved standup draft as JSON
  standup save [--date D] [--highlights S] [--tasks S] [--blockers S]
               [--priorities S]       Save the editable standup draft
  goal [set <text>|done|clear] [--date D] [--json]   Today's day goal + streak
  insights [day|week|month] [--json]  Focus, category, app, and distraction analytics
  review [day|week|month] [--json]  AI-generated weekly review with corrections and advice

Control:
  pause | resume | toggle   Control screen capture
  config                  Print config path and current config
  config set <key> <val>  Update config (model, api_base_url, capture_interval_sec,
                          block_minutes, frames_per_block, jpeg_quality, keep_frames,
                          frame_max_dim, retention_days, max_storage_mb, auto_pause_locked,
                          ignore_apps, output, capture_command, openrouter_api_key,
                          provider, filter_inappropriate, panel_expanded, debug)
                          Use "-" as the value to read it from stdin (keeps
                          secrets out of argv and shell history)
  config patch <json|-> Merge a JSON object into the config
  key set|status|del    Store/inspect API keys in OmaSeal instead of config.json
  log <msg>             Append a UI action line to debug.log
  log [--limit N] [--json]   Tail debug.log (default last 50 lines)
  provider [list]       List configured providers and routing
  provider add <id> <kind>          Add a provider (openrouter, local, custom,
                                    gemini, chatgpt, claude, mcp, cli)
  provider set <id> <key> <value>   Update a provider (name, kind, api_base_url,
                                    api_key, model, enabled, vision, chat,
                                    title_prompt, summary_prompt,
                                    detailed_prompt, chat_prompt;
                                    cli: command, args, cli_timeout_sec,
                                    env_passthrough, scratch_home,
                                    allow_hot_path)
  provider remove <id>              Remove a provider
  provider test <id|task>           Test a provider or a routed task (vision,
                                    summary, detailed, chat, review, standup)
  ignore [--active|class] Add an app to the ignore list (--active = focused window)
  unignore <class>        Remove an app from the ignore list
  events [--json] [-n N]  Recent event log (captures, skips, errors, summaries)
  usage [--days N] [--json]   Token/cost usage over the last N local days
                          (default: all retained rows; shows coverage floor)
  stats [--json]          Storage, block counts, date range, and API usage
  week | month [--json]   Timeline rollups
  weekly [--json]        Weekly analytics payload (donut, treemap, context shifts, highlights)
  export [day|YYYY-MM-DD|week|month] [--brief] [--out <file>|--copy]
                                    Markdown export (--brief = one line per span)
  mcp [--read-only]       Run the MCP server over stdio (for agents);
                          --read-only hides and blocks the chat tool
  tui                     Interactive terminal timeline (day/week/month, search, standup, insights)
  search <query>          Search block titles, summaries, and apps
                          (--reindex rebuilds the search index)
  chat [message] [--conversation-id N] [--json]  Ask a question about the journal
  conversations [--json]  List saved chat conversations
  conversation <id> [--json]  Print one conversation's messages
  retry                   Reset failed/dead blocks for re-summarization
  reconcile [--dry-run]   Report or quarantine frame files missing from the index
  backup [dir] [--no-frames]  Snapshot db + config (redacted) + frames to dir
  restore <dir> [--force] Restore a backup (stop capture service first)
  backup-verify <dir>     Check a backup's manifest and db integrity
  scrub <query>           Delete blocks whose title or summary contains <query>
  edit <start> <field> <value>   Correct a block's title, category, summary,
                          or productive flag. <start> is a Unix timestamp or
                          "YYYY-MM-DD HH:MM" (local time)
  edits <start> [--json]  List the edit history for a block

Setup & health:
  setup                   Interactive AI-provider onboarding (OpenRouter or local endpoint)
  models                  List vision-capable models on your OpenRouter account
  doctor [--json] [--deep]  Check session, grim, key, model, endpoint;
                          --deep runs a full sqlite integrity check
  detect [--json]         Probe for local model endpoints (Ollama, LM Studio)
  fixtures capture <source> [--db P] [--out F]   Regenerate an agent-store
                          contract fixture — schema + sentinel rows only,
                          never real payloads (sources: claude codex
                          opencode devin cursor; run from engine/)

Config: %s
Data:   %s
`, version, configPath(), dataDir())
	os.Exit(2)
}

func hasFlag(args []string, f string) bool {
	for _, a := range args {
		if a == "--" {
			return false // everything after -- is positional, not flags
		}
		if a == f {
			return true
		}
	}
	return false
}

// flagValue returns the value following a `--name value` or `--name=value` flag.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == "--" {
			return "" // everything after -- is positional, not flags
		}
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return a[len(name)+1:]
		}
	}
	return ""
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd, args := os.Args[1], os.Args[2:]
	jsonOut := hasFlag(args, "--json")

	cfg, err := loadConfig()
	if err != nil {
		fatal(err)
	}

	switch cmd {
	case "daemon":
		fatal(runDaemon(cfg))

	case "summarize":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		if hasFlag(args, "--retry") {
			m, err := resetFailedBlocks(db)
			fatal(err)
			fmt.Fprintf(os.Stderr, "reset %d failed/dead block(s)\n", m)
		}
		n, err := summarizePending(db, cfg, hasFlag(args, "--now"))
		fatal(err)
		if !jsonOut {
			fmt.Printf("summarized %d block(s)\n", n)
		} else {
			json.NewEncoder(os.Stdout).Encode(map[string]int{"summarized": n})
		}

	case "today":
		printTimeline(cfg, time.Now(), jsonOut)

	case "timeline":
		// timeline [--json] [YYYY-MM-DD]
		d, err := dateArg(args, time.Now())
		fatal(err)
		printTimeline(cfg, d, jsonOut)

	case "day":
		var d time.Time
		for _, a := range args {
			if a != "" && a[0] != '-' {
				var err error
				d, err = time.ParseInLocation("2006-01-02", a, time.Local)
				fatal(err)
			}
		}
		if d.IsZero() {
			if hasFlag(args, "--grid") {
				d = time.Now()
			} else {
				usage()
			}
		}
		if hasFlag(args, "--grid") {
			printDailyGrid(cfg, d, jsonOut)
			break
		}
		printTimeline(cfg, d, jsonOut)

	case "status":
		printStatus(cfg, jsonOut)

	case "frames":
		// frames [YYYY-MM-DD] [--json] — list captured frames for a day
		d, err := dateArg(args, time.Now())
		fatal(err)
		db, err := openDB()
		fatal(err)
		defer db.Close()
		printFrames(db, d, jsonOut)

	case "playback":
		// playback [on|off|status] [--json] — opt-in frame retention for
		// timelapse playback; enabling applies the standard storage cap.
		sub := "status"
		for _, a := range args {
			if a != "" && a[0] != '-' {
				sub = a
			}
		}
		switch sub {
		case "on", "off":
			cfg2, changed := setPlayback(cfg, sub == "on")
			if changed {
				fatal(writeConfig(cfg2))
			}
			cfg = cfg2
			printPlaybackStatus(cfg, jsonOut)
		case "status":
			printPlaybackStatus(cfg, jsonOut)
		default:
			usage()
		}

	case "agents":
		// agents [YYYY-MM-DD] [--json] [--no-recaps] — coding-agent sessions
		// with generated recaps (cached; --no-recaps for a fast local list)
		d, err := dateArg(args, time.Now())
		fatal(err)
		// Drift bookkeeping reads/writes meta+events even under --no-recaps
		// — degrade to metadata-only rather than failing the command.
		db := openDBLenient("agents")
		if db != nil {
			defer db.Close()
		}
		printAgentSessions(db, cfg, d, jsonOut, !hasFlag(args, "--no-recaps"))

	case "briefing":
		// briefing [YYYY-MM-DD] [--json] [--refresh] — the day's agent
		// briefing: sessions grouped into workstreams with condensed
		// turns + per-thread status. Model prose under agent_recaps;
		// deterministic fallback otherwise. --refresh regenerates past
		// the per-day cache.
		d, err := dateArg(args, time.Now())
		fatal(err)
		db := openDBLenient("briefing")
		if db != nil {
			defer db.Close()
		}
		printBriefing(db, cfg, d, jsonOut, hasFlag(args, "--refresh"))

	case "ingest":
		db := openDBLenient("ingest")
		if db == nil {
			os.Exit(1)
		}
		defer db.Close()
		if hasFlag(args, "--reindex") {
			fatal(resetAgentIndex(db))
		}
		n, early, err := ingestAgentChats(db, agentIngestCLIBudget)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"indexed": n, "pending": early})
		} else {
			fmt.Printf("indexed %d agent turn(s)\n", n)
			if early {
				fmt.Println("budget reached — more turns pending; run again or let the daemon finish")
			}
		}

	case "search-agents":
		db := openDBLenient("search-agents")
		if db == nil {
			os.Exit(1)
		}
		defer db.Close()
		q := strings.Join(positionalArgs(args), " ")
		if q == "" {
			usage()
		}
		printAgentSearch(db, cfg, q, jsonOut)

	case "ask":
		// ask <question> [--json] — one-shot journal+agent-history chat;
		// same engine as `dayflow chat`, no conversation flag needed.
		db := openDBLenient("ask")
		if db == nil {
			os.Exit(1)
		}
		defer db.Close()
		msg := strings.Join(positionalArgs(args), " ")
		if msg == "" {
			usage()
		}
		res, err := chatWithJournal(db, cfg, 0, msg)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(res)
		} else {
			fmt.Println(res.Reply)
		}

	case "forecast":
		// forecast [YYYY-MM-DD] [--json] — predict a day's category mix from
		// same-weekday history (default: tomorrow)
		d, err := dateArg(args, time.Now().AddDate(0, 0, 1))
		fatal(err)
		db, err := openDB()
		fatal(err)
		defer db.Close()
		printForecast(db, cfg, d, jsonOut)

	case "blocks":
		printFailed(cfg, jsonOut)

	case "pause":
		setPaused(true)
		db, _ := openDB()
		logEvent(db, "paused", "")
		fmt.Println("paused")
	case "resume":
		setPaused(false)
		db, _ := openDB()
		logEvent(db, "resumed", "")
		fmt.Println("resumed")
	case "toggle":
		setPaused(!paused())
		db, _ := openDB()
		if paused() {
			logEvent(db, "paused", "")
		} else {
			logEvent(db, "resumed", "")
		}
		if paused() {
			fmt.Println("paused")
		} else {
			fmt.Println("resumed")
		}

	case "config":
		jsonOut := false
		if len(args) >= 1 && args[0] == "--json" {
			jsonOut = true
			args = args[1:]
		}
		if len(args) >= 2 && args[0] == "set" {
			if len(args) < 3 {
				fatal(fmt.Errorf("usage: dayflow config set <key> <value|->"))
			}
			val := args[2]
			if val == "-" {
				val = strings.TrimSpace(readStdin())
			}
			if args[1] == "model" {
				if vis, ok := isVisionModel(cfg, val); ok && !vis {
					fatal(fmt.Errorf("model %q cannot read images — dayflow needs a vision model (see 'dayflow models')", val))
				}
			}
			fatal(setConfigValue(args[1], val))
			fmt.Println("set", args[1])
			break
		}
		if len(args) >= 1 && args[0] == "patch" {
			if len(args) < 2 {
				fatal(fmt.Errorf("usage: dayflow config patch <json|->"))
			}
			patch := args[1]
			if patch == "-" {
				patch = readStdin() // keeps key material out of argv
			}
			if patch == "" {
				fatal(fmt.Errorf("config patch requires a JSON object"))
			}
			fatal(patchConfig(patch))
			fmt.Println("config patched")
			break
		}
		masked := cfg
		if masked.OpenRouterAPIKey != "" {
			masked.OpenRouterAPIKey = "***redacted***"
		}
		for i := range masked.Providers {
			if masked.Providers[i].APIKey != "" {
				masked.Providers[i].APIKey = "***redacted***"
			}
		}
		if jsonOut {
			b, _ := json.MarshalIndent(map[string]any{"path": configPath(), "config": masked}, "", "  ")
			fmt.Println(string(b))
		} else {
			b, _ := json.MarshalIndent(masked, "", "  ")
			fmt.Printf("%s\n%s\n", configPath(), b)
		}

	case "ignore":
		var cls string
		for _, a := range args {
			if a == "--active" {
				cls = activeWindowClass()
			} else if a != "" && a[0] != '-' {
				cls = a
			}
		}
		if cls == "" {
			fatal(fmt.Errorf("no app class given — use --active on Hyprland or pass a class"))
		}
		for _, ig := range cfg.IgnoreApps {
			if strings.EqualFold(ig, cls) {
				if jsonOut {
					json.NewEncoder(os.Stdout).Encode(map[string]string{"already_ignored": cls})
				} else {
					fmt.Println("already ignored:", cls)
				}
				return
			}
		}
		if err := setConfigValue("ignore_apps", strings.Join(append(cfg.IgnoreApps, cls), ",")); err != nil {
			fatal(err)
		}
		db, _ := openDB()
		logEvent(db, "app_ignored", cls)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]string{"ignored": cls})
		} else {
			fmt.Println("Now ignoring:", cls)
		}

	case "unignore":
		if len(args) == 0 {
			usage()
		}
		cls := args[0]
		var kept []string
		for _, ig := range cfg.IgnoreApps {
			if !strings.EqualFold(ig, cls) {
				kept = append(kept, ig)
			}
		}
		fatal(setConfigValue("ignore_apps", strings.Join(kept, ",")))
		db, _ := openDB()
		logEvent(db, "app_unignored", cls)
		fmt.Println("unignored:", cls)

	case "events":
		printEvents(cfg, args, jsonOut)

	case "usage":
		// usage [--days N] [--json] — N bounds the window to the last N
		// local days; omitted = all retained rows.
		days, err := usageDays(args)
		fatal(err)
		printUsage(cfg, jsonOut, days)

	case "stats":
		printStats(cfg, jsonOut)

	case "week", "month":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		var start, end time.Time
		if cmd == "week" {
			start, end = weekBounds(time.Now())
		} else {
			start, end = monthBounds(time.Now())
		}
		blocks, err := blocksBetween(db, start, end)
		fatal(err)
		if jsonOut {
			payload := timelineJSON(blocks)
			payload["start"] = start.Format("2006-01-02")
			payload["end"] = end.Format("2006-01-02")
			json.NewEncoder(os.Stdout).Encode(payload)
		} else {
			fmt.Print(markdownTimeline(blocks, "dayflow "+cmd))
		}

	case "weekly":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		start, end := weekBounds(time.Now())
		p, err := generateWeeklyPayload(db, cfg, start, end)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(p)
		} else {
			fmt.Print(formatWeeklyPayload(p, start, end))
		}

	case "standup":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		// standup draft|save — subcommand is the first positional arg.
		var sub string
		var rest []string
		for i, a := range args {
			if i == 0 && a != "" && a[0] != '-' {
				sub = a
			} else {
				rest = append(rest, a)
			}
		}
		today := time.Now().Format("2006-01-02")
		switch sub {
		case "draft", "load":
			date := flagValue(rest, "--date")
			if date == "" {
				date = today
			}
			d, err := loadStandupDraft(db, date)
			fatal(err)
			json.NewEncoder(os.Stdout).Encode(d)
			return
		case "save":
			date := flagValue(rest, "--date")
			if date == "" {
				date = today
			}
			fatal(saveStandupDraft(db, date,
				flagValue(rest, "--highlights"),
				flagValue(rest, "--tasks"),
				flagValue(rest, "--blockers"),
				flagValue(rest, "--priorities")))
			if jsonOut {
				d, _ := loadStandupDraft(db, date)
				json.NewEncoder(os.Stdout).Encode(d)
			} else {
				fmt.Println("saved standup draft for", date)
			}
			return
		case "":
			// normal standup generation below
		default:
			usage()
		}
		md, j, err := generateStandup(db, cfg, jsonOut)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(j)
		} else {
			fmt.Print(md)
		}

	case "goal":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		// goal [set <text>|done|clear] [--date YYYY-MM-DD]
		var sub string
		var rest []string
		for i, a := range args {
			if i == 0 && a != "" && a[0] != '-' {
				sub = a
			} else {
				rest = append(rest, a)
			}
		}
		date := flagValue(rest, "--date")
		if date == "" {
			date = time.Now().Format("2006-01-02")
		} else if _, err := time.ParseInLocation("2006-01-02", date, time.Local); err != nil {
			fatal(fmt.Errorf("bad --date %q — expected YYYY-MM-DD", date))
		}
		switch sub {
		case "set":
			text := ""
			for i := 0; i < len(rest); i++ {
				a := rest[i]
				if a == "--date" {
					i++
					continue
				}
				if a != "" && a[0] != '-' {
					text = a
					break
				}
			}
			if text == "" {
				fatal(fmt.Errorf("goal set requires text: dayflow goal set \"...\" [--date D]"))
			}
			fatal(setGoal(db, date, text))
		case "done":
			fatal(completeGoal(db, date, true))
		case "clear":
			fatal(completeGoal(db, date, false))
		case "":
			// show
		default:
			usage()
		}
		g, err := getGoal(db, date)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(g)
		} else {
			if g.Goal == "" {
				fmt.Println("no goal set for", date)
			} else {
				mark := " "
				if g.Completed {
					mark = "✓"
				}
				fmt.Printf("[%s] %s — %s\n", mark, date, g.Goal)
			}
			if g.Streak.Current > 0 || g.Streak.Total > 0 {
				fmt.Printf("    streak %dd · best %dd · %d total\n",
					g.Streak.Current, g.Streak.Best, g.Streak.Total)
			}
		}

	case "insights":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		var start, end time.Time
		label := "dayflow insights"
		sel := "week"
		for _, a := range args {
			if a != "" && a[0] != '-' {
				sel = a
			}
		}
		switch sel {
		case "day", "today":
			start, end = dayBounds(time.Now())
			label = "dayflow insights — today"
		case "yesterday":
			start, end = dayBounds(time.Now().AddDate(0, 0, -1))
			label = "dayflow insights — yesterday"
		case "week":
			start, end = weekBounds(time.Now())
			label = "dayflow insights — this week"
		case "month":
			start, end = monthBounds(time.Now())
			label = "dayflow insights — this month"
		default:
			usage()
		}
		in, err := generateInsights(db, cfg, start, end)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(in.JSON())
		} else {
			fmt.Print(formatInsightsMarkdown(in, start, end, label))
		}

	case "review":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		var start, end time.Time
		rangeLabel := "this week"
		sel := "week"
		for _, a := range args {
			if a != "" && a[0] != '-' {
				sel = a
			}
		}
		switch sel {
		case "day", "today":
			start, end = dayBounds(time.Now())
			rangeLabel = "today"
		case "yesterday":
			start, end = dayBounds(time.Now().AddDate(0, 0, -1))
			rangeLabel = "yesterday"
		case "week":
			start, end = weekBounds(time.Now())
		case "month":
			start, end = monthBounds(time.Now())
			rangeLabel = "this month"
		default:
			usage()
		}
		text, pt, ct, err := reviewRange(db, cfg, start, end, rangeLabel)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{
				"review":            text,
				"prompt_tokens":     pt,
				"completion_tokens": ct,
			})
		} else {
			fmt.Println(text)
			if pt+ct > 0 {
				fmt.Printf("\n(tokens: %d prompt + %d completion)\n", pt, ct)
			}
		}

	case "search":
		query, reindex := parseSearchArgs(args)
		if reindex {
			printReindex(jsonOut)
			break
		}
		if query == "" {
			usage()
		}
		printSearch(query, jsonOut)

	case "chat":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		var convID int64
		if s := flagValue(args, "--conversation-id"); s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				convID = n
			}
		}
		var msgParts []string
		endOfFlags := false
		for i := 0; i < len(args); i++ {
			a := args[i]
			if !endOfFlags {
				if a == "--" {
					endOfFlags = true
					continue
				}
				if a == "--conversation-id" {
					i++ // consume the value — it is not message text
					continue
				}
				if strings.HasPrefix(a, "--conversation-id=") || (a != "" && a[0] == '-') {
					continue // flag, not message text
				}
			}
			msgParts = append(msgParts, a)
		}
		msg := strings.Join(msgParts, " ")
		if msg == "" {
			if jsonOut {
				fatal(fmt.Errorf("interactive chat does not support --json"))
			}
			fmt.Println("Chat with your journal. Type 'exit' to quit.")
			sc := bufio.NewScanner(os.Stdin)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "exit" {
					break
				}
				if line == "" {
					continue
				}
				res, err := chatWithJournal(db, cfg, convID, line)
				fatal(err)
				fmt.Println(res.Reply)
				convID = res.ConversationID
			}
			break
		}
		res, err := chatWithJournal(db, cfg, convID, msg)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(res)
		} else {
			fmt.Println(res.Reply)
		}

	case "conversations":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		convs, err := listConversations(db, 20)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(convs)
		} else {
			if len(convs) == 0 {
				fmt.Println("no conversations")
			} else {
				for _, c := range convs {
					fmt.Printf("%d: %s\n", c.ID, c.Title)
				}
			}
		}

	case "conversation":
		// conversation <id> [--json] — print one thread's messages
		var pos []string
		for _, a := range args {
			if a != "" && a[0] != '-' {
				pos = append(pos, a)
			}
		}
		if len(pos) == 0 {
			fatal(fmt.Errorf("conversation requires an id"))
		}
		convID, err := strconv.ParseInt(pos[0], 10, 64)
		fatal(err)
		db, err := openDB()
		fatal(err)
		defer db.Close()
		conv, err := getConversation(db, convID)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(conv)
		} else {
			fmt.Printf("%s\n", conv.Title)
			for _, m := range conv.Messages {
				fmt.Printf("\n[%s] %s\n%s\n",
					time.Unix(m.CreatedAt, 0).Local().Format("15:04"),
					strings.ToUpper(m.Role), m.Content)
			}
		}

	case "scrub":
		if len(args) == 0 || args[0][0] == '-' {
			usage()
		}
		db, err := openDB()
		fatal(err)
		defer db.Close()
		n, err := deleteBlocksLike(db, args[0])
		fatal(err)
		fmt.Printf("deleted %d block(s) matching %q\n", n, args[0])

	case "edit":
		// edit <start_ts|"YYYY-MM-DD HH:MM"> <field> <value...>
		var pos []string
		for _, a := range args {
			if a != "" && a[0] != '-' {
				pos = append(pos, a)
			}
		}
		ts, rest, err := parseBlockStart(pos)
		fatal(err)
		if len(rest) < 2 {
			fatal(fmt.Errorf("usage: dayflow edit <start> <title|category|summary|productive> <value>"))
		}
		db, err := openDB()
		fatal(err)
		defer db.Close()
		field := rest[0]
		value := strings.Join(rest[1:], " ")
		fatal(saveBlockEdit(db, cfg, ts, field, value))
		logEvent(db, "block_edited", fmt.Sprintf("%d %s", ts, field))
		if jsonOut {
			b, _ := loadBlockWithEdits(db, ts)
			json.NewEncoder(os.Stdout).Encode(map[string]any{"block": b})
		} else {
			fmt.Printf("edited %s on block %s\n", field,
				time.Unix(ts, 0).Local().Format("2006-01-02 15:04"))
		}

	case "edits":
		var pos []string
		for _, a := range args {
			if a != "" && a[0] != '-' {
				pos = append(pos, a)
			}
		}
		ts, _, err := parseBlockStart(pos)
		fatal(err)
		db, err := openDB()
		fatal(err)
		defer db.Close()
		edits, err := editsForBlock(db, ts)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(edits)
			break
		}
		if len(edits) == 0 {
			fmt.Println("no edits for this block")
			break
		}
		for _, e := range edits {
			fmt.Printf("%s  %-10s %q -> %q\n",
				time.Unix(e.EditedAt, 0).Local().Format("2006-01-02 15:04"),
				e.Field, e.OldValue, e.NewValue)
		}

	case "retry":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		n, err := resetFailedBlocks(db)
		fatal(err)
		fmt.Printf("reset %d failed/dead block(s) for re-summarization\n", n)

	case "reconcile":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		dry := hasFlag(args, "--dry-run")
		res, err := reconcileFrames(db, cfg, dry)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(res)
		} else if dry {
			fmt.Printf("dry run: %d orphan file(s) under %s, no changes made\n", len(res.Orphans), framesDir())
			for _, p := range res.Orphans {
				fmt.Println(" ", p)
			}
		} else {
			fmt.Printf("quarantined %d orphan(s), removed %d stale row(s), purged %d expired, skipped %d fresh\n",
				res.Quarantined, res.StaleRows, res.Purged, res.Skipped)
		}

	case "backup":
		db, err := openDB()
		fatal(err)
		defer db.Close()
		dest := ""
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				dest = a
				break
			}
		}
		dir, err := runBackup(db, cfg, dest, !hasFlag(args, "--no-frames"))
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]string{"backup": dir})
		} else {
			fmt.Printf("backup written to %s\n", dir)
		}

	case "restore":
		dir := ""
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				dir = a
				break
			}
		}
		if dir == "" {
			fatal(fmt.Errorf("restore requires a backup directory"))
		}
		fatal(runRestore(dir, hasFlag(args, "--force")))
		fmt.Println("restored — restart dayflow-capture.service to resume")

	case "backup-verify":
		dir := ""
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				dir = a
				break
			}
		}
		if dir == "" {
			fatal(fmt.Errorf("backup-verify requires a backup directory"))
		}
		m, err := backupVerify(dir)
		fatal(err)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(m)
		} else {
			fmt.Printf("ok  %s (engine %s, schema v%d, %d blocks, %d frames)\n",
				dir, m.EngineVersion, m.SchemaVersion, m.Blocks, m.FramesCount)
		}

	case "export":
		// export [day|YYYY-MM-DD|week|month] [--copy] [--out path] [--brief]
		db, err := openDB()
		fatal(err)
		defer db.Close()
		var start, end time.Time
		label := "dayflow"
		now := time.Now()
		sel := "today"
		for i, a := range args {
			if a == "--out" || (i > 0 && args[i-1] == "--out") {
				continue // flag or flag value, not a range
			}
			if a != "" && a[0] != '-' {
				sel = a
			}
		}
		switch sel {
		case "today", "day":
			start, end = dayBounds(now)
			label = "dayflow — " + now.Format("2006-01-02")
		case "yesterday":
			start, end = dayBounds(now.AddDate(0, 0, -1))
			label = "dayflow — " + now.AddDate(0, 0, -1).Format("2006-01-02")
		case "week":
			start, end = weekBounds(now)
			label = "dayflow — week of " + start.Format("2006-01-02")
		case "month":
			start, end = monthBounds(now)
			label = "dayflow — " + now.Format("January 2006")
		default:
			t, err := time.ParseInLocation("2006-01-02", sel, time.Local)
			fatal(err)
			start, end = dayBounds(t)
			label = "dayflow — " + sel
		}
		blocks, err := blocksBetween(db, start, end)
		fatal(err)
		md := markdownTimeline(blocks, label)
		if hasFlag(args, "--brief") {
			md = markdownBrief(blocks, label)
		}
		out := ""
		for i, a := range args {
			if a == "--out" && i+1 < len(args) {
				out = args[i+1]
			}
		}
		if out != "" {
			fatal(writeExportFile(out, []byte(md)))
			fmt.Println("wrote", out)
		} else if hasFlag(args, "--copy") {
			c := exec.Command("wl-copy")
			c.Stdin = strings.NewReader(md)
			fatal(c.Run())
			fmt.Println("copied to clipboard")
		} else {
			fmt.Print(md)
		}
		// Export tail — when knowledge sync is enabled, push yesterday's
		// finalized journal + workstream atoms to the brain. Errors are
		// logged, never fatal to the export itself.
		knowledgeSyncAfterExport(db, cfg)

	case "sync":
		// sync [YYYY-MM-DD|today|yesterday] [--json] — push the day's
		// distilled atoms (journal brief + agent workstreams) to the
		// configured Kurultai brain profile. Requires knowledge_sync.
		d, err := dateArg(args, time.Now())
		fatal(err)
		db := openDBLenient("sync")
		if db != nil {
			defer db.Close()
		}
		printSync(db, cfg, d, jsonOut)

	case "provider":
		var pargs []string
		for _, a := range args {
			if a != "" && a[0] != '-' {
				pargs = append(pargs, a)
			}
		}
		fatal(runProvider(cfg, pargs, jsonOut))

	case "mcp":
		fatal(runMCP(cfg, hasFlag(args, "--read-only") || os.Getenv("DAYFLOW_MCP_READONLY") == "1"))
	case "tui":
		fatal(runTUI(cfg))

	case "setup":
		fatal(runSetup())
	case "key":
		// key set <account>   store a secret in OmaSeal (reads stdin) and
		//                     scrub the plaintext copy from config.json
		// key status          which dayflow accounts exist in the keyring
		// key del <account>   remove a stored secret
		if !keyringAvailable() {
			fatal(fmt.Errorf("omaseal not found on PATH — install it or keep file-based keys"))
		}
		sub := "status"
		if len(args) >= 1 {
			sub = args[0]
		}
		switch sub {
		case "set":
			if len(args) < 2 {
				fatal(fmt.Errorf("usage: dayflow key set <account>  (key on stdin)"))
			}
			account := args[1]
			secret := readStdin()
			if secret == "" {
				fatal(fmt.Errorf("no secret on stdin"))
			}
			fatal(keyringSet(account, secret))
			// Scrub the plaintext copy so config.json stays secret-free.
			scrubbed := false
			if account == "openrouter" && cfg.OpenRouterAPIKey == secret {
				cfg.OpenRouterAPIKey = ""
				scrubbed = true
			}
			for i := range cfg.Providers {
				if cfg.Providers[i].APIKey == secret {
					cfg.Providers[i].APIKey = ""
					scrubbed = true
				}
			}
			if scrubbed {
				fatal(writeConfig(cfg))
			}
			fmt.Println("stored in keyring:", account)
		case "status":
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			out, err := exec.CommandContext(ctx, "omaseal", "list", keyringService).CombinedOutput()
			cancel()
			if err != nil {
				fatal(fmt.Errorf("omaseal list failed: %s", strings.TrimSpace(string(out))))
			}
			fmt.Print(string(out))
		case "del":
			if len(args) < 2 {
				fatal(fmt.Errorf("usage: dayflow key del <account>"))
			}
			fatal(keyringDel(args[1]))
			fmt.Println("deleted:", args[1])
		default:
			fatal(fmt.Errorf("usage: dayflow key set|status|del"))
		}

	case "log":
		// UI action log channel — the panel calls this for clicks/actions.
		// Always on (actions are sparse); gated by nothing so it works even
		// when engine debug is off. Any flag arg (e.g. --limit) means this is
		// the read path: tail the log instead of appending.
		writeMode := len(args) >= 1
		for _, a := range args {
			if a == "" || a[0] == '-' {
				writeMode = false
			}
		}
		if writeMode {
			appendLog("ui: " + strings.Join(args, " "))
			break
		}
		limit := 50
		if s := flagValue(args, "--limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		lines := tailLogLines(limit)
		if jsonOut {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"lines": lines})
		} else {
			for _, l := range lines {
				fmt.Println(l)
			}
		}

	case "doctor":
		runDoctor(cfg, jsonOut, hasFlag(args, "--deep"))
	case "fixtures":
		// Drift-watch fixture regeneration (U6a): schema-only capture of an
		// agent store plus synthesized sentinel rows — see docs/maintenance.md.
		fatal(runFixtures(args))
	case "detect":
		runDetect(jsonOut)
	case "models":
		if len(args) >= 1 && args[0] == "--json" {
			printModelPresets()
		} else {
			listModels(cfg)
		}

	case "install":
		fatal(installUnits())
	case "uninstall":
		fatal(uninstallUnits())

	case "version", "--version", "-v":
		fmt.Println(version)

	default:
		usage()
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "dayflow:", err)
		os.Exit(1)
	}
}

// openDBLenient opens the journal for commands that degrade to
// metadata-only output when it isn't available (agents, briefing) —
// stderr note + nil rather than a fatal exit.
func openDBLenient(cmd string) *sql.DB {
	db, err := openDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: database unavailable: %v\n", cmd, err)
		return nil
	}
	return db
}

func printTimeline(cfg Config, day time.Time, asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	blocks, err := blocksForDay(db, day, true)
	fatal(err)
	if asJSON {
		payload := timelineJSON(blocks)
		payload["date"] = day.Format("2006-01-02")
		json.NewEncoder(os.Stdout).Encode(payload)
		return
	}
	fmt.Printf("== %s ==\n", day.Format("Monday, 2 January 2006"))
	if len(blocks) == 0 {
		fmt.Println("(no summarized blocks)")
		return
	}
	for _, b := range blocks {
		app := ""
		if b.App != "" {
			app = "  @" + b.AppName
		}
		fmt.Printf("\n%s-%s  %s  [%s]%s\n  %s\n", b.StartStr, b.EndStr, b.Title, catDisplay(b.Category), app, b.Summary)
		for _, a := range b.Activities {
			fmt.Printf("    • %s: %s [%s]\n", appDisplayName(a.App), a.Title, catDisplay(a.Category))
		}
	}
}

// printDailyGrid renders the daily workflow grid (day --grid): JSON for the
// panel, or a compact text grid for the terminal.
func printDailyGrid(cfg Config, day time.Time, asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	wf, err := generateDailyWorkflow(db, cfg, day)
	fatal(err)
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(wf)
		return
	}
	fmt.Printf("== %s — daily workflow (%d min slots) ==\n",
		day.Format("Monday, 2 January 2006"), wf.SlotMinutes)
	if len(wf.Slots) == 0 {
		fmt.Println("(no summarized blocks)")
		return
	}
	for _, s := range wf.Slots {
		if s.Category == "" {
			fmt.Printf("%s  ·\n", s.Time)
		} else {
			fmt.Printf("%s  %-14s %s\n", s.Time, s.Category, s.Title)
		}
	}
	fmt.Println()
	for _, c := range wf.Categories {
		fmt.Printf("%-14s %s\n", c.Display, fmtDur(c.Minutes))
	}
	fmt.Printf("total tracked: %s\n", fmtDur(wf.TotalMinutes))
}

func printStatus(cfg Config, asJSON bool) {
	_, configured := configuredVisionProvider(cfg)
	db, err := openDB()
	fatal(err)
	defer db.Close()
	now := time.Now()
	frames, _ := countFramesToday(db, now)
	blocksDone, _ := countBlocksToday(db, now)
	pending, _ := pendingBlocks(db, cfg, now)
	lastTS, _ := lastFrameTS(db)
	last := ""
	if lastTS > 0 {
		last = time.Unix(lastTS, 0).Local().Format("3:04 PM")
	}
	var blocksTotal int
	db.QueryRow(`SELECT COUNT(1) FROM blocks WHERE status='done'`).Scan(&blocksTotal)
	storage := storageBytesFast(db)
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"paused":         paused(),
			"capture_state":  captureState(db, cfg),
			"frames_today":   frames,
			"blocks_done":    blocksDone,
			"blocks_pending": len(pending),
			"blocks_total":   blocksTotal,
			"last_frame":     last,
			"model":          cfg.Model,
			"ignored_apps":   cfg.IgnoreApps,
			"active_app":     activeWindowClass(),
			"configured":     configured,
			"panel_expanded": cfg.PanelExpanded,
			"storage_bytes":  storage,
			"storage_text":   humanBytes(storage),
			"version":        version,
		})
		return
	}
	fmt.Printf("state: %s\nframes today: %d\nblocks summarized: %d\nblocks pending: %d\nlast frame: %s\nmodel: %s\nstorage: %s\n",
		captureState(db, cfg), frames, blocksDone, len(pending), last, cfg.Model, humanBytes(storage))
}

func printFailed(cfg Config, asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	rows, err := db.Query(`SELECT start_ts, error FROM blocks WHERE status='failed' ORDER BY start_ts DESC LIMIT 20`)
	fatal(err)
	defer rows.Close()
	type F struct {
		Start string `json:"start"`
		Error string `json:"error"`
	}
	var out []F
	for rows.Next() {
		var ts int64
		var e string
		if err := rows.Scan(&ts, &e); err != nil {
			continue
		}
		out = append(out, F{time.Unix(ts, 0).Local().Format("2006-01-02 15:04"), e})
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	for _, f := range out {
		fmt.Printf("%s  %s\n", f.Start, f.Error)
	}
	if len(out) == 0 {
		fmt.Println("no failed blocks")
	}
}

func printEvents(cfg Config, args []string, asJSON bool) {
	limit := 50
	for i, a := range args {
		if a == "-n" && i+1 < len(args) {
			if n, err := strconv.Atoi(args[i+1]); err == nil {
				limit = n
			}
		}
	}
	db, err := openDB()
	fatal(err)
	defer db.Close()
	rows, err := db.Query(`SELECT ts, type, detail FROM events ORDER BY ts DESC LIMIT ?`, limit)
	fatal(err)
	defer rows.Close()
	type E struct {
		Time   string `json:"time"`
		Type   string `json:"type"`
		Detail string `json:"detail"`
	}
	var out []E
	for rows.Next() {
		var ts int64
		var t, d string
		if err := rows.Scan(&ts, &t, &d); err != nil {
			continue
		}
		out = append(out, E{time.Unix(ts, 0).Local().Format("2006-01-02 15:04:05"), t, d})
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	for _, e := range out {
		fmt.Printf("%s  %-18s  %s\n", e.Time, e.Type, e.Detail)
	}
}

func printSearch(query string, asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	blocks, err := searchBlocks(db, query)
	fatal(err)
	type M struct {
		Start, End, Title, Summary, Category, App string
	}
	out := []M{}
	for _, b := range blocks {
		out = append(out, M{
			Start:    b.Start.Format("2006-01-02 3:04 PM"),
			End:      b.End.Format("3:04 PM"),
			Title:    b.Title,
			Summary:  b.Summary,
			Category: b.Category,
			App:      b.App,
		})
	}
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	for _, m := range out {
		fmt.Printf("%s–%s  %s  [%s] @%s\n  %s\n", m.Start, m.End, m.Title, m.Category, m.App, m.Summary)
	}
	if len(out) == 0 {
		fmt.Println("no matches")
	}
}

// printReindex rebuilds the derived FTS search indexes from scratch
// (`dayflow search --reindex`) — the repair path for a stale or missing index.
func printReindex(asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	fatal(rebuildSearchIndex(db))
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "rebuilt"})
		return
	}
	fmt.Println("search index rebuilt")
}

// usageDays parses the `usage --days` flag: absent = 0 (all retained rows),
// a bare `--days`/`--days=` or a non-positive/non-numeric value is a hard
// error — silently widening to full history would misreport exactly the
// window the user asked to bound.
func usageDays(args []string) (int, error) {
	if v := flagValue(args, "--days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return 0, fmt.Errorf("--days must be a positive integer")
		}
		return n, nil
	}
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--days" || strings.HasPrefix(a, "--days=") {
			return 0, fmt.Errorf("--days requires a positive integer (e.g. --days 7)")
		}
	}
	return 0, nil
}

func printUsage(cfg Config, asJSON bool, days int) {
	db, err := openDB()
	fatal(err)
	defer db.Close()
	sum, err := usageSummaryWindow(db, days, cfg)
	fatal(err)
	if asJSON {
		json.NewEncoder(os.Stdout).Encode(sum)
		return
	}
	window := "all retained data"
	if days > 0 {
		window = fmt.Sprintf("last %d day(s)", days)
	}
	if since, _ := sum["data_since"].(string); since != "" {
		fmt.Printf("usage: %s · data since %s\n", window, since)
	} else {
		fmt.Printf("usage: %s · no call rows recorded\n", window)
	}
	calls := sum["api_calls"].(int)
	fmt.Printf("api calls: %d (%d ok, %d failed, %.1f%% fail) avg %.0f ms\n",
		calls, sum["ok"], sum["failed"], sum["failure_rate"].(float64)*100, sum["avg_latency_ms"].(float64))
	other := sum["other_llm_calls"].(int)
	if other > 0 {
		fmt.Printf("other llm calls: %d (%d ok, %d failed, %.1f%% fail) avg %.0f ms\n",
			other, sum["other_ok"], sum["other_failed"],
			sum["other_failure_rate"].(float64)*100, sum["other_avg_latency_ms"].(float64))
	}
	fmt.Printf("prompt tokens: %d\ncompletion tokens: %d\n",
		sum["total_prompt_tokens"], sum["total_completion_tokens"])
	if cost, ok := sum["est_cost_usd"].(float64); ok {
		fmt.Printf("estimated cost: $%.2f", cost)
		// The total covers only models with pricing configured — label it
		// so a partial sum is never read as the whole window's cost.
		if unpriced, ok := sum["unpriced_models"].([]string); ok && len(unpriced) > 0 {
			fmt.Printf(" (partial — no pricing for %d model(s): %s)", len(unpriced), strings.Join(unpriced, ", "))
		}
		fmt.Println()
	}
	breakdown, _ := sum["breakdown"].(map[string]any)
	for _, dim := range []struct {
		label, key string
	}{{"by day", "by_day"}, {"by task", "by_task"}, {"by provider", "by_provider"}, {"by model", "by_model"}} {
		rows, _ := breakdown[dim.key].(map[string]usageRow)
		if len(rows) == 0 {
			continue
		}
		names := make([]string, 0, len(rows))
		for name := range rows {
			names = append(names, name)
		}
		sort.Strings(names)
		fmt.Printf("%s:\n", dim.label)
		for _, name := range names {
			r := rows[name]
			fmt.Printf("  %-32s %d calls (%d ok, %d failed, %.1f%% fail) avg %.0f ms",
				name, r.Calls, r.OK, r.Failed, r.FailureRate*100, r.AvgLatencyMs)
			if r.EstCostUSD != nil {
				fmt.Printf("  $%.2f", *r.EstCostUSD)
			}
			fmt.Println()
		}
	}
}

// printStats reports storage usage, journal counts, date coverage, and API
// usage — the "how much is this thing using" view.
func printStats(cfg Config, asJSON bool) {
	db, err := openDB()
	fatal(err)
	defer db.Close()

	var blocksTotal, blocksDone, blocksFailed, blocksDead, framesPending, eventsTotal int
	db.QueryRow(`SELECT COUNT(1),
	  COALESCE(SUM(CASE WHEN status='done' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(CASE WHEN status='dead' THEN 1 ELSE 0 END),0)
	  FROM blocks`).Scan(&blocksTotal, &blocksDone, &blocksFailed, &blocksDead)
	db.QueryRow(`SELECT COUNT(1) FROM frames`).Scan(&framesPending)
	db.QueryRow(`SELECT COUNT(1) FROM events`).Scan(&eventsTotal)

	var firstTS, lastTS sql.NullInt64
	db.QueryRow(`SELECT MIN(start_ts), MAX(end_ts) FROM blocks WHERE status='done'`).Scan(&firstTS, &lastTS)
	firstDay, lastDay := "", ""
	if firstTS.Valid {
		firstDay = time.Unix(firstTS.Int64, 0).Local().Format("2006-01-02")
	}
	if lastTS.Valid {
		lastDay = time.Unix(lastTS.Int64, 0).Local().Format("2006-01-02")
	}

	var calls, okCalls, failedCalls, promptTok, completionTok int
	var avgLatency float64
	db.QueryRow(`SELECT COUNT(1),
	  COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(CASE WHEN status!='ok' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
	  COALESCE(AVG(latency_ms),0)
	  FROM api_calls`).Scan(&calls, &okCalls, &failedCalls, &promptTok, &completionTok, &avgLatency)

	var dbBytes, walBytes int64
	if fi, err := os.Stat(dbPath()); err == nil {
		dbBytes = fi.Size()
	}
	if fi, err := os.Stat(dbPath() + "-wal"); err == nil {
		walBytes = fi.Size()
	}
	framesBytes, frameFiles := dirStats(framesDir())
	totalBytes := dataDirSize()

	if asJSON {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"storage": map[string]any{
				"total_bytes": totalBytes, "total": humanBytes(totalBytes),
				"db_bytes": dbBytes, "db": humanBytes(dbBytes),
				"wal_bytes": walBytes, "wal": humanBytes(walBytes),
				"frames_bytes": framesBytes, "frames": humanBytes(framesBytes),
				"frame_files":   frameFiles,
				"data_dir":      dataDir(),
				"cap_mb":        cfg.MaxStorageMB,
				"frames_cap_mb": cfg.MaxFramesMB, "db_cap_mb": cfg.MaxDBMB,
			},
			"blocks": map[string]any{
				"total": blocksTotal, "done": blocksDone,
				"failed": blocksFailed, "dead": blocksDead,
				"pending_frames": framesPending,
				"first_day":      firstDay, "last_day": lastDay,
			},
			"events": eventsTotal,
			"api": map[string]any{
				"calls": calls, "ok": okCalls, "failed": failedCalls,
				"prompt_tokens": promptTok, "completion_tokens": completionTok,
				"avg_latency_ms": int(avgLatency),
			},
			"config": map[string]any{
				"provider": cfg.Provider, "model": cfg.Model,
				"retention_days": cfg.RetentionDays, "keep_frames": cfg.KeepFrames,
				"max_storage_mb": cfg.MaxStorageMB, "debug": cfg.Debug,
				"max_frames_mb": cfg.MaxFramesMB, "max_db_mb": cfg.MaxDBMB,
			},
		})
		return
	}

	fmt.Printf("Storage\n")
	fmt.Printf("  data dir:   %s (%s)\n", humanBytes(totalBytes), dataDir())
	fmt.Printf("  database:   %s + %s wal\n", humanBytes(dbBytes), humanBytes(walBytes))
	fmt.Printf("  frames:     %s (%d files awaiting summary)\n", humanBytes(framesBytes), frameFiles)
	fmt.Printf("  caps:       frames %s · db %s\n",
		map[bool]string{true: "unlimited", false: fmt.Sprintf("%d MB", cfg.MaxFramesMB)}[cfg.MaxFramesMB == 0],
		map[bool]string{true: "unlimited", false: fmt.Sprintf("%d MB", cfg.MaxDBMB)}[cfg.MaxDBMB == 0])
	if cfg.MaxStorageMB > 0 {
		fmt.Printf("  legacy cap: %d MB (whole dir)\n", cfg.MaxStorageMB)
	}
	fmt.Printf("Journal\n")
	fmt.Printf("  blocks:     %d total (%d done, %d failed, %d dead)\n", blocksTotal, blocksDone, blocksFailed, blocksDead)
	if firstDay != "" {
		fmt.Printf("  coverage:   %s → %s\n", firstDay, lastDay)
	}
	fmt.Printf("  events:     %d log rows\n", eventsTotal)
	fmt.Printf("API\n")
	fmt.Printf("  calls:      %d (%d ok, %d failed)\n", calls, okCalls, failedCalls)
	fmt.Printf("  tokens:     %d in / %d out\n", promptTok, completionTok)
	fmt.Printf("  avg latency: %.0f ms\n", avgLatency)
	fmt.Printf("Config\n")
	fmt.Printf("  provider:   %s\n  model:      %s\n", cfg.Provider, cfg.Model)
	fmt.Printf("  retention:  %d days, keep_frames=%v\n", cfg.RetentionDays, cfg.KeepFrames)
	fmt.Printf("  debug log:  %v (%s)\n", cfg.Debug, debugLogPath())
}
