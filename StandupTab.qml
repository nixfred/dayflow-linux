import QtQuick
import qs.Commons

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  width: parent ? parent.width : 0
  height: parent ? parent.height : implicitHeight
  implicitHeight: 360
  property string draftField: "highlights"
  Row {
    id: tools; spacing: 8; height: 28
    CompactButton { dayflow: root.dayflow; text: "Copy standup"; onClicked: { var proc = root.dayflow.procByName("standupProc"); if (!proc.running) proc.running = true } }
    CompactButton { dayflow: root.dayflow; text: root.dayflow.draftDirty ? "Save draft •" : "Save draft"; active: root.dayflow.draftDirty; onClicked: root.dayflow.saveDraft() }
    CompactButton { dayflow: root.dayflow; text: root.dayflow.dayGoal.completed ? "Goal: Completed" : "Complete goal"; enabled: root.dayflow.dayGoal.goal !== ""; onClicked: { var proc=root.dayflow.procByName("goalSetProc"); proc.command=["dayflow","goal",root.dayflow.dayGoal.completed ? "clear" : "done","--json"]; proc.running=true } }
  }
  Row {
    y: 38; width: parent.width; height: parent.height-y; spacing: 12
    Repeater {
      model: ["yesterday", "today"]
      RecordCard {
        required property string modelData
        width: (parent.width - 2*parent.spacing) / 3; height: parent.height
        dayflow: root.dayflow; title: modelData
        records: (root.dayflow.standup[modelData] || {}).entries || []
        formatRecord: function(record) { return [root.dayflow.fmtDur(record.minutes),record.title,(record.app || "") + " · " + root.dayflow.catDisplay(record.category),record.span].filter(function(value) {return !!value}).join("\n\n") }
      }
    }
    DashboardCard {
      id: draft; width: (parent.width - 2*parent.spacing) / 3; height: parent.height; dayflow: root.dayflow; title: "Standup draft and goal"
      Flow {
        id: draftTabs
        width: parent.width
        spacing: 4
        Repeater {
          model: ["highlights", "tasks", "blockers", "priorities", "goal"]
          CompactButton { required property string modelData; dayflow: root.dayflow; text: modelData.charAt(0).toUpperCase()+modelData.slice(1); active: root.draftField === modelData; onClicked: root.draftField=modelData; }
        }
      }
      PagedText {
        width: parent.width; dayflow: root.dayflow; readOnly: false; label: root.draftField
        bodyHeight: Math.max(28,draft.height-draftTabs.implicitHeight-92)
        text: root.draftField === "goal" ? root.goalDraft : root.dayflow.draft[root.draftField] || ""
        onEdited: function(value) { if (root.draftField === "goal") { root.goalDraft = value } else { var next=Object.assign({},root.dayflow.draft); next[root.draftField]=value; root.dayflow.draft=next; root.dayflow.draftDirty=true } }
        onAccepted: function(value) { if(root.draftField === "goal") {var proc=root.dayflow.procByName("goalSetProc"); proc.command=["dayflow","goal","set",value,"--json"]; proc.running=true} }
      }
    }
  }
  property string goalDraft: dayflow ? dayflow.dayGoal.goal : ""
}
