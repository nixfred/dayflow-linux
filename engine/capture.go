package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	xdraw "golang.org/x/image/draw"
)

func configMtime() time.Time {
	if fi, err := os.Stat(configPath()); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

func paused() bool {
	_, err := os.Stat(pausePath())
	return err == nil
}

// captureState reports capture-loop health for status surfaces. Paused wins;
// a locked screen with auto-pause is a legitimately quiet loop; otherwise the
// events heartbeat (written every tick, dedup included) going stale means the
// daemon process is gone — the failure mode that otherwise stays invisible.
func captureState(db *sql.DB, cfg Config) string {
	if paused() {
		return "paused"
	}
	if cfg.AutoPauseLocked && screenLocked() {
		return "locked"
	}
	var last int64
	db.QueryRow(`SELECT COALESCE(MAX(ts),0) FROM events
	  WHERE type IN ('capture_saved','capture_deduped','capture_ignored','capture_error')`).Scan(&last)
	stale := int64(60)
	if s := int64(3 * cfg.CaptureIntervalSec); s > stale {
		stale = s
	}
	if last == 0 || time.Now().Unix()-last > stale {
		return "down"
	}
	return "recording"
}

func setPaused(p bool) {
	if p {
		os.MkdirAll(dataDir(), 0o700)
		os.WriteFile(pausePath(), []byte("paused\n"), 0o600)
	} else {
		os.Remove(pausePath())
	}
}

// tickExecTimeout bounds the short status probes spawned on the daemon's
// tick path (hyprctl, loginctl). A hung helper must not wedge the
// single-goroutine capture loop — a var so tests can shrink it.
var tickExecTimeout = 3 * time.Second

// activeWindowClass returns the class of the focused window on Hyprland.
// It falls back to the window title when no class is reported, and returns
// "" when there is no focused window / not running under Hyprland.
func activeWindowClass() string {
	ctx, cancel := context.WithTimeout(context.Background(), tickExecTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hyprctl", "activewindow", "-j").Output()
	if err != nil || len(out) == 0 {
		return ""
	}
	var w struct {
		Class        string `json:"class"`
		InitialClass string `json:"initialClass"`
		Title        string `json:"title"`
		InitialTitle string `json:"initialTitle"`
	}
	if json.Unmarshal(out, &w) != nil {
		return ""
	}
	if w.Class != "" {
		return w.Class
	}
	if w.InitialClass != "" {
		return w.InitialClass
	}
	if w.Title != "" {
		return w.Title
	}
	return w.InitialTitle
}

func isIgnored(cfg Config, class string) bool {
	if class == "" {
		return false
	}
	for _, ig := range cfg.IgnoreApps {
		if strings.EqualFold(strings.TrimSpace(ig), class) {
			return true
		}
	}
	return false
}

// screenLocked uses loginctl to detect whether the current graphical session is locked.
// It is best-effort: if loginctl is unavailable or the session cannot be identified,
// it returns false so capture continues.
func screenLocked() bool {
	sessions := []string{os.Getenv("XDG_SESSION_ID")}
	if sessions[0] == "" {
		// Fall back to the active graphical session for the current user.
		ctx, cancel := context.WithTimeout(context.Background(), tickExecTimeout)
		out, err := exec.CommandContext(ctx, "loginctl", "list-sessions", "--no-legend").Output()
		cancel()
		if err != nil {
			return false
		}
		user := os.Getenv("USER")
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// fields: ID UID USER SEAT [TTY ...]
			if fields[2] == user && fields[3] != "-" {
				sessions = append(sessions, fields[0])
			}
		}
		if len(sessions) == 1 {
			return false
		}
		sessions = sessions[1:]
	}
	for _, sid := range sessions {
		if sid == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), tickExecTimeout)
		out, err := exec.CommandContext(ctx, "loginctl", "show-session", sid, "--property=LockedHint").Output()
		cancel()
		if err != nil || len(out) == 0 {
			continue
		}
		if strings.Contains(string(out), "yes") {
			return true
		}
	}
	return false
}

// frameHash is a 256-bit perceptual hash (16x16 grayscale average hash).
type frameHash [4]uint64

// ahash computes a 16x16 grayscale average-hash of the image.
func ahash(img image.Image) frameHash {
	const size = 16
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var px [size * size]uint32
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			sx := b.Min.X + x*w/size
			sy := b.Min.Y + y*h/size
			r, g, bl, _ := img.At(sx, sy).RGBA()
			px[y*size+x] = (r + g + bl) / 3 >> 8
		}
	}
	var sum uint64
	for _, v := range px {
		sum += uint64(v)
	}
	avg := uint32(sum / (size * size))
	var hash frameHash
	for i, v := range px {
		if v >= avg {
			hash[i/64] |= 1 << (i % 64)
		}
	}
	return hash
}

