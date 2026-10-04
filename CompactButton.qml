import QtQuick
import qs.Commons

Rectangle {
  id: root
  property var dayflow: null
  property string text: ""
  property bool active: false
  signal clicked()
  implicitWidth: label.implicitWidth + Style.space(14)
  implicitHeight: Style.space(26)
  radius: Style.cornerRadius
  color: dayflow ? (active ? dayflow.accentFill(0.16) : dayflow.btnBg(mouse.containsMouse)) : "transparent"
  border.color: dayflow ? dayflow.accentFill(active ? 0.6 : 0.25) : Color.accent
  opacity: enabled ? 1 : 0.45
  Text {
    id: label
    anchors.centerIn: parent
    text: root.text
    textFormat: Text.PlainText
    color: root.dayflow ? root.dayflow.foreground : Color.foreground
    font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
    font.pixelSize: Math.max(12, Style.font.caption)
  }
  MouseArea {
    id: mouse
    anchors.fill: parent
    enabled: root.enabled
    hoverEnabled: true
    cursorShape: Qt.PointingHandCursor
    onClicked: root.clicked()
  }
}
