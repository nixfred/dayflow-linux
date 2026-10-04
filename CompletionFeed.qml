import QtQuick
import qs.Commons

Column {
  id: root
  property var dayflow
  property var records: []
  property int index: 0
  readonly property var current: records.length ? records[Math.min(index,records.length-1)] : ({})
  onRecordsChanged: index = Math.max(0,Math.min(index,records.length-1))
  spacing: Style.space(4)
  Row {
    spacing: Style.space(5)
    Text {
      anchors.verticalCenter: parent.verticalCenter
      text: "Recorded agent replies · " + root.records.length
      font.family: root.dayflow.fontFamily
      font.pixelSize: Math.max(12,Style.font.body)
      font.bold: true
      color: root.dayflow.foreground
    }
    CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.index > 0; onClicked: root.index-- }
    Text { anchors.verticalCenter: parent.verticalCenter; text: (root.index+1)+" / "+root.records.length; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12,Style.font.caption) }
    CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.index+1 < root.records.length; onClicked: root.index++ }
  }
  PagedText {
    width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(96)
    text: (root.current.source || "") + " · " + (root.current.project || "") + " · " + (root.current.completed_at ? Qt.formatDateTime(new Date(root.current.completed_at * 1000), "hh:mm") : "") + "\n" + (root.current.summary || "")
  }
}