func hamming(a, b frameHash) int {
	n := 0
	for i := range a {
		for d := a[i] ^ b[i]; d != 0; d >>= 1 {
			n += int(d & 1)
		}
	}
	return n
}

// grimArgv returns the grim command line for one grab. output "" captures
// all outputs composited; a name passes it through as `grim -o <name>`.
func grimArgv(cfg Config, output string) []string {
	args := []string{"grim", "-t", "jpeg", "-q", fmt.Sprint(cfg.JPEGQuality)}
	if output != "" {
		args = append(args, "-o", output)
	}
	return append(args, "-")
}

// resolveCaptureCommand picks the screenshot tool. grim (wlroots: Hyprland,
// sway, river, ...) is the only built-in backend; capture_command in config can
// point at anything that writes a JPEG/PNG to stdout. For the grim backend a
// literal output name bakes -o into the returned argv; "auto" returns the
// composite argv — captureOnce re-resolves the focused output per tick, so a
// dock/focus change can't pin capture to the start-time monitor.
func resolveCaptureCommand(cfg Config) ([]string, error) {
	if cfg.CaptureCommand != "" {
		return strings.Fields(cfg.CaptureCommand), nil
	}
	if _, err := exec.LookPath("grim"); err != nil {
		return nil, fmt.Errorf("grim not found — install it (wlroots compositors) or set capture_command in %s", configPath())
	}
	output := cfg.Output
	if output == "auto" {
		output = "" // resolved per tick by autoOutput
	}
	return grimArgv(cfg, output), nil
}

// focusFailCeiling negative-caches focus resolution: after this many
// consecutive `hyprctl monitors` failures the daemon stops spawning hyprctl
// until focusRetryBackoff elapses — on non-Hyprland systems output=auto
// would otherwise pay a dead exec every tick forever, but a daemon started
// during compositor init must not stay latched off for its whole lifetime.
const focusFailCeiling = 5

// focusRetryBackoff time-boxes the negative cache: once it elapses the
// daemon spends a single probe on `hyprctl monitors` — success clears the
// latch, failure re-arms it for another interval. A var so tests can
// shrink it.
var focusRetryBackoff = time.Hour

var (
	focusFails      int       // consecutive hyprctl monitors failures
	focusDisabled   bool      // negative cache: focus resolution switched off
	focusDisabledAt time.Time // when the latch last engaged
	lastAutoOutput  string    // last recorded -o target ("composite" when none)
)

// focusedOutput asks Hyprland which output currently holds focus
// (`hyprctl monitors -j`, focused:true). It returns "" — composite
// capture — when no output is focused or the focused output is a
// HEADLESS-* node (grim can't -o those). Errors count toward the
// negative cache in autoOutput.
func focusedOutput() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tickExecTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "hyprctl", "monitors", "-j").Output()
	if err != nil {
		return "", err
	}
	var mons []struct {
		Name    string `json:"name"`
		Focused bool   `json:"focused"`
	}
	if err := json.Unmarshal(out, &mons); err != nil {
		return "", fmt.Errorf("hyprctl monitors: %w", err)
	}
	for _, m := range mons {
		if m.Focused && !strings.HasPrefix(m.Name, "HEADLESS-") {
			return m.Name, nil
		}
	}
	return "", nil
}

// autoOutput resolves the grim -o target for one tick under output=auto:
// the focused output's name, or "" for a composite grab. Consecutive
// hyprctl failures negative-cache the lookup for focusRetryBackoff (one
// event, not per-tick spam), after which a single re-probe decides whether
// to unlatch or re-arm — so a daemon that started while the compositor was
// still coming up recovers instead of staying composite-only forever. The
// resolved target is logged only when it changes so mixed-resolution frame
// streams stay attributable.
func autoOutput(db *sql.DB, cfg Config) string {
	if focusDisabled && time.Since(focusDisabledAt) < focusRetryBackoff {
		return ""
	}
	name, err := focusedOutput()
	if err != nil {
		if focusDisabled {
			// Post-backoff probe still failing — re-arm the latch without
			// paying a full fail streak every interval.
			focusDisabledAt = time.Now()
			debugf(cfg, "capture: output=auto re-probe failed, re-latching: %v", err)
			return ""
		}
		focusFails++
		if focusFails >= focusFailCeiling {
			focusDisabled = true
			focusDisabledAt = time.Now()
			logEvent(db, "capture_output_disabled",
				fmt.Sprintf("hyprctl monitors failing: %v", err))
			debugf(cfg, "capture: output=auto disabled after %d consecutive hyprctl failures: %v",
				focusFails, err)
		}
	} else {
		if focusDisabled {
			focusDisabled = false
			focusDisabledAt = time.Time{}
			logEvent(db, "capture_output_enabled", "hyprctl monitors reachable")
			debugf(cfg, "capture: output=auto re-enabled after %s backoff", focusRetryBackoff)
		}
		focusFails = 0
	}
	resolved := name
	if resolved == "" {
		resolved = "composite"
	}
	if resolved != lastAutoOutput {
		logEvent(db, "capture_output", resolved)
		debugf(cfg, "capture: output %s", resolved)
		lastAutoOutput = resolved
	}
	return name
}

