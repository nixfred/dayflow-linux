import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

Panel {
  id: dayflow
  moduleName: "io.github.duketopceo.dayflow"
  manageIpc: false

  signal statusChanged()

  property var anchorItem: null
  property var hostWidget: null
  property var blocks: []
  property var completions: []
  property string dateLabel: ""
  property bool paused: false
  property string captureState: "unknown"
  property bool configured: true
  property bool onboardingSkipped: false
  property string errorText: ""
  // Skew flags tracked per loader — a successful daily load must not clear a
  // skew the week loader raised, and vice versa.
  property bool daySkew: false
  property bool weekSkew: false
  property string modelName: ""
  property string activeApp: ""
  property var ignoredApps: []
  property int framesToday: 0
  property int blocksPending: 0
  property string storageText: ""
  property string notice: ""
  property string noticeTone: "neutral"
  readonly property bool noticeIsError: noticeTone === "error"
  property string currentTab: "today"
  property var config: ({})
  property var configDraft: ({})
  property bool configLoaded: false
  property var standup: ({ yesterday: { date: "", total_minutes: 0, entries: [] }, today: { date: "", total_minutes: 0, entries: [] } })
  property var dayGoal: ({ date: "", goal: "", completed: false })
  property var draft: ({ date: "", highlights: "", tasks: "", blockers: "", priorities: "" })
  property bool draftDirty: false
  property var workflow: ({ date: "", slot_minutes: 15, total_minutes: 0, slots: [], categories: [] })
  property var insights: ({ total_minutes: 0, focus_minutes: 0, distraction_minutes: 0, idle_minutes: 0, categories: [], apps: [], top_distractions: [], focus_blocks: [], days: 0 })
  property var weekBlocks: []
  property var weekCards: []
  property string weekStart: ""
  property string weekEnd: ""
  property string weekSummary: ""
  property bool weekSummaryLoading: false
  property bool timelineLoading: false
  property bool timelineQueued: false
  property var weeklyPayload: ({ start: "", end: "", total_minutes: 0, focus_minutes: 0, distraction_minutes: 0, idle_minutes: 0, category_donut: [], app_treemap: [], context_shifts: [], context_shift_count: 0, top_distractions: [], focus_blocks: [], highlights: [], suggestions: [], heatmap: [] })
  property var spans: []
  property int dayOffset: 0
  property bool expanded: false
  property bool expandedLoaded: false
  property bool fullViewOpen: false
  // Bump with manifest.json version — compared against the engine's
  // reported version to warn when the plugin and binary drift apart.
  readonly property string pluginVersion: "1.5.0"
  property string engineVersion: ""
  // Engine install state: statusProc's FailedToStart path sets
  // engineMissing (binary absent → InstallPrompt surface instead of
  // tabs); a successful status reply clears it. installLog carries the
  // last output line of scripts/install.sh for that surface. The script
  // path resolves relative to this file, so it works from the installed
  // plugin dir and a dev checkout alike.
  property bool engineMissing: false
  property string installLog: ""
  property string installErr: ""
  // Skew banner dismiss is keyed on the version pair — a different
  // mismatch later re-shows the banner instead of staying silenced.
  property string skewDismissedFor: ""
  readonly property string installScriptPath:
      decodeURIComponent(String(Qt.resolvedUrl("scripts/install.sh")).replace(/^file:\/\//, ""))
  readonly property bool engineInstalling: engineInstallProc.running

  // Agents tab briefing state — AgentBriefingLoader is shared with
  // FullView's rail section; the aliases give AgentsTab the same
  // host.* surface the pane already uses there.
  AgentBriefingLoader {
    id: agentLoader
    panel: dayflow
  }
  readonly property alias agentBriefing: agentLoader.briefing
  readonly property alias agentSources: agentLoader.sources
  readonly property alias agentRecapsEnabled: agentLoader.recapsEnabled
  readonly property alias agentsLoading: agentLoader.loading
  readonly property alias agentsError: agentLoader.error
  function agentsLoad(refresh) { agentLoader.load(refresh) }

  function lastLine(t) {
    var lines = String(t).split("\n").filter(function(l) { return l.trim() !== "" })
    return lines.length ? lines[lines.length - 1].trim() : ""
  }

  // Strict semver compare (string compare lies past .9).
  function versionNewer(a, b) {
    var pa = String(a).split("."), pb = String(b).split(".")
    for (var i = 0; i < 3; i++) {
      var na = parseInt(pa[i] || "0", 10), nb = parseInt(pb[i] || "0", 10)
      if (na !== nb) return na > nb
    }
    return false
  }

  readonly property color foreground: Color.popups.text
  readonly property color dim: Qt.alpha(dayflow.foreground, 0.68)
  readonly property string fontFamily: dayflow.bar ? dayflow.bar.fontFamily : Style.font.family

  onNoticeChanged: {
    if (notice === "") noticeTone = "neutral"
    else if (/failed|error|could not|denied|unavailable|not found/i.test(notice)) noticeTone = "error"
    else noticeTone = "success"
  }

  function open() {
    dayflow.controller.show()
    refreshAll()
  }

  function close() {
    dayflow.controller.hide()
  }

  function switchPanel(direction) {
    if (dayflow.bar && typeof dayflow.bar.switchPanelFrom === "function")
      return dayflow.bar.switchPanelFrom(dayflow.hostWidget || dayflow, direction)
    return false
  }

  // refreshForTab fetches only what the given tab renders — the panel no
  // longer fires standup/insights/weekly/config on every open.
  function refreshForTab(tab) {
    if (tab === "standup") {
      if (!standupFetchProc.running) standupFetchProc.running = true
      if (!goalProc.running) goalProc.running = true
    } else if (tab === "week" || tab === "context") {
      if (!insightsFetchProc.running) insightsFetchProc.running = true
      if (!weekTimelineProc.running) weekTimelineProc.running = true
      if (!weeklyProc.running) weeklyProc.running = true
    } else if (tab === "settings") {
      if (!configProc.running) configProc.running = true
    } else if (tab === "agents") {
      // Lazy + staleness-aware: load once, reload when the viewed day
      // drifted since the briefing was fetched.
      if (dayflow.agentBriefing === null ||
          (dayflow.agentBriefing.day || "") !== dayflow.viewDateStr())
        dayflow.agentsLoad(false)
    }
    // "chat" loads its own processes when the Loader instantiates ChatTab
  }

  function refreshAll() {
    // Don't probe mid-install: the binary may be staged mid-swap, and a
    // FailedToStart would wedge the "engine answers" check the success
    // path relies on.
    if (!statusProc.running && !dayflow.engineInstalling) statusProc.running = true
    if (dayflow.engineMissing) return
    dayflow.loadTimeline()
    refreshForTab(dayflow.currentTab)
  }

  Timer {
    interval: dayflow.currentTab === "today" ? 2000 : 5000
    repeat: true
    running: (dayflow.opened || dayflow.fullViewOpen) && !dayflow.engineMissing
    onTriggered: if (!timelineProc.running) dayflow.loadTimeline()
  }

  Timer {
    interval: 5000; repeat: true
    running: (dayflow.opened || dayflow.fullViewOpen) && !dayflow.engineMissing
    onTriggered: if (!statusProc.running && !dayflow.engineInstalling) statusProc.running = true
  }

  onCurrentTabChanged: refreshForTab(currentTab)

  function cloneConfig(obj) {
    return JSON.parse(JSON.stringify(obj || {}))
  }

  function applyConfig(raw) {
    try {
      var data = JSON.parse(raw)
      var cfg = data.config || data
      dayflow.config = cfg
      dayflow.configDraft = dayflow.cloneConfig(cfg)
      settingsCatModel.clear()
      var cats = cfg.categories || []
      for (var i = 0; i < cats.length; i++) {
        var c = cats[i]
        settingsCatModel.append({ name: c.name || "", description: c.description || "", color: c.color || "" })
      }
      dayflow.configLoaded = true
      dayflow.notice = ""
    } catch (e) {
      dayflow.notice = "config load failed"
    }
  }

  function saveConfig() {
    var patch = dayflow.cloneConfig(dayflow.configDraft)
    if (patch.openrouter_api_key === "***redacted***") delete patch.openrouter_api_key
    // Advanced provider fields are saved independently. Round-tripping this
    // stale snapshot would undo prompt overrides written since loadConfig.
    delete patch.providers
    delete patch.routing
    patch.categories = []
    for (var i = 0; i < settingsCatModel.count; i++) {
      var item = settingsCatModel.get(i)
      if (item.name) patch.categories.push({ name: item.name, description: item.description, color: item.color || "" })
    }
    patchProc.pendingPatch = JSON.stringify(patch)
    patchProc.command = ["dayflow", "config", "patch", "-"]
    patchProc.running = true
  }

  function loadConfig() {
    if (!configProc.running) configProc.running = true
  }

  function viewDate() {
    var d = new Date()
    d.setDate(d.getDate() + dayflow.dayOffset)
    return d
  }

  function viewDateStr() {
    var d = dayflow.viewDate()
    var m = ("0" + (d.getMonth() + 1)).slice(-2)
    var dd = ("0" + d.getDate()).slice(-2)
    return d.getFullYear() + "-" + m + "-" + dd
  }

  function viewDateLabel() {
    if (dayflow.dayOffset === 0) return "Today"
    if (dayflow.dayOffset === -1) return "Yesterday"
    var names = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
    var d = dayflow.viewDate()
    return names[d.getDay()] + " " + dayflow.viewDateStr().substring(5)
  }

  function goDay(delta) {
    var next = dayflow.dayOffset + delta
    if (next > 0) return
    dayflow.dayOffset = next
    dayflow.blocks = []
    dayflow.spans = []
    dayflow.loadTimeline()
  }

  // Fire-and-forget UI action logging -> debug.log. A single shared Process;
  // overlapping actions may drop a line, which is fine for debug telemetry.
  Process { id: uiLogProc }
  function uilog(msg) {
    uiLogProc.command = ["dayflow", "log", msg]
    if (!uiLogProc.running) uiLogProc.running = true
  }

  function procByName(n) {
    return ({
      copyProc: copyProc, copyWeekProc: copyWeekProc,
      copyMiniProc: copyMiniProc, copyWeekMiniProc: copyWeekMiniProc,
      standupFetchProc: standupFetchProc, goalProc: goalProc, goalSetProc: goalSetProc,
      standupProc: standupProc, draftSaveProc: draftSaveProc,
      weeklyProc: weeklyProc, weekTimelineProc: weekTimelineProc,
      insightsFetchProc: insightsFetchProc, reviewProc: reviewProc
    })[n]
  }

  function loadTimeline() {
    dayflow.uilog("timeline load " + dayflow.viewDateStr())
    timelineProc.command = ["dayflow", "timeline", "--json", dayflow.viewDateStr()]
    dayflow.timelineLoading = true
    // command changes on a running Process only affect the next start —
    // queue a rerun so the requested day isn't dropped mid-flight.
    if (timelineProc.running) {
      dayflow.timelineQueued = true
    } else {
      timelineProc.running = true
    }
    dayflow.loadWorkflow()
  }

  // cardToSpan aliases an engine-emitted card (mergeCards in Go — the single
  // merge implementation) onto the view contract the delegates were written
  // against: count, appName, and span-level start_ts/end_ts. This is a field
  // map, not a merge — merging happens once, engine-side.
  function cardToSpan(c) {
    c.count = Number(c.blocks || 0)
    c.appName = c.app_name || dayflow.appDisplayName(c.app)
    var kids = c.children || []
    if (kids.length > 0) {
      c.start_ts = kids[0].start_ts
      c.end_ts = kids[kids.length - 1].end_ts
    }
    return c
  }

  // ---- block editing ----
  // Pending `dayflow edit` argv arrays, run one at a time so several field
  // changes on the same card don't race each other.
  property var editQueue: []

  // Queue one edit per changed field, then drain via editProc.
  function saveBlockEdits(startTs, title, category, productive, orig) {
    var q = []
    if (title !== (orig.title || ""))
      q.push(["dayflow", "edit", String(startTs), "title", title])
    if (category !== (orig.category || ""))
      q.push(["dayflow", "edit", String(startTs), "category", category])
    if (productive !== (orig.productive === true))
      q.push(["dayflow", "edit", String(startTs), "productive", productive ? "true" : "false"])
    if (q.length === 0) {
      dayflow.notice = "no changes"
      return
    }
    dayflow.editQueue = q
    dayflow.runNextEdit()
  }

  function runNextEdit() {
    if (dayflow.editQueue.length === 0) {
      dayflow.notice = "edits saved"
      dayflow.loadTimeline()
      return
    }
    var next = dayflow.editQueue[0]
    dayflow.editQueue = dayflow.editQueue.slice(1)
    editProc.command = next
    editProc.running = true
  }

  function fmtDur(mins) {
    mins = Math.round(Number(mins) || 0)
    if (mins < 60) return mins + "m"
    var h = Math.floor(mins / 60)
    var m = mins % 60
    return m ? h + "h " + m + "m" : h + "h"
  }

  function appIcon(cls) {
    if (!cls || cls === "") return ""
    var tail = String(cls).split(".").pop()
    var names = [cls, String(cls).toLowerCase(), tail, tail.toLowerCase()]
    for (var i = 0; i < names.length; i++) {
      try {
        var p = Quickshell.iconPath(names[i], true)
        if (p && String(p).length > 0) return p
      } catch (e) {}
    }
    return ""
  }

  // Message shown when the dayflow binary on PATH predates engine-side card
  // emission — a missing "cards" key must surface as version skew, not an
  // empty day. engineVersion comes from status --json and may still be ""
  // on the first load, so it is optional.
  function engineSkewNotice() {
    return "engine upgrade required — the dayflow binary on PATH does not emit timeline cards" +
      (dayflow.engineVersion !== "" ? " (engine " + dayflow.engineVersion + ")" : "")
  }

  // errorText reflects whichever skew flags are set; a non-skew error raised
  // by a loader is preserved until that loader succeeds or skew appears.
  function syncSkewError() {
    if (dayflow.daySkew || dayflow.weekSkew) {
      dayflow.errorText = dayflow.engineSkewNotice()
    } else if (dayflow.errorText.indexOf("engine upgrade required") === 0) {
      dayflow.errorText = ""
    }
  }

  function applyTimeline(raw) {
    dayflow.timelineLoading = false
    try {
      var d = JSON.parse(raw)
      dayflow.blocks = d.blocks || []
      dayflow.completions = d.completions || []
      // Engine-merged cards (mergeCards in Go), newest first to match the
      // timeline's previous display order. null distinguishes a missing
      // key (older binary) from a genuinely empty array.
      var cards = ("cards" in d) ? (d.cards || []) : null
      if (cards === null) {
        dayflow.spans = []
        dayflow.dateLabel = d.date || ""
        // Old binary + empty day: indistinguishable from a real empty day,
        // so the flag only sets when blocks exist.
        dayflow.daySkew = dayflow.blocks.length > 0
        dayflow.syncSkewError()
        return
      }
      dayflow.daySkew = false
      var spans = []
      for (var i = cards.length - 1; i >= 0; i--) {
        spans.push(dayflow.cardToSpan(cards[i]))
      }
      dayflow.spans = spans
      dayflow.dateLabel = d.date || ""
      dayflow.syncSkewError()
    } catch (e) {
      dayflow.blocks = []
      dayflow.completions = []
      dayflow.spans = []
      dayflow.errorText = "could not read timeline"
    }
  }

  function applyStatus(raw) {
    try {
      var s = JSON.parse(raw)
      dayflow.paused = s.paused === true
      dayflow.captureState = s.capture_state || "unknown"
      dayflow.configured = s.configured !== false
      dayflow.modelName = s.model || ""
      dayflow.activeApp = s.active_app || ""
      dayflow.ignoredApps = s.ignored_apps || []
      dayflow.framesToday = Number(s.frames_today || 0)
      dayflow.blocksPending = Number(s.blocks_pending || 0)
      dayflow.storageText = s.storage_text || ""
      dayflow.engineVersion = s.version || ""
      // Apply the persisted expand preference once; later polls must not
      // fight an in-flight `config set` from the Expand click.
      if (!dayflow.expandedLoaded) {
        dayflow.expanded = s.panel_expanded === true
        dayflow.expandedLoaded = true
      }
    } catch (e) {}
  }

  function applyGoal(raw) {
    try {
      dayflow.dayGoal = JSON.parse(raw)
    } catch (e) {}
  }

  function applyStandup(raw) {
    try {
      var d = JSON.parse(raw)
      dayflow.standup = d
      if (d.draft && !dayflow.draftDirty) {
        dayflow.draft = d.draft
      }
    } catch (e) {}
  }

  function applyWorkflow(raw) {
    try {
      var d = JSON.parse(raw)
      dayflow.workflow = d
    } catch (e) {
      dayflow.workflow = ({ date: "", slot_minutes: 15, total_minutes: 0, slots: [], categories: [] })
    }
  }

  function todayStr() {
    var d = new Date()
    var m = ("0" + (d.getMonth() + 1)).slice(-2)
    var dd = ("0" + d.getDate()).slice(-2)
    return d.getFullYear() + "-" + m + "-" + dd
  }

  function saveDraft() {
    var d = dayflow.draft || {}
    draftSaveProc.command = ["dayflow", "standup", "save",
      "--date", d.date || dayflow.todayStr(),
      "--highlights", String(d.highlights || ""),
      "--tasks", String(d.tasks || ""),
      "--blockers", String(d.blockers || ""),
      "--priorities", String(d.priorities || "")]
    if (!draftSaveProc.running) draftSaveProc.running = true
  }

  function loadWorkflow() {
    gridProc.command = ["dayflow", "day", dayflow.viewDateStr(), "--grid", "--json"]
    if (!gridProc.running) gridProc.running = true
  }

  function applyInsights(raw) {
    try {
      var d = JSON.parse(raw)
      dayflow.insights = d
    } catch (e) {}
  }

  function applyWeekTimeline(raw) {
    try {
      var d = JSON.parse(raw)
      dayflow.weekBlocks = d.blocks || []
      // null = the binary predates the "cards" key — a version-skew state,
      // not an empty week. weekDaySpans treats null like empty, but the
      // skew is surfaced via errorText while blocks exist.
      dayflow.weekCards = ("cards" in d) ? (d.cards || []) : null
      dayflow.weekSkew = dayflow.weekCards === null && dayflow.weekBlocks.length > 0
      dayflow.syncSkewError()
      dayflow.weekStart = d.start || ""
      dayflow.weekEnd = d.end || ""
    } catch (e) {
      dayflow.weekBlocks = []
      dayflow.weekCards = []
    }
  }

  function applyWeekReview(raw) {
    dayflow.weekSummaryLoading = false
    try {
      var d = JSON.parse(raw)
      dayflow.weekSummary = d.review || ""
    } catch (e) {
      dayflow.weekSummary = "Could not load weekly review."
    }
  }

  function applyWeeklyPayload(raw) {
    try {
      var d = JSON.parse(raw)
      dayflow.weeklyPayload = d
    } catch (e) {
      dayflow.weeklyPayload = ({ start: "", end: "", total_minutes: 0, focus_minutes: 0, distraction_minutes: 0, idle_minutes: 0, category_donut: [], app_treemap: [], context_shifts: [], context_shift_count: 0, top_distractions: [], focus_blocks: [], highlights: [], suggestions: [], heatmap: [] })
    }
  }

  function weekStartDate() {
    if (dayflow.weekStart === "") return new Date()
    return new Date(dayflow.weekStart + "T00:00:00")
  }

  function categoryForHour(dayIndex, hour) {
    if (!dayflow.weekBlocks.length) return ""
    var base = dayflow.weekStartDate().getTime() + dayIndex * 86400000 + hour * 3600000
    for (var i = 0; i < dayflow.weekBlocks.length; i++) {
      var b = dayflow.weekBlocks[i]
      var s = Number(b.start_ts || 0) * 1000
      var e = Number(b.end_ts || 0) * 1000
      if (s < base + 3600000 && e > base) {
        return b.category
      }
    }
    return ""
  }

  function dayName(index) {
    var names = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
    return names[index] || ""
  }

  // Index of today within the Mon..Sun week (Mon=0).
  function todayIndex() {
    return (new Date().getDay() + 6) % 7
  }

  // Day order for the week timeline: today first so it needs no scroll,
  // then the rest of the week in order.
  function weekDayOrder() {
    var t = dayflow.todayIndex()
    var order = [t]
    for (var i = 0; i < 7; i++) if (i !== t) order.push(i)
    return order
  }

  function weekDayBlocks(dayIndex) {
    if (!dayflow.weekBlocks.length) return []
    var dayStart = dayflow.weekStartDate().getTime() + dayIndex * 86400000
    var dayEnd = dayStart + 86400000
    var out = []
    for (var i = 0; i < dayflow.weekBlocks.length; i++) {
      var s = Number(dayflow.weekBlocks[i].start_ts || 0) * 1000
      if (s >= dayStart && s < dayEnd) out.push(dayflow.weekBlocks[i])
    }
    return out
  }

  // Cards for one week day, newest first — filtered from the week payload's
  // engine-emitted cards array. A card emits one span per day its children
  // touch, built from only that day's children, so a card spanning midnight
  // renders a continuation span in the next day's column instead of being
  // owned wholly by its start day.
  function weekDaySpans(dayIndex) {
    if (!dayflow.weekCards || !dayflow.weekCards.length) return []
    var dayStart = dayflow.weekStartDate().getTime() + dayIndex * 86400000
    var dayEnd = dayStart + 86400000
    var spans = []
    for (var i = 0; i < dayflow.weekCards.length; i++) {
      var c = dayflow.weekCards[i]
      var kids = c.children || []
      var dayKids = []
      for (var k = 0; k < kids.length; k++) {
        var s = Number(kids[k].start_ts || 0) * 1000
        if (s >= dayStart && s < dayEnd) dayKids.push(kids[k])
      }
      if (dayKids.length === 0) continue
      // cardToSpan derives start_ts/end_ts from children and mutates the
      // object it is given — copy the card so each day gets its own span
      // instead of overwriting the shared payload entry. start/end strings
      // and the child count reflect this day's segment of the card only.
      var span = {}
      for (var key in c) span[key] = c[key]
      span.children = dayKids
      span.blocks = dayKids.length
      span.start = dayKids[0].start
      span.end = dayKids[dayKids.length - 1].end
      spans.push(dayflow.cardToSpan(span))
    }
    spans.reverse()
    return spans
  }

  function weekDayMinutes(dayIndex) {
    var list = dayflow.weekDayBlocks(dayIndex)
    var total = 0
    for (var i = 0; i < list.length; i++)
      total += (Number(list[i].end_ts) - Number(list[i].start_ts)) / 60
    return Math.round(total)
  }

  function catDisplay(cat) {
    if (!cat || cat === "") return ""
    return cat.charAt(0).toUpperCase() + cat.slice(1)
  }

  function modelShort() {
    var m = dayflow.modelName
    return m.indexOf("/") >= 0 ? m.split("/").pop() : m
  }

  // Resolve a window class or app name to a proper display name.
  // Prefers the desktop entry's real Name, then a cleaned-up tail.
  function appDisplayName(cls) {
    if (!cls || cls === "") return ""
    try {
      var entries = DesktopEntries.applications.values || []
      var lc = String(cls).toLowerCase()
      var tail = lc.split(".").pop()
      for (var i = 0; i < entries.length; i++) {
        var e = entries[i]
        var eid = String(e.id || "").toLowerCase()
        if (eid === lc || eid === tail ||
            eid === lc + ".desktop" || eid === tail + ".desktop") {
          return e.name
        }
      }
    } catch (e2) {}
    return dayflow.titleize(cls)
  }

  function titleize(cls) {
    var s = String(cls)
    var i = s.indexOf("__")
    if (i >= 0) s = s.substring(0, i)
    var tail = s
    var j = tail.lastIndexOf(".")
    if (j >= 0) tail = tail.substring(j + 1)
    var lt = tail.toLowerCase()
    if (tail === "" || lt === "com" || lt === "org" || lt === "net" ||
        lt === "default" || tail.length <= 2) {
      var k = s.indexOf(".")
      tail = k > 0 ? s.substring(0, k) : s
      lt = tail.toLowerCase()
    }
    var junk = ["-default", "-browser", "-bin", ".bin"]
    for (var m = 0; m < junk.length; m++) {
      var suf = junk[m]
      if (lt.length >= suf.length &&
          lt.substring(lt.length - suf.length) === suf) {
        tail = tail.substring(0, tail.length - suf.length)
        lt = tail.toLowerCase()
      }
    }
    tail = tail.replace(/^[_\-. ]+|[_\-. ]+$/g, "")
    if (tail === "") tail = s
    var words = tail.split(/[\s_\-]+/)
    for (var w = 0; w < words.length; w++) {
      if (words[w] !== "")
        words[w] = words[w].charAt(0).toUpperCase() + words[w].substring(1)
    }
    return words.join(" ")
  }

  function categoryColor(cat) {
    switch (cat) {
      case "coding":        return Qt.rgba(0.22, 0.55, 0.95, 1.0)
      case "communication": return Qt.rgba(0.95, 0.45, 0.15, 1.0)
      case "browsing":      return Qt.rgba(0.55, 0.35, 0.95, 1.0)
      case "writing":       return Qt.rgba(0.20, 0.75, 0.55, 1.0)
      case "meetings":      return Qt.rgba(0.95, 0.70, 0.15, 1.0)
      case "design":        return Qt.rgba(0.95, 0.25, 0.55, 1.0)
      case "media":         return Qt.rgba(0.95, 0.25, 0.25, 1.0)
      case "system":        return Qt.rgba(0.50, 0.50, 0.55, 1.0)
      case "idle":          return dayflow.dim
      case "personal":      return Color.urgent !== undefined ? Color.urgent : Qt.rgba(0.95, 0.25, 0.35, 1.0)
      case "failed":        return Color.urgent !== undefined ? Color.urgent : Qt.rgba(0.95, 0.25, 0.35, 1.0)
      case "other":         return Qt.darker(dayflow.foreground, 1.4)
      default:              return dayflow.foreground
    }
  }

  function cellColor(cat) {
    if (!cat || cat === "") return dayflow.fgFill(0.06)
    var c = dayflow.categoryColor(cat)
    return Qt.rgba(c.r, c.g, c.b, 0.85)
  }

  function pillBgColor(cat) {
    var c = dayflow.categoryColor(cat)
    return Qt.rgba(c.r, c.g, c.b, 0.15)
  }

  // payloadColor prefers the engine-emitted hex (which honors custom category
  // colors) and falls back to the hardcoded name palette.
  function payloadColor(hex, name) {
    if (hex && hex !== "") {
      var rgb = dayflow._rgb(hex)
      return Qt.rgba(rgb[0], rgb[1], rgb[2], 1.0)
    }
    return dayflow.categoryColor(name)
  }

  function payloadFill(hex, name, alpha) {
    var c = dayflow.payloadColor(hex, name)
    return Qt.rgba(c.r, c.g, c.b, alpha)
  }

  function _rgb(c) {
    if (typeof c === "string") {
      var h = c.charAt(0) === "#" ? c.substring(1) : c
      if (h.length === 8) h = h.substring(0, 6)
      if (h.length === 3) {
        h = h.charAt(0) + h.charAt(0) + h.charAt(1) + h.charAt(1) + h.charAt(2) + h.charAt(2)
      }
      if (h.length === 6) {
        return [parseInt(h.substring(0, 2), 16) / 255,
                parseInt(h.substring(2, 4), 16) / 255,
                parseInt(h.substring(4, 6), 16) / 255]
      }
      return [1, 1, 1]
    }
    if (c === undefined || c === null) return [1, 1, 1]
    return [c.r, c.g, c.b]
  }

  function accentFill(alpha) {
    var c = (Color.accent === undefined || Color.accent === null)
      ? dayflow.foreground : Color.accent
    var rgb = dayflow._rgb(c)
    return Qt.rgba(rgb[0], rgb[1], rgb[2], alpha)
  }

  function fgFill(alpha) {
    var rgb = dayflow._rgb(dayflow.foreground)
    return Qt.rgba(rgb[0], rgb[1], rgb[2], alpha)
  }

  function btnBg(hot) {
    return hot
      ? dayflow.accentFill(0.12)
      : "transparent"
  }

  function fmtHours(mins) {
    if (mins === undefined || isNaN(mins)) return "0.0"
    return (Number(mins) / 60).toFixed(1)
  }

  Process {
    id: timelineProc
    command: ["dayflow", "timeline", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyTimeline(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) {
        dayflow.timelineLoading = false
        dayflow.errorText = "dayflow CLI not found on PATH"
      }
      if (dayflow.timelineQueued) {
        dayflow.timelineQueued = false
        dayflow.timelineLoading = true
        timelineProc.running = true
      }
    }
    // FailedToStart emits neither exited nor streamFinished — without this
    // the loading flag sticks forever when the binary can't launch.
    onRunningChanged: {
      if (!timelineProc.running) {
        dayflow.timelineLoading = false
        if (dayflow.timelineQueued) {
          dayflow.timelineQueued = false
          dayflow.timelineLoading = true
          timelineProc.running = true
        }
      }
    }
  }

  Process {
    id: statusProc
    property bool didStart: false
    property bool gotStatus: false
    property string statusErr: ""
    command: ["dayflow", "status", "--json"]
    onStarted: statusProc.didStart = true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        if (text.trim() !== "") {
          statusProc.gotStatus = true
          dayflow.applyStatus(text)
          if (dayflow.engineMissing) {
            // The tab loaders were gated while the engine was missing —
            // now that it answers, fill them (not just statusProc data).
            dayflow.engineMissing = false
            dayflow.loadTimeline()
            refreshForTab(dayflow.currentTab)
          }
        }
      }
    }
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: statusProc.statusErr = dayflow.lastLine(text)
    }
    // FailedToStart emits neither exited nor streamFinished — a missing
    // binary is the install surface's trigger, not a stuck loader. A
    // binary that spawns then exits 126/127 is unrunnable (wrong arch,
    // bad interpreter) — same fix. Any other non-zero exit means the
    // binary is healthy enough to run but the engine itself failed
    // (corrupt DB, migration error): reinstalling can't fix that, so
    // surface it as a notice instead of looping through InstallPrompt.
    onExited: function(exitCode) {
      if (exitCode === 0 || statusProc.gotStatus || dayflow.engineInstalling) return
      if (exitCode === 126 || exitCode === 127) {
        dayflow.engineMissing = true
      } else {
        dayflow.notice = "engine error" + (statusProc.statusErr ? " — " + statusProc.statusErr : " (status exited " + exitCode + ")")
      }
    }
    onRunningChanged: {
      if (statusProc.running) { statusProc.gotStatus = false; statusProc.statusErr = "" }
      if (!statusProc.running && !statusProc.didStart && !dayflow.engineInstalling)
        dayflow.engineMissing = true
      if (!statusProc.running) statusProc.didStart = false
    }
  }

  // Shared engine-install runner — InstallPrompt and the skew banner's
  // Update button both funnel through requestEngineInstall(). install.sh
  // resolves its target version itself (manifest-pinned), so no args.
  Process {
    id: engineInstallProc
    property bool didStart: false
    command: ["bash", dayflow.installScriptPath]
    onStarted: engineInstallProc.didStart = true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.installLog = dayflow.lastLine(text) || dayflow.installLog
    }
    // stderr carries the fail() reason — kept in installErr so a trailing
    // stdout progress line can't clobber it when the collectors race.
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.installErr = dayflow.lastLine(text) || dayflow.installErr
    }
    onRunningChanged: {
      if (engineInstallProc.running) {
        dayflow.installLog = ""
        dayflow.installErr = ""
      }
      if (!engineInstallProc.running && !engineInstallProc.didStart) {
        dayflow.installErr = "could not launch installer — missing " + dayflow.installScriptPath
        dayflow.notice = "engine install failed"
      }
      if (!engineInstallProc.running) engineInstallProc.didStart = false
    }
    onExited: function(exitCode) {
      if (exitCode === 0) {
        // Success — but install.sh may have warned on stderr (e.g. a
        // PATH-shadowed `dayflow`). That warning is still actionable
        // after the surface closes, so promote it to the notice rather
        // than silently dropping it.
        if (dayflow.installErr !== "") {
          dayflow.notice = "engine installed — " + dayflow.installErr
          dayflow.installErr = ""
        } else {
          dayflow.notice = "engine installed"
        }
        // statusProc's reply clears engineMissing — the loader stays on
        // the install surface until the new binary actually answers.
        if (!statusProc.running) statusProc.running = true
      } else {
        if (dayflow.installErr === "")
          dayflow.installErr = "install failed — run " + dayflow.installScriptPath + " in a terminal"
        dayflow.notice = "engine install failed"
      }
    }
  }

  // The installer is a Process with no natural deadline — curl has
  // timeouts inside the script, but a wedged systemctl/go build can hang
  // it too. The panel is persistent, so an unbounded hang would leave
  // "Installing…" forever. 10 min covers a slow-link binary download.
  Timer {
    id: installWatchdog
    interval: 600000
    running: engineInstallProc.running
    onTriggered: {
      engineInstallProc.running = false
      dayflow.installErr = "install timed out — check the network and re-run " + dayflow.installScriptPath + " in a terminal"
      dayflow.notice = "engine install timed out"
    }
  }

  function requestEngineInstall() {
    if (!engineInstallProc.running) engineInstallProc.running = true
  }

  Process {
    id: goalProc
    command: ["dayflow", "goal", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyGoal(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "goal load failed"
    }
  }

  Process {
    id: goalSetProc
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyGoal(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "goal save failed"
    }
  }

  Process {
    id: standupFetchProc
    command: ["dayflow", "standup", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyStandup(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "standup load failed"
    }
  }

  Process {
    id: gridProc
    command: ["dayflow", "day", "--grid", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyWorkflow(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "grid load failed"
    }
  }

  Process {
    id: draftSaveProc
    command: ["dayflow", "standup", "save"]
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: { if (text.trim() !== "") dayflow.notice = text.trim() }
    }
    onExited: function(exitCode) {
      if (exitCode === 0) {
        dayflow.draftDirty = false
        dayflow.notice = "draft saved"
        if (!standupFetchProc.running) standupFetchProc.running = true
      } else {
        dayflow.notice = "draft save failed"
      }
    }
  }

  Process {
    id: insightsFetchProc
    command: ["dayflow", "insights", "week", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyInsights(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "insights load failed"
    }
  }

  Process {
    id: weekTimelineProc
    command: ["dayflow", "week", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyWeekTimeline(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "week timeline load failed"
    }
  }

  Process {
    id: reviewProc
    command: ["dayflow", "review", "week", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyWeekReview(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) {
        dayflow.weekSummaryLoading = false
        dayflow.notice = "weekly review failed"
      }
    }
    // FailedToStart watchdog — applyWeekReview/onExited never fire.
    onRunningChanged: {
      if (!reviewProc.running) dayflow.weekSummaryLoading = false
    }
  }

  Process {
    id: weeklyProc
    command: ["dayflow", "weekly", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyWeeklyPayload(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "weekly stats load failed"
    }
  }

  Process {
    id: toggleProc
    command: ["dayflow", "toggle"]
    onExited: {
      dayflow.statusChanged()
      Qt.callLater(dayflow.refreshAll)
    }
  }

  Process {
    id: ignoreProc
    command: ["dayflow", "ignore", "--active", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        var raw = text.trim()
        try {
          var resp = JSON.parse(raw)
          var cls = resp.ignored || resp.already_ignored || ""
          var name = dayflow.appDisplayName(cls) || cls
          if (resp.ignored !== undefined) {
            dayflow.notice = "Now ignoring " + name
          } else if (resp.already_ignored !== undefined) {
            dayflow.notice = "Already ignoring " + name
          } else {
            dayflow.notice = raw
          }
        } catch (e) {
          dayflow.notice = raw
        }
      }
    }
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: { if (text.trim() !== "") dayflow.notice = text.trim() }
    }
    onExited: {
      dayflow.statusChanged()
      Qt.callLater(dayflow.refreshAll)
    }
  }

  Process {
    id: summarizeProc
    command: ["dayflow", "summarize", "--now"]
    onExited: Qt.callLater(dayflow.refreshAll)
  }

  Process {
    id: copyProc
    command: ["bash", "-c", "dayflow export today | wl-copy"]
    onExited: function(exitCode) {
      dayflow.notice = exitCode === 0 ? "copied day (markdown)" : "copy failed"
    }
  }

  Process {
    id: copyWeekProc
    command: ["bash", "-c", "dayflow export week | wl-copy"]
    onExited: function(exitCode) {
      dayflow.notice = exitCode === 0 ? "copied week (markdown)" : "copy failed"
    }
  }

  Process {
    id: copyMiniProc
    command: ["bash", "-c", "dayflow export today --brief | wl-copy"]
    onExited: function(exitCode) {
      dayflow.notice = exitCode === 0 ? "copied day (mini)" : "copy failed"
    }
  }

  Process {
    id: copyWeekMiniProc
    command: ["bash", "-c", "dayflow export week --brief | wl-copy"]
    onExited: function(exitCode) {
      dayflow.notice = exitCode === 0 ? "copied week (mini)" : "copy failed"
    }
  }

  Process {
    id: editProc
    command: ["dayflow", "edit"]
    stderr: StdioCollector {
      waitForEnd: true
      onStreamFinished: { if (text.trim() !== "") dayflow.notice = text.trim() }
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) {
        dayflow.editQueue = []
        dayflow.notice = "edit failed"
        return
      }
      Qt.callLater(dayflow.runNextEdit)
    }
  }

  Process {
    id: standupProc
    command: ["bash", "-c", "dayflow standup | wl-copy"]
    onExited: function(exitCode) {
      dayflow.notice = exitCode === 0 ? "copied standup" : "standup copy failed"
    }
  }

  Process {
    id: persistExpandedProc
    command: ["dayflow", "config", "set", "panel_expanded", "false"]
    onExited: function(exitCode) {
      if (exitCode !== 0) dayflow.notice = "could not save panel size"
    }
  }

  Process {
    id: configProc
    property bool didStart: false
    command: ["dayflow", "config", "--json"]
    onStarted: configProc.didStart = true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: dayflow.applyConfig(text)
    }
    // FailedToStart emits neither exited nor streamFinished — without this,
    // Settings would sit on "Loading settings..." forever.
    onRunningChanged: {
      if (!configProc.running && !configProc.didStart) {
        dayflow.configLoaded = true
        dayflow.notice = "config load failed"
      }
      if (!configProc.running) configProc.didStart = false
    }
  }

  Process {
    id: patchProc
    property string pendingPatch: ""
    property bool didStart: false
    stdinEnabled: true
    onStarted: { write(pendingPatch + "\n"); pendingPatch = ""; patchProc.didStart = true }
    command: ["dayflow", "config", "patch", "-"]
    onExited: function(exitCode) {
      if (exitCode === 0) {
        dayflow.notice = "settings saved"
        configProc.running = true
      } else {
        dayflow.notice = "settings save failed"
      }
    }
    onRunningChanged: {
      if (!patchProc.running && !patchProc.didStart) dayflow.notice = "settings save failed"
      if (!patchProc.running) patchProc.didStart = false
    }
  }


  ListModel {
    id: settingsCatModel
  }

  // Dashboard actions keep engine processes owned by this persistent panel.
  function toggleCapture() { if (!toggleProc.running) toggleProc.running = true }
  function ignoreCurrentApp() { if (activeApp !== "" && !ignoreProc.running) ignoreProc.running = true }
  function summarizeNow() { if (blocksPending > 0 && !summarizeProc.running) summarizeProc.running = true }
  readonly property bool summarizing: summarizeProc.running

  KeyboardPanel {
    id: panel
    anchorItem: dayflow.anchorItem
    owner: dayflow.hostWidget || dayflow
    bar: dayflow.bar
    open: dayflow.opened
    property var state: dayflow
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(1440))
    contentHeight: panel.fittedContentHeight(Style.space(740))
    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onCloseRequested: dayflow.close()
      onTabRequested: function(direction) { dayflow.switchPanel(direction) }
      Dashboard { anchors.fill: parent; dayflow: panel.state; onCloseRequested: dayflow.close() }
    }
  }
}
