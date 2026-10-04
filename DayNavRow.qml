import QtQuick
import qs.Commons
import qs.Ui

// ‹ Today › navigation row shared by the FullView day-scoped panes.
// The panel keeps its own variant (it disables forward-nav past today).
Row {
  id: nav

  required property var dayflow
  spacing: Style.space(6)

  Repeater {
    model: [
      { label: "‹", act: -1 },
      { label: "Today", act: 0 },
      { label: "›", act: 1 }
    ]

    delegate: Rectangle {
      required property var modelData
      readonly property bool canAdvance: modelData.act <= 0 || (nav.dayflow && nav.dayflow.dayOffset < 0)
      height: Style.space(28)
      width: navText.implicitWidth + Style.space(16)
      radius: 8
      color: navMouse.containsMouse && nav.dayflow && canAdvance
        ? nav.dayflow.fgFill(0.08)
        : "transparent"
      border.color: nav.dayflow ? nav.dayflow.fgFill(0.15) : "transparent"
      opacity: canAdvance ? 1 : 0.4

      Text {
        id: navText
        anchors.centerIn: parent
        text: modelData.label
        textFormat: Text.PlainText
        color: nav.dayflow ? nav.dayflow.foreground : "white"
        font.family: nav.dayflow ? nav.dayflow.fontFamily : ""
        font.pixelSize: Math.max(12,Style.font.body)
      }

      MouseArea {
        id: navMouse
        anchors.fill: parent
        hoverEnabled: true
        enabled: parent.canAdvance
        cursorShape: Qt.PointingHandCursor
        onClicked: {
          if (!nav.dayflow) return
          if (modelData.act === 0) {
            if (nav.dayflow.dayOffset !== 0) {
              nav.dayflow.dayOffset = 0
              nav.dayflow.loadTimeline()
            }
          } else {
            nav.dayflow.goDay(modelData.act)
          }
        }
      }
    }
  }

  Text {
    anchors.verticalCenter: parent.verticalCenter
    text: nav.dayflow ? nav.dayflow.viewDateLabel() : ""
    textFormat: Text.PlainText
    color: nav.dayflow ? nav.dayflow.dim : "gray"
    font.family: nav.dayflow ? nav.dayflow.fontFamily : ""
    font.pixelSize: Math.max(12,Style.font.body)
  }
}
