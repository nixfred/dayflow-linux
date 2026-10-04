# Dayflow for Linux

<p align="center">
  <img src="docs/assets/social.png" alt="Dayflow for Linux" width="640" />
</p>

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT" /></a>
  <a href="https://github.com/duketopceo/dayflow-linux/releases"><img src="https://img.shields.io/github/v/release/duketopceo/dayflow-linux" alt="release" /></a>
  <a href="https://github.com/duketopceo/dayflow-linux/actions/workflows/ci.yml"><img src="https://github.com/duketopceo/dayflow-linux/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
</p>

**A private, automatic work journal for Linux.** A port of [Dayflow](https://www.dayflow.so/) (macOS) — records your screen, summarizes what you were actually doing, and shows a readable timeline of your day.

Dayflow captures a lightweight screenshot every 10 seconds, deduplicates unchanged frames, and every 15 minutes asks a vision model to write a plain-language summary. The result: a timeline you can skim, search, chat with, and export — plus standup drafts, agent-session recaps, and a next-day forecast.

- **Local-first** — frames and the SQLite journal live in `~/.local/share/dayflow/`. Nothing leaves your machine except what you enable: sampled frames sent for summarization (or a fully local model — Ollama/LM Studio supported), small per-block descriptors for Jev classification, and — only if you opt in — scrubbed agent-transcript excerpts for session recaps (see the privacy model below).
- **Cheap** — ~30 JPEG frames per block, `google/gemma-4-31b-it` by default (~$0.09/M tokens). Any OpenRouter vision model, OpenAI-compatible endpoint, MCP tool, or agent CLI (`cursor-agent`, `opencode`) works.
- **Light** — one static Go binary, ~25 MB RAM, sub-1% CPU. No Electron.
- **Controllable** — pause toggle, per-app ignore list, automatic frame deletion, retention pruning, `dayflow scrub`.

## Install

### Omarchy / Quickshell (panel UI)

```sh
omarchy plugin add https://github.com/duketopceo/dayflow-linux.git --enable
```

Open the panel and click **Install engine** — the widget downloads the checksum-verified release binary matching the plugin version and enables the capture units for you. Then the onboarding wizard walks provider setup. Bar widget + full-window panel (timeline, timelapse, context shifts, agent recaps, forecast).

### Any Linux (engine only)

The engine needs no Omarchy — it's a standalone CLI + TUI + MCP server. Works on any wlroots compositor (Hyprland, sway, river, …) via `grim`; on KDE/GNOME/X11 set `capture_command` to any tool that writes an image to stdout.

```sh
curl -fsSL https://github.com/duketopceo/dayflow-linux/releases/latest/download/install.sh | bash
dayflow setup        # interactive: paste key, validates + picks a vision model
dayflow doctor       # sanity-check session, grim, key, model, stores
```

The script fetches the `amd64`/`arm64` release binary, verifies it against the release's `SHA256SUMS`, places it in `~/.local/bin`, and runs `dayflow install` for the systemd user units. See [docs/install-linux.md](docs/install-linux.md) for manual steps and options (`--version`, source build).

**Requirements:** `grim` (or a `capture_command`), `systemd --user`, `hyprctl` (optional — app ignore list and `output: "auto"` focus-follow are Hyprland-only), an OpenRouter key or local endpoint.

## What you get

**Journal** — semantic activity cards (consecutive same-activity blocks merge), per-app activity segments, 15-minute workflow grid, inline editing of title/category/summary, FTS5 full-text search (`dayflow search`), markdown export, daily goals + streaks.

**Analytics** — weekly category donut, app treemap, context-shift flow, focus blocks, week-over-week trends, next-day forecast from your history.

**Agents** — a workstream briefing of your coding-agent sessions across Claude Code, Codex, OpenCode, Devin, and Cursor: sessions grouped by project into named workstreams, each thread carrying a derived status (in progress / review ready / blocked / completed) and a condensed turn narrative with key decisions flagged. Fully deterministic offline; opting into `agent_recaps` adds model-written prose from a bounded, scrubbed skeleton. Plus per-session recaps (`dayflow agents`) with Jev scoring, a local FTS5 index of every conversation turn (`dayflow ingest` / `search-agents` — scrubbed before storage, deduped by fingerprint, queryable from chat and MCP), and per-source drift reporting so a broken store is visible, not silently empty.

**Ops** — desktop notifications (capture stall on by default — silence means data loss), focus-following capture on multi-monitor setups (`output: "auto"`), usage/cost reporting (`dayflow usage --days 7` with optional pricing), backups with integrity verification, `doctor` health checks, MCP server for agent access.

**Chat** — ask natural-language questions about your timeline: `dayflow chat "What did I work on this week?"` or the panel's Chat tab.

<details>
<summary><b>Privacy model</b> — what's collected, what egresses, what you control</summary>

- `dayflow pause` (or right-click the bar widget) drops a flag file the daemon checks before every capture.
- Capture auto-pauses while your session is locked (`auto_pause_locked`, via `loginctl`).
- Ignored apps are skipped at capture time — their frames never touch disk.
- Frames are deleted after summarization unless `keep_frames` is on; `max_frames_mb`/`max_db_mb` cap each pool; `retention_days` prunes by age on top.
- The FTS index follows deletes — `dayflow scrub` removes the text from the journal *and* the index, including the `block_edits` overlay.
- Agent recaps are **off by default**: enabling them sends a bounded, scrubbed transcript excerpt (paths → `~`, token shapes → `[redacted]`) to your chat provider and OpenRouter's decisions endpoint. Jev classification (`jev_classification`, default on) sends small per-block descriptors to the same endpoint — set `false` to keep every block local.
- Everything lives in `~/.local/share/dayflow/` — `rm -rf` wipes all data.

Plain-language version: [PRIVACY.md](PRIVACY.md).

</details>

## CLI

```sh
dayflow today                  # today's timeline
dayflow day 2026-09-03         # any day          ·  day --grid  # workflow grid
dayflow tui                    # interactive terminal timeline, standup, insights
dayflow status                 # state, counts, model
dayflow standup [save|draft]   # standup update / saved draft fields
dayflow insights [day|week|month]   # focus, categories, apps, distractions
dayflow weekly | week | month  # rollups + charts
dayflow agents [day]           # coding-agent sessions + recaps (--no-recaps)
dayflow briefing [day]         # workstream briefing: grouped sessions, statuses, condensed turns (--refresh)
dayflow ingest                 # index agent-chat turns into the local FTS store
dayflow search-agents <query>  # FTS5 search indexed agent conversations (all five harnesses)
dayflow ask "<question>"       # one-shot chat over journal + agent history
dayflow forecast [day]         # predict a day's category mix (default: tomorrow)
dayflow search <query>         # FTS5 search over titles/summaries/categories/apps
dayflow search --reindex       # rebuild the index
dayflow usage [--days N]       # tokens, latency, failure rate, est. cost
dayflow edit <start> <field> <value>  # correct a block (title/category/summary/productive)
dayflow chat "<question>"      # ask the journal (--conversation-id N continues)
dayflow goal [set|done|clear]  # daily goal + streak
dayflow export week [--copy|--out path]   # markdown export
dayflow playback on|off        # opt-in frame retention for timelapse (10 GB cap)
dayflow frames [day] | blocks  # raw frames / failed summaries (auto-retried)
dayflow pause | resume | toggle
dayflow ignore <class> | --active | unignore   # never capture an app
dayflow retry | reconcile [--dry-run] | scrub <query>
dayflow summarize --retry      # reset failed/dead blocks AND sweep — fills gaps
dayflow backup [dir] | backup-verify | restore <dir> [--force]
dayflow provider list|add|set|remove|test     # multi-provider routing
dayflow key set|status|del     # store API keys in the system keyring
dayflow events -n 20           # audit log: captures, skips, errors
dayflow mcp                    # MCP server for agents (stdio)
dayflow config set <k> <v> | patch '<json>'   # live settings
dayflow fixtures capture <source>  # regenerate agent-store fixtures
dayflow summarize --now        # force summarization incl. current block
dayflow uninstall              # remove systemd units (data stays)
```

All query commands accept `--json`.

## Config

`~/.config/dayflow/config.json` — set via `dayflow config set <k> <v>` or `config patch '<json>'` for nested values.

| key | default | notes |
|---|---|---|
| `model` | `google/gemma-4-31b-it` | any vision model |
| `api_base_url` | `""` | OpenAI-compatible endpoint; empty = OpenRouter. `http://localhost:11434/v1` for Ollama |
| `capture_interval_sec` | 10 | frame interval |
| `block_minutes` | 15 | summary granularity |
| `frames_per_block` | 30 | frames sampled per API call |
| `jpeg_quality` | 55 | grim JPEG quality |
| `frame_max_dim` | 1920 | downscale stored frames so long edge ≤ N px; `0` = native res. Docked composites reduce per-display fidelity |
| `keep_frames` | false | keep raw frames after summarizing |
| `retention_days` | 0 | prune frames/events/api logs older than N days; `0` = caps only |
| `max_frames_mb` | 20480 | cap on frames + quarantine; `0` = unlimited |
| `max_db_mb` | 10240 | cap on the journal DB (blocks, events, calls); `0` = unlimited |
| `auto_pause_locked` | true | pause capture while the session is locked |
| `ignore_apps` | `[]` | window classes never captured (Hyprland) |
| `output` | `""` | one monitor via `grim -o`; `"auto"` follows the focused monitor (Hyprland; falls back to composite). Ignored when `capture_command` is set |
| `capture_command` | `""` | custom screenshot command writing image to stdout; whitespace-separated, no quoting support |
| `notifications` | `{enabled:true, classes:{stall:true}}` | desktop notifications; `classes` gates `stall`/`paused`/`recovered`/`standup`/`goal` via `config patch` |
| `pricing` | `{}` | model slug → USD/1M tok; `usage` renders `$` only for configured models. E.g. `dayflow config patch '{"pricing":{"google/gemma-4-31b-it":0.09}}'` |
| `openrouter_api_key` | `""` | API key (or `OPENROUTER_API_KEY` env, or `~/.config/openrouter/keys.json`) |
| `jev_classification` | `true` | Jev calibrated judgments — category, merge, quality, triage, forecast. Egresses small descriptors to OpenRouter's decisions endpoint; `false` disables Jev classification (summarization still uses your configured chat provider unless that provider is local) |
| `agent_completions` | `false` | local final-reply watcher; records within a few seconds, independent of model calls; [setup and hooks](docs/immediate-completions.md) |
| `agent_recaps` | `false` | opt-in: recap generation egresses a bounded, scrubbed excerpt to the chat provider + decisions endpoint |
| `agent_recap_batch` | `false` | submit uncached recaps as one OpenRouter Batch API job (~50% off, async up to 24h) instead of inline calls; results land on the next briefing pass. Requires the `agent_recap` route to resolve to an OpenRouter endpoint — point `routing.task_provider.agent_recap` at a batch-capable model to run recaps on a different (cheaper or smarter) model than chat |
| `classification_model` | `typesafe/jev-1.13` | Jev model slug |
| `site_name` | `dayflow-linux` | X-Title header for OpenRouter |

## Upgrading

```sh
cd engine && go build -o dayflow . && install -Dm755 dayflow ~/.local/bin/dayflow
systemctl --user restart dayflow-capture.service
```

Restart long-lived `dayflow mcp` clients too (agents keep the old binary). `dayflow doctor` reports engine↔schema↔manifest consistency and probes each detected agent store — run it after every upgrade. Migrations are automatic and additive. Full checklist: [docs/maintenance.md](docs/maintenance.md).

## Backups

`dayflow install` enables a daily `dayflow-backup.timer` — snapshots the DB, a secret-redacted config, and retained frames to `~/.local/share/dayflow-backups/` (keeps 7, `DAYFLOW_BACKUP_DIR` overrides).

```sh
dayflow backup [dir]           # snapshot now  ·  backup-verify <dir>  # integrity check
systemctl --user stop dayflow-capture.service dayflow-summarize.timer
dayflow restore ~/.local/share/dayflow-backups/dayflow-<timestamp>
systemctl --user start dayflow-capture.service dayflow-summarize.timer
```

Restore refuses a live DB without `--force` and rejects newer-schema backups. API keys are redacted from backups on purpose — re-set them after restore.

## Uninstall

```sh
# engine first, while the plugin dir still exists:
bash ~/.config/omarchy/plugins/io.github.duketopceo.dayflow/scripts/uninstall.sh   # units + binary (data kept)
# (after plugin removal the equivalent is: dayflow uninstall && rm ~/.local/bin/dayflow)
omarchy plugin disable io.github.duketopceo.dayflow && omarchy plugin remove io.github.duketopceo.dayflow   # panel

# wipe history + config too, if wanted:
dayflow pause
rm -rf ~/.local/share/dayflow
rm -rf ~/.config/dayflow
```

## Knowledge sync (Kurultai)

Opt-in. When `knowledge_sync` is on, the daily export pass also pushes the
previous day's distilled journal — one markdown document (`Journal` +
`Agent workstreams` sections) — to a Kurultai brain via `POST /ingest`,
deduplicated by content hash (`dayflow/<date>.md` source_ids upsert
server-side). Raw frames, transcripts, and turn text never leave the
machine.

