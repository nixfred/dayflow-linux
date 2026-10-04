# Immediate agent completion recording

`dayflow complete --source claude|codex|manual` accepts a Stop-hook JSON object on stdin. It commits the final assistant reply into a local completion journal immediately, without API requests, transcript indexing, screenshot summarization or notifications. Today shows recorded replies alongside screen summaries and polls every two seconds while open. Finished turns can contain questions or partial results; they do not certify an entire project is complete.

Input fields: `session_id`, `turn_id`, `cwd`, `last_assistant_message`, `transcript_path`, optional `hook_event_name: "Stop"`. When older Claude hooks omit the reply, the recorder reads the last 1 MiB of the transcript and selects the last text-bearing assistant reply. Codex commentary/reasoning is excluded. Replies over 64 KiB and payloads over 1 MiB are rejected. Existing home-path and credential-pattern redactions run before storage; these are not a general PII filter. Identical hook retries deduplicate; supplied turn IDs or Claude message UUIDs distinguish repeated replies across turns. Retention days apply to this journal.

Append a command hook under `hooks.Stop` in Claude's `~/.claude/settings.json` or Codex's `~/.codex/hooks.json`, preserving existing entries:

```json
{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/absolute/path/to/dayflow-linux/scripts/completion-hook.sh codex","timeout":5}]}]}}
```

Use `claude` for Claude Code. The wrapper has a four-second deadline, emits valid empty hook JSON on failure, and never plays completion audio. Codex requires review/trust of changed hook definitions before they run; use the host's hook review flow. See [official hook documentation](https://learn.chatgpt.com/docs/hooks). Existing running sessions may need a reload/new session to discover hooks.

Inspect with `dayflow completions --json` or `dayflow today --json` (the additional `completions` array). Hooks do not backfill historical turns automatically. Screen capture and generated agent briefings remain separate features with their existing consent/settings.

## Local watcher fallback

Enable `agent_completions` and restart the capture service to watch supported transcript stores every two seconds, separately from capture. This defaults off and makes no model calls. It records explicit Claude `end_turn`/`stop_sequence` replies and Codex `final`/`final_answer` messages, including the native Codex `phase` field; commentary, reasoning, tool replies and unfinished retained messages are ignored. Changed files are read only from their last 1 MiB, with a rolling 24-hour timestamp window. The bounded startup scan can recover recent replies, but is not a complete historical import. Claude projects use the standard one-level store; Codex uses today/yesterday date directories. Other agent sources remain on the existing briefing/index pipeline. The fast Stop hook is preferred; the watcher provides a near-immediate fallback while hook trust/reload is pending.
