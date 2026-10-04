import QtQuick
import qs.Commons

DashboardCard {
  id: root
  property var records: []
  property int index: 0
  property bool followLatest: false
  property string emptyText: "No records for this day."
  property var formatRecord: function(record) { return String(record) }
  readonly property var current: records.length ? records[Math.min(index, records.length - 1)] : ({})
  onRecordsChanged: index = followLatest ? Math.max(0,records.length-1) : Math.max(0, Math.min(index, records.length - 1))
  Row {
    spacing: 6
    CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.index > 0; onClicked: {root.followLatest=false;root.index--} }
    Text { anchors.verticalCenter: parent.verticalCenter; text: root.records.length ? (root.index + 1) + " / " + root.records.length : "0 records"; color: root.dayflow ? root.dayflow.dim : Color.muted; font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family; font.pixelSize: Math.max(12, Style.font.caption) }
    CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.index + 1 < root.records.length; onClicked: {root.followLatest=false;root.index++} }
    CompactButton { dayflow: root.dayflow; text: "Latest"; enabled: root.records.length > 0; onClicked: {root.followLatest=true;root.index = root.records.length - 1} }
  }
  PagedText {
    width: parent.width; dayflow: root.dayflow
    bodyHeight: Math.max(28, root.height - 118)
    text: root.records.length ? root.formatRecord(root.current) : root.emptyText
  }
}
