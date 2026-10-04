import QtQuick
import qs.Commons

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  width: parent ? parent.width : 0
  height: parent ? parent.height : implicitHeight
  implicitHeight: 360
  property string section: "timeline"
  property bool calendarOpen: false
  property bool editing: false
  property string editTitle: ""
  property string editCategory: ""
  property bool editProductive: false
  function beginEdit() { editTitle = timeline.current.title || ""; editCategory = timeline.current.category || ""; editProductive = timeline.current.productive === true; editing = true }
  Row {
    id: tools; width: parent.width; spacing: 8; height: 28
    DayNavRow { dayflow: root.dayflow }
    CompactButton { dayflow: root.dayflow; text: "Calendar"; active: root.calendarOpen; onClicked: root.calendarOpen = !root.calendarOpen }
    CompactButton { dayflow: root.dayflow; text: root.section === "timeline" ? "Workflow" : "Timeline"; onClicked: root.section = root.section === "timeline" ? "workflow" : "timeline" }
    CopyButton { dayflow: root.dayflow; label: "Copy day"; proc: root.dayflow.procByName("copyProc"); onActivated: { proc.command = ["bash", "-c", "dayflow export " + root.dayflow.viewDateStr() + " | wl-copy"]; proc.running = true } }
    CopyButton { dayflow: root.dayflow; label: "Copy mini"; proc: root.dayflow.procByName("copyMiniProc"); onActivated: { proc.command = ["bash", "-c", "dayflow export " + root.dayflow.viewDateStr() + " --mini | wl-copy"]; proc.running = true } }
  }
  Row {
    y: 38; width: parent.width; height: parent.height - y; spacing: 12
    DashboardCard {
      id: timeline
      width: (parent.width - parent.spacing) / 2; height: parent.height
      dayflow: root.dayflow; title: root.section === "timeline" ? "Screen timeline" : "Daily workflow"
      property var records: root.section === "timeline" ? root.dayflow.spans : (root.dayflow.workflow.slots || [])
      property string emptyText: root.dayflow.timelineLoading ? "Loading the selected day…" : "No screen summaries yet. Capture produces a summary at each block interval. Completed agent replies appear separately, immediately."
      property var formatRecord: function(record) {
        if (root.section === "workflow") return (record.time || "") + "\n" + root.dayflow.catDisplay(record.category || "") + "\n" + root.dayflow.fmtDur(root.dayflow.workflow.total_minutes || 0) + " tracked this day."
        var detail = (record.children || []).map(function(child) { return (child.start || "") + " " + (child.title || "") + "\n" + (child.activities || []).map(function(a) { return root.dayflow.appDisplayName(a.app) + " · " + a.title }).join("\n") }).join("\n\n")
        return [record.start + "–" + record.end + " · " + root.dayflow.fmtDur(record.minutes), root.dayflow.catDisplay(record.category) + (record.productive ? " · productive" : "") + (record.low_confidence ? " · low confidence" : ""), record.title, record.summary, detail].filter(function(value) { return !!value }).join("\n\n")
      }
      property int index: 0
      readonly property var current: records.length ? records[Math.min(index,records.length-1)] : ({})
      onRecordsChanged: index = Math.max(0,Math.min(index,records.length-1))
      // Leave space for the edit action beneath the text pager.
      ActivityStrip { width: parent.width; dayflow: root.dayflow }
      Row {
        id: recordControls; spacing: 6
        CompactButton { dayflow: root.dayflow; text: "‹"; enabled: timeline.index > 0; onClicked: timeline.index-- }
        Text { anchors.verticalCenter: parent.verticalCenter; text: timeline.records.length ? (timeline.index+1)+" / "+timeline.records.length : "0 records"; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption) }
        CompactButton { dayflow: root.dayflow; text: "›"; enabled: timeline.index+1 < timeline.records.length; onClicked: timeline.index++ }
      }
      PagedText { id: recordText; width: parent.width; dayflow: root.dayflow; bodyHeight: Math.max(28,timeline.height-210); text: timeline.records.length ? timeline.formatRecord(timeline.current) : timeline.emptyText }
      CompactButton { id: editAction; dayflow: root.dayflow; text: "Edit classification"; enabled: root.section === "timeline" && timeline.records.length > 0; onClicked: root.beginEdit() }
    }
    RecordCard {
      width: (parent.width - parent.spacing) / 2; height: parent.height
      dayflow: root.dayflow; title: "Completed agent replies"; followLatest: true
      records: root.dayflow.completions || []
      emptyText: "No completed replies recorded for this day. Enable immediate completion recording in Settings → Privacy."
      formatRecord: function(record) { return [record.source + " · " + Qt.formatTime(new Date(record.completed_at * 1000), "hh:mm:ss"), record.project, record.summary].filter(function(value) { return !!value }).join("\n\n") }
    }
  }
  DashboardCard {
    visible: root.calendarOpen; anchors.fill: parent; dayflow: root.dayflow; title: "Jump to a day"; color: Color.popups.background
    CompactButton { dayflow: root.dayflow; text: "Back"; onClicked: root.calendarOpen = false }
    CalendarPicker { width: parent.width; dayflow: root.dayflow }
  }
  DashboardCard {
    visible: root.editing; anchors.fill: parent; dayflow: root.dayflow; title: "Edit screen classification"; color: Color.popups.background
    Row {
      spacing: 8
      CompactButton { dayflow: root.dayflow; text: "Save"; active: true; onClicked: { root.dayflow.saveBlockEdits(timeline.current.start_ts, root.editTitle, root.editCategory, root.editProductive, timeline.current); root.editing = false } }
      CompactButton { dayflow: root.dayflow; text: "Cancel"; onClicked: root.editing = false }
      CompactButton { dayflow: root.dayflow; text: root.editProductive ? "Productive: Yes" : "Productive: No"; active: root.editProductive; onClicked: root.editProductive = !root.editProductive }
    }
    PagedText { width: parent.width; dayflow: root.dayflow; readOnly: false; label: "Title"; bodyHeight: Math.max(28,(root.height-190)/2); text: root.editTitle; onEdited: function(value) { root.editTitle = value } }
    PagedText { width: parent.width; dayflow: root.dayflow; readOnly: false; label: "Category"; bodyHeight: Math.max(28,(root.height-190)/2); text: root.editCategory; onEdited: function(value) { root.editCategory = value } }
  }
}
