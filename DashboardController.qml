import QtQuick
import Quickshell.Io

// Shared auxiliary state for popup and floating dashboard. No duplicate briefing.
Item {
  id: root
  property var dayflow
  readonly property string section: dayflow ? dayflow.currentTab : "today"
  // ---- forecast ----
  property var forecast: null
  property bool forecastLoading: false
  property string forecastError: ""

  Process {
    id: forecastProc
    command: ["dayflow", "forecast", "--json"]
    property bool didStart: false
    onStarted: { didStart=true; root.forecastLoading=true; root.forecastError="" }
    onExited: function(code) { root.forecastLoading=false; if(code!==0) root.forecastError="Forecast unavailable. Refresh to retry." }
    onRunningChanged: if(!running) {root.forecastLoading=false; if(!didStart) root.forecastError="Could not start the forecast command."; didStart=false}
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try { root.forecast = JSON.parse(text) } catch (e) { root.forecast = null; root.forecastError="Could not read the forecast result." }
      }
    }
  }

  // ---- agents pane state ----
  // Briefing machinery lives in AgentBriefingLoader (shared with the
  // compact panel's Agents tab); these forwards keep the AgentsPane host
  // contract intact.
  readonly property var agentBriefing: dayflow ? dayflow.agentBriefing : null
  readonly property var agentSources: dayflow ? dayflow.agentSources : []
  readonly property bool agentRecapsEnabled: dayflow ? dayflow.agentRecapsEnabled : false
  readonly property bool agentsLoading: dayflow ? dayflow.agentsLoading : false
  readonly property string agentsError: dayflow ? dayflow.agentsError : ""
  function agentsLoad(refresh) { if (dayflow) dayflow.agentsLoad(refresh) }

  // ---- timelapse state ----
  property var tlFrames: []
  property int tlIndex: 0
  property bool tlPlaying: false
  property bool tlLoading: false
  property string tlError: ""

  property bool tlPending: false

  function tlLoad() {
    if (!root.dayflow) return
    root.dayflow.uilog("timelapse load " + root.dayflow.viewDateStr())
    framesProc.command = ["dayflow", "frames", root.dayflow.viewDateStr(), "--json"]
    root.tlLoading = true
    root.tlPlaying = false
    // command changes on a running Process only affect the next start —
    // queue a rerun so the requested day isn't dropped mid-flight.
    if (framesProc.running) {
      root.tlPending = true
    } else {
      framesProc.running = true
    }
  }

  function tlSeek(i) {
    if (root.tlFrames.length === 0) return
    root.tlIndex = Math.max(0, Math.min(root.tlFrames.length - 1, i))
  }

  Process {
    id: framesProc
    command: ["dayflow", "frames", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        root.tlLoading = false
        try {
          var d = JSON.parse(text)
          var list = (d.frames || []).filter(function(f) { return f.exists })
          root.tlFrames = list
          root.tlIndex = 0
          root.tlError = ""
        } catch (e) {
          root.tlFrames = []
          root.tlError = "could not load frames"
        }
      }
    }
    onExited: function(exitCode) {
      root.tlLoading = false
      if (exitCode !== 0) {
        root.tlFrames = []
        root.tlError = "frames listing failed"
      }
      if (root.tlPending) {
        root.tlPending = false
        root.tlLoad()
      }
    }
    onRunningChanged: {
      if (!framesProc.running) {
        root.tlLoading = false
        if (root.tlPending) {
          root.tlPending = false
          root.tlLoad()
        }
      }
    }
  }

  Process {
    id: playbackProc
    property string pendingArg: "status"
    command: ["dayflow", "playback", pendingArg]
    onExited: function(exitCode) {
      if (root.dayflow) {
        root.dayflow.notice = exitCode === 0
          ? "playback " + playbackProc.pendingArg
          : "playback toggle failed"
      }
      if (exitCode === 0) {
        playbackStatusProc.running = true
        root.tlLoad()
      }
    }
  }

  // Called by TimelapsePane's enable button.
  function enablePlayback() {
    if (root.dayflow) root.dayflow.uilog("playback enable")
    playbackProc.pendingArg = "on"
    playbackProc.running = true
  }

  Timer {
    interval: 250
    repeat: true
    running: root.tlPlaying && root.section === "timelapse" && root.tlFrames.length > 1
    onTriggered: {
      if (root.tlIndex >= root.tlFrames.length - 1) {
        root.tlPlaying = false
      } else {
        root.tlIndex++
      }
    }
  }

  // playback on/off state for the pane gate
  property bool tlPlaybackOn: false

  Process {
    id: playbackStatusProc
    command: ["dayflow", "playback", "status", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          var d = JSON.parse(text)
          root.tlPlaybackOn = d.enabled === true
        } catch (e) {}
      }
    }
  }


  onSectionChanged: {
    if (section === "timelapse") { playbackStatusProc.running = true; tlLoad() }
    if (section === "week" && !forecastProc.running) forecastProc.running = true
  }
  Connections {
    target: root.dayflow
    function onDayOffsetChanged() {
      root.tlFrames = []; root.tlIndex = 0; root.tlPlaying = false
      if (root.section === "timelapse") root.tlLoad()
    }
  }
}
