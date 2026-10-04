import QtQuick
import qs.Commons

Item {
  id: pane
  property var host
  readonly property var dayflow: host ? host.dayflow : null
  readonly property var briefing: host ? host.agentBriefing : null
  readonly property var workstreams: briefing ? briefing.workstreams || [] : []
  width: parent ? parent.width : 0
  height: parent ? parent.height : 360
  implicitHeight: 360
  // Status/highlight palettes — adapted to the panel theme rather than
  // upstream's fixed light palette.
  readonly property var statusMeta: ({
    "inProgress":  { label: "in progress",  color: Qt.rgba(0.40, 0.70, 1.00, 1.0) },
    "reviewReady": { label: "review ready", color: Qt.rgba(0.30, 0.80, 0.55, 1.0) },
    "blocked":     { label: "blocked",      color: Qt.rgba(0.90, 0.35, 0.30, 1.0) },
    "completed":   { label: "completed",    color: "gray" }
  })
  readonly property var highlightMeta: ({
    "keyDecision":    { label: "decision",   color: Qt.rgba(0.62, 0.45, 0.95, 1.0) },
    "keyInfo":        { label: "key info",   color: Qt.rgba(0.35, 0.60, 0.95, 1.0) },
    "readyForReview": { label: "for review", color: Qt.rgba(0.95, 0.55, 0.20, 1.0) }
  })
  function statusColor(status) {
    var m = statusMeta[status]
    if (!m || status === "completed") return pane.dayflow ? pane.dayflow.dim : "gray"
    return m.color
  }
  function statusLabel(status) {
    return statusMeta[status] ? statusMeta[status].label : "completed"
  }
  function highlightColor(kind) {
    return highlightMeta[kind] ? highlightMeta[kind].color : "transparent"
  }
  // Re-alpha a theme/status color for fills and borders.
  function tint(c, a) { return Qt.rgba(c.r, c.g, c.b, a) }

  // Per-status totals across all workstreams — bound expression evaluates
  // once per briefing load, not per legend delegate.
  readonly property var statusTotals: statusTotalsOf(workstreams)
  function statusTotalsOf(ws) {
    var t = { blocked: 0, reviewReady: 0, inProgress: 0, completed: 0 }
    for (var i = 0; i < ws.length; i++) {
      var ths = ws[i].threads || []
      for (var j = 0; j < ths.length; j++) {
        var s = ths[j].status
        if (t[s] !== undefined) t[s]++
      }
    }
    return t
  }

  // Per-source aggregates: sessions, active span, turn count — the "stats
  // by AI conversation" strip. Derived from the same payload, no extra
  // process.
  // Clamp to the briefing day — sessions persist across days, so a raw
  // started_at→ended_at span overcounts massively.
  readonly property var sourceStats: sourceStatsOf(workstreams)
  function sourceStatsOf(ws) {
    var dayStart = 0, dayEnd = 0
    if (pane.briefing && pane.briefing.day) {
      var d = new Date(pane.briefing.day + "T00:00:00")
      dayStart = d.getTime() / 1000
      dayEnd = dayStart + 86400
    }
    var bySrc = {}
    var order = []
    for (var i = 0; i < ws.length; i++) {
      var ths = ws[i].threads || []
      for (var j = 0; j < ths.length; j++) {
        var th = ths[j]
        var src = th.source || "unknown"
        if (bySrc[src] === undefined) { bySrc[src] = { source: src, sessions: 0, minutes: 0, turns: 0 }; order.push(src) }
        var agg = bySrc[src]
        agg.sessions++
        // Turns carry the session's condensed narrative, not just today —
        // only timestamps inside the briefing day count toward span/turns.
        var trn = th.turns || []
        var lo = 0, hi = 0
        var dayTurns = 0
        for (var t = 0; t < trn.length; t++) {
          var ts = trn[t].ts || 0
          if (ts <= 0 || (dayStart > 0 && (ts < dayStart || ts >= dayEnd))) continue
          dayTurns++
          if (lo === 0 || ts < lo) lo = ts
          if (ts > hi) hi = ts
        }
        if (lo === 0) {
          lo = dayStart > 0 ? Math.max(th.started_at, dayStart) : th.started_at
          hi = dayEnd > 0 ? Math.min(th.ended_at, dayEnd) : th.ended_at
        }
        if (hi > lo) agg.minutes += Math.round((hi - lo) / 60)
        agg.turns += dayTurns
      }
    }
    var out = []
    for (var k = 0; k < order.length; k++) out.push(bySrc[order[k]])
    out.sort(function(a, b) { return b.minutes - a.minutes })
    return out
  }
  function fmtMinutes(mins) {
    if (mins < 60) return mins + "m"
    var h = Math.floor(mins / 60)
    var m = mins % 60
    return m > 0 ? h + "h" + (m < 10 ? "0" : "") + m + "m" : h + "h"
  }


  Row {
    id: tools; spacing: 8; height: 28
    DayNavRow { dayflow: pane.dayflow }
    CompactButton { dayflow: pane.dayflow; text: (host && host.agentsLoading) ? "Loading…" : "Refresh briefing"; enabled: !(host && host.agentsLoading); onClicked: host.agentsLoad(true) }
    CompactButton { dayflow: pane.dayflow; text: "Sources / status"; onClicked: pane.showSources = !pane.showSources }
  }
  Row {
    y: 38; width: parent.width; height: parent.height-y; spacing: 12
    RecordCard {
      id: streams; width: (parent.width-parent.spacing)*0.37; height: parent.height; dayflow: pane.dayflow; title: "Workstreams"
      records: pane.workstreams
      emptyText: (host && host.agentsLoading) ? "Rebuilding the day’s briefing…" : (host ? host.agentsError : "") || "No agent sessions for this day."
      formatRecord: function(record) {return [record.name,record.summary,(record.bullets || []).map(function(b){return "• "+b}).join("\n"),(record.threads || []).length + " threads"].filter(function(v){return !!v}).join("\n\n")}
    }
    Column {
      width: (parent.width-parent.spacing)*0.63; height: parent.height; spacing: 10
      RecordCard {
        id: threads; width: parent.width; height: parent.height*0.50; dayflow: pane.dayflow; title: "Threads"
        records: streams.current.threads || []
        tint: pane.statusColor(current.status)
        formatRecord: function(record) {return [record.source+" · "+pane.statusLabel(record.status), record.title,record.latest_outcome,Qt.formatTime(new Date(record.started_at*1000),"hh:mm")+"–"+Qt.formatTime(new Date(record.ended_at*1000),"hh:mm")].filter(function(v){return !!v}).join("\n\n")}
      }
      DashboardCard {
        id: narrative; width: parent.width; height: parent.height-threads.height-parent.spacing; dayflow: pane.dayflow; title: "Thread narrative"
        Row {
          spacing: 6
          CompactButton { dayflow: pane.dayflow; text: "‹"; enabled: pane.turnIndex>0; onClicked: pane.turnIndex-- }
          Text { anchors.verticalCenter: parent.verticalCenter; text: pane.turns.length ? (pane.turnIndex+1)+" / "+pane.turns.length : "0 turns"; color: pane.dayflow.dim; font.family: pane.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption) }
          CompactButton { dayflow: pane.dayflow; text: "›"; enabled: pane.turnIndex+1<pane.turns.length; onClicked: pane.turnIndex++ }
          CompactButton { dayflow: pane.dayflow; text: "Artifact folder"; enabled: String(pane.turn.artifact_path || "")[0] === "/"; onClicked: { var path=pane.turn.artifact_path; Qt.openUrlExternally("file://"+encodeURI(path.substring(0,path.lastIndexOf("/")) || "/")) } }
        }
        PagedText { width: parent.width; bodyHeight: Math.max(28,narrative.height-118); dayflow: pane.dayflow; text: [pane.turn.role,pane.turn.highlight,pane.turn.text,pane.turn.artifact_name,pane.turn.artifact_path].filter(function(v){return !!v}).join("\n\n") || "No narrative turns for this thread." }
      }
    }
  }
  property bool showSources: false
  property int turnIndex: 0
  readonly property var turns: threads.current.turns || []
  readonly property var turn: pane.turns.length ? pane.turns[Math.min(turnIndex,pane.turns.length-1)] : ({})
  onTurnsChanged: turnIndex = Math.max(0,Math.min(turnIndex,turns.length-1))
  DashboardCard {
    visible: pane.showSources; anchors.fill: parent; dayflow: pane.dayflow; title: "Agent sources and status"; color: Color.popups.background
    CompactButton { dayflow: pane.dayflow; text: "Back"; onClicked: pane.showSources=false }
    PagedText {
      width: parent.width; dayflow: pane.dayflow; bodyHeight: pane.height-118
      text: [(host ? host.agentsError : ""),(host && host.agentRecapsEnabled) ? "Model recaps are enabled." : "Recaps are off. Transcripts stay local; the briefing is deterministic.",Object.keys(pane.statusTotals).map(function(key){return pane.statusTotals[key]+" "+pane.statusLabel(key)}).join(" · "),pane.sourceStats.map(function(s){return s.source+" · "+s.sessions+" sessions · "+pane.fmtMinutes(s.minutes)+" · "+s.turns+" turns"}).join("\n"),(host ? host.agentSources || [] : []).map(function(s){return s.source+": "+s.status+(s.note ? " · "+s.note : "")+(s.drift ? " · store may have drifted" : "")}).join("\n")].filter(function(v){return !!v}).join("\n\n")
    }
  }
}
