package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// schemaVersion is the highest migration this binary knows how to apply.
// Bump it and add an applyMigration case when the schema changes.
// Derived-index versions (schemaVersionFTS, schemaVersionFTSCleanup) are
// applied by applyDerivedIndexMigrations, not the linear chain in migrate.
const schemaVersion = 5

// schema is the base (v1) schema: capture and journal tables only.
const schema = `
CREATE TABLE IF NOT EXISTS frames (
  id   INTEGER PRIMARY KEY,
  ts   INTEGER NOT NULL,
  path TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS frames_ts ON frames(ts);

CREATE TABLE IF NOT EXISTS blocks (
  start_ts    INTEGER PRIMARY KEY,
  end_ts      INTEGER NOT NULL,
  title       TEXT NOT NULL DEFAULT '',
  summary     TEXT NOT NULL DEFAULT '',
  category    TEXT NOT NULL DEFAULT '',
  frame_count INTEGER NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'done',
  error       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  productive  INTEGER DEFAULT NULL,
  category_confidence REAL DEFAULT NULL,
  quality_confidence REAL DEFAULT NULL,
  triaged     INTEGER NOT NULL DEFAULT 0,
  same_as_prev INTEGER DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS blocks_status ON blocks(status);

-- Full audit log: every capture decision, pause change, summarizer run, error.
CREATE TABLE IF NOT EXISTS events (
  id     INTEGER PRIMARY KEY,
  ts     INTEGER NOT NULL,
  type   TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS events_ts ON events(ts);

-- One row per OpenRouter call: cost accounting + failure forensics.
CREATE TABLE IF NOT EXISTS api_calls (
  id                INTEGER PRIMARY KEY,
  ts                INTEGER NOT NULL,
  block_start       INTEGER NOT NULL,
  model             TEXT NOT NULL,
  frames_sent       INTEGER NOT NULL,
  prompt_tokens     INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms        INTEGER NOT NULL DEFAULT 0,
  status            TEXT NOT NULL DEFAULT 'ok',
  error             TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS api_calls_ts ON api_calls(ts);

-- Small key/value scratch table for engine bookkeeping (e.g. one-time
-- backfill flags). Created in the base schema so every open converges.
CREATE TABLE IF NOT EXISTS meta (
  k TEXT PRIMARY KEY,
  v TEXT NOT NULL DEFAULT ''
);

-- Cached AI recaps of coding-agent sessions, keyed by transcript path and
-- invalidated by file mtime+size (transcripts are append-mostly).
-- worthy_confidence records the Jev worthiness score; a judged-unworthy
-- session persists with recap='' so it is never re-judged.
CREATE TABLE IF NOT EXISTS agent_recaps (
  path               TEXT PRIMARY KEY,
  source             TEXT NOT NULL DEFAULT '',
  session_start      INTEGER NOT NULL DEFAULT 0,
  recap              TEXT NOT NULL DEFAULT '',
  worthy_confidence  REAL DEFAULT NULL,
  quality_confidence REAL DEFAULT NULL,
  file_mtime         INTEGER NOT NULL DEFAULT 0,
  file_size          INTEGER NOT NULL DEFAULT 0,
  model              TEXT NOT NULL DEFAULT '',
  created_at         INTEGER NOT NULL
);

-- Cached agent briefings, one row per day, invalidated by a fingerprint
-- over every session's adapter fingerprint. mode records whether the model
-- prose pass ran ("model") or the payload is deterministic-only ("fallback").
CREATE TABLE IF NOT EXISTS agent_briefings (
  day         TEXT PRIMARY KEY,
  fingerprint TEXT NOT NULL DEFAULT '',
  payload     TEXT NOT NULL DEFAULT '',
  mode        TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL
);
`

