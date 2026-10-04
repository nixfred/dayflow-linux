import QtQuick
import qs.Commons

// Every captured slot is drawn; detailed labels remain available in Workflow.
Item {
  id: root
  property var dayflow
  readonly property var slots: dayflow ? dayflow.workflow.slots || [] : []
  height: 48
  Rectangle {
    width: parent.width; height: 20; radius: 4
    color: root.dayflow.fgFill(0.06)
    Row {
      anchors.fill: parent; spacing: 1
      Repeater {
        model: root.slots
        Rectangle {
          required property var modelData
          width: Math.max(0,(root.width-Math.max(0,root.slots.length-1))/Math.max(1,root.slots.length)); height: 20; radius: 2
          color: modelData.category ? root.dayflow.categoryColor(modelData.category) : root.dayflow.fgFill(0.06)
        }
      }
    }
  }
  Text { y: 26; text: root.slots.length ? root.slots[0].time + " · captured activity" : "No captured activity slots"; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12,Style.font.caption) }
  Text { y: 26; anchors.right: parent.right; text: root.slots.length ? root.slots[root.slots.length-1].time : ""; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12,Style.font.caption) }
}