// grabFrameTimeout bounds one capture_command run — a hung screenshot tool
// must not stall the capture loop forever. A var so tests can shrink it.
var grabFrameTimeout = 30 * time.Second

func grabFrame(cmdArgs []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), grabFrameTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	// grim output is bounded by screen size, but a custom capture_command
	// can stream arbitrarily — cap before an infinite stream OOMs us.
	out := &cappedBuffer{limit: 64 << 20}
	cmd.Stdout = out
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("capture command timed out after %s: %w", grabFrameTimeout, err)
		}
		return nil, err
	}
	return out.buf.Bytes(), nil
}

const dedupThreshold = 5 // hamming distance out of 256 bits

// waylandReachable reports whether the configured Wayland socket exists.
// grim exits 1 instantly when it doesn't ("failed to create display") —
// no session means nothing to capture, so the daemon pauses quietly instead
// of error-storming every interval. Returns true when it can't check (unset
// env, inspection error, or non-socket layout) so the capture command
// remains the arbiter.
func waylandReachable() bool {
	disp := os.Getenv("WAYLAND_DISPLAY")
	if disp == "" {
		return true
	}
	path := disp
	if !filepath.IsAbs(disp) {
		rt := os.Getenv("XDG_RUNTIME_DIR")
		if rt == "" {
			return true
		}
		path = filepath.Join(rt, disp)
	}
	st, err := os.Stat(path)
	if err != nil {
		// Only a confirmed missing path means no session; permission or
		// other fs errors defer to the capture command's own reporting.
		return !os.IsNotExist(err)
	}
	return st.Mode()&os.ModeSocket != 0
}

// captureFailVisible throttles repeated capture errors: the first failure
// and every ~5-minute mark thereafter are logged; the rest are silent so a
// dead-session window can't flood the debug log, events table, and journald.
func captureFailVisible(streak int) bool {
	return streak == 1 || streak%30 == 0
}

// captureOnce samples the screen and stores a frame when it has changed.
// lastHash is the previous frame's hash, or nil when no frame has been
// sampled yet (e.g. after unlock or pause). The returned hash is the latest
// sampled hash — unchanged on early returns. attempted reports whether a
// frame was actually grabbed — paused/ignored skips return false so the
// caller doesn't count them as recovery from a failure streak.
func captureOnce(db *sql.DB, cfg Config, cmdArgs []string, lastHash *frameHash) (*frameHash, bool, error) {
	if paused() {
		return lastHash, false, nil
	}
	cls := activeWindowClass()
	if isIgnored(cfg, cls) {
		logEvent(db, "capture_ignored", cls)
		debugf(cfg, "capture: ignored app %s", cls)
		return lastHash, false, nil
	}
	// output=auto re-resolves the focused monitor every tick and injects
	// -o into a freshly built grim argv. The injection is grim-path only —
	// a custom capture_command is used verbatim and auto is ignored there.
	args := cmdArgs
	var composite []string
	if cfg.CaptureCommand == "" && cfg.Output == "auto" &&
		len(args) > 0 && filepath.Base(args[0]) == "grim" {
		if name := autoOutput(db, cfg); name != "" {
			composite = grimArgv(cfg, "")
			args = grimArgv(cfg, name)
		}
	}
	raw, err := grabFrame(args)
	if err != nil && composite != nil {
		// resolve→exec race (e.g. the output was unplugged between
		// `hyprctl monitors` and `grim -o`, or a dock transition): one
		// composite retry this tick before the failure counts.
		debugf(cfg, "capture: grim -o failed (%v); retrying composite", err)
		raw, err = grabFrame(composite)
	}
	if err != nil {
		// Logging is the caller's job — it streak-throttles so a dead-session
		// window doesn't flood every sink each interval.
		return lastHash, true, fmt.Errorf("capture: %w", err)
	}
	img, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return lastHash, true, fmt.Errorf("decode: %w", err)
	}
	h := ahash(img)
	if lastHash != nil && hamming(h, *lastHash) <= dedupThreshold {
		logEvent(db, "capture_deduped", "")
		debugf(cfg, "capture: deduped (hamming %d, app %s)", hamming(h, *lastHash), cls)
		return lastHash, true, nil // screen unchanged
	}

	stored := storedFrame(img, raw, format, cfg)
	now := time.Now()
	dayDir := filepath.Join(framesDir(), now.Format("2006-01-02"))
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		return lastHash, true, err
	}
	path := filepath.Join(dayDir, now.Format("150405")+".jpg")
	if err := os.WriteFile(path, stored, 0o600); err != nil {
		return lastHash, true, err
	}
	if err := insertFrameApp(db, now, path, cls, int64(len(stored))); err != nil {
		os.Remove(path)
		return lastHash, true, err
	}
	logEvent(db, "capture_saved", path)
	debugf(cfg, "capture: saved %s (%d bytes, app %s)", filepath.Base(path), len(stored), cls)
	return &h, true, nil
}