// schemaV2 is migration version 2: chat, standup, journal, goals, LLM-call
// logging, and block-edit overlay tables.
const schemaV2 = `
-- Conversations for the chat-with-your-journal feature.
CREATE TABLE IF NOT EXISTS chat_conversations (
  id         INTEGER PRIMARY KEY,
  title      TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS chat_conversations_updated ON chat_conversations(updated_at);

CREATE TABLE IF NOT EXISTS chat_messages (
  id              INTEGER PRIMARY KEY,
  conversation_id INTEGER NOT NULL,
  role            TEXT NOT NULL,
  content         TEXT NOT NULL,
  tool_calls      TEXT NOT NULL DEFAULT '',
  created_at      INTEGER NOT NULL,
  FOREIGN KEY (conversation_id) REFERENCES chat_conversations(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS chat_messages_conversation ON chat_messages(conversation_id);

-- Editable standup drafts keyed by date (YYYY-MM-DD).
CREATE TABLE IF NOT EXISTS standup_drafts (
  date       TEXT PRIMARY KEY,
  highlights TEXT NOT NULL DEFAULT '',
  tasks      TEXT NOT NULL DEFAULT '',
  blockers   TEXT NOT NULL DEFAULT '',
  priorities TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

-- Journal beta: morning/evening notes and AI summary.
CREATE TABLE IF NOT EXISTS journal_entries (
  date       TEXT PRIMARY KEY,
  morning    TEXT NOT NULL DEFAULT '',
  evening    TEXT NOT NULL DEFAULT '',
  summary    TEXT NOT NULL DEFAULT '',
  updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS day_goals (
  id         INTEGER PRIMARY KEY,
  date       TEXT NOT NULL,
  goal       TEXT NOT NULL,
  completed  INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS day_goals_date ON day_goals(date);

-- Generic log for all LLM calls (chat, review, standup, etc.).
CREATE TABLE IF NOT EXISTS llm_calls (
  id                INTEGER PRIMARY KEY,
  ts                INTEGER NOT NULL,
  task              TEXT NOT NULL DEFAULT '',
  provider          TEXT NOT NULL DEFAULT '',
  model             TEXT NOT NULL DEFAULT '',
  prompt_tokens     INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  latency_ms        INTEGER NOT NULL DEFAULT 0,
  status            TEXT NOT NULL DEFAULT 'ok',
  error             TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS llm_calls_ts ON llm_calls(ts);

-- User edits to timeline blocks.
CREATE TABLE IF NOT EXISTS block_edits (
  id         INTEGER PRIMARY KEY,
  start_ts   INTEGER NOT NULL,
  field      TEXT NOT NULL,
  old_value  TEXT NOT NULL DEFAULT '',
  new_value  TEXT NOT NULL DEFAULT '',
  edited_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS block_edits_start_ts ON block_edits(start_ts);
`

// schemaV3 enforces one goal per day: drop duplicate rows (keeping the most
// recently inserted) and replace the plain date index with a unique one so
// concurrent upserts can't create dupes.
const schemaV3 = `
DELETE FROM day_goals WHERE id NOT IN (
  SELECT id FROM (
    SELECT id, ROW_NUMBER() OVER (
      PARTITION BY date ORDER BY completed DESC, id DESC
    ) rn FROM day_goals
  ) WHERE rn = 1
);
DROP INDEX IF EXISTS day_goals_date;
CREATE UNIQUE INDEX IF NOT EXISTS day_goals_date ON day_goals(date);
`

// columnPatches adds columns to databases created before the columns existed.
// Each is applied only when the column is actually missing.
var columnPatches = []struct {
	table, column, ddl string
}{
	{"frames", "app", `ALTER TABLE frames ADD COLUMN app TEXT NOT NULL DEFAULT ''`},
	{"frames", "bytes", `ALTER TABLE frames ADD COLUMN bytes INTEGER NOT NULL DEFAULT 0`},
	{"blocks", "attempts", `ALTER TABLE blocks ADD COLUMN attempts INTEGER NOT NULL DEFAULT 0`},
	{"blocks", "app", `ALTER TABLE blocks ADD COLUMN app TEXT NOT NULL DEFAULT ''`},
	{"blocks", "activities", `ALTER TABLE blocks ADD COLUMN activities TEXT NOT NULL DEFAULT ''`},
	{"blocks", "productive", `ALTER TABLE blocks ADD COLUMN productive INTEGER DEFAULT NULL`},
	{"blocks", "category_confidence", `ALTER TABLE blocks ADD COLUMN category_confidence REAL DEFAULT NULL`},
	{"blocks", "quality_confidence", `ALTER TABLE blocks ADD COLUMN quality_confidence REAL DEFAULT NULL`},
	{"blocks", "triaged", `ALTER TABLE blocks ADD COLUMN triaged INTEGER NOT NULL DEFAULT 0`},
	{"blocks", "same_as_prev", `ALTER TABLE blocks ADD COLUMN same_as_prev INTEGER DEFAULT NULL`},
}

