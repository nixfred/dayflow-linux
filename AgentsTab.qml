import QtQuick
Item {
  property var dayflow: parent && parent.panel ? parent.panel : null
  width: parent ? parent.width : 0
  height: parent ? parent.height : 360
  implicitHeight: 360
  AgentsPane { anchors.fill: parent; host: parent.parent.dashboardHost }
}
