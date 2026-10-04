import QtQuick
import qs.Commons

Rectangle {
  id: root
  property var dayflow
  property string label: ""
  property string value: ""
  property color tint: Color.accent
  property real fraction: 0
  radius: 10
  color: root.dayflow.fgFill(0.045)
  border.color: Qt.alpha(root.tint, 0.25)
  Text { x: 12; y: 9; text: root.label.toUpperCase(); color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption); font.letterSpacing: 1 }
  Text { x: 12; y: 28; text: root.value; color: root.dayflow.foreground; font.family: root.dayflow.fontFamily; font.pixelSize: 24; font.bold: true }
  Rectangle {
    x: 12; y: parent.height - 8; width: parent.width - 24; height: 2
    color: root.dayflow.fgFill(0.08)
    Rectangle { width: parent.width * Math.max(0, Math.min(1, root.fraction)); height: parent.height; color: root.tint; Behavior on width { NumberAnimation { duration: 180 } } }
  }
}
