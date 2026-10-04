package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompletionRecordDedupAndDay(t *testing.T) {
	testEnv(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 10, 4, 18, 0, 0, 0, time.Local)
	input := `{"session_id":"s","turn_id":"t","cwd":"/project","last_assistant_message":"Fixed tests. sk-or-123456789012345678901234","hook_event_name":"Stop"}`
	c, err := decodeCompletion(strings.NewReader(input), "codex", now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.Summary, "sk-or-") {
		t.Fatal("secret retained")
	}
	for i := 0; i < 2; i++ {
		if err := recordCompletion(db, c); err != nil {
			t.Fatal(err)
		}
	}
	other, err := decodeCompletion(strings.NewReader(strings.Replace(input, `"turn_id":"t"`, `"turn_id":"t2"`, 1)), "codex", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordCompletion(db, other); err != nil {
		t.Fatal(err)
	}
	rows, err := completionsForDay(db, now)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	rows, err = completionsForDay(db, now.AddDate(0, 0, 1))
	if err != nil || len(rows) != 0 {
		t.Fatalf("next day rows=%v err=%v", rows, err)
	}
	pruneOldEvents(db, now.Add(time.Second))
	rows, err = completionsForDay(db, now)
	if err != nil || len(rows) != 0 {
		t.Fatalf("retention rows=%v err=%v", rows, err)
	}
}

func TestCompletionHookTranscriptFallback(t *testing.T) {
	testEnv(t)
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	contents := `{"type":"assistant","uuid":"a","timestamp":"2026-10-04T20:00:00Z","message":{"content":[{"type":"text","text":"Delivered the fix."}]}}` + "\n" + `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(completionInput{Session: "s", Transcript: path, Event: "Stop"})
	c, err := decodeCompletion(strings.NewReader(string(b)), "claude", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.Summary != "Delivered the fix." {
		t.Fatalf("summary=%q", c.Summary)
	}
	// New Claude turns with the same text still record separately.
	if err := os.WriteFile(path, []byte(strings.Replace(contents, `"uuid":"a"`, `"uuid":"b"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := decodeCompletion(strings.NewReader(string(b)), "claude", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == d.ID {
		t.Fatal("distinct turn UUIDs deduplicated")
	}
	for _, bad := range []string{`{}`, `{"last_assistant_message":"ok","hook_event_name":"PostToolUse"}`, `{} {}`} {
		if _, err := decodeCompletion(strings.NewReader(bad), "claude", time.Now()); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestCompletionCodexExcludesCommentary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex.jsonl")
	contents := `{"timestamp":"2026-10-04T20:00:00Z","payload":{"type":"message","role":"assistant","channel":"final","content":[{"type":"output_text","text":"Completed a fix."}]}}` + "\n" + `{"payload":{"type":"message","role":"assistant","channel":"commentary","content":[{"type":"output_text","text":"Starting another task."}]}}` + "\n"
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	text, _, _ := completionTail(path, "codex")
	if text != "Completed a fix." {
		t.Fatalf("text=%q", text)
	}
}

func TestCompletionWatcherNativePhaseAndRetention(t *testing.T) {
	testEnv(t)
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	raw := `{"timestamp":"` + now.Add(-time.Minute).Format(time.RFC3339) + `","metadata":{"retained_source":{"complete":true}},"payload":{"type":"message","role":"assistant","phase":"final_answer","internal_chat_message_metadata_passthrough":{"turn_id":"t"},"content":[{"type":"output_text","text":"Shipped native fix."}]}}`
	path := filepath.Join(t.TempDir(), "native.jsonl")
	header := `{"type":"session_meta","payload":{"id":"s","cwd":"/project"}}` + "\n"
	if err := os.WriteFile(path, []byte(header+raw+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := scanCompletionFile(db, path, "codex", now); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := completionsForDay(db, now)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].Session != "s" || rows[0].Project != "/project" || rows[0].CompletedAt != now.Add(-time.Minute).Unix() {
		t.Fatalf("row=%v", rows[0])
	}
	for _, bad := range []string{strings.Replace(raw, "final_answer", "commentary", 1), strings.Replace(raw, `"complete":true`, `"complete":false`, 1)} {
		if _, ok := transcriptCompletion([]byte(bad), "codex"); ok {
			t.Fatal("unfinished/commentary accepted")
		}
	}
	// The native phase must also be excluded by older-hook tail fallback.
	if err := os.WriteFile(path, []byte(header+raw+"\n"+strings.Replace(raw, "final_answer", "commentary", 1)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text, _, _ := completionTail(path, "codex")
	if text != "Shipped native fix." {
		t.Fatal(text)
	}
}
