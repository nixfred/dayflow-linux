package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// This fallback watches explicit final-reply records, not inferred idle
// sessions. It makes no model calls and runs separately from capture.
func transcriptCompletion(raw []byte, source string) (completionInput, bool) {
	var e struct {
		UUID      string `json:"uuid"`
		Session   string `json:"sessionId"`
		Cwd       string `json:"cwd"`
		Timestamp string `json:"timestamp"`
		Message   struct {
			StopReason string `json:"stop_reason"`
		} `json:"message"`
		Metadata struct {
			Retained struct {
				Complete *bool `json:"complete"`
			} `json:"retained_source"`
		} `json:"metadata"`
		Payload struct {
			Channel string `json:"channel"`
			Phase   string `json:"phase"`
			Meta    struct {
				Turn string `json:"turn_id"`
			} `json:"internal_chat_message_metadata_passthrough"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &e) != nil {
		return completionInput{}, false
	}
	var role, text string
	var ts int64
	p := completionInput{Session: e.Session, Cwd: e.Cwd, Turn: e.UUID}
	if source == "claude" {
		if e.Message.StopReason != "end_turn" && e.Message.StopReason != "stop_sequence" {
			return p, false
		}
		role, text, ts = claudeLineTurn(raw)
	} else {
		if e.Payload.Channel != "final" && e.Payload.Phase != "final_answer" {
			return p, false
		}
		if e.Metadata.Retained.Complete != nil && !*e.Metadata.Retained.Complete {
			return p, false
		}
		role, text, ts = codexLineTurn(raw)
		p.Turn = e.Payload.Meta.Turn
	}
	p.Summary = text
	p.CompletedAt = ts
	return p, role == "assistant" && strings.TrimSpace(text) != "" && ts > 0
}

func scanCompletionFile(db *sql.DB, path, source string, now time.Time) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// Codex's first session_meta line supplies stable session ID and cwd.
	var header struct {
		Payload struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		} `json:"payload"`
	}
	first := bufio.NewScanner(io.LimitReader(f, 1<<20))
	first.Buffer(make([]byte, 4096), 1<<20)
	if first.Scan() {
		_ = json.Unmarshal(first.Bytes(), &header)
	}
	st, err := f.Stat()
	if err != nil {
		return err
	}
	offset := int64(0)
	if st.Size() > 1<<20 {
		offset = st.Size() - (1 << 20)
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		p, ok := transcriptCompletion(sc.Bytes(), source)
		if !ok || p.CompletedAt < now.Add(-24*time.Hour).Unix() {
			continue
		}
		if source == "codex" {
			p.Session = header.Payload.ID
			p.Cwd = header.Payload.Cwd
		}
		if p.Session == "" {
			p.Session = strings.TrimSuffix(filepath.Base(path), ".jsonl")
		}
		b, _ := json.Marshal(p)
		c, err := decodeCompletion(strings.NewReader(string(b)), source, now)
		if err != nil {
			continue
		}
		if err := recordCompletion(db, c); err != nil {
			return err
		}
	}
	return sc.Err()
}

func watchCompletions(ctx context.Context, db *sql.DB) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	seen := map[string]recapFingerprint{}
	for {
		now := time.Now()
		files := map[string]string{}
		claude, _ := filepath.Glob(filepath.Join(claudeDir(), "*", "*.jsonl"))
		for _, p := range claude {
			files[p] = "claude"
		}
		for _, d := range []time.Time{now, now.AddDate(0, 0, -1)} {
			codex, _ := filepath.Glob(filepath.Join(codexDir(), d.Format("2006/01/02"), "*.jsonl"))
			for _, p := range codex {
				files[p] = "codex"
			}
		}
		for path, source := range files {
			select {
			case <-ctx.Done():
				return
			default:
			}
			fp, ok := fingerprint(path)
			if !ok || fp.Mtime < now.Add(-24*time.Hour).Unix() {
				continue
			}
			if old, ok := seen[path]; ok && old == fp {
				continue
			}
			if err := scanCompletionFile(db, path, source, now); err == nil {
				seen[path] = fp
			}
		}
		for p := range seen {
			if _, ok := files[p]; !ok {
				delete(seen, p)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
