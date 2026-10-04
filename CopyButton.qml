import QtQuick
import qs.Commons
import qs.Ui

// Small action button bound to a dayflow Process. Shows "Copying…" and
// ignores clicks while its process runs so a click mid-flight can't
// rewrite the command under a running copy.
Rectangle {
  id: btn

  required property var dayflow
  required property string label
  required property var proc

  signal activated()

  height: Style.space(24)
  width: lbl.implicitWidth + Style.space(12)
  radius: 8
  color: btn.proc && btn.proc.running
    ? btn.dayflow.accentFill(0.10)
    : btn.dayflow.btnBg(ma.containsMouse)
  border.color: btn.proc && btn.proc.running
    ? btn.dayflow.accentFill(0.45)
    : btn.dayflow.fgFill(0.2)
  opacity: btn.proc && btn.proc.running ? 0.8 : 1

  Text {
    id: lbl
    anchors.centerIn: parent
    text: btn.proc && btn.proc.running ? "Copying…" : btn.label
    textFormat: Text.PlainText
    color: btn.dayflow.foreground
    font.family: btn.dayflow.fontFamily
    font.pixelSize: Math.max(12,Style.font.caption)
  }

  MouseArea {
    id: ma
    anchors.fill: parent
    hoverEnabled: true
    enabled: !btn.proc || !btn.proc.running
    cursorShape: Qt.PointingHandCursor
    onClicked: btn.activated()
  }
}
