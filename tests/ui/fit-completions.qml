import QtQuick
import Quickshell

ShellRoot {
  id: runner
  property int scenario: -1
  property bool capturing: false
  property string outputPath: OUTPUT_PATH
  property var cases: []
  property int failures: 0

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
    function viewDateStr() {return "2026-10-04"}
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
      TodayTab { id: today; width: stage.width; dayflow: model }
      Settings { id: settings; dayflow: model; visible: false }
      Item {
        id: onboardingHost
        width: Math.min(stage.width, 620)
        height: stage.height
        visible: false
        Onboarding { id: onboarding; dayflow: model }
      }
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
  function applyCase(test) {
    window.implicitWidth = test.width
    window.implicitHeight = test.height
    stage.width = test.width - 120
    stage.height = test.height - 180
    model.light = test.light
    model.foreground = test.light ? "#202020" : "#e4e4e4"
    model.dim = test.light ? "#505050" : "#a5a5a5"
    model.notice = test.error ? Array(60).join("Long error: complete diagnostic line.\n") : ""
    settings.syncStatus = test.error ? Array(35).join("Sync destination returned an error.\n") : ""
    model.completions = [{source:"codex",project:"~/Projects/example",summary:Array(100).join("Finished a meaningful task; every diagnostic and result is retained.\n"),completed_at:1791154800}]
    model.spans = [{title:Array(30).join("A complete screen summary title. "),summary:Array(60).join("Detailed screen evidence and diagnostics.\n"),start:"18:00",end:"18:15",minutes:15,count:1,category:"coding",app:"Terminal",appName:"Terminal",productive:true,children:[{start_ts:1,end_ts:900,activities:[{app:"Terminal",title:Array(60).join("Complete activity detail. ")}]}]}]

  }
  Timer {
    interval: 200
    running: true
    repeat: true
    onTriggered: {
      if (runner.capturing) return
      if (runner.scenario >= 0) {
        var test = runner.cases[runner.scenario]
        var target = today
        runner.checkTree(target, test.name)
        console.log("FIT_RESULT " + JSON.stringify({name:test.name,width:test.width,height:test.height,contentHeight:target.implicitHeight,availableHeight:stage.height,failures:runner.failures}))
        if (target.implicitHeight > stage.height + 1) runner.fail(test.name + " content too tall")
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
    for (var i=0; i<120; i++) categories.append({name:"Category "+i,description:Array(10).join("Complete category description. "),color:""})
    var list=[]
    for (var size of [{width:1280,height:800},{width:1280,height:720},{width:1920,height:1080}]) {
      for (var light of [false,true]) {
        list.push({name:"completions-"+size.width+"x"+size.height+"-"+(light?"light":"dark"),kind:"settings",width:size.width,height:size.height,light:light,error:true})

      }
    }
    cases=list
    verifyTextPages()
  }
}
