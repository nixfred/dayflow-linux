import QtQuick
import qs.Commons

Item {
  id: root
  property var dayflow
  property bool floating: false
  signal closeRequested()
  readonly property var pages: ["today", "standup", "chat", "week", "agents", "context", "timelapse", "settings"]
  readonly property bool ready: dayflow && !dayflow.engineMissing && dayflow.configured
  readonly property string health: !dayflow ? "CONNECTING" : dayflow.engineMissing ? "ENGINE MISSING" : !dayflow.configured ? "SETUP REQUIRED" : dayflow.errorText !== "" ? "NEEDS ATTENTION" : dayflow.paused ? "CAPTURE PAUSED" : dayflow.captureState === "recording" ? "RECORDING" : "CAPTURE " + dayflow.captureState.toUpperCase()
  readonly property color healthTint: ready && !dayflow.paused && dayflow.captureState === "recording" && dayflow.errorText === "" ? Color.accent : Color.urgent
  readonly property real trackedMinutes: dayflow ? (dayflow.spans || []).reduce(function(sum, span) { return sum + Number(span.minutes || 0) }, 0) : 0
  readonly property real focusMinutes: dayflow ? (dayflow.spans || []).reduce(function(sum, span) { return sum + (span.productive ? Number(span.minutes || 0) : 0) }, 0) : 0
  DashboardController { id: auxiliary; dayflow: root.dayflow }
  Rectangle { anchors.fill: parent; radius: 14; color: Color.popups.background }

  Column {
    id: layout
    anchors.fill: parent
    anchors.margins: 12
    spacing: 10
    Item {
      width: parent.width; height: 46
      Column {
        spacing: 2
        Text { text: "DAYFLOW"; color: root.dayflow.foreground; font.family: root.dayflow.fontFamily; font.pixelSize: 22; font.bold: true; font.letterSpacing: 3 }
        Text { text: "Your work, as it happens · v" + root.dayflow.pluginVersion; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption) }
      }
      Row {
        anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter; spacing: 8
        Rectangle {
          width: healthLabel.implicitWidth + 36; height: 32; radius: 16
          color: Qt.alpha(root.healthTint, 0.12); border.color: Qt.alpha(root.healthTint, 0.55)
          Row {
            anchors.centerIn: parent; spacing: 8
            Rectangle {
              width: 6; height: 6; radius: 3; color: root.healthTint; anchors.verticalCenter: parent.verticalCenter
              SequentialAnimation on opacity { running: root.visible && root.ready && !root.dayflow.paused; loops: Animation.Infinite; NumberAnimation { to: 0.35; duration: 1100 } NumberAnimation { to: 1; duration: 1100 } }
            }
            Text { id: healthLabel; text: root.health; color: root.dayflow.foreground; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption); font.bold: true }
          }
        }
        CompactButton { dayflow: root.dayflow; text: root.dayflow.paused ? "Resume" : "Pause"; enabled: root.ready; onClicked: root.dayflow.toggleCapture() }
        CompactButton { dayflow: root.dayflow; text: "Refresh"; onClicked: root.dayflow.refreshAll() }
        CompactButton { dayflow: root.dayflow; text: root.floating ? "Close" : "Full window"; onClicked: { if (root.floating) root.closeRequested(); else { root.dayflow.fullViewOpen = true; root.dayflow.close() } } }
      }
    }
    Row {
      width: parent.width; spacing: 6; height: 32
      Repeater {
        model: root.pages
        CompactButton {
          required property string modelData
          dayflow: root.dayflow; text: modelData === "today" ? "Overview" : modelData.charAt(0).toUpperCase() + modelData.slice(1)
          active: root.dayflow.currentTab === modelData; height: 32
          onClicked: root.dayflow.currentTab = modelData
        }
      }
    }
    Row {
      id: metrics
      visible: root.ready && root.dayflow.currentTab !== "settings"
      width: parent.width; spacing: 10; height: visible ? 72 : 0
      Repeater {
        model: [
          {label:root.dayflow.dayOffset === 0 ? "Tracked today" : "Tracked selected day", value:root.dayflow.fmtDur(root.trackedMinutes), fraction:0},
          {label:"Focus", value:root.dayflow.fmtDur(root.focusMinutes), fraction:root.focusMinutes/Math.max(1,root.trackedMinutes)},
          {label:"Agent replies", value:String(root.dayflow.completions.length), fraction:0},
          {label:"Frames today", value:String(root.dayflow.framesToday), fraction:0},
          {label:"Awaiting summary", value:String(root.dayflow.blocksPending), fraction:0}
        ]
        MetricCard { required property var modelData; width: (metrics.width - 4 * metrics.spacing) / 5; height: metrics.height; dayflow: root.dayflow; label: modelData.label; value: modelData.value; fraction: modelData.fraction }
      }
    }
    Loader {
      id: page
      objectName: "dashboardPage"
      width: parent.width
      height: layout.height - y - footer.height - layout.spacing
      property var panel: root.dayflow
      property var dashboardHost: auxiliary
      source: !root.dayflow ? "" : root.dayflow.engineMissing ? "InstallPrompt.qml" : !root.dayflow.configured && !root.dayflow.onboardingSkipped ? "Onboarding.qml" : root.dayflow.currentTab === "context" ? "ContextPane.qml" : root.dayflow.currentTab === "timelapse" ? "TimelapsePane.qml" : root.dayflow.currentTab === "settings" ? "Settings.qml" : root.dayflow.currentTab.charAt(0).toUpperCase() + root.dayflow.currentTab.slice(1) + "Tab.qml"
      onLoaded: {
        if (item.host !== undefined) item.host = auxiliary
        if (item.dismissed) item.dismissed.connect(function() { root.dayflow.onboardingSkipped = true })
      }
    }
    Item {
      id: footer
      width: parent.width; height: 32
      Row {
        anchors.left: parent.left; spacing: 8
        CompactButton { dayflow: root.dayflow; text: root.dayflow.summarizing ? "Summarizing…" : "Summarize now"; enabled: root.ready && root.dayflow.blocksPending > 0 && !root.dayflow.summarizing; onClicked: root.dayflow.summarizeNow() }
        CompactButton { dayflow: root.dayflow; text: "Ignore current app"; enabled: root.ready && root.dayflow.activeApp !== ""; onClicked: root.dayflow.ignoreCurrentApp() }
      }
      CompactButton { anchors.right: parent.right; dayflow: root.dayflow; text: root.dayflow.errorText !== "" || root.dayflow.notice !== "" ? "Status / result •" : "Status / details"; active: root.dayflow.errorText !== ""; onClicked: statusDrawer.visible = !statusDrawer.visible }
    }
  }
  DashboardCard {
    id: statusDrawer
    objectName: "statusDrawer"
    visible: false; z: 10
    anchors.fill: parent; anchors.margins: 12
    dayflow: root.dayflow; title: "Live status and action results"
    color: Color.popups.background
    Row {
      spacing: 8
      CompactButton { dayflow: root.dayflow; text: "Back"; onClicked: statusDrawer.visible = false }
      CompactButton { dayflow: root.dayflow; text: "Update engine"; visible: root.dayflow.engineVersion !== "" && root.dayflow.engineVersion !== root.dayflow.pluginVersion && !root.dayflow.versionNewer(root.dayflow.engineVersion,root.dayflow.pluginVersion); enabled: !root.dayflow.engineInstalling; onClicked: root.dayflow.requestEngineInstall() }
    }
    PagedText {
      width: parent.width; dayflow: root.dayflow; bodyHeight: statusDrawer.height - 124
      text: [root.health, "Provider: " + root.dayflow.modelName, "Storage: " + (root.dayflow.storageText || "—"), "Current app: " + (root.dayflow.activeApp || "—"), "Ignoring: " + root.dayflow.ignoredApps.join(", "), "Engine v" + root.dayflow.engineVersion + " · Panel v" + root.dayflow.pluginVersion, root.dayflow.errorText, root.dayflow.notice, root.dayflow.installErr].filter(function(value) { return !!value }).join("\n\n")
    }
  }
}
