import QtQuick
import qs.Commons

DashboardCard {
  id: root
  property var host
  readonly property var dayflowState: host ? host.dayflow : null
  dayflow: dayflowState
  width: parent ? parent.width : 0
  height: parent ? parent.height : 360
  title: "Frame playback"
  Row {
    id: controls; width: parent.width; spacing: 8
    DayNavRow { dayflow: root.dayflow }
    CompactButton { dayflow: root.dayflow; text: host && host.tlPlaying ? "Pause" : "Play"; enabled: !!host && host.tlFrames.length > 1; onClicked: host.tlPlaying=!host.tlPlaying }
    CompactButton { dayflow: root.dayflow; text: "Reload"; enabled: !!host && !host.tlLoading; onClicked: host.tlLoad() }
    CompactButton { dayflow: root.dayflow; text: "Enable playback"; visible: !!host && !host.tlPlaybackOn; enabled: !!host && !host.tlLoading; onClicked: host.enablePlayback() }
    Text { anchors.verticalCenter: parent.verticalCenter; text: host && host.tlFrames.length ? "Frame "+(host.tlIndex+1)+" / "+host.tlFrames.length+" · "+Qt.formatTime(new Date(host.tlFrames[host.tlIndex].ts*1000),"hh:mm:ss") : host && host.tlLoading ? "Loading frames…" : "No frames"; color: root.dayflow ? root.dayflow.dim : Color.muted; font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family; font.pixelSize: Math.max(12, Style.font.caption) }
  }
  Rectangle {
    width: parent.width; height: root.height - 154; radius: 8
    color: root.dayflow ? root.dayflow.fgFill(0.04) : "transparent"; border.color: root.dayflow ? root.dayflow.fgFill(0.10) : Color.muted
    Image { anchors.fill: parent; fillMode: Image.PreserveAspectFit; sourceSize.width: Math.round(width*2); sourceSize.height: Math.round(height*2); source: host && host.tlFrames.length ? "file://"+host.tlFrames[host.tlIndex].path : "" }
    PagedText { anchors.centerIn: parent; width: parent.width-28; bodyHeight: Math.max(28,parent.height-54); dayflow: root.dayflow; visible: !host || host.tlFrames.length === 0; text: host && host.tlError ? host.tlError : host && host.tlPlaybackOn ? "No frames kept for this day." : "Frame playback is off. Frames are deleted after summarization. Enable playback to keep them, with the standard 10 GB cap." }
  }
  Rectangle {
    width: parent.width; height: 20; radius: 6; color: root.dayflow ? root.dayflow.fgFill(0.08) : "transparent"
    Rectangle { height: parent.height; width: host && host.tlFrames.length>1 ? parent.width*host.tlIndex/(host.tlFrames.length-1) : 0; radius: 6; color: Color.accent }
    MouseArea { anchors.fill:parent; enabled: !!host && host.tlFrames.length>0; cursorShape:Qt.PointingHandCursor; onPressed:function(event){host.tlSeek(Math.round(event.x/width*(host.tlFrames.length-1)))}
      onPositionChanged:function(event){if(pressed)host.tlSeek(Math.round(event.x/width*(host.tlFrames.length-1)))} }
  }
  Text { text:"Drag the strip to seek · playback remains silent"; color:root.dayflow ? root.dayflow.dim : Color.muted; font.family:root.dayflow ? root.dayflow.fontFamily : Style.font.family; font.pixelSize:Math.max(12,Style.font.caption) }
}