// storedFrame returns the bytes to persist for a frame that survived dedup.
// Frames are always stored as JPEG: files carry .jpg names and summarize
// labels them data:image/jpeg, so a non-JPEG capture source (a custom
// capture_command that emits PNG) is re-encoded even when no resize is
// needed. When frame_max_dim is set and the frame's longer edge exceeds
// it, the decoded image is downscaled with its aspect ratio preserved;
// anything already under the cap — or the cap disabled with 0 — keeps the
// capture command's original bytes when they are already JPEG, so
// normalization costs nothing on frames that don't need it. grim emits an
// all-outputs composite, so the cap bounds the composite's long edge, not
// any single display.
func storedFrame(img image.Image, raw []byte, format string, cfg Config) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	long := w
	if h > w {
		long = h
	}
	resize := cfg.FrameMaxDim > 0 && long > cfg.FrameMaxDim
	if format == "jpeg" && !resize {
		return raw
	}
	dw, dh := w, h
	if resize {
		scale := float64(cfg.FrameMaxDim) / float64(long)
		dw = int(math.Round(float64(w) * scale))
		dh = int(math.Round(float64(h) * scale))
		if dw < 1 {
			dw = 1
		}
		if dh < 1 {
			dh = 1
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: cfg.JPEGQuality}); err != nil {
		return raw
	}
	return buf.Bytes()
}

// runRetention reconciles the frames dir with the frames table, then deletes
// frames and old log rows past the retention window.
func runRetention(db *sql.DB, cfg Config) {
	if _, err := reconcileFrames(db, cfg, false); err != nil {
		logEvent(db, "reconcile_error", err.Error())
	}
	if cfg.RetentionDays > 0 {
		cutoff := time.Now().Add(-time.Duration(cfg.RetentionDays) * 24 * time.Hour)
		paths, err := framesBefore(db, cutoff)
		if err != nil {
			logEvent(db, "retention_error", err.Error())
		} else {
			for _, p := range paths {
				os.Remove(p)
			}
			if len(paths) > 0 {
				logEvent(db, "retention_pruned", fmt.Sprintf("%d frames", len(paths)))
			}
		}
		pruneOldEvents(db, cutoff)
		// Indexed agent turns age out with the rest of the journal —
		// dedup keys in agent_ingest outlive them harmlessly (the rolling
		// watermark never rescans pruned days).
		db.Exec(`DELETE FROM agent_msgs_fts WHERE ts < ?`, cutoff.Unix())
	}
	// the storage caps are independent of day retention — they always run.
	// Split caps own their own pools; a legacy max_storage_mb still bounds
	// the whole dir on configs that carry it.
	enforceFrameCap(db, cfg)
	enforceDBCap(db, cfg)
	if cfg.MaxStorageMB > 0 {
		enforceStorageCap(db, cfg)
	}
	// drop empty day directories
	days, _ := filepath.Glob(filepath.Join(framesDir(), "*"))
	for _, d := range days {
		if entries, _ := os.ReadDir(d); len(entries) == 0 {
			os.Remove(d)
		}
	}
}

