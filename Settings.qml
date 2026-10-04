import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import QtQuick.Layouts

Column {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  property var presets: []
  property var providers: []
  property string usageText: dayflow ? dayflow.storageText : ""

  width: parent ? parent.width : 0
  spacing: Style.space(6)
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
    property real labelFraction: 0.37
    width: parent.width
    spacing: Style.space(5)
    height: Math.max(Style.space(26), Math.ceil(Math.max(12, Style.font.body) * 1.5) + Style.space(10))
    Text {
      width: parent.width * parent.labelFraction
      anchors.verticalCenter: parent.verticalCenter
      text: parent.label
      color: root.dayflow.dim
      font.family: root.dayflow.fontFamily
      font.pixelSize: Math.max(12, Style.font.caption)
    }
    Rectangle {
      width: parent.width - parent.width * parent.labelFraction - parent.spacing
      height: parent.height
      radius: Style.cornerRadius
      color: root.dayflow.fgFill(0.04)
      border.color: root.dayflow.fgFill(0.12)
      TextInput {
        anchors.fill: parent
        anchors.margins: Style.space(5)
        text: String(root.draft(parent.parent.key, parent.parent.fallback))
        echoMode: parent.parent.secret ? TextInput.Password : TextInput.Normal
        selectByMouse: true
        color: root.dayflow.foreground
        font.family: root.dayflow.fontFamily
        font.pixelSize: Math.max(12, Style.font.body)
        inputMethodHints: parent.parent.numeric ? Qt.ImhDigitsOnly : Qt.ImhNone
        onTextEdited: {
          var value = parent.parent.numeric ? parseInt(text, 10) : text
          if (!parent.parent.numeric || !isNaN(value)) root.setDraft(parent.parent.key, value)
        }
      }
    }
  }

  BusyBar { width: parent.width; pal: root.dayflow; active: modelsProc.running || providersProc.running || providerSetProc.running }
  Note { visible: !root.dayflow || !root.dayflow.configLoaded; text: "Loading settings…" }

  Row {
    visible: root.dayflow && root.dayflow.configLoaded
    width: parent.width
    spacing: Style.space(12)

    Column {
      width: (parent.width - 2 * parent.spacing) / 3
      spacing: Style.space(4)
      Row {
        spacing: Style.space(8)
        Heading { width: implicitWidth; anchors.verticalCenter: parent.verticalCenter; text: "AI provider" }
        CompactButton { dayflow: root.dayflow; text: root.showKey ? "Hide key" : "Show key"; onClicked: root.showKey = !root.showKey }
      }
      Field { label: "Provider"; key: "provider"; fallback: "openrouter" }
      Field { label: "Model"; key: "model"; fallback: "google/gemma-4-31b-it" }
      Field { label: "API URL"; key: "api_base_url" }
      Field { label: "API key"; key: "openrouter_api_key"; secret: !root.showKey }
      Field { label: "App name"; key: "site_name"; fallback: "dayflow-linux" }
      Note { text: "openrouter / local / custom / mcp. Keys stay masked. App name identifies calls to OpenRouter." }
      Row {
        spacing: Style.space(4)
        CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.presetIndex > 0; onClicked: root.presetIndex-- }
        Note { width: implicitWidth; anchors.verticalCenter: parent.verticalCenter; text: "Preset " + (root.presets.length ? root.presetIndex + 1 : 0) + " / " + root.presets.length }
        CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.presetIndex + 1 < root.presets.length; onClicked: root.presetIndex++ }
        CompactButton { dayflow: root.dayflow; text: "Use"; enabled: root.presets.length > 0; onClicked: root.setDraft("model", root.currentPreset.slug) }
      }
      PagedText { width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(50); text: [root.currentPreset.name, root.currentPreset.notes].filter(function(value) { return !!value }).join("\n") }
      Heading { text: "Privacy and automation" }
      Row {
        spacing: Style.space(6)
        CompactButton { dayflow: root.dayflow; text: "Agent recaps: " + (root.draft("agent_recaps", false) ? "On" : "Off"); active: root.draft("agent_recaps", false); onClicked: root.setDraft("agent_recaps", !root.draft("agent_recaps", false)) }
        CompactButton { dayflow: root.dayflow; text: "Sync: " + (root.draft("knowledge_sync", false) ? "On" : "Off"); active: root.draft("knowledge_sync", false); onClicked: root.setDraft("knowledge_sync", !root.draft("knowledge_sync", false)) }
      }
      Note { text: "Recaps are opt-in: scrubbed transcript excerpts go to your chat provider and OpenRouter's decisions endpoint. Agent lists read local stores either way." }
      Note { text: "Sync is opt-in: distilled journal/workstream summaries go to your configured Kurultai brain. No raw frames or transcripts. Configure its destination in config.json." }
      CompactButton {
        dayflow: root.dayflow
        text: syncNowProc.running ? "Syncing…" : "Sync now"
        enabled: root.dayflow.config.knowledge_sync === true && !syncNowProc.running
        onClicked: { root.syncStatus = ""; syncNowProc.running = true }
      }
    }

    Column {
      width: (parent.width - 2 * parent.spacing) / 3
      spacing: Style.space(4)
      Heading { text: "Capture and storage" }
      Note { text: "More frames mean more detail and more model usage. Zero retention/caps means no limit of that kind." }
      Grid {
        id: captureGrid
        width: parent.width
        columns: 2
        spacing: Style.space(4)
      Repeater {
        model: [
          {label: "Interval (s)", key: "capture_interval_sec", value: 10},
          {label: "Block (min)", key: "block_minutes", value: 15},
          {label: "Frames/block", key: "frames_per_block", value: 30},
          {label: "JPEG quality", key: "jpeg_quality", value: 55},
          {label: "Max dim (px)", key: "frame_max_dim", value: 1920},
          {label: "Retention days", key: "retention_days", value: 0},
          {label: "Frames (MB)", key: "max_frames_mb", value: 20480},
          {label: "Text (MB)", key: "max_db_mb", value: 10240},
          {label: "Total MB (old)", key: "max_storage_mb", value: 0}
        ]
        delegate: Field {
          width: (captureGrid.width - captureGrid.spacing) / 2
          required property var modelData
          label: modelData.label; key: modelData.key; fallback: modelData.value; numeric: true; labelFraction: 0.62
        }
      }
      }
      Note { text: "Stored: " + (root.usageText || "—") + ". Max dim 0 keeps native resolution." }
      Heading { text: "Category buckets" }
      Row {
        spacing: Style.space(4)
        CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.categoryIndex > 0; onClicked: root.categoryIndex-- }
        Note { width: implicitWidth; anchors.verticalCenter: parent.verticalCenter; text: root.categoryCount ? (root.categoryIndex + 1) + " / " + root.categoryCount : "No categories" }
        CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.categoryIndex + 1 < root.categoryCount; onClicked: root.categoryIndex++ }
        CompactButton { dayflow: root.dayflow; text: "+"; onClicked: { root.dayflow.settingsCatModel.append({name: "", description: "", color: ""}); root.categoryIndex = root.categoryCount - 1 } }
        CompactButton { dayflow: root.dayflow; text: "Remove"; enabled: root.categoryCount > 0; onClicked: root.dayflow.settingsCatModel.remove(root.categoryIndex) }
      }
      PagedText {
        visible: root.categoryCount > 0
        width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(26); readOnly: false
        text: root.currentCategory.name || ""
        onEdited: function(value) { root.dayflow.settingsCatModel.setProperty(root.categoryIndex, "name", value) }
      }
      PagedText {
        visible: root.categoryCount > 0
        width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(40); readOnly: false
        text: root.currentCategory.description || ""
        onEdited: function(value) { root.dayflow.settingsCatModel.setProperty(root.categoryIndex, "description", value) }
      }
    }

    Column {
      width: (parent.width - 2 * parent.spacing) / 3
      spacing: Style.space(4)
      Heading { text: "Classification instructions" }
      Note { text: "Extra instructions for every summary: describe what counts as work or personal." }
      PagedText {
        width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(66); readOnly: false
        text: root.draft("classification_prompt", "")
        onEdited: function(value) { root.setDraft("classification_prompt", value) }
      }
      Heading { text: "Provider prompt overrides" }
      Note { text: "Ctrl+Enter or leave a field to save. Blank uses the built-in prompt." }
      Row {
        spacing: Style.space(4)
        CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.providerIndex > 0; onClicked: root.providerIndex-- }
        Note { width: implicitWidth; anchors.verticalCenter: parent.verticalCenter; text: root.providers.length ? (root.providerIndex + 1) + " / " + root.providers.length : "No providers" }
        CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.providerIndex + 1 < root.providers.length; onClicked: root.providerIndex++ }
      }
      PagedText { visible: root.providers.length > 0; width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(26); text: root.currentProvider.name || root.currentProvider.id || "" }
      Grid {
        id: overrideGrid
        width: parent.width
        columns: 2
        spacing: Style.space(6)
      Repeater {
        model: root.providers.length ? [
          {label: "Title", key: "title_prompt"}, {label: "Summary", key: "summary_prompt"},
          {label: "Detailed", key: "detailed_prompt"}, {label: "Chat", key: "chat_prompt"}
        ] : []
        delegate: Column {
          required property var modelData
          width: (overrideGrid.width - overrideGrid.spacing) / 2
          spacing: Style.space(2)
          PagedText {
            label: modelData.label
            showApplyButton: false
            width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(26); readOnly: false
            text: root.overrideText(modelData.key)
            onEdited: function(value) { root.editOverride(modelData.key, value) }
            onAccepted: function(value) { root.queueProviderWrite(["dayflow", "provider", "set", root.currentProvider.id, modelData.key, value]) }
          }
        }
      }
      }
    }
  }

  Row {
    width: parent.width
    visible: root.dayflow && root.dayflow.configLoaded
    spacing: Style.space(12)
    Column {
      width: parent.width * 0.46
      spacing: Style.space(4)
      Row {
        spacing: Style.space(6)
        CompactButton { dayflow: root.dayflow; text: "Save settings"; active: true; enabled: !providerSetProc.running && root.pendingProviderWrites.length === 0; onClicked: root.dayflow.saveConfig() }
        CompactButton { dayflow: root.dayflow; text: "Reload"; onClicked: root.dayflow.loadConfig() }
      }
      Note { text: "Capture changes require a capture-service restart." }
    }
    PagedText {
      visible: text !== ""
      width: parent.width - parent.width * 0.46 - parent.spacing
      dayflow: root.dayflow; bodyHeight: Style.space(26)
      text: [root.syncStatus, root.dayflow ? root.dayflow.errorText || "" : "", root.dayflow ? root.dayflow.notice : ""].filter(function(value) { return value !== "" }).join("\n")
    }
  }
}