```sh
dayflow sync                 # push today (manual)
dayflow sync 2026-09-30 --json
```

Transports (`knowledge_transport`):

- `"http"` (default) — direct `POST <knowledge_url>/ingest`, secret in the
  `Authorization` header. Requires `knowledge_url` and a brain that
  accepts authenticated remote ingest.
- `"ssh"` — fallback for brains whose `/ingest` is loopback-only: relays
  through `ssh <knowledge_ssh_host> docker exec -i <knowledge_container>`
  into the container's loopback. Requires `knowledge_ssh_host` and
  `knowledge_container` (`knowledge_port` defaults to `8421`); the secret
  travels on stdin, never argv.

Both transports require `knowledge_secret_ref` — an
`omaseal://service/account` reference (or a literal secret). Nothing is
baked in: with `knowledge_sync` off, or any required field unset, sync
refuses loudly (`dayflow sync`) or skips silently (export tail).

## MCP / agent access

`dayflow mcp` is a stdio MCP server exposing `get_timeline`, `get_status`, `search_journal`, `search_agent_sessions`, `get_events`, `get_usage`, `get_stats`, `get_standup`, `get_insights`, `chat`. All read-only except `chat`; `--read-only` (or `DAYFLOW_MCP_READONLY=1`) hides it. Contract: [docs/agent-contract.md](docs/agent-contract.md).

