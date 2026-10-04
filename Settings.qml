import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import QtQuick.Layouts

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  property var presets: []
  property var providers: []
  property string usageText: dayflow ? dayflow.storageText : ""

  width: parent ? parent.width : 0
  height: parent ? parent.height : 420
  implicitHeight: 420
  property string section: "Provider"
  property string promptKey: "title_prompt"
  property bool showKey: false
  property int presetIndex: 0
  readonly property var currentPreset: presets.length ? presets[Math.min(presetIndex, presets.length - 1)] : ({})
  property int categoryIndex: 0
  property int providerIndex: 0
  property var overrideDrafts: ({})
  readonly property var currentProvider: providers.length ? providers[Math.min(providerIndex, providers.length - 1)] : ({})
  readonly property int categoryCount: dayflow ? dayflow.settingsCatModel.count : 0
  readonly property var currentCategory: categoryCount ? dayflow.settingsCatModel.get(Math.min(categoryIndex, categoryCount - 1)) : ({name: "", description: ""})
  onCategoryCountChanged: categoryIndex = Math.max(0, Math.min(categoryIndex, categoryCount - 1))

  function setDraft(key, value) {
    var draft = Object.assign({}, dayflow.configDraft)
    draft[key] = value
    dayflow.configDraft = draft
  }
  function draft(key, fallback) {
    return dayflow && dayflow.configDraft[key] !== undefined ? dayflow.configDraft[key] : fallback
  }
  function overrideText(key) {
    var id = currentProvider.id + "/" + key
    return overrideDrafts[id] !== undefined ? overrideDrafts[id] : ((currentProvider.prompt_overrides || {})[key] || "")
  }
  function editOverride(key, value) {
    var drafts = Object.assign({}, overrideDrafts)
    drafts[currentProvider.id + "/" + key] = value
    overrideDrafts = drafts
  }

  function applyPresets(raw) {
    try {
      root.presets = JSON.parse(raw)
    } catch (e) {
      console.warn("dayflow: could not parse model presets")
    }
  }

  Process {
    id: modelsProc
    command: ["dayflow", "models", "--json"]
    running: true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyPresets(text)
    }
  }

  Process {
    id: providersProc
    command: ["dayflow", "provider", "list", "--json"]
    running: true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          var d = JSON.parse(text)
          root.providers = d.providers || []
        } catch (e) {
          root.providers = []
        }
      }
    }
  }

  // Writes a single provider field immediately (prompt overrides bypass the
  // configDraft save path — `provider set` writes config itself). Writes are
  // queued: reassigning command on a running Process drops the second write.
  property var pendingProviderWrites: []

  function queueProviderWrite(cmd) {
    root.pendingProviderWrites = root.pendingProviderWrites.concat([cmd])
    if (!providerSetProc.running) {
      providerSetProc.command = root.pendingProviderWrites[0]
      providerSetProc.didStart = false
      providerSetProc.running = true
    }
  }

  function drainProviderWrites(exitCode) {
    if (exitCode !== 0 && root.dayflow) root.dayflow.notice = "prompt override save failed"
    root.pendingProviderWrites = root.pendingProviderWrites.slice(1)
    if (root.pendingProviderWrites.length > 0) {
      providerSetProc.command = root.pendingProviderWrites[0]
      providerSetProc.didStart = false // reset before arming — a failed start must drain
      providerSetProc.running = true
    }
  }

  Process {
    id: providerSetProc
    property bool didStart: false
    onStarted: providerSetProc.didStart = true
    onExited: function(exitCode) { root.drainProviderWrites(exitCode) }
    // FailedToStart emits no exited — drain the queue anyway so queued
    // writes are not stranded forever.
    onRunningChanged: {
      if (!providerSetProc.running && !providerSetProc.didStart) root.drainProviderWrites(-1)
      if (!providerSetProc.running) providerSetProc.didStart = false
    }
  }

  Connections {
    target: dayflow
    function onStorageTextChanged() { root.usageText = dayflow.storageText }
  }

  // "Sync now" — pushes today's distilled atoms to the configured
  // Kurultai profile. Only armed when knowledge_sync is on; the engine
  // refuses otherwise and the error lands in syncStatus.
  property string syncStatus: ""

  Process {
    id: syncNowProc
    command: ["dayflow", "sync", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          var d = JSON.parse(text)
          root.syncStatus = d.error && d.error !== ""
            ? "sync failed: " + d.error
            : "synced " + d.day + " — pushed " + d.pushed + ", unchanged " + d.unchanged
        } catch (e) {
          root.syncStatus = "sync failed (no result)"
        }
      }
    }
    onExited: function(code) {
      if (code !== 0 && root.syncStatus === "") root.syncStatus = "sync failed"
    }
  }


  component Heading: Text {
    width: parent.width
    color: root.dayflow ? root.dayflow.foreground : Color.foreground
    font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
    font.pixelSize: Math.max(12, Style.font.body)
    font.bold: true
    textFormat: Text.PlainText
    wrapMode: Text.WordWrap
  }
  component Note: Text {
    width: parent.width
    color: root.dayflow ? root.dayflow.dim : Color.muted
    font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
    font.pixelSize: Math.max(12, Style.font.caption)
    textFormat: Text.PlainText
    wrapMode: Text.WordWrap
  }
  component Field: Row {
    property string label: ""
    property string key: ""
    property var fallback: ""
    property bool numeric: false
    property bool secret: false
    width: parent.width
    spacing: 8
    height: numeric ? 28 : Math.max(28, value.implicitHeight)
    Text {width: parent.width*0.30; anchors.verticalCenter:parent.verticalCenter; text:parent.label; color:root.dayflow.dim; font.family:root.dayflow.fontFamily; font.pixelSize:Math.max(12,Style.font.caption); wrapMode:Text.WordWrap}
    Rectangle {
      visible: parent.numeric
      width: parent.width*0.70-parent.spacing; height: 28; radius: 8; color: root.dayflow.fgFill(0.04); border.color: root.dayflow.fgFill(0.12)
      TextInput {
        anchors.fill: parent; anchors.margins: 5; maximumLength: 11; selectByMouse: true
        text: String(root.draft(parent.parent.key,parent.parent.fallback)); inputMethodHints: Qt.ImhDigitsOnly
        color: root.dayflow.foreground; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12, Style.font.caption)
        onTextEdited: {var parsed=parseInt(text,10);if(!isNaN(parsed))root.setDraft(parent.parent.key,parsed)}
      }
    }
    PagedText {
      visible: !parent.numeric
      id:value; width:parent.width*0.70-parent.spacing; bodyHeight:28; dayflow:root.dayflow; readOnly:parent.secret; showApplyButton:false
      text:parent.secret ? "••••••••" : String(root.draft(parent.key,parent.fallback))
      onEdited:function(value){ if(parent.secret)return; var parsed=parent.numeric ? parseInt(value,10) : value; if(!parent.numeric || !isNaN(parsed))root.setDraft(parent.key,parsed) }
    }
  }

  Row {
    id:tools; spacing:6; height:28
    Repeater {model:["Provider","Capture","Prompts","Privacy"]
        CompactButton {required property string modelData; dayflow:root.dayflow; text:modelData; active:root.section===modelData; onClicked:root.section=modelData}}
    CompactButton {dayflow:root.dayflow; text:"Save settings"; active:true; enabled:!providerSetProc.running && root.pendingProviderWrites.length===0; onClicked:root.dayflow.saveConfig()}
    CompactButton {dayflow:root.dayflow; text:"Reload"; onClicked:root.dayflow.loadConfig()}
  }
  Row {
    y:38; width:parent.width; height:parent.height-y; spacing:12
    DashboardCard {
      id:left; width:(parent.width-parent.spacing)/2; height:parent.height; dayflow:root.dayflow
      title:root.section === "Provider" ? "AI provider" : root.section === "Capture" ? "Capture and storage" : root.section === "Prompts" ? "Classification instructions" : "Privacy and automation"
      Column {
        visible:root.section === "Provider"; width:parent.width; spacing:4
        Field {label:"Provider"; key:"provider"; fallback:"openrouter"}
        Field {label:"Model"; key:"model"; fallback:"google/gemma-4-31b-it"}
        Field {label:"API URL"; key:"api_base_url"}
        Field {label:"API key"; key:"openrouter_api_key"; secret:!root.showKey}
        Field {label:"App name"; key:"site_name"; fallback:"dayflow-linux"}
        CompactButton {dayflow:root.dayflow; text:root.showKey ? "Hide key" : "Edit key"; onClicked:root.showKey=!root.showKey}
      }
      Column {
        visible:root.section === "Capture"; width:parent.width; spacing:4
        Repeater {
          model:[{label:"Interval (s)",key:"capture_interval_sec",value:10},{label:"Block (min)",key:"block_minutes",value:15},{label:"Frames / block",key:"frames_per_block",value:30},{label:"JPEG quality",key:"jpeg_quality",value:55},{label:"Max dimension",key:"frame_max_dim",value:1920},{label:"Retention days",key:"retention_days",value:0},{label:"Frames (MB)",key:"max_frames_mb",value:20480},{label:"Text (MB)",key:"max_db_mb",value:10240},{label:"Total MB (legacy)",key:"max_storage_mb",value:0}]
          Field {required property var modelData; label:modelData.label; key:modelData.key; fallback:modelData.value; numeric:true}
        }
      }
      PagedText {
        visible:root.section === "Prompts"; width:parent.width; dayflow:root.dayflow; bodyHeight:left.height-90; readOnly:false; label:"Instructions"
        text:root.draft("classification_prompt",""); onEdited:function(value){root.setDraft("classification_prompt",value)}
      }
      Column {
        visible:root.section === "Privacy"; width:parent.width; spacing:12
        CompactButton {dayflow:root.dayflow; text:"Immediate completions: "+(root.draft("agent_completions",false)?"On":"Off"); active:root.draft("agent_completions",false); onClicked:root.setDraft("agent_completions",!root.draft("agent_completions",false))}
        CompactButton {dayflow:root.dayflow; text:"Agent recaps: "+(root.draft("agent_recaps",false)?"On":"Off"); active:root.draft("agent_recaps",false); onClicked:root.setDraft("agent_recaps",!root.draft("agent_recaps",false))}
        CompactButton {dayflow:root.dayflow; text:"Knowledge sync: "+(root.draft("knowledge_sync",false)?"On":"Off"); active:root.draft("knowledge_sync",false); onClicked:root.setDraft("knowledge_sync",!root.draft("knowledge_sync",false))}
        CompactButton {dayflow:root.dayflow; text:syncNowProc.running?"Syncing…":"Sync now"; enabled:root.dayflow.config.knowledge_sync === true && !syncNowProc.running; onClicked:{root.syncStatus="";syncNowProc.running=true}}
      }
    }
    DashboardCard {
      id:right; width:(parent.width-parent.spacing)/2; height:parent.height; dayflow:root.dayflow
      title:root.section === "Provider" ? "Model presets" : root.section === "Capture" ? "Category buckets" : root.section === "Prompts" ? "Provider prompt overrides" : "Data destinations and results"
      Row {
        visible:root.section === "Provider"; spacing:6
        CompactButton {dayflow:root.dayflow; text:"‹"; enabled:root.presetIndex>0; onClicked:root.presetIndex--}
        Note {width:implicitWidth; anchors.verticalCenter:parent.verticalCenter; text:"Preset "+(root.presets.length ? root.presetIndex+1 : 0)+" / "+root.presets.length}
        CompactButton {dayflow:root.dayflow; text:"›"; enabled:root.presetIndex+1<root.presets.length; onClicked:root.presetIndex++}
        CompactButton {dayflow:root.dayflow; text:"Use"; enabled:root.presets.length>0; onClicked:root.setDraft("model",root.currentPreset.slug)}
      }
      PagedText {visible:root.section === "Provider"; width:parent.width; dayflow:root.dayflow; bodyHeight:right.height-118; text:[root.currentPreset.name,root.currentPreset.notes,"Provider types: openrouter / local / custom / mcp.","API keys stay masked until you choose Edit key. App name identifies OpenRouter calls."].filter(function(v){return !!v}).join("\n\n")}
      Row {
        visible:root.section === "Capture"; spacing:6
        CompactButton {dayflow:root.dayflow; text:"‹"; enabled:root.categoryIndex>0; onClicked:root.categoryIndex--}
        Note {width:implicitWidth; anchors.verticalCenter:parent.verticalCenter; text:root.categoryCount ? (root.categoryIndex+1)+" / "+root.categoryCount : "No categories"}
        CompactButton {dayflow:root.dayflow; text:"›"; enabled:root.categoryIndex+1<root.categoryCount; onClicked:root.categoryIndex++}
        CompactButton {dayflow:root.dayflow; text:"Add"; onClicked:{root.dayflow.settingsCatModel.append({name:"",description:"",color:""});root.categoryIndex=root.categoryCount-1}}
        CompactButton {dayflow:root.dayflow; text:"Remove"; enabled:root.categoryCount>0; onClicked:root.dayflow.settingsCatModel.remove(root.categoryIndex)}
      }
      PagedText {visible:root.section === "Capture"; width:parent.width; dayflow:root.dayflow; bodyHeight:28; readOnly:false; label:"Name"; text:root.currentCategory.name || ""; onEdited:function(value){if(root.categoryCount)root.dayflow.settingsCatModel.setProperty(root.categoryIndex,"name",value)}}
      PagedText {visible:root.section === "Capture"; width:parent.width; dayflow:root.dayflow; bodyHeight:Math.max(28,right.height-280); readOnly:false; label:"Description"; text:root.currentCategory.description || ""; onEdited:function(value){if(root.categoryCount)root.dayflow.settingsCatModel.setProperty(root.categoryIndex,"description",value)}}
      PagedText {visible:root.section === "Capture"; width:parent.width; dayflow:root.dayflow; bodyHeight:48; text:"Stored: "+(root.usageText || "—")+". More frames mean more detail and model usage. Zero caps/retention means unlimited. Capture changes require a service restart."}
      Row {
        visible:root.section === "Prompts"; spacing:6
        CompactButton {dayflow:root.dayflow; text:"‹"; enabled:root.providerIndex>0; onClicked:root.providerIndex--}
        Note {width:implicitWidth; anchors.verticalCenter:parent.verticalCenter; text:root.providers.length ? (root.providerIndex+1)+" / "+root.providers.length : "No providers"}
        CompactButton {dayflow:root.dayflow; text:"›"; enabled:root.providerIndex+1<root.providers.length; onClicked:root.providerIndex++}
      }
      PagedText {visible:root.section === "Prompts"; width:parent.width; dayflow:root.dayflow; bodyHeight:28; text:root.currentProvider.name || root.currentProvider.id || "No provider overrides"}
      Row {
        visible:root.section === "Prompts"; spacing:5
        Repeater {model:[{label:"Title",key:"title_prompt"},{label:"Summary",key:"summary_prompt"},{label:"Detailed",key:"detailed_prompt"},{label:"Chat",key:"chat_prompt"}]
        CompactButton {required property var modelData; dayflow:root.dayflow; text:modelData.label; active:root.promptKey===modelData.key; onClicked:root.promptKey=modelData.key}}
      }
      PagedText {
        id:overrideEditor; visible:root.section === "Prompts"; width:parent.width; dayflow:root.dayflow; bodyHeight:Math.max(28,right.body.height-y-overrideHelp.implicitHeight-37); readOnly:false; label:"Override"
        acceptOnBlur:false
        text:root.overrideText(root.promptKey)
        onEdited:function(value){root.editOverride(root.promptKey,value)}
        onAccepted:function(value){if(root.providers.length)root.queueProviderWrite(["dayflow","provider","set",root.currentProvider.id,root.promptKey,value])}
      }
      Note {id:overrideHelp; visible:root.section === "Prompts"; text:"Apply / Ctrl+Enter saves immediately. Blank uses the built-in prompt."}
      PagedText {
        visible:root.section === "Privacy"; width:parent.width; dayflow:root.dayflow; bodyHeight:right.height-84
        text:["Immediate completions read finished Claude/Codex turns locally. No model call. Watcher changes take effect after restarting capture.","Agent recaps are opt-in: scrubbed transcript excerpts go to your chat provider and configured decisions endpoint (OpenRouter by default). Agent lists read local stores either way.","Knowledge sync is opt-in: distilled journal/workstream summaries go to your configured Kurultai brain. No raw frames or transcripts. Set its destination in config.json.",root.syncStatus,root.dayflow.errorText,root.dayflow.notice].filter(function(v){return !!v}).join("\n\n")
      }
    }
  }
}