// dirSize returns the total byte size of regular files under root.
func dirSize(root string) int64 {
	var total int64
	filepath.Walk(root, func(_ string, fi os.FileInfo, _ error) error {
		if fi != nil && fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// frameBytes is the media pool: frames/ + quarantine/.
func frameBytes() int64 { return dirSize(framesDir()) + dirSize(quarantineDir()) }

// dbBytes is the text-data pool: the journal database files.
func dbBytes() int64 {
	var total int64
	for _, f := range []string{dbPath(), dbPath() + "-wal", dbPath() + "-shm"} {
		if fi, err := os.Stat(f); err == nil {
			total += fi.Size()
		}
	}
	return total
}

// purgeQuarantine deletes quarantined orphans oldest-ish first until
// *total <= limit. Phase 0 of every frame-cap pass: already superseded,
// cheapest to drop.
func purgeQuarantine(db *sql.DB, total *int64, limit int64) {
	qpurged := 0
	for _, p := range quarantineFiles() {
		if *total <= limit {
			break
		}
		if fi, err := os.Stat(p); err == nil && os.Remove(p) == nil {
			*total -= fi.Size()
			qpurged++
		}
	}
	if qpurged > 0 {
		logEvent(db, "storage_cap_quarantine", fmt.Sprintf("%d files", qpurged))
	}
}

// reclaimFrames deletes frames oldest-first until *total <= limit. Frames
// under terminal blocks (done/failed/dead — never re-summarized) go first;
// if that is not enough, the oldest frames go regardless — a screenshot is
// cheaper to lose than a journal block (and during a provider outage
// pending-only protection would let frames starve the journal).
func reclaimFrames(db *sql.DB, cfg Config, total *int64, limit int64) {
	blocks, berr := terminalBlockStarts(db)
	if berr != nil {
		logEvent(db, "storage_cap_error", "block index: "+berr.Error())
	}
	rows, err := db.Query(`SELECT ts, path FROM frames ORDER BY ts ASC`)
	if err != nil {
		return
	}
	var reclaim, rest []frameRow
	for rows.Next() {
		var fr frameRow
		if err := rows.Scan(&fr.ts, &fr.path); err != nil {
			continue
		}
		if blocks[blockStart(time.Unix(fr.ts, 0), cfg.BlockMinutes).Unix()] {
			reclaim = append(reclaim, fr)
		} else {
			rest = append(rest, fr)
		}
	}
	rows.Close()
	freed, removed := deleteFramesUntil(db, append(reclaim, rest...), total, limit)
	if removed > 0 {
		logEvent(db, "storage_cap_frames", fmt.Sprintf("%d frames, %s", removed, humanBytes(freed)))
	}
}

// trimLogTables caps the log tables at their newest 2000 rows (rowid order =
// insertion order; immune to ts ties that would make a ts-based cutoff
// delete nothing), then checkpoints and vacuums to reclaim pages. Returns
// false when the vacuum fails — another process holds the DB and deleting
// journal blocks would destroy data without reclaiming pages.
func trimLogTables(db *sql.DB) bool {
	db.Exec(`DELETE FROM events WHERE rowid <= (
		SELECT rowid FROM events ORDER BY rowid DESC LIMIT 1 OFFSET 2000)`)
	db.Exec(`DELETE FROM api_calls WHERE rowid <= (
		SELECT rowid FROM api_calls ORDER BY rowid DESC LIMIT 1 OFFSET 2000)`)
	db.Exec(`DELETE FROM llm_calls WHERE rowid <= (
		SELECT rowid FROM llm_calls ORDER BY rowid DESC LIMIT 1 OFFSET 2000)`)
	checkpointWAL(db)
	if _, err := db.Exec(`VACUUM`); err != nil {
		logEvent(db, "storage_cap_error", "vacuum: "+err.Error())
		return false
	}
	// VACUUM itself can leave rebuilt pages in the WAL — checkpoint again so
	// callers measuring dbBytes() see the settled size, not a transient one.
	checkpointWAL(db)
	return true
}

// checkpointWAL runs a TRUNCATE checkpoint and logs failures.
func checkpointWAL(db *sql.DB) {
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		logEvent(db, "storage_cap_error", "checkpoint: "+err.Error())
	}
}

// pruneOldestBlocks drops the oldest journal blocks — the last resort of
// every db-cap pass — one bounded chunk per call so an oversized database
// converges across passes without a single huge delete.
func pruneOldestBlocks(db *sql.DB) {
	pruned, err := db.Exec(`DELETE FROM blocks WHERE start_ts IN (
		SELECT start_ts FROM blocks ORDER BY start_ts ASC LIMIT 100)`)
	if err != nil {
		logEvent(db, "storage_cap_error", "block prune: "+err.Error())
		return
	}
	pb, _ := pruned.RowsAffected()
	checkpointWAL(db)
	if _, err := db.Exec(`VACUUM`); err != nil {
		logEvent(db, "storage_cap_error", "vacuum: "+err.Error())
	}
	checkpointWAL(db)
	logEvent(db, "storage_cap_db", fmt.Sprintf("%d oldest blocks pruned, vacuumed", pb))
}

// enforceFrameCap keeps frames + quarantine under max_frames_mb.
func enforceFrameCap(db *sql.DB, cfg Config) {
	if cfg.MaxFramesMB <= 0 {
		return
	}
	limit := int64(cfg.MaxFramesMB) << 20
	total := frameBytes()
	if total <= limit {
		return
	}
	purgeQuarantine(db, &total, limit)
	if total > limit {
		reclaimFrames(db, cfg, &total, limit)
	}
}

// enforceDBCap keeps the journal database (blocks, events, calls — the text
// data) under max_db_mb: trim logs, then oldest blocks as a last resort.
func enforceDBCap(db *sql.DB, cfg Config) {
	if cfg.MaxDBMB <= 0 {
		return
	}
	limit := int64(cfg.MaxDBMB) << 20
	if dbBytes() <= limit {
		return
	}
	if !trimLogTables(db) {
		return
	}
	if dbBytes() <= limit {
		return
	}
	pruneOldestBlocks(db)
	if total := dbBytes(); total > limit {
		logEvent(db, "storage_cap_floor",
			fmt.Sprintf("journal db still %dMB over cap after pruning; will retry next pass",
				(total-limit)>>20))
	}
}

// enforceStorageCap is the legacy combined cap: when a config still carries
// max_storage_mb it bounds the whole data dir (quarantine + frames + db +
// wal) under one number, same eviction order. New installs leave it off and
// use the split frame/db caps instead.
func enforceStorageCap(db *sql.DB, cfg Config) {
	if cfg.MaxStorageMB <= 0 {
		return
	}
	limit := int64(cfg.MaxStorageMB) << 20
	total := dataDirSize()
	if total <= limit {
		return
	}
	purgeQuarantine(db, &total, limit)
	if total <= limit {
		return
	}
	reclaimFrames(db, cfg, &total, limit)
	if total <= limit {
		return
	}
	if !trimLogTables(db) {
		return
	}
	if total = dataDirSize(); total <= limit {
		return
	}
	pruneOldestBlocks(db)
	if total = dataDirSize(); total > limit {
		logEvent(db, "storage_cap_floor",
			fmt.Sprintf("data dir still %dMB over cap after pruning; will retry next pass",
				(total-limit)>>20))
	}
}

type frameRow struct {
	ts   int64
	path string
}

// terminalBlockStarts returns the start_ts set of blocks in a terminal state.
func terminalBlockStarts(db *sql.DB) (map[int64]bool, error) {
	rows, err := db.Query(`SELECT start_ts FROM blocks WHERE status IN ('done','failed','dead')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]bool{}
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			return m, err
		}
		m[ts] = true
	}
	return m, rows.Err()
}

// deleteFramesUntil removes frames oldest-first until *total <= limit,
// batching the row deletes in a single transaction.
func deleteFramesUntil(db *sql.DB, list []frameRow, total *int64, limit int64) (freed int64, removed int) {
	tx, err := db.Begin()
	if err != nil {
		return 0, 0
	}
	for _, fr := range list {
		if *total-freed <= limit {
			break
		}
		fi, serr := os.Stat(fr.path)
		rerr := error(nil)
		if serr == nil {
			rerr = os.Remove(fr.path)
		}
		// Delete the row only when the file is actually gone — otherwise the
		// row is dropped but the file stays, an orphan no later pass can find.
		if (serr == nil && rerr == nil) || os.IsNotExist(serr) || os.IsNotExist(rerr) {
			if serr == nil && rerr == nil {
				freed += fi.Size()
			}
			if _, err := tx.Exec(`DELETE FROM frames WHERE ts=? AND path=?`, fr.ts, fr.path); err == nil {
				removed++
			}
		}
	}
	tx.Commit()
	*total -= freed
	return freed, removed
}

func dataDirSize() int64 { return dirSize(dataDir()) }

// backfillFrameBytes populates frames.bytes once for databases created before
// the column existed. A meta flag bounds it to a single walk ever.
func backfillFrameBytes(db *sql.DB) {
	var v string
	if err := db.QueryRow(`SELECT v FROM meta WHERE k='frames_bytes_v1'`).Scan(&v); err != nil || v == "1" {
		return
	}
	tx, err := db.Begin()
	if err != nil {
		return
	}
	filepath.Walk(framesDir(), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi != nil && fi.Mode().IsRegular() {
			tx.Exec(`UPDATE frames SET bytes=? WHERE path=? AND bytes=0`, fi.Size(), p)
		}
		return nil
	})
	tx.Exec(`INSERT INTO meta(k,v) VALUES('frames_bytes_v1','1')
		ON CONFLICT(k) DO UPDATE SET v='1'`)
	tx.Commit()
}

// frameStats returns total bytes and file count across all recorded frames.
// Prefers the frames.bytes column; falls back to a dir walk when the column
// is unavailable (read-only opens of pre-column databases).
func frameStats(db *sql.DB) (int64, int) {
	has, err := hasColumn(db, "frames", "bytes")
	if err == nil && has {
		var b int64
		var n int
		if db.QueryRow(`SELECT COALESCE(SUM(bytes),0), COUNT(1) FROM frames`).Scan(&b, &n) == nil {
			return b, n
		}
	}
	return dirStats(framesDir())
}

// storageBytesFast approximates dataDirSize without walking the frames tree:
// SUM(frames.bytes) + the db files + a walk of everything outside frames/.
// `status` runs on the bar's 60s poll, so the O(files) walk belongs to the
// hourly retention pass, not here. Falls back to the exact walk when the
// bytes column is unavailable (read-only opens of pre-column databases).
func storageBytesFast(db *sql.DB) int64 {
	has, err := hasColumn(db, "frames", "bytes")
	if err != nil || !has {
		return dataDirSize()
	}
	backfillFrameBytes(db)
	var total int64
	db.QueryRow(`SELECT COALESCE(SUM(bytes),0) FROM frames`).Scan(&total)
	for _, p := range []string{dbPath(), dbPath() + "-wal", dbPath() + "-shm"} {
		if fi, err := os.Stat(p); err == nil {
			total += fi.Size()
		}
	}
	framesRoot := framesDir()
	filepath.Walk(dataDir(), func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil {
			return nil
		}
		if fi.IsDir() {
			if p == framesRoot {
				return filepath.SkipDir // already counted via the frames table
			}
			return nil
		}
		if fi.Mode().IsRegular() {
			total += fi.Size()
		}
		return nil
	})
	return total
}

func runDaemon(cfg Config) error {
	db, err := openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if cfg.AgentCompletions {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); watchCompletions(ctx, db) }()
		defer func() { cancel(); <-done }()
	}

	cmdArgs, err := resolveCaptureCommand(cfg)
	if err != nil {
		return err
	}
	logEvent(db, "daemon_start", strings.Join(cmdArgs, " "))
	cfgMtime := configMtime()

	// SIGTERM/SIGINT (systemctl stop, Ctrl-C) is an intentional stop, not a
	// wedge — write a terminal event so the stall detector reads the
	// silence as known-quiet instead of a dead daemon. Registered before
	// the first capture so a stop during it still lands in the channel.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)

	// hot-reload config when the file changes so `dayflow config set` and
	// `ignore` apply without a restart
	reloadIfChanged := func() {
		m := configMtime()
		if m.Equal(cfgMtime) {
			return
		}
		if nc, err := loadConfig(); err == nil {
			cfg = nc
			cfgMtime = m
			// A reload can mean a compositor change — drop the output=auto
			// negative cache so the next tick re-probes hyprctl.
			focusFails, focusDisabled, focusDisabledAt = 0, false, time.Time{}
			if na, err := resolveCaptureCommand(cfg); err == nil {
				cmdArgs = na
			}
			logEvent(db, "config_reloaded", "")
			debugf(cfg, "config reloaded: provider=%s model=%s interval=%ds block=%dm debug=%v",
				cfg.Provider, cfg.Model, cfg.CaptureIntervalSec, cfg.BlockMinutes, cfg.Debug)
		}
	}

	var lastHash *frameHash // nil = no prior sample; force first capture
	locked := false
	noSession := false          // wayland socket absent — capture skipped, logged once
	grabFails := 0              // consecutive capture errors with a session present
	var grabFailSince time.Time // first failure of the current streak
	tick := time.NewTicker(time.Duration(cfg.CaptureIntervalSec) * time.Second)
	defer tick.Stop()
	retentionTick := time.NewTicker(time.Hour)
	defer retentionTick.Stop()
	lockTick := time.NewTicker(time.Minute)
	defer lockTick.Stop()
	notifyTick := time.NewTicker(time.Minute)
	defer notifyTick.Stop()
	log.Printf("dayflow daemon: capturing every %ds -> %s", cfg.CaptureIntervalSec, framesDir())
	debugf(cfg, "daemon start: provider=%s model=%s endpoint=%s interval=%ds block=%dm quality=%d retention=%dd keep_frames=%v debug=%v",
		cfg.Provider, cfg.Model, chatURL(cfg), cfg.CaptureIntervalSec, cfg.BlockMinutes,
		cfg.JPEGQuality, cfg.RetentionDays, cfg.KeepFrames, cfg.Debug)

	capture := func() {
		reloadIfChanged()
		// Per-tick liveness for the stall detector — one meta row, immune
		// to the events table's 2000-row cap. Written before any early
		// return so paused/no-session ticks still prove the loop is alive.
		metaSet(db, metaCaptureHeartbeat, strconv.FormatInt(time.Now().Unix(), 10))
		// Built-in grim exits 1 instantly when no wayland session exists
		// (greeter, compositor down/restarting) — pause quietly and log the
		// transition, not an error per tick. Custom capture_command stays
		// ungated: its failure modes are its own.
		if cfg.CaptureCommand == "" && !waylandReachable() {
			if !noSession {
				noSession = true
				lastHash = nil
				logEvent(db, "capture_paused", "no wayland session")
				debugf(cfg, "capture: paused — wayland socket absent")
				notifyAsync(db, cfg, notifyClassPaused, "dayflow", "capture paused")
			}
			return
		}
		if noSession {
			noSession = false
			logEvent(db, "capture_resumed", "wayland session")
			debugf(cfg, "capture: resumed — wayland session present")
			notifyAsync(db, cfg, notifyClassRecovered, "dayflow", "capture resumed")
		}
		h, attempted, err := captureOnce(db, cfg, cmdArgs, lastHash)
		if err != nil {
			if grabFails == 0 {
				grabFailSince = time.Now()
			}
			grabFails++
			if captureFailVisible(grabFails) {
				logEvent(db, "capture_error", err.Error())
				debugf(cfg, "capture: %v (streak %d)", err, grabFails)
				log.Printf("capture: %v (streak %d)", err, grabFails)
			}
			// A streak spanning ~5 minutes of wall-clock failures (with a
			// session present) is a stall, not a transient — alert off-tick;
			// the quiet period + daily cap bound repeats. The streak floor
			// plus monotonic elapsed keep a lone first-tick failure at a
			// large interval from alerting. The summarize oneshot covers a
			// fully wedged loop that can't reach this line.
			if stallAlertable(grabFails, grabFailSince) {
				notifyAsync(db, cfg, notifyClassStall, "dayflow", "capture stalled")
			}
			return
		}
		if !attempted {
			return // paused or ignored — not a recovery, keep any streak
		}
		if grabFails > 0 {
			logEvent(db, "capture_recovered", fmt.Sprintf("after %d failures", grabFails))
			debugf(cfg, "capture: recovered after %d failures", grabFails)
			if stallAlertable(grabFails, grabFailSince) {
				notifyAsync(db, cfg, notifyClassRecovered, "dayflow", "capture resumed")
			}
			grabFails = 0
			grabFailSince = time.Time{}
		}
		lastHash = h
	}
	capture()
	runRetention(db, cfg)
	for {
		select {
		case sig := <-sigCh:
			// Intentional stop — record a terminal event so the stall
			// detector treats the silence as known-quiet.
			logEvent(db, "daemon_stop", sig.String())
			debugf(cfg, "daemon: stopping on %s", sig)
			return nil
		case <-tick.C:
			if cfg.AutoPauseLocked && locked {
				// locked: do not capture, reset hash so we don't leak last frame.
				// Keep the liveness heartbeat fresh too: the loop is alive and
				// on purpose quiet, and the stall oneshot's captureQuietNow can
				// miss a locked screen (loginctl failure, missing XDG_SESSION_ID
				// in the oneshot env) — without a heartbeat it would then report
				// a false "capture stalled" after 30 minutes of locked screen.
				metaSet(db, metaCaptureHeartbeat, strconv.FormatInt(time.Now().Unix(), 10))
				lastHash = nil
				continue
			}
			capture()
		case <-retentionTick.C:
			runRetention(db, cfg)
			// Ingest off-loop: a decode pass can take minutes, and a
			// synchronous call here would drop capture ticks (missed
			// frames + false capture-stall heartbeats). ingestMu shared
			// with the lazy search path so passes never stack.
			if ingestMu.TryLock() {
				go func() {
					defer ingestMu.Unlock()
					n, early, err := ingestAgentChats(db, agentIngestBudget)
					switch {
					case err != nil:
						debugf(cfg, "agent chat ingest failed: %v", err)
					case early:
						debugf(cfg, "agent chat ingest paused at budget (%d turns so far); resumes next pass", n)
					case n > 0:
						debugf(cfg, "agent chat ingest: %d turns", n)
					}
				}()
			}
			debugf(cfg, "retention run complete; data dir %s", humanBytes(dataDirSize()))
		case <-lockTick.C:
			if !cfg.AutoPauseLocked {
				continue
			}
			isLocked := screenLocked() // one loginctl spawn per tick, not two
			if isLocked && !paused() && !locked {
				locked = true
				lastHash = nil
				logEvent(db, "auto_paused", "screen locked")
				debugf(cfg, "auto-paused: screen locked")
				notifyAsync(db, cfg, notifyClassPaused, "dayflow", "capture paused")
			} else if !isLocked && locked {
				locked = false
				logEvent(db, "auto_resumed", "screen unlocked")
				debugf(cfg, "auto-resumed: screen unlocked")
				notifyAsync(db, cfg, notifyClassRecovered, "dayflow", "capture resumed")
			}
		case <-notifyTick.C:
			// Once-a-day operator nudges (standup ready, goal pending).
			// The checks are cheap indexed queries; emission is async and
			// meta-capped, so the tick never blocks on dbus.
			checkNudges(db, cfg)
		}
	}
}