```sh
claude mcp add dayflow -- ~/.local/bin/dayflow mcp
# remote over Tailscale SSH (never Funnel):
claude mcp add dayflow-remote -- ssh <host>.<tailnet>.ts.net ~/.local/bin/dayflow mcp --read-only
```

## Repository layout

```
manifest.json      # Omarchy plugin manifest (repo root, per marketplace rules)
BarWidget.qml      # bar indicator: recording state, click for panel
Panel.qml          # timeline panel + settings footer
*.qml              # full view: panes, tabs, onboarding, settings
engine/            # Go CLI/daemon/TUI/MCP — the tracking engine
scripts/           # install.sh/uninstall.sh (engine lifecycle), test-install.sh (offline battery),
                   # stress.sh (live stress), smoke-install.sh (sandboxed install smoke)
docs/              # plans, research, maintenance runbook, agent contract, publish docs
preview.png        # marketplace preview
```

## Development

```sh
cd engine && go test ./...     # unit + end-to-end tests with stubbed providers
cd .. && scripts/smoke-install.sh   # sandboxed install smoke (scoped dirs, stubbed systemctl)
cd .. && scripts/stress.sh          # live stress against a sandboxed data dir
```

See [CONTRIBUTING.md](CONTRIBUTING.md), [docs/maintenance.md](docs/maintenance.md) (drift-watch + upgrade safety), and [docs/research/](docs/research/) for the audio-capture spike and macOS Agents-section analysis.

## Not a 1:1 port

No audio capture (spike doc says conditional-go), no menu-bar app — the bar widget + full-window panel take that role. The Agents section goes past upstream: five sources instead of two, with drift surfacing and a gated recap pipeline. MIT licensed, like the [original](https://github.com/JerryZLiu/Dayflow).
