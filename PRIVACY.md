# Dayflow Privacy Notice

Dayflow is a **local-first** automatic work journal. This notice describes what data is collected, how it is used, and the controls available to you.

## What Dayflow collects

- **Screenshots**: a lightweight JPEG frame is captured at the interval you configure (default every 10 seconds) while Dayflow is recording and your session is not paused or locked.
- **Focused window information**: the active window class/app name is recorded alongside each frame so the journal can attribute activity to apps.
- **Journal entries**: the SQLite database in `~/.local/share/dayflow/` stores `{title, summary, category, app, timestamps}` for each summarized block.
- **Derived search index**: a local FTS5 index over block and standup text, maintained inside the same database by triggers — deleted rows stop being searchable, including after `dayflow scrub` and retention pruning.
- **Audit metadata**: `events`, `api_calls`, and usage logs are stored locally for diagnostics and token accounting.

## What leaves your machine

Only the **sampled frames** sent to your chosen AI provider for summarization leave your machine — plus these config-gated additions: TypeSafe Jev judge calls send small block/session descriptors (titles, apps, durations) to the decisions endpoint (`jev_classification: false` disables), and agent-session recaps send a bounded, scrubbed transcript excerpt — to the decisions endpoint for worthiness/quality judging, and to the chat provider for the recap text itself (**off by default** — `agent_recaps: true` opts in; scrubbing removes home paths, tokens, and URL credentials). By default the provider is OpenRouter; you may also configure a local endpoint such as Ollama or LM Studio. If you separately enable knowledge sync, distilled journal and agent-workstream summaries also go to the knowledge brain you configure (see below).

### CLI providers (`kind: "cli"`)

You may configure a provider of kind `cli`, which runs a locally installed agent CLI (e.g. `cursor-agent`, `opencode`) as a subprocess instead of calling an HTTP API. **This changes credential management, not data residency.** The CLI uses its own subscription sign-in, so Dayflow holds no API key for it — but every prompt sent to it (including chat history, journal excerpts, and agent-recap material) and every frame file it is pointed at are forwarded to that tool's configured model backend, which is typically a cloud service. A local subprocess does **not** mean data stays local.

To limit blast radius, the subprocess boundary is hardened: the command runs argv-style with no shell; prompt text is delivered on stdin or after a `--` terminator, never where it could be parsed as a flag; permission-escalation flags (`--yolo`, `--force`, `--auto`, `-f`, and equivalents) are rejected at config time and again on the final argv at exec time; the child receives a minimal environment (PATH, HOME, LANG, TMPDIR — Dayflow's own environment, including your provider API keys, is not inherited; additional variables require an explicit `env_passthrough` list, and `scratch_home` can replace the real HOME); the working directory is a fresh scratch dir under the system temp; and each call is bounded by a timeout (`cli_timeout_sec`, default 180s). Cli providers serve text tasks (chat, review, agent recaps) by default; routing the per-block `vision`/`summary` loop to one requires opting in with `allow_hot_path`, since subprocess calls run at minutes-scale latency.

Residual risk: the deny-list constrains the argv Dayflow builds, not the CLI's own permission profile — the subprocess can still act within whatever tools and permissions its own configuration grants it.

### Knowledge sync (Kurultai)

Knowledge sync is **off by default** (`knowledge_sync: false`). Enabling it adds an export destination: a Kurultai knowledge brain you configure. The daily export pass sends the previous day's distilled journal and agent-workstream summaries; `dayflow sync [date]` requests a sync manually. Unchanged documents are skipped using a content hash. Raw screenshots, transcripts, and individual conversation turns are never included in the sync document.

With `knowledge_transport: "http"`, the document is posted to `<knowledge_url>/ingest` with an authentication secret in the Authorization header. With `"ssh"`, it travels over SSH to `knowledge_ssh_host` and is relayed into `knowledge_container`'s local ingest endpoint. The secret travels on stdin for the SSH relay. Both transports require `knowledge_secret_ref`, which can refer to OmaSeal or contain a literal secret. The destination is yours to choose; no personal host or endpoint is built into Dayflow. The receiving brain controls storage and retention of exported summaries.

Set `knowledge_sync` to `false` to stop future exports. Turning sync off, scrubbing the local journal, or deleting local data does not remove copies already stored in the receiving brain; manage those copies there. Configuration details are in [Knowledge sync](README.md#knowledge-sync-kurultai).

### Desktop notifications

Optional `notify-send` alerts (capture stall, pause/resume, standup ready, goal pending) go to your session's local notification daemon only — nothing is transmitted. Bodies carry event-class labels such as "capture stalled", never journal text. `notifications.enabled` (master switch) and `notifications.classes` (per-class gates) control them; only `stall` is on by default.

Dayflow does **not** include telemetry, usage analytics reporting, or automatic crash reporting. External knowledge synchronization is a separate, explicitly enabled feature described above.

## Where your data lives

All local data is stored in `~/.local/share/dayflow/` and configuration in `~/.config/dayflow/`. You can wipe everything at any time:

```sh
dayflow pause
rm -rf ~/.local/share/dayflow ~/.config/dayflow
```

## Controls

- **Pause / resume**: `dayflow pause` or right-click the bar widget. Pausing creates a flag file the daemon checks before every capture.
- **Auto-pause on lock**: `auto_pause_locked` is enabled by default; the daemon pauses while your session is locked.
- **Ignore apps**: add window classes to `ignore_apps` so their frames are never captured.
- **Retention**: `retention_days` and `max_storage_mb` prune old frames, events, and API logs automatically.
- **Scrub**: `dayflow scrub <query>` deletes existing blocks matching a title or summary; the search index follows the delete.
- **Fixture capture** (developer tool): `dayflow fixtures capture` reads agent-store *schema* only and writes synthesized sentinel rows — it never serializes real payload data.
- **No idle tracking**: empty blocks (no frames) are not stored, so idle or screen-off time does not appear in the journal.

## Sensitive content

Dayflow can see whatever is on your screen. If you handle passwords, payment cards, government IDs, or other sensitive information, **pause capture** or **add the app to `ignore_apps`** before viewing it. Dayflow includes a sensitive-content filter, but it is a best-effort aid, not a guarantee. You are responsible for ensuring sensitive information is not captured.

## Changes

This notice may be updated in the repository. The version shipped with your installed release applies unless you pull a newer one.

## License

Dayflow is MIT licensed. This privacy notice is provided for transparency and is not a legal contract or warranty.