func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func applyColumnPatches(db *sql.DB) error {
	for _, p := range columnPatches {
		has, err := hasColumn(db, p.table, p.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec(p.ddl); err != nil {
			return fmt.Errorf("add column %s.%s: %w", p.table, p.column, err)
		}
	}
	return nil
}

// applyMigration runs one schema version's statements plus its version stamp
// in a single transaction.
func applyMigration(db *sql.DB, v int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	switch v {
	case 2:
		if _, err := tx.Exec(schemaV2); err != nil {
			return err
		}
	case 3:
		if _, err := tx.Exec(schemaV3); err != nil {
			return err
		}
	case 4:
		// FTS5 search index: virtual table + maintenance triggers +
		// backfill, all in this transaction — a killed mid-backfill can
		// never leave a stamped-but-empty index. The backfill is idempotent
		// (NOT IN) so a retry over an index a `search --reindex` already
		// populated is a no-op rather than a double-indexing pass.
		if _, err := tx.Exec(schemaV4); err != nil {
			return err
		}
		if _, err := tx.Exec(ftsBackfillStmt); err != nil {
			return err
		}
		// A successful build clears the failure marker inside the same
		// commit so the backoff can't outlive the index it was throttling.
		if _, err := tx.Exec(`DELETE FROM meta WHERE k=?`, metaFTSFailedAt); err != nil {
			return err
		}
	case 5:
		if _, err := tx.Exec(schemaV5); err != nil {
			return err
		}
		// Upgrade v4-era trigger bodies in place — but only when a real FTS
		// index is present. A degraded v4 leaves no blocks_fts (or a plain
		// squatter table), and triggers pointing at a missing index would
		// fail every blocks write.
		if ftsIndexPresentTx(tx) {
			if _, err := tx.Exec(schemaV5TriggerRefresh); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("no migration defined for schema version %d", v)
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, v, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// migrate brings the database to schemaVersion. It never drops data and
// returns the first real error rather than continuing on a partial schema.
func migrate(db *sql.DB) error {
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		return err
	}
	var cur int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&cur); err != nil {
		return err
	}
	if cur > schemaVersion {
		return fmt.Errorf("database schema v%d is newer than this binary supports (v%d)", cur, schemaVersion)
	}
	// Column patches are idempotent (guarded by hasColumn) and cheap; run them
	// on every open so any pre-versioning install converges.
	if err := applyColumnPatches(db); err != nil {
		return err
	}
	if cur == 0 {
		// Unversioned install (fresh or pre-1.0.1): the base schema plus
		// column patches above constitute v1.
		if _, err := db.Exec(`INSERT INTO schema_migrations(version, applied_at) VALUES(1, ?)`, time.Now().Unix()); err != nil {
			return err
		}
		cur = 1
	}
	for v := cur + 1; v <= schemaVersion; v++ {
		if v == schemaVersionFTS || v == schemaVersionFTSCleanup {
			continue // derived-index state; applyDerivedIndexMigrations runs it
		}
		if err := applyMigration(db, v); err != nil {
			return fmt.Errorf("migration to schema v%d: %w", v, err)
		}
	}
	// The FTS index is derived state: it applies outside the linear chain so
	// a failure degrades to LIKE fallback instead of wedging openDB — and so
	// its retry bookkeeping doesn't ride on MAX(version), which a later
	// migration may already have stamped. Unapplied versions stay unstamped;
	// a recorded failure backs the retry off for ftsRetryBackoff.
	applyDerivedIndexMigrations(db)
	return nil
}

// dbSchemaVersion reports the recorded schema version, or 0 when the database
// has never been versioned (unmigrated or absent).
func dbSchemaVersion(db *sql.DB) int {
	var v int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0
	}
	return v
}

func openDB() (*sql.DB, error) {
	if err := os.MkdirAll(dataDir(), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath()+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// openDBReadOnly opens the database without creating dirs or running
// migrations — used by `mcp --read-only` so a read-only agent session can
// never write, even via schema changes. Errors if the database is missing.
func openDBReadOnly() (*sql.DB, error) {
	if _, err := os.Stat(dbPath()); err != nil {
		return nil, err
	}
	return sql.Open("sqlite", "file:"+dbPath()+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
}

func insertFrame(db *sql.DB, ts time.Time, path string) error {
	_, err := db.Exec(`INSERT INTO frames(ts, path) VALUES(?, ?)`, ts.Unix(), path)
	return err
}

func insertFrameApp(db *sql.DB, ts time.Time, path, app string, bytes int64) error {
	_, err := db.Exec(`INSERT INTO frames(ts, path, app, bytes) VALUES(?, ?, ?, ?)`, ts.Unix(), path, app, bytes)
	return err
}

// dominantApp returns the most common non-empty app among frames in [start,end).
func dominantApp(db *sql.DB, start, end time.Time) string {
	var app string
	db.QueryRow(`SELECT app FROM frames WHERE ts >= ? AND ts < ? AND app != ''
	  GROUP BY app ORDER BY COUNT(1) DESC LIMIT 1`, start.Unix(), end.Unix()).Scan(&app)
	return app
}

// framesBetween returns frame rows in [start, end).
func framesBetween(db *sql.DB, start, end time.Time) ([]struct {
	TS   int64
	Path string
}, error) {
	rows, err := db.Query(`SELECT ts, path FROM frames WHERE ts >= ? AND ts < ? ORDER BY ts`,
		start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		TS   int64
		Path string
	}
	for rows.Next() {
		var r struct {
			TS   int64
			Path string
		}
		if err := rows.Scan(&r.TS, &r.Path); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func deleteFrames(db *sql.DB, start, end time.Time) error {
	_, err := db.Exec(`DELETE FROM frames WHERE ts >= ? AND ts < ?`, start.Unix(), end.Unix())
	return err
}

func upsertBlock(db *sql.DB, start, end time.Time, title, summary, category string, frameCount int, status, errStr string) error {
	_, err := db.Exec(`INSERT INTO blocks(start_ts,end_ts,title,summary,category,frame_count,status,error,created_at)
	  VALUES(?,?,?,?,?,?,?,?,?)
	  ON CONFLICT(start_ts) DO UPDATE SET end_ts=excluded.end_ts, title=excluded.title,
	    summary=excluded.summary, category=excluded.category, frame_count=excluded.frame_count,
	    status=excluded.status, error=excluded.error`,
		start.Unix(), end.Unix(), title, summary, category, frameCount, status, errStr, time.Now().Unix())
	return err
}

// upsertBlockFull additionally stores the dominant app and per-app activities JSON.
func upsertBlockFull(db *sql.DB, start, end time.Time, title, summary, category, app, activities string, frameCount, attempts int, status, errStr string, productive *bool) error {
	prodArg := sql.NullBool{Valid: productive != nil}
	if productive != nil {
		prodArg.Bool = *productive
	}
	_, err := db.Exec(`INSERT INTO blocks(start_ts,end_ts,title,summary,category,app,activities,frame_count,attempts,status,error,created_at,productive)
	  VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
	  ON CONFLICT(start_ts) DO UPDATE SET end_ts=excluded.end_ts, title=excluded.title,
	    summary=excluded.summary, category=excluded.category, app=excluded.app,
	    activities=excluded.activities, frame_count=excluded.frame_count, attempts=excluded.attempts,
	    status=excluded.status, error=excluded.error, productive=excluded.productive`,
		start.Unix(), end.Unix(), title, summary, category, app, activities, frameCount, attempts, status, errStr, time.Now().Unix(), prodArg)
	return err
}

// setBlockJudgment stores Jev's calibrated judgments on an existing block row.
// Nil pointers leave the column NULL — "no opinion" stays distinguishable
// from a confident zero.
func setBlockJudgment(db *sql.DB, start time.Time, confidence, quality *float64, sameAsPrev *bool) error {
	sets := []string{}
	args := []any{}
	if confidence != nil {
		sets = append(sets, "category_confidence = ?")
		args = append(args, *confidence)
	}
	if quality != nil {
		sets = append(sets, "quality_confidence = ?")
		args = append(args, *quality)
	}
	if sameAsPrev != nil {
		v := int64(0)
		if *sameAsPrev {
			v = 1
		}
		sets = append(sets, "same_as_prev = ?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, start.Unix())
	_, err := db.Exec(`UPDATE blocks SET `+strings.Join(sets, ", ")+` WHERE start_ts = ?`, args...)
	return err
}

// flagFailedBlock gives dead/failed blocks a visible identity at read time
// (DB rows stay untouched): they render as "Recording failed" entries so
// gaps in timelines and exports are explainable instead of invisible.
func flagFailedBlock(b *Block) {
	if b.Status == "done" {
		return
	}
	b.Title = "Recording failed"
	b.Category = "failed"
	if b.Summary == "" && b.Error != "" {
		b.Summary = strings.SplitN(b.Error, "\n", 2)[0]
	}
}

func blockExists(db *sql.DB, start time.Time) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(1) FROM blocks WHERE start_ts = ? AND status IN ('done','dead')`, start.Unix()).Scan(&n)
	return n > 0, err
}

// blockAttempts returns the retry count for a (usually failed) block.
func blockAttempts(db *sql.DB, start time.Time) int {
	var n int
	db.QueryRow(`SELECT attempts FROM blocks WHERE start_ts = ?`, start.Unix()).Scan(&n)
	return n
}

// resetFailedBlocks deletes failed/dead blocks so they re-summarize — and
// deletes their block_edits rows too, otherwise an edit made on a failed
// block would overlay whatever the retry summarizes at the same start_ts.
// The match set is snapshotted first because the same WHERE can't re-derive
// it once the blocks are gone, and the edits delete must run second: the
// blocks_fts_ad trigger reads block_edits to un-index the effective OLD
// text.
func resetFailedBlocks(db *sql.DB) (int64, error) {
	return deleteBlocksWhere(db, `status IN ('failed','dead')`)
}

// deleteBlocksWhere deletes blocks rows matching pred (plus their
// block_edits overlay rows) in one transaction. blocks go first — the
// blocks_fts_ad trigger's unindex must still see the edits — and the
// explicit edits delete also covers a degraded database where the index
// (and therefore the cascade inside the trigger) doesn't exist.
func deleteBlocksWhere(db *sql.DB, pred string, args ...any) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT start_ts FROM blocks WHERE `+pred, args...)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	var total int64
	for i := 0; i < len(ids); i += 500 {
		j := min(i+500, len(ids))
		ph := strings.TrimSuffix(strings.Repeat("?,", j-i), ",")
		chunk := make([]any, 0, j-i)
		for _, id := range ids[i:j] {
			chunk = append(chunk, id)
		}
		r, err := tx.Exec(`DELETE FROM blocks WHERE start_ts IN (`+ph+`)`, chunk...)
		if err != nil {
			return 0, err
		}
		if n, err := r.RowsAffected(); err == nil {
			total += n
		}
		if _, err := tx.Exec(`DELETE FROM block_edits WHERE start_ts IN (`+ph+`)`, chunk...); err != nil {
			return 0, err
		}
	}
	return total, tx.Commit()
}

type Activity struct {
	App        string `json:"app"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Category   string `json:"category"`
	Productive *bool  `json:"productive,omitempty"`
}

type Block struct {
	Start              time.Time  `json:"-"`
	End                time.Time  `json:"-"`
	StartTs            int64      `json:"start_ts"`
	EndTs              int64      `json:"end_ts"`
	StartStr           string     `json:"start"`
	EndStr             string     `json:"end"`
	Title              string     `json:"title"`
	Summary            string     `json:"summary"`
	Category           string     `json:"category"`
	App                string     `json:"app"`
	AppName            string     `json:"app_name"`
	Productive         *bool      `json:"productive,omitempty"`
	CategoryConfidence *float64   `json:"category_confidence,omitempty"`
	QualityConfidence  *float64   `json:"quality_confidence,omitempty"`
	SameAsPrev         *bool      `json:"same_as_prev,omitempty"`
	LowConfidence      bool       `json:"low_confidence,omitempty"`
	Activities         []Activity `json:"activities,omitempty"`
	FrameCount         int        `json:"frame_count"`
	Status             string     `json:"status"`
	Error              string     `json:"error,omitempty"`
}

// IsProductive returns true for blocks the LLM flagged as productive, or
// falls back to the legacy category/app heuristic for older blocks.
func (b Block) IsProductive() bool {
	if b.Productive != nil {
		return *b.Productive
	}
	return !isDistractionCategory(b.Category) && !isDistractionApp(b.App)
}

func blocksForDay(db *sql.DB, day time.Time, desc bool) ([]Block, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	end := start.AddDate(0, 0, 1)
	order := "ASC"
	if desc {
		order = "DESC"
	}
	q := `SELECT start_ts,end_ts,title,summary,category,frame_count,app,activities,productive,category_confidence,quality_confidence,same_as_prev,status,COALESCE(error,'') FROM blocks
	  WHERE start_ts >= ? AND start_ts < ? AND status IN ('done','dead','failed') ORDER BY start_ts ` + order
	rows, err := db.Query(q, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Block
	for rows.Next() {
		var b Block
		var s, e int64
		var acts string
		var prod sql.NullBool
		var conf, qual sql.NullFloat64
		var same sql.NullInt64
		if err := rows.Scan(&s, &e, &b.Title, &b.Summary, &b.Category, &b.FrameCount, &b.App, &acts, &prod, &conf, &qual, &same, &b.Status, &b.Error); err != nil {
			return nil, err
		}
		if prod.Valid {
			b.Productive = &prod.Bool
		}
		if conf.Valid {
			v := conf.Float64
			b.CategoryConfidence = &v
		}
		if qual.Valid {
			v := qual.Float64
			b.QualityConfidence = &v
		}
		if same.Valid {
			v := same.Int64 == 1
			b.SameAsPrev = &v
		}
		if acts != "" {
			json.Unmarshal([]byte(acts), &b.Activities)
		}
		flagFailedBlock(&b)
		b.LowConfidence = blockLowConfidence(b)
		b.Start = time.Unix(s, 0).Local()
		b.End = time.Unix(e, 0).Local()
		b.StartTs = s
		b.EndTs = e
		b.StartStr = b.Start.Format("3:04 PM")
		b.EndStr = b.End.Format("3:04 PM")
		b.AppName = appDisplayName(b.App)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applyBlockEdits(db, out, start.Unix(), end.Unix())
}

func countFramesToday(db *sql.DB, now time.Time) (int, error) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var n int
	err := db.QueryRow(`SELECT COUNT(1) FROM frames WHERE ts >= ?`, start.Unix()).Scan(&n)
	return n, err
}

func countBlocksToday(db *sql.DB, now time.Time) (int, error) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var n int
	err := db.QueryRow(`SELECT COUNT(1) FROM blocks WHERE start_ts >= ? AND status='done'`, start.Unix()).Scan(&n)
	return n, err
}

func lastFrameTS(db *sql.DB) (int64, error) {
	var ts int64
	err := db.QueryRow(`SELECT COALESCE(MAX(ts),0) FROM frames`).Scan(&ts)
	return ts, err
}

func logEvent(db *sql.DB, typ, detail string) {
	if db == nil {
		return
	}
	db.Exec(`INSERT INTO events(ts, type, detail) VALUES(?,?,?)`, time.Now().Unix(), typ, detail)
}

func logAPICall(db *sql.DB, blockStart time.Time, model string, framesSent, promptTok, completionTok, latencyMs int, status, errStr string) {
	if db == nil {
		return
	}
	db.Exec(`INSERT INTO api_calls(ts, block_start, model, frames_sent, prompt_tokens, completion_tokens, latency_ms, status, error)
	  VALUES(?,?,?,?,?,?,?,?,?)`,
		time.Now().Unix(), blockStart.Unix(), model, framesSent, promptTok, completionTok, latencyMs, status, errStr)
}

type usageRow struct {
	Calls        int     `json:"calls"`
	OK           int     `json:"ok"`
	Failed       int     `json:"failed"`
	FailureRate  float64 `json:"failure_rate"` // failed/calls, per the status != 'ok' convention
	PromptTok    int     `json:"prompt_tokens"`
	ComplTok     int     `json:"completion_tokens"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	// EstCostUSD is set only on by_model rows when the pricing config names
	// the model — absent means the output stays token-only for that row.
	EstCostUSD *float64 `json:"est_cost_usd,omitempty"`

	latSum int64 // merged across ledgers so AVG survives the fold
}

// add folds another group's totals into this row — the same dimension is
// grouped per-ledger (api_calls + llm_calls) and then merged.
func (r *usageRow) add(o usageRow) {
	r.Calls += o.Calls
	r.OK += o.OK
	r.Failed += o.Failed
	r.PromptTok += o.PromptTok
	r.ComplTok += o.ComplTok
	r.latSum += o.latSum
}

// finalize derives the rate and latency fields once raw totals are merged.
func (r *usageRow) finalize() {
	if r.Calls > 0 {
		r.FailureRate = float64(r.Failed) / float64(r.Calls)
		r.AvgLatencyMs = float64(r.latSum) / float64(r.Calls)
	}
}

// usageGroup aggregates one ledger grouped by an expression. The select must
// return (key, calls, ok, failed, prompt_tokens, completion_tokens, lat_sum).
func usageGroup(db *sql.DB, query string, args ...any) (map[string]usageRow, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]usageRow{}
	for rows.Next() {
		var k string
		var r usageRow
		if err := rows.Scan(&k, &r.Calls, &r.OK, &r.Failed, &r.PromptTok, &r.ComplTok, &r.latSum); err != nil {
			return nil, err
		}
		if k == "" {
			k = "unknown"
		}
		r.finalize()
		out[k] = r
	}
	return out, rows.Err()
}

// usageGroupSelect takes (group expression, table, where clause or "",
// group expression). The where clause is always a bound `ts` predicate —
// never interpolated input.
const usageGroupSelect = `SELECT %s, COUNT(1),
  COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0),
  COALESCE(SUM(CASE WHEN status!='ok' THEN 1 ELSE 0 END),0),
  COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
  COALESCE(SUM(latency_ms),0)
  FROM %s%s GROUP BY %s`

// usageWindow converts a --days N window into a cutoff. N counts local
// calendar days ending today (1 = today only), matching the localtime day
// bucketing below and blocksForDay's time.Local day boundaries. days <= 0
// means the full retained history.
func usageWindow(days int) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return start.AddDate(0, 0, -(days - 1))
}

// usageSummary aggregates both LLM ledgers: api_calls (block summarization,
// always OpenRouter) and llm_calls (chat/review/standup, any provider). This
// keeps the original signature for existing callers; it reports over the
// full retained history with no pricing.
func usageSummary(db *sql.DB) (map[string]any, error) {
	return usageSummaryWindow(db, 0, Config{})
}

// usageSummaryWindow is usageSummary over a bounded local-day window (days
// <= 0 = all retained rows). When cfg.Pricing is set it adds per-model dollar
// estimates to by_model plus a total est_cost_usd. data_since reports the
// earliest row actually counted, so ledger trimming (the 2000-row cap and
// retention_days) is visible rather than silently clipping totals.
func usageSummaryWindow(db *sql.DB, days int, cfg Config) (map[string]any, error) {
	where := ""
	var args []any
	if since := usageWindow(days); !since.IsZero() {
		where = " WHERE ts >= ?"
		args = append(args, since.Unix())
	}
	var calls, pt, ct, okn, failed int
	var latSum int64
	if err := db.QueryRow(`SELECT COUNT(1), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
	  COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status!='ok' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(latency_ms),0)
	  FROM api_calls`+where, args...).Scan(&calls, &pt, &ct, &okn, &failed, &latSum); err != nil {
		return nil, err
	}
	var lcalls, lpt, lct, lok, lfailed int
	var llatSum int64
	db.QueryRow(`SELECT COUNT(1), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0),
	  COALESCE(SUM(CASE WHEN status='ok' THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN status!='ok' THEN 1 ELSE 0 END),0),
	  COALESCE(SUM(latency_ms),0)
	  FROM llm_calls`+where, args...).Scan(&lcalls, &lpt, &lct, &lok, &lfailed, &llatSum)

	// Coverage floor: earliest row in the counted window across both ledgers.
	var dataSince string
	var minTS int64
	for _, tbl := range []string{"api_calls", "llm_calls"} {
		var m sql.NullInt64
		db.QueryRow(`SELECT MIN(ts) FROM `+tbl+where, args...).Scan(&m)
		if m.Valid && (minTS == 0 || m.Int64 < minTS) {
			minTS = m.Int64
		}
	}
	if minTS > 0 {
		dataSince = time.Unix(minTS, 0).Local().Format("2006-01-02")
	}

	apiRow := usageRow{Calls: calls, OK: okn, Failed: failed, PromptTok: pt, ComplTok: ct, latSum: latSum}
	apiRow.finalize()
	otherRow := usageRow{Calls: lcalls, OK: lok, Failed: lfailed, PromptTok: lpt, ComplTok: lct, latSum: llatSum}
	otherRow.finalize()

	byTask, err := usageGroup(db, fmt.Sprintf(usageGroupSelect, "task", "llm_calls", where, "task"), args...)
	if err != nil {
		return nil, err
	}
	if calls > 0 {
		r := byTask["summarize"]
		r.add(apiRow)
		r.finalize()
		byTask["summarize"] = r
	}
	byProvider, err := usageGroup(db, fmt.Sprintf(usageGroupSelect, "provider", "llm_calls", where, "provider"), args...)
	if err != nil {
		return nil, err
	}
	// api_calls has no provider column — derive the bucket from the model
	// label summarize logged: 'cli:<command|id>' means a cli vision provider
	// (summarize.go), anything else went over OpenRouter.
	const apiProviderExpr = `CASE WHEN model LIKE 'cli:%' THEN 'cli' ELSE 'openrouter' END`
	apiProviders, err := usageGroup(db, fmt.Sprintf(usageGroupSelect, apiProviderExpr, "api_calls", where, apiProviderExpr), args...)
	if err != nil {
		return nil, err
	}
	for k, gr := range apiProviders {
		r := byProvider[k]
		r.add(gr)
		r.finalize()
		byProvider[k] = r
	}
	byModel := map[string]usageRow{}
	byDay := map[string]usageRow{}
	// Day buckets use the engine's localtime convention — date(ts,'unixepoch',
	// 'localtime') matches blocksForDay's time.Local day boundaries.
	const dayExpr = `date(ts,'unixepoch','localtime')`
	for _, tbl := range []string{"api_calls", "llm_calls"} {
		mg, err := usageGroup(db, fmt.Sprintf(usageGroupSelect, "model", tbl, where, "model"), args...)
		if err != nil {
			return nil, err
		}
		for k, gr := range mg {
			m := byModel[k]
			m.add(gr)
			m.finalize()
			byModel[k] = m
		}
		dg, err := usageGroup(db, fmt.Sprintf(usageGroupSelect, dayExpr, tbl, where, dayExpr), args...)
		if err != nil {
			return nil, err
		}
		for k, gr := range dg {
			d := byDay[k]
			d.add(gr)
			d.finalize()
			byDay[k] = d
		}
	}
	sum := map[string]any{
		"window_days": days, "data_since": dataSince,
		"api_calls": calls, "ok": okn, "failed": failed,
		"failure_rate": apiRow.FailureRate, "avg_latency_ms": apiRow.AvgLatencyMs,
		"prompt_tokens": pt, "completion_tokens": ct,
		"other_llm_calls": lcalls, "other_ok": lok, "other_failed": lfailed,
		"other_failure_rate": otherRow.FailureRate, "other_avg_latency_ms": otherRow.AvgLatencyMs,
		"other_prompt_tokens": lpt, "other_completion_tokens": lct,
		"total_prompt_tokens": pt + lpt, "total_completion_tokens": ct + lct,
		// The row caps in trimLogTables (capture.go, literal 2000) apply even
		// when retention_days is 0 — data_since makes any clipping visible.
		"coverage": map[string]any{
			"api_calls_row_cap": 2000,
			"llm_calls_row_cap": 2000,
			"retention_days":    cfg.RetentionDays,
		},
		"breakdown": map[string]any{
			"by_task": byTask, "by_provider": byProvider, "by_model": byModel, "by_day": byDay,
		},
	}
	if len(cfg.Pricing) > 0 {
		total := 0.0
		var unpriced []string
		for k, r := range byModel {
			if rate, ok := cfg.Pricing[k]; ok {
				c := float64(r.PromptTok+r.ComplTok) * rate / 1e6
				r.EstCostUSD = &c
				byModel[k] = r
				total += c
			} else {
				unpriced = append(unpriced, k)
			}
		}
		sum["est_cost_usd"] = total
		if len(unpriced) > 0 {
			sort.Strings(unpriced)
			sum["unpriced_models"] = unpriced
		}
	}
	return sum, nil
}

// framesBefore deletes frame rows (and optionally files) older than cutoff.
func framesBefore(db *sql.DB, cutoff time.Time) ([]string, error) {
	rows, err := db.Query(`SELECT path FROM frames WHERE ts < ?`, cutoff.Unix())
	if err != nil {
		return nil, err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	_, err = db.Exec(`DELETE FROM frames WHERE ts < ?`, cutoff.Unix())
	return paths, err
}

// deleteBlocksLike removes done/failed blocks whose title or summary matches
// the case-insensitive LIKE pattern — and their block_edits overlay rows,
// whose old_value/new_value would otherwise keep the scrubbed text forever.
// The match covers edited text too: the edit overlay is what users see
// (applyBlockEdits) and what the FTS index holds (ftsEffExpr), so a scrub
// must catch a term that exists only in block_edits.new_value — and still
// catch the raw term an edit renamed away (old_value). It returns the
// number of blocks rows deleted.
func deleteBlocksLike(db *sql.DB, pattern string) (int64, error) {
	like := "%" + pattern + "%"
	return deleteBlocksWhere(db, `status IN ('done','failed') AND
	  (LOWER(title) LIKE LOWER(?) OR LOWER(summary) LIKE LOWER(?)
	   OR EXISTS (SELECT 1 FROM block_edits e WHERE e.start_ts = blocks.start_ts
	     AND (LOWER(e.old_value) LIKE LOWER(?) OR LOWER(e.new_value) LIKE LOWER(?))))`,
		like, like, like, like)
}

func pruneOldEvents(db *sql.DB, cutoff time.Time) {
	// Optional table: created on the first completion hook.
	db.Exec(`DELETE FROM agent_completions WHERE completed_at < ?`, cutoff.Unix())
	db.Exec(`DELETE FROM events WHERE ts < ?`, cutoff.Unix())
	db.Exec(`DELETE FROM api_calls WHERE ts < ?`, cutoff.Unix())
	// llm_calls joins the retention window (U5): the cost ledger must not
	// silently outlive the journal rows it accounts. trimLogTables
	// (capture.go) also caps it at the newest 2000 rows alongside
	// events/api_calls.
	db.Exec(`DELETE FROM llm_calls WHERE ts < ?`, cutoff.Unix())
}
