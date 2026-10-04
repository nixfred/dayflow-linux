import QtQuick
import Quickshell
import Quickshell.Io
import qs.Ui

BarWidget {
  id: root
  moduleName: "io.github.duketopceo.dayflow"

  property bool paused: false
  property int blocksDone: 0
  property int framesToday: 0
  property bool available: false

  readonly property bool opened: panelLoader.item
    ? panelLoader.item.opened === true
    : false
  readonly property bool popoutSwitchClosing: panelLoader.item
    ? panelLoader.item.popoutSwitchClosing === true
    : false

  function open() {
    if (panelLoader.item) panelLoader.item.open()
  }

  function close() {
    if (panelLoader.item) panelLoader.item.close()
  }

  function toggle() {
    if (panelLoader.item) panelLoader.item.toggle()
  }

  function closeForPopoutSwitch() {
    if (panelLoader.item) panelLoader.item.closeForPopoutSwitch()
  }

  function injectPanel() {
    if (!panelLoader.item) return
    panelLoader.item.bar = root.bar
    panelLoader.item.anchorItem = button
    panelLoader.item.hostWidget = root
  }

  function refresh() {
    if (!statusProc.running) statusProc.running = true
  }

  function applyStatus(raw) {
    try {
      var s = JSON.parse(raw)
      root.paused = s.paused === true
      root.blocksDone = Number(s.blocks_done || 0)
      root.framesToday = Number(s.frames_today || 0)
      root.available = true
    } catch (e) {
      root.available = false
    }
  }

  // Always render — a missing engine is exactly when the panel's install
  // surface needs to be reachable. Unavailable state shows a dimmed icon.
  visible: true
  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onBarChanged: injectPanel()

  Process {
    id: statusProc
    property bool didStart: false
    command: ["dayflow", "status", "--json"]
    onStarted: statusProc.didStart = true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyStatus(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) root.available = false
    }
    // FailedToStart (binary absent) emits neither exited nor
    // streamFinished — the widget must still render so the panel's
    // install surface is reachable.
    onRunningChanged: {
      if (!statusProc.running && !statusProc.didStart) root.available = false
      if (!statusProc.running) statusProc.didStart = false
    }
  }

  Timer {
    interval: 60000
    running: true
    repeat: true
    triggeredOnStart: true
    onTriggered: root.refresh()
  }

  Loader {
    id: panelLoader
    active: true
    source: Qt.resolvedUrl("Panel.qml")
    visible: false
    onLoaded: {
      root.injectPanel()
      Qt.callLater(root.injectPanel)
    }
  }

  // The floating full view lives here, not inside the popup PanelWindow —
  // a window created inside the popup is transient-parented to it and gets
  // unmapped the moment the popup auto-dismisses.
  Loader {
    id: fullViewLoader
    active: panelLoader.item && panelLoader.item.fullViewOpen === true
    source: Qt.resolvedUrl("FullView.qml")
    onLoaded: item.dayflow = panelLoader.item
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: root.available ? (root.paused ? "ᛯ " : "󰚯 ") + root.blocksDone : "󰚯"
    opacity: root.available ? 1 : 0.45
    tooltipText: !root.available
      ? "Dayflow engine not installed — click to set it up"
      : (root.paused
        ? "Dayflow · capture paused\n" + root.blocksDone + " summaries · " + root.framesToday + " frames today\nClick: dashboard · Right-click: resume"
        : "Dayflow · " + root.blocksDone + " summaries · " + root.framesToday + " frames today\nClick: dashboard · Right-click: pause")
    onPressed: function(buttonCode) {
      if (buttonCode === Qt.LeftButton) {
        root.toggle()
      } else if (buttonCode === Qt.RightButton) {
        // No engine → nothing to toggle; the click-through stays left-only.
        if (root.available) toggleProc.running = true
      }
    }
  }

  Process {
    id: toggleProc
    command: ["dayflow", "toggle"]
    onExited: Qt.callLater(root.refresh)
  }

  Connections {
    target: panelLoader.item
    ignoreUnknownSignals: true
    function onStatusChanged() { root.refresh() }
  }
}
