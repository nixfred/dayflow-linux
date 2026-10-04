import QtQuick
import Quickshell
import qs.Commons

// Both surfaces use the same dashboard, navigation and action contract.
FloatingWindow {
  id: root
  property var dayflow: null
  title: "Dayflow"
  color: Color.popups.background
  implicitWidth: 1440
  implicitHeight: 820
  minimumSize: Qt.size(1100, 660)
  visible: dayflow !== null
  Dashboard {
    anchors.fill: parent
    anchors.margins: 16
    dayflow: root.dayflow
    floating: true
    onCloseRequested: root.visible = false
  }
  onDayflowChanged: if (dayflow) dayflow.refreshAll()
  onVisibleChanged: if (!visible && dayflow) dayflow.fullViewOpen = false
}
