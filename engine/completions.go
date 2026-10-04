package main

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Completion hooks persist agent replies locally, before any model work.
// They represent finished turns, not a claim that an entire project is done.
type agentCompletion struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Session     string `json:"session"`
	Project     string `json:"project"`
	Summary     string `json:"summary"`
	CompletedAt int64  `json:"completed_at"`
}

const completionDDL = `CREATE TABLE IF NOT EXISTS agent_completions (
 id TEXT PRIMARY KEY, source TEXT NOT NULL, session TEXT NOT NULL,
 project TEXT NOT NULL, summary TEXT NOT NULL, completed_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS agent_completions_time ON agent_completions(completed_at);`

type completionInput struct {
	Session     string `json:"session_id"`
	Turn        string `json:"turn_id"`
	Cwd         string `json:"cwd"`
	Summary     string `json:"last_assistant_message"`
	Transcript  string `json:"transcript_path"`
	CompletedAt int64  `json:"completed_at"`
	Event       string `json:"hook_event_name"`
}

// Older Claude hooks omit the reply. Read only a bounded transcript tail;
// skip tool calls, reasoning and Codex commentary when selecting the reply.
func completionTail(path, source string) (text, id string, ts int64) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return
	}
	if st.Size() > 1<<20 {
		_, _ = f.Seek(st.Size()-(1<<20), io.SeekStart)
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		var envelope struct {
			UUID    string `json:"uuid"`
			Payload struct {
				Channel string `json:"channel"`
				Phase   string `json:"phase"`
			} `json:"payload"`
		}
		if json.Unmarshal(scan.Bytes(), &envelope) != nil {
			continue
		}
		var role, body string
		var at int64
		if source == "claude" {
			role, body, at = claudeLineTurn(scan.Bytes())
		} else {
			if (envelope.Payload.Channel != "" && envelope.Payload.Channel != "final") || (envelope.Payload.Phase != "" && envelope.Payload.Phase != "final_answer") {
				continue
			}
			role, body, at = codexLineTurn(scan.Bytes())
		}
		if role == "assistant" && body != "" {
			text, id, ts = body, envelope.UUID, at
		}
	}
	return
}

func decodeCompletion(input io.Reader, source string, now time.Time) (agentCompletion, error) {
	c := agentCompletion{Source: source, CompletedAt: now.Unix()}
	if source != "claude" && source != "codex" && source != "manual" {
		return c, fmt.Errorf("unsupported completion source %q", source)
	}
	var p completionInput
	// Hooks close stdin after the JSON object. Reject trailing or oversized
	// input instead of silently journaling a truncated/partial payload.
	b, err := io.ReadAll(io.LimitReader(input, (1<<20)+1))
	if err != nil {
		return c, err
	}
	if len(b) > 1<<20 {
		return c, fmt.Errorf("completion input exceeds 1 MiB")
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return c, err
	}
	if p.Event != "" && p.Event != "Stop" {
		return c, fmt.Errorf("expected Stop event")
	}
	tail, messageID, _ := completionTail(p.Transcript, source)
	if p.Summary == "" {
		p.Summary = tail
	}
	if strings.TrimSpace(p.Summary) == "" {
		return c, fmt.Errorf("completion has no assistant reply")
	}
	if len(p.Summary) > 64<<10 {
		return c, fmt.Errorf("completion reply exceeds 64 KiB")
	}
	if p.CompletedAt != 0 {
		if p.CompletedAt < 0 || p.CompletedAt > now.Add(time.Minute).Unix() {
			return c, fmt.Errorf("invalid completion timestamp")
		}
		c.CompletedAt = p.CompletedAt
	}
	c.Session = scrubText(p.Session)
	c.Project = scrubText(filepath.Clean(p.Cwd))
	c.Summary = scrubText(strings.TrimSpace(p.Summary))
	if p.Turn == "" {
		p.Turn = messageID
	}
	// Stable retries deduplicate; distinct turn IDs with identical replies
	// still produce distinct records. Claude UUIDs come from its transcript.
	h := sha256.Sum256([]byte(source + "\x00" + p.Session + "\x00" + p.Cwd + "\x00" + p.Turn + "\x00" + c.Summary))
	c.ID = hex.EncodeToString(h[:])
	return c, nil
}

func recordCompletion(db *sql.DB, c agentCompletion) error {
	if _, err := db.Exec(completionDDL); err != nil {
		return err
	}
	_, err := db.Exec(`INSERT INTO agent_completions(id,source,session,project,summary,completed_at)
	 VALUES(?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, c.ID, c.Source, c.Session, c.Project, c.Summary, c.CompletedAt)
	return err
}

func completionsForDay(db *sql.DB, day time.Time) ([]agentCompletion, error) {
	out := []agentCompletion{}
	var present int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_completions'`).Scan(&present); err != nil {
		return out, err
	}
	if present == 0 {
		return out, nil
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	rows, err := db.Query(`SELECT id,source,session,project,summary,completed_at FROM agent_completions
	 WHERE completed_at>=? AND completed_at<? ORDER BY completed_at DESC,id`, start.Unix(), start.AddDate(0, 0, 1).Unix())
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c agentCompletion
		if err := rows.Scan(&c.ID, &c.Source, &c.Session, &c.Project, &c.Summary, &c.CompletedAt); err != nil {
			return out, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
