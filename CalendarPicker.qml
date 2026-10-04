import QtQuick
import qs.Commons
import qs.Ui

// Month-grid calendar picker for jumping the viewed day. `visible` is the
// open flag — the caller's "Cal" button toggles it; picking a day hides it.
Column {
  id: cal

  required property var dayflow

  property int calYear: new Date().getFullYear()
  property int calMonth: new Date().getMonth()

  spacing: Style.space(4)

  function calShift(delta) {
    var d = new Date(cal.calYear, cal.calMonth + delta, 1)
    cal.calYear = d.getFullYear()
    cal.calMonth = d.getMonth()
  }

  function calCells() {
    var first = new Date(cal.calYear, cal.calMonth, 1)
    var lead = (first.getDay() + 6) % 7           // Mon=0 blanks before day 1
    var dim = new Date(cal.calYear, cal.calMonth + 1, 0).getDate()
    var cells = []
    for (var i = 0; i < lead; i++) cells.push(0)
    for (i = 1; i <= dim; i++) cells.push(i)
    while (cells.length % 7 !== 0) cells.push(0)
    return cells
  }

  function calPick(day) {
    if (day <= 0) return
    var sel = new Date(cal.calYear, cal.calMonth, day)
    var today = new Date()
    today.setHours(0, 0, 0, 0)
    cal.dayflow.dayOffset = Math.round((sel - today) / 86400000)
    cal.visible = false
    cal.dayflow.uilog("calendar pick " + Qt.formatDate(sel, "yyyy-MM-dd"))
    cal.dayflow.loadTimeline()
  }

  Row {
    width: parent.width
    spacing: Style.space(6)
    Text {
      text: "<"
      textFormat: Text.PlainText
      color: cal.dayflow.foreground
      font.pixelSize: Math.max(12,Style.font.body)
      anchors.verticalCenter: parent.verticalCenter
      MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: cal.calShift(-1) }
    }
    Text {
      text: Qt.formatDate(new Date(cal.calYear, cal.calMonth, 1), "MMMM yyyy")
      textFormat: Text.PlainText
      color: cal.dayflow.foreground
      font.family: cal.dayflow.fontFamily
      font.pixelSize: Math.max(12,Style.font.body)
      font.bold: true
      anchors.verticalCenter: parent.verticalCenter
    }
    Text {
      text: ">"
      textFormat: Text.PlainText
      color: cal.dayflow.foreground
      font.pixelSize: Math.max(12,Style.font.body)
      anchors.verticalCenter: parent.verticalCenter
      MouseArea { anchors.fill: parent; cursorShape: Qt.PointingHandCursor; onClicked: cal.calShift(1) }
    }
  }

  Grid {
    width: parent.width
    columns: 7
    Repeater {
      model: ["M", "T", "W", "T", "F", "S", "S"]
      delegate: Text {
        width: parent.width / 7
        horizontalAlignment: Text.AlignHCenter
        text: modelData
        color: cal.dayflow.dim
        font.family: cal.dayflow.fontFamily
        font.pixelSize: Math.max(12,Style.font.caption)
      }
    }
  }

  Grid {
    width: parent.width
    columns: 7
    Repeater {
      model: cal.calCells()
      delegate: Rectangle {
        width: parent.width / 7
        height: Style.space(24)
        radius: 8
        property int dayNum: modelData
        property bool isToday: dayNum === new Date().getDate()
          && cal.calMonth === new Date().getMonth()
          && cal.calYear === new Date().getFullYear()
        property bool isFuture: dayNum > 0 &&
          new Date(cal.calYear, cal.calMonth, dayNum) > new Date()
        color: isToday ? cal.dayflow.accentFill(0.18)
          : (dayMa.containsMouse && dayNum > 0 && !isFuture ? cal.dayflow.accentFill(0.08) : "transparent")
        border.color: isToday ? cal.dayflow.accentFill(0.5) : "transparent"
        Text {
          anchors.centerIn: parent
          text: dayNum > 0 ? dayNum : ""
          textFormat: Text.PlainText
          color: isFuture ? cal.dayflow.dim : cal.dayflow.foreground
          font.family: cal.dayflow.fontFamily
          font.pixelSize: Math.max(12,Style.font.caption)
        }
        MouseArea {
          id: dayMa
          anchors.fill: parent
          hoverEnabled: true
          enabled: dayNum > 0 && !isFuture
          cursorShape: Qt.PointingHandCursor
          onClicked: cal.calPick(dayNum)
        }
      }
    }
  }
}
