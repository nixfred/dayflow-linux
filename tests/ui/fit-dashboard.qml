import QtQuick
import Quickshell
import qs.Commons

ShellRoot {
  id: runner
  property int scenario: -1
  property bool capturing: false
  property string outputPath: OUTPUT_PATH
  property string caseFilter: CASE_FILTER
  property bool demoMode: DEMO_MODE
  property var cases: []
  property int failures: 0
  property var baseline: ({})

  QtObject {
    id: model
    property bool light: false
    property color foreground: "#e4e4e4"
    property color dim: "#a5a5a5"
    property color accent: "#7aa2f7"
    property string fontFamily: "Sans Serif"
    property bool configLoaded: true
    property var config: ({knowledge_sync: false})
    property var configDraft: ({provider: "local", model: "gemma3:4b", api_base_url: "http://localhost:11434/v1", classification_prompt: "Classify productive work honestly."})
    property string storageText: "32 MB"
    property string notice: ""
    property string errorText: ""
    property var settingsCatModel: categories
    function fgFill(alpha) { return Qt.rgba(foreground.r, foreground.g, foreground.b, alpha) }
    function accentFill(alpha) { return Qt.rgba(accent.r, accent.g, accent.b, alpha) }
    function btnBg(hover) { return fgFill(hover ? 0.10 : 0.03) }
    function saveConfig() { notice = "settings saved" }
    function loadConfig() {}
    function uilog(value) {}
    property bool configured: true
    property bool timelineLoading: false
    property int dayOffset: 0
    property var completions: []
    property var spans: []
    function goDay(delta) {dayOffset+=delta}
    function viewDateLabel() {return "Today"}
    function todayIndex() {return 6}
    function dayName(index) {return ["Mon","Tue","Wed","Thu","Fri","Sat","Sun"][index]}
    function loadTimeline() {}
    function procByName(name) {return inertProcess}
    function appDisplayName(app) {return app}
    function appIcon(app) {return ""}
    function fmtDur(minutes) {return minutes+" min"}
    function catDisplay(category) {return category}
    function categoryColor(category) {return accent}
    function pillBgColor(category) {return accent}
    function saveBlockEdits() {}
    property string pluginVersion:"1.5.0"
    property string engineVersion:"1.5.0"
    property bool engineMissing:false
    property bool engineInstalling:false
    property string installErr:""
    property string installLog:""
    property string installScriptPath:"scripts/install.sh"
    property bool onboardingSkipped:false
    property bool paused:false
    property string captureState:"recording"
    property string modelName:"gemma3:4b"
    property int framesToday:180
    property int blocksPending:2
    property var blocks:[{},{}]
    property bool summarizing:false
    property string activeApp:"Terminal"
    property var ignoredApps:["Privacy browser"]
    property string currentTab:"today"
    property bool fullViewOpen:false
    property bool draftDirty:false
    property var draft:({highlights:"Delivered a tested fix.",tasks:"Review deployment.",blockers:"",priorities:"Keep capture working."})
    property var dayGoal:({goal:"Ship the dashboard",completed:false})
    property var workflow:({total_minutes:90,slots:[{time:"18:00",category:"coding"},{time:"18:15",category:"research"}],categories:[]})
    property var standup:({yesterday:{entries:[{title:"Completed the previous task",minutes:45,category:"coding"}]},today:{entries:[{title:"Revamped the UI",minutes:90,category:"coding"}]}})
    property var insights:({})
    property var weekCards:[{title:"A weekly screen summary",summary:"Complete details.",minutes:30}]
    property string weekSummary:Array(80).join("Weekly review evidence and recommendations.\n")
    property bool weekSummaryLoading:false
    property var weeklyPayload:({start:"2026-09-28",end:"2026-10-04",total_minutes:480,focus_minutes:300,idle_minutes:80,distraction_minutes:100,context_shift_count:12,category_donut:[{name:"coding",display:"Coding",minutes:300,percentage:62.5},{name:"research",minutes:100,percentage:20.8}],app_treemap:[{name:"Terminal",minutes:300}],heatmap:[],context_shifts:[{source:"coding",target:"research",count:12,minutes:20}],trends:{has_prev:true,total_delta_minutes:30,focus_delta_minutes:45,distraction_delta_minutes:-15,shift_delta_count:2,categories:[{name:"coding",delta_minutes:45}]},highlights:["Delivered a tested improvement"],suggestions:["Keep a clear goal"],focus_blocks:[{title:"Focused work",minutes:45}],top_distractions:[{title:"Browsing",minutes:15}]})
    property var agentBriefing:({day:"2026-10-04",workstreams:[{name:"Dashboard",summary:"UI improvements",bullets:["Verified actual Qt layout"],threads:[{source:"codex",title:"Build the dashboard",status:"reviewReady",latest_outcome:"Ready for review",started_at:1791154800,ended_at:1791158400,turns:[{role:"assistant",text:Array(40).join("Complete narrative detail. "),ts:1791158400}]}]}]})
    property var agentSources:[{source:"codex",status:"available"}]
    property bool agentRecapsEnabled:false
    property bool agentsLoading:false
    property string agentsError:""
    property string dateLabel:"Today"
    function saveDraft(){}
    function refreshAll(){}
    function close(){}
    function toggleCapture(){}
    function ignoreCurrentApp(){}
    function summarizeNow(){}
    function agentsLoad(refresh){}
    function versionNewer(a,b){return false}
    function requestEngineInstall(){}
    function viewDateStr(){return "2026-10-04"}
    property string longText: ""
  }
  ListModel { id: categories }
  QtObject { id: inertProcess; property var command: []; property bool running: false }

  FloatingWindow {
    id: window
    implicitWidth: 1280
    implicitHeight: 800
    color: model.light ? "#f5f5f5" : "#171b24"
    title: "Dayflow isolated layout verification"
    Rectangle {
      id: stage
      color: window.color
      x: 40; y: 90
      width: window.width - 80
      height: window.height - 180
      Dashboard { id: dashboard; anchors.fill:parent; dayflow:model }
    }
    PagedText {
      id: textProbe
      visible: false
      width: 320
      dayflow: model
      bodyHeight: 54
      readOnly: false
      text: model.longText
      onEdited: function(value) { model.longText = value }
    }
  }

  function fail(message) { failures++; console.error("FIT_FAIL " + message) }
  function checkTree(item, label) {
    if (!item.visible || item.opacity === 0) return
    var point = item.mapToItem(stage, 0, 0)
    if (point.x < -1 || point.y < -1 || point.x + item.width > stage.width + 1 || point.y + item.height > stage.height + 1)
      fail(label + " bounds: " + item + " at " + point.x + "," + point.y + " size " + item.width + "x" + item.height)
    if (item.text !== undefined && item.font !== undefined && item.font.pixelSize < 12)
      fail(label + " unreadable font: " + item.font.pixelSize)
    if (item.objectName === "dashboardCard" && item.body.implicitHeight > item.body.height+1) fail(label+" card body overflow: "+item.title+" "+item.body.implicitHeight+" > "+item.body.height)
    if (item.objectName === "pageEditor" && item.contentHeight > item.height + 1)
      fail(label + " page overflow: " + item.contentHeight + " > " + item.height + " text=" + item.text.substring(0,100) + " width=" + item.width)
    if (item.children) for (var i = 0; i < item.children.length; i++) checkTree(item.children[i], label)
  }
  function verifyTextPages() {
    var original = Array(60).join("A long wrapped line with unicode 🙂 and a newline.\n")
    model.longText = original
    var rebuilt = ""
    for (var i = 0; i < textProbe.pageCount; i++) { textProbe.page = i; rebuilt += textProbe.displayedText }
    if (rebuilt !== original) fail("paging dropped or duplicated text")
    textProbe.page = 1
    var range = textProbe.ranges[1]
    var expected = original.substring(0, range.start) + "changed page" + original.substring(range.end)
    var editor = textProbe.children[1].children[0]
    editor.text = "changed page"
    if (model.longText !== expected) fail("editing a page damaged surrounding text")
  }
  function findPage(item) { if(item.objectName === "dashboardPage")return item.item; for(var child of item.children || []) {var found=findPage(child);if(found)return found} return null }
  function applyCase(test) {
    stage.width=test.width-80;stage.height=test.height-80
    model.light=test.light; Color.light=test.light
    model.foreground=test.light ? "#202020" : "#e4e4e4";model.dim=test.light ? "#505050" : "#a5a5a5"
    model.currentTab=test.tab
    var empty=test.mode === "empty"
    model.standup=empty ? {yesterday:{entries:[]},today:{entries:[]}} : baseline.standup
    model.weeklyPayload=empty ? {category_donut:[],app_treemap:[],context_shifts:[],heatmap:[],highlights:[],suggestions:[]} : baseline.week
    model.workflow=empty ? {slots:[],total_minutes:0} : baseline.workflow
    model.agentBriefing=empty ? {workstreams:[]} : baseline.agents
    model.agentsError=test.mode === "error" ? Array(40).join("Agent source unavailable: complete diagnostic.\n") : ""

    model.engineMissing=test.mode === "install";model.configured=test.mode !== "onboarding"
    model.errorText=test.mode === "error" ? Array(30).join("Long diagnostic: all information is retained.\n") : ""
    model.notice=test.mode === "error" ? model.errorText : ""
    model.spans=test.mode === "empty" ? [] : [{title:Array(25).join("Complete work title. "),summary:Array(80).join("Screen summary evidence.\n"),start:"18:00",end:"18:15",minutes:15,count:1,category:"coding",appName:"Terminal",productive:true,children:[{activities:[{app:"Terminal",title:Array(30).join("Full activity detail. ")}]}]}]
    model.completions=test.mode === "empty" ? [] : [{source:"codex",project:"~/Projects/example",summary:Array(100).join("Finished the task and verified the result.\n"),completed_at:1791154800}]
    if(demoMode) {
      model.spans=[{title:"Building the Dayflow dashboard",summary:"Reworked navigation and metric cards, checked the layout at laptop and desktop sizes, and reviewed the changes before publication.",start:"09:00",end:"10:15",minutes:75,count:1,category:"coding",appName:"Terminal",productive:true,children:[{activities:[{app:"Terminal",title:"Qt runtime layout checks"},{app:"Browser",title:"Reviewing the proposed changes"}]}]},{title:"Research and planning",summary:"Reviewed the current interface and grouped controls around the work they affect.",start:"10:15",end:"10:45",minutes:30,count:1,category:"research",productive:true,children:[]}]
      model.completions=[{source:"claude",project:"~/Projects/example",summary:"Implemented the requested fix and verified the result.",completed_at:1791126000},{source:"codex",project:"~/Projects/dayflow-linux",summary:"Finished the interface review. All dashboard pages fit without scrolling, including long summaries and action results. The changes are ready for review.",completed_at:1791129600}]
      var slots=[];for(var i=0;i<24;i++)slots.push({time:(9+Math.floor(i/4))+":"+String((i%4)*15).padStart(2,"0"),category:i<10 ? "coding" : i<15 ? "research" : i<21 ? "coding" : ""});model.workflow=Object.assign({},model.workflow,{slots:slots})
    }
    Qt.callLater(function(){var page=findPage(dashboard);if(!page)return;if(test.tab === "settings") {page.presets=[{name:"Gemma 3",slug:"gemma3:4b",notes:Array(30).join("Complete preset notes. ")}];page.providers=[{id:"local",name:Array(20).join("Local provider "),prompt_overrides:{title_prompt:Array(40).join("Preserve every instruction.\n")}}]};if(test.section !== undefined)page.section=test.section;if(test.tab === "chat" && test.mode !== "empty"){page.conversations=[{id:1,title:Array(30).join("A saved conversation ")}];page.chatMessages=[{role:"user",content:"A question"},{role:"assistant",content:Array(80).join("Complete model answer.\n")}]}if(test.mode === "edit")page.beginEdit();if(test.mode === "calendar")page.calendarOpen=true;if(test.step !== undefined){page.step=test.step;page.testResult=model.errorText || Array(30).join("Check result: complete detail.\n")};if(test.status){function reveal(item){if(item.objectName === "statusDrawer")item.visible=true;for(var child of item.children || [])reveal(child)}reveal(dashboard)}})
  }
  Timer {
    interval: 200
    running: true
    repeat: true
    onTriggered: {
      if (runner.capturing) return
      if (runner.scenario >= 0) {
        var test = runner.cases[runner.scenario]
        var target = dashboard
        runner.checkTree(target, test.name)
        console.log("FIT_RESULT " + JSON.stringify({name:test.name,width:test.width,height:test.height,contentHeight:target.height,availableHeight:stage.height,failures:runner.failures}))
        if (target.height > stage.height + 1) runner.fail(test.name + " content too tall")
        runner.capturing = true
        stage.grabToImage(function(result) {
          result.saveToFile(runner.outputPath + "/" + test.name + ".png")
          runner.capturing = false
          runner.nextCase()
        })
      } else runner.nextCase()
    }
  }
  function nextCase() {
    scenario++
    if (scenario >= cases.length) { console.log("FIT_COMPLETE failures="+failures); Qt.quit(); return }
    applyCase(cases[scenario])
  }
  Component.onCompleted: {
    for(var i=0;i<120;i++)categories.append({name:"Category "+i,description:Array(30).join("Complete category description. "),color:""})
    var list=[]
    for(var size of [{width:1280,height:720},{width:1280,height:800},{width:1920,height:1080}])for(var light of [false,true]) {
      var prefix=size.width+"x"+size.height+"-"+(light?"light":"dark")
      for(var tab of ["today","standup","chat","week","agents","context","timelapse","settings"])for(var mode of ["normal","empty","error"])list.push({name:prefix+"-"+tab+"-"+mode,width:size.width,height:size.height,light:light,tab:tab,mode:mode})
      for(var section of ["Capture","Prompts","Privacy"])list.push({name:prefix+"-settings-"+section,width:size.width,height:size.height,light:light,tab:"settings",section:section})
      for(var section of ["Activity","Trends","Forecast","Review"])list.push({name:prefix+"-week-"+section,width:size.width,height:size.height,light:light,tab:"week",section:section})
      for(var mode of ["edit","calendar","install"])list.push({name:prefix+"-today-"+mode,width:size.width,height:size.height,light:light,tab:"today",mode:mode})
      list.push({name:prefix+"-status-error",width:size.width,height:size.height,light:light,tab:"today",mode:"error",status:true})
      for(var step=0;step<5;step++)list.push({name:prefix+"-setup-"+step,width:size.width,height:size.height,light:light,tab:"today",mode:"onboarding",step:step})
    }
    var hours=[];for(var h=0;h<24;h++)hours.push({hour:h,category:h>8 && h<17 ? "coding" : "",minutes:h>8 && h<17 ? 45 : 0})
    model.weeklyPayload=Object.assign({},model.weeklyPayload,{heatmap:[0,1,2,3,4,5,6].map(function(day){return {day:day,hours:hours}})})
    baseline={standup:model.standup,week:model.weeklyPayload,workflow:model.workflow,agents:model.agentBriefing}
    cases=caseFilter ? list.filter(function(test){return test.name.indexOf(caseFilter)>=0}) : list; verifyTextPages()
  }
}
