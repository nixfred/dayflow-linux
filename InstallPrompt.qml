import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Ui

// Engine-missing surface. Shown by Panel.qml instead of the tabs (and ahead
// of Onboarding) when `dayflow status --json` failed to spawn — the binary
// isn't on PATH. One click runs scripts/install.sh (resolved relative to
// this file, so it works from an installed plugin dir or a dev checkout);
// output tails surface here on failure along with the terminal fallback.
Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null

  width: parent ? parent.width : 0
  implicitHeight: 360
  height: parent ? parent.height : implicitHeight

  DashboardCard {
    anchors.fill:parent; dayflow:root.dayflow; title:"Install Dayflow engine"
    CompactButton {dayflow:root.dayflow; text:root.dayflow.engineInstalling ? "Installing…" : "Install engine"; active:true; enabled:!root.dayflow.engineInstalling; onClicked:root.dayflow.requestEngineInstall()}
    PagedText {width:parent.width; dayflow:root.dayflow; bodyHeight:root.height-122; text:["The widget is installed, but the engine is missing. Install downloads the verified release, places it in ~/.local/bin, and starts capture units. Choose a provider during setup afterwards.",root.dayflow.installLog,root.dayflow.installErr,"Terminal fallback: bash "+root.dayflow.installScriptPath].filter(function(value){return !!value}).join("\n\n")}
  }
}
