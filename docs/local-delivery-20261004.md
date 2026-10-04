# Local Dayflow delivery — 2026-10-04

The public fork is https://github.com/nixfred/dayflow-linux. This checkout and the installed Omarchy plugin track `work/fred-dayflow-20261004`, a combined development build of the upstream PRs. The engine binary was built locally and installed atomically; it is not a new upstream release.

Upstream PRs: [#51 provider readiness](https://github.com/duketopceo/dayflow-linux/pull/51), [#52 Settings route consistency](https://github.com/duketopceo/dayflow-linux/pull/52), [#53 agent route tests](https://github.com/duketopceo/dayflow-linux/pull/53), [#54 sync privacy](https://github.com/duketopceo/dayflow-linux/pull/54), [#55 Settings/onboarding fit](https://github.com/duketopceo/dayflow-linux/pull/55), [#57 immediate completion recording](https://github.com/duketopceo/dayflow-linux/pull/57). The first four merged; the final two require upstream review.

## Recording

The local configuration enables capture and `agent_completions`. Claude/Codex silent Stop hooks were appended, preserving existing hook entries. Codex requires `/hooks` review/trust of its new hook; the enabled local transcript watcher operates independently every two seconds. Today refreshes while open. A direct hook recorded the completed local validation in 11 ms. Internal Codex approval-review transcripts are excluded. Finished turns can contain partial results or questions; they do not certify a whole project as done.

The first-run bounded watcher recovered recent actual Claude/Codex replies. Screen capture had started at 18:16, so earlier screenshots cannot be recovered from Dayflow. Recent transcript outcomes remain recoverable within the watcher bounds.

The stale OpenRouter route and uninstalled local model caused missing summaries. The route now uses the installed Gemma 3 4B vision model on vic over the private tailnet, following the local GPU preference. Gus's GPU was occupied and its Ollama CPU fallback timed out. Vic summarized two pending blocks successfully; the live journal then had three completed screen blocks and none pending. One frame per block keeps requests within the local context budget; this yields less visual coverage than the original 30-frame default. No paid API credentials or completion sounds were enabled.

## Validation and recovery

The combined Go suite, vet and build passed. Forty-two Qt runtime layout scenarios passed at logical 1280×720, 1280×800 and 1920×1080 in dark/light palettes, with long text, complete-page roundtrips, Unicode and surrounding-text preservation on edits. The changed Settings, onboarding and Today views are covered; other pre-existing tabs are not certified for no-scroll fit. A native live view using the current host theme was inspected at 1280×900 on the left.

Local backups: engine `~/.local/bin/dayflow.before-fred-20261004`; config `~/.config/dayflow/config.before-recording-fix-20261004.json`; both hook configuration files have `.before-dayflow-20261004` backups. A SQLite backup before removal of internal approval records is `~/.local/share/dayflow/dayflow.before-guardian-cleanup-20261004.db`. Raw native captures and aggregate verification are under `/tmp/dayflow-delivery/` and were not committed. Restoring the old engine loses these fixes; it ignores the additive completion table rather than changing its schema version.
