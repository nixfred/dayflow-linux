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
    property string longText: ""
  }
  ListModel { id: categories }

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
      Settings { id: settings; dayflow: model; visible: runner.scenario >= 0 && runner.scenario < runner.cases.length && runner.cases[runner.scenario].kind === "settings" }
      Item {
        id: onboardingHost
        width: Math.min(stage.width, 620)
        height: stage.height
        visible: !settings.visible
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
    settings.presets = [
      {name:"Gemma 4 31B — default",slug:"google/gemma-4-31b-it",notes:"Best overall balance on OpenRouter; fast, cheap, and accurate."},
      {name:"Gemma 3 27B — bigger still small",slug:"google/gemma-3-27b-it",notes:"More capable than the 12B without being huge."},
      {name:"Gemma 3 12B — cheap minimal",slug:"google/gemma-3-12b-it",notes:"Small, fast, cheapest."},
      {name:"Qwen2.5-VL 7B — local minimal",slug:"qwen2.5vl:7b",notes:"Local option for Ollama/LM Studio."},
      {name:"Gemma 3 4B — local tiny",slug:"gemma3:4b",notes:"Minimal local vision option."}
    ]
    var providers = []
    for (var p = 0; p < 20; p++) providers.push({id:"provider-"+p,name:"Provider " + p, prompt_overrides:{title_prompt:Array(30).join("Title instructions.\n"),summary_prompt:"Summary instructions",detailed_prompt:"Detailed instructions",chat_prompt:"Chat instructions"}})
    settings.providers = providers
    model.configDraft = {provider:"local",model:"gemma3:4b",api_base_url:"http://localhost:11434/v1",classification_prompt:Array(25).join("Classify actual work; keep all instructions.\n")}
    onboarding.detected = {ollama:true,lmstudio:true,agents:{codex:true},presets:settings.presets}
    onboarding.step = test.step || 0
    onboarding.mode = test.mode || "openrouter"
    onboarding.testResult = test.error ? model.notice : "Checks passed."
  }
  Timer {
    interval: 200
    running: true
    repeat: true
    onTriggered: {
      if (runner.capturing) return
      if (runner.scenario >= 0) {
        var test = runner.cases[runner.scenario]
        var target = test.kind === "settings" ? settings : onboarding
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
        list.push({name:"settings-"+size.width+"x"+size.height+"-"+(light?"light":"dark"),kind:"settings",width:size.width,height:size.height,light:light,error:true})
        for (var step=0;step<5;step++) list.push({name:"onboarding-"+size.width+"x"+size.height+"-"+(light?"light":"dark")+"-"+step,kind:"onboarding",width:size.width,height:size.height,light:light,step:step,error:step===4})
      }
    }
    cases=list
    verifyTextPages()
  }
}
