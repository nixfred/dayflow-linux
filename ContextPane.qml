import QtQuick
import qs.Commons

Item {
  id: root
  property var host
  readonly property var dayflow: host ? host.dayflow : null
  readonly property var links: dayflow ? dayflow.weeklyPayload.context_shifts || [] : []
  width: parent ? parent.width : 0
  height: parent ? parent.height : 360
  Row {
    anchors.fill: parent; spacing: 12
    DashboardCard {
      id: graph; width:(parent.width-parent.spacing)*0.6; height:parent.height; dayflow:root.dayflow; title:"Attention moves · selected transitions"
      Canvas {
        width:parent.width; height:Math.max(100,graph.height-84)
        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()
        Connections {target:root; function onLinksChanged(){parent.requestPaint()}}
        Connections {target:detail; function onIndexChanged(){parent.requestPaint()}}
        onPaint: {
          var ctx=getContext("2d");ctx.reset()
          var slice=root.links.slice(Math.floor(detail.index/5)*5,Math.floor(detail.index/5)*5+5)
          var max=1;for(var i=0;i<slice.length;i++) max=Math.max(max,Number(slice[i].minutes || 0))
          for(i=0;i<slice.length;i++) {
            var row=slice[i],y=(i+0.5)*height/5,w=(width-100)*Number(row.minutes || 0)/max
            ctx.fillStyle=root.dayflow.categoryColor(row.source);ctx.fillRect(30,y-8,w,16)
            ctx.fillStyle=root.dayflow.foreground;ctx.font="12px "+root.dayflow.fontFamily;ctx.fillText(String(row.count || 0)+"×",w+38,y+4)
          }
        }
      }
    }
    RecordCard {
      id: detail; width:(parent.width-parent.spacing)*0.4; height:parent.height; dayflow:root.dayflow; title:"Context transitions"
      records:root.links
      emptyText:"No context shifts in this week."
      formatRecord:function(record){return [root.dayflow.catDisplay(record.source)+" → "+root.dayflow.catDisplay(record.target),record.count+" transitions",root.dayflow.fmtDur(record.minutes),"Chart shows five transitions at a time. Select any record to read its full labels."].join("\n\n")}
    }
  }
}
