import QtQuick
import qs.Commons

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  property var host: parent ? parent.dashboardHost : null
  width: parent ? parent.width : 0
  height: parent ? parent.height : 360
  implicitHeight: 360
  property string section: "Overview"
  readonly property var week: dayflow.weeklyPayload
  readonly property var trends: week.trends || ({})
  function duration(value) { return dayflow.fmtDur(Number(value || 0)) }
  function detail(record) { return Object.keys(record).map(function(key) { var value=record[key]; return key.replace(/_/g," ")+": "+(typeof value === "object" ? JSON.stringify(value,null,2) : value) }).join("\n\n") }
  Row {
    height: 28; spacing: 6
    Repeater {
      model: ["Overview","Activity","Trends","Forecast","Review"]
      CompactButton { required property string modelData; dayflow: root.dayflow; text: modelData; active: root.section===modelData; onClicked: root.section=modelData }
    }
    CopyButton { dayflow: root.dayflow; label: "Copy week"; proc: root.dayflow.procByName("copyWeekProc"); onActivated: proc.running=true }
    CopyButton { dayflow: root.dayflow; label: "Copy mini"; proc: root.dayflow.procByName("copyWeekMiniProc"); onActivated: proc.running=true }
  }
  Row {
    y: 38; width: parent.width; height: parent.height-y; spacing: 12
    DashboardCard {
      id: summary; width: (parent.width-parent.spacing)*0.5; height: parent.height; dayflow: root.dayflow; title: root.section === "Overview" ? "Weekly focus" : root.section
      Row {
        visible: root.section === "Overview"; width: parent.width; height: visible ? 66 : 0; spacing: 8
        Repeater {
          model: [{label:"Tracked",value:root.duration(root.week.total_minutes)},{label:"Focus",value:root.duration(root.week.focus_minutes)},{label:"Distraction",value:root.duration(root.week.distraction_minutes)}]
          MetricCard { required property var modelData; width:(parent.width-16)/3; height:66; dayflow:root.dayflow; label:modelData.label; value:modelData.value }
        }
      }
      Item {
        id: heatmap; visible: root.section === "Overview"; width: parent.width; height: visible ? Math.min(156,summary.height-230) : 0
        Column {
          width: parent.width; spacing: 4
          Text { text:"Hourly activity · Mon–Sun · 00–23h"; color:root.dayflow.dim; font.family:root.dayflow.fontFamily; font.pixelSize:Math.max(12,Style.font.caption) }
          Repeater {
            model: root.week.heatmap || []
            Row {
              required property var modelData
              width: parent.width; spacing: 3
              Text { width:36; text:root.dayflow.dayName(modelData.day); color:root.dayflow.dim; font.family:root.dayflow.fontFamily; font.pixelSize:Math.max(12,Style.font.caption) }
              Row {
                width:parent.width-39; spacing:2
                Repeater {
                  model: modelData.hours || []
                  Rectangle { required property var modelData; width:(parent.width-46)/24; height:14; radius:2; color: modelData.minutes > 0 ? root.dayflow.categoryColor(modelData.category) : root.dayflow.fgFill(0.07); opacity:modelData.minutes > 0 ? 0.35+0.65*Math.min(1,modelData.minutes/60) : 1 }
                }
              }
            }
          }
        }
      }
      Row {
        visible: root.section === "Review"; spacing: 8
        CompactButton { dayflow: root.dayflow; text: root.dayflow.weekSummaryLoading ? "Generating…" : "Generate review"; enabled: !root.dayflow.weekSummaryLoading; onClicked: { var proc=root.dayflow.procByName("reviewProc"); if (!proc.running) {root.dayflow.weekSummaryLoading=true;proc.running=true} } }
      }
      PagedText {
        width: parent.width; dayflow: root.dayflow
        bodyHeight: Math.max(28,summary.height-(root.section === "Overview" ? heatmap.height+156 : root.section === "Review" ? 118 : 84))
        text: root.section === "Overview" ? [root.week.start+" — "+root.week.end,"Idle: "+root.duration(root.week.idle_minutes),"Context shifts: "+(root.week.context_shift_count || 0)].join("\n") : root.section === "Review" ? root.dayflow.weekSummary || "Generate a weekly review for advice based on your summarized activity." : root.section === "Trends" ? root.trends.has_prev ? "Compared with the previous week\n\nTracked: "+root.duration(root.trends.total_delta_minutes)+" change\nFocus: "+root.duration(root.trends.focus_delta_minutes)+" change\nDistraction: "+root.duration(root.trends.distraction_delta_minutes)+" change\nContext shifts: "+(root.trends.shift_delta_count || 0)+" change" : "A previous week is needed for comparison." : root.section === "Forecast" ? root.host && root.host.forecast ? root.host.forecast.weekday+" · "+root.host.forecast.date+"\n"+root.duration(root.host.forecast.total_minutes)+" predicted\n"+root.host.forecast.samples+" comparable days · "+root.host.forecast.confidence+" confidence" : root.host && root.host.forecastError ? root.host.forecastError : "No forecast available yet." : "Browse all recorded activity, categories, apps, focus blocks and distractions using the controls beside this card."
      }
    }
    Column {
      width: (parent.width-parent.spacing)*0.5; height: parent.height; spacing: 8
      Row {
        id: kinds; height: 26; spacing: 5; visible: root.section === "Activity"
        Repeater {
          model: ["Categories","Apps","Distractions","Focus blocks","Timeline"]
          CompactButton { required property string modelData; dayflow: root.dayflow; text: modelData; active: root.activityKind===modelData; onClicked:root.activityKind=modelData }
        }
      }
      RecordCard {
        width: parent.width; height: parent.height-(kinds.visible ? 34 : 0); dayflow:root.dayflow
        title: root.section === "Activity" ? root.activityKind : root.section === "Trends" ? "Category changes" : root.section === "Forecast" ? "Predicted categories" : root.section === "Review" ? "Highlights and suggestions" : "Category distribution"
        records: root.section === "Activity" ? root.activityKind === "Apps" ? root.week.app_treemap || [] : root.activityKind === "Distractions" ? root.week.top_distractions || [] : root.activityKind === "Focus blocks" ? root.week.focus_blocks || [] : root.activityKind === "Timeline" ? root.dayflow.weekCards : root.week.category_donut || [] : root.section === "Trends" ? root.trends.categories || [] : root.section === "Forecast" ? root.host && root.host.forecast ? root.host.forecast.items || [] : [] : root.section === "Review" ? (root.week.highlights || []).concat(root.week.suggestions || []) : root.week.category_donut || []
        formatRecord: function(record) {if(typeof record === "string") return record; return root.detail(record)}
      }
    }
  }
  property string activityKind: "Categories"
}
