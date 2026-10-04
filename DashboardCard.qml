import QtQuick
import qs.Commons

Rectangle {
  id: root
  objectName: "dashboardCard"
  property var dayflow
  property string title: ""
  property color tint: Color.accent
  default property alias contents: body.data
  readonly property alias body: body
  implicitHeight: body.implicitHeight + 52
  radius: 12
  color: dayflow ? dayflow.fgFill(0.045) : "transparent"
  border.color: Qt.alpha(tint, 0.28)
  border.width: 1
  Text {
    x: 14; y: 12; width: parent.width - 28
    text: root.title.toUpperCase()
    textFormat: Text.PlainText
    color: root.dayflow ? root.dayflow.dim : Color.foreground
    font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
    font.pixelSize: Math.max(12, Style.font.caption); font.bold: true; font.letterSpacing: 1.3
  }
  Column { id: body; x: 14; y: 38; width: parent.width - 28; height: parent.height - 52; spacing: 8 }
}
