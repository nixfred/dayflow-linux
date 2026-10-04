import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons

// First-run wizard. Shown instead of the tab content when `status --json`
// reports the install unconfigured. Every step has a skip affordance —
// the wizard is an offer, never a gate.
Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  property int step: 0
  property var detected: ({})
  // "openrouter" | "local" | "custom"
  property string mode: "openrouter"
  property string apiKey: ""
  property string baseUrl: ""
  property int presetIndex: 0
  readonly property var presetCandidates: (detected.presets || []).filter(function(p){return root.mode === "openrouter" || p.slug.indexOf(":")>=0})
  readonly property var currentPreset: presetCandidates.length ? presetCandidates[Math.min(presetIndex,presetCandidates.length-1)] : ({})
  onPresetCandidatesChanged: presetIndex=Math.max(0,Math.min(presetIndex,presetCandidates.length-1))
  property string modelSlug: "google/gemma-4-31b-it"
  property var catPicks: ({})
  // Set only by the explicit "Enable recaps" click on the consent step —
  // the "Not now" and Back handlers clear it explicitly, so the patch
  // never carries agent_recaps: true unless the user opted in. The consent
  // step's apply writes the flag explicitly (true or false) so an existing
  // config's agent_recaps: true can't silently survive "Not now".
  property bool recapsOptIn: false
  property string testResult: ""
  property bool testing: false
  // True while any config write or its doctor check is still in flight —
  // the result actions stay hidden until the last apply settles, so a
  // queued consent patch can't be overtaken by the previous check's finish.
  property bool busy: root.testing || applyProc.running || root.applyQueued || keySetProc.running

  signal dismissed()

  width: parent ? parent.width : 0
  implicitHeight: col.implicitHeight + 64
  height: parent ? parent.height : implicitHeight

  Process {
    id: detectProc
    command: ["dayflow", "detect", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        try {
          root.detected = JSON.parse(text)
          if (root.detected.ollama) root.mode = "ollama"
        } catch (e) {
          root.detected = {}
        }
      }
    }
  }

  // config patch -> doctor --json for the connection test.
  // The patch JSON carries the API key, so it goes over stdin, not argv.
  Process {
    id: applyProc
    property string pendingPatch: ""
    property bool didStart: false
    stdinEnabled: true
    onStarted: { write(pendingPatch + "\n"); pendingPatch = ""; applyProc.didStart = true }
    onExited: function(exitCode) {
      if (exitCode === 0) {
        if (root.applyQueued) {
          root.applyQueued = false
          root.applyPatch()
          return
        }
        testProc.running = true
      } else {
        root.applyQueued = false
        root.testResult = "config write failed"
        root.testing = false
      }
    }
    // FailedToStart fires no exited/streamFinished — surface it instead of
    // stranding "Testing..." forever.
    onRunningChanged: {
      if (!applyProc.running && !applyProc.didStart) {
        root.testResult = "config write failed"
        root.testing = false
      }
      if (!applyProc.running) applyProc.didStart = false
    }
  }

  Process {
    id: testProc
    property bool didStart: false
    command: ["dayflow", "doctor", "--json"]
    onStarted: testProc.didStart = true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: {
        root.testing = false
        try {
          var d = JSON.parse(text)
          if (d.failures === 0) {
            root.testResult = "Connection OK — all checks passed."
          } else {
            var bad = []
            for (var i = 0; i < d.checks.length; i++) {
              if (d.checks[i].status === "fail") bad.push(d.checks[i].name)
            }
            root.testResult = "Failing checks: " + bad.join(", ")
          }
        } catch (e) {
          root.testResult = "doctor did not return JSON"
        }
      }
    }
    onRunningChanged: {
      if (!testProc.running && !testProc.didStart) {
        root.testResult = "connection test failed to run"
        root.testing = false
      }
      if (!testProc.running) testProc.didStart = false
    }
    stderr: StdioCollector {}
  }

  Process {
    id: installProc
    command: ["dayflow", "install"]
    onExited: function(exitCode) {
      root.testResult += exitCode === 0 ? " Services installed." : " Install failed — run `dayflow install` in a terminal."
      if (root.dayflow) root.dayflow.loadConfig()
    }
  }

  // The API key goes to OmaSeal over stdin when the keyring is available;
  // on failure we fall back to embedding it in the config patch.
  Process {
    id: keySetProc
    property string pendingKey: ""
    property bool didStart: false
    stdinEnabled: true
    onStarted: { write(pendingKey + "\n"); pendingKey = ""; keySetProc.didStart = true }
    onExited: function(exitCode) {
      if (exitCode !== 0) root.keyInPatch = true
      root.applyPatch()
    }
    // FailedToStart — keep the chain moving via the config-patch fallback.
    onRunningChanged: {
      if (!keySetProc.running && !keySetProc.didStart) {
        root.keyInPatch = true
        root.applyPatch()
      }
      if (!keySetProc.running) keySetProc.didStart = false
    }
  }

  property bool keyInPatch: false
  // Set by apply() when the agent-recaps consent decision is being applied;
  // applyPatch reads it so the decision survives the keySetProc and queued
  // re-apply chains.
  property bool consentApply: false
  // Set when apply() is re-invoked while a config write is in flight —
  // applyProc re-enters applyPatch on exit so the latest patch (e.g.
  // recapsOptIn flipped on the consent step) is never dropped.
  property bool applyQueued: false

  function apply(consentStep) {
    // consentStep is passed (true or false) only by the agent-recaps
    // decision buttons; every other caller leaves it undefined. When it is
    // defined, the patch writes agent_recaps explicitly so the user's
    // choice always lands — an existing agent_recaps: true can't silently
    // survive "Not now".
    root.consentApply = (consentStep !== undefined)
    if (dayflow) dayflow.uilog("onboarding apply " + root.mode)
    root.testing = true
    root.testResult = "Testing..."
    if (root.mode === "openrouter" && root.apiKey !== "") {
      root.keyInPatch = false
      keySetProc.pendingKey = root.apiKey
      keySetProc.command = ["dayflow", "key", "set", "openrouter"]
      keySetProc.running = true
      return
    }
    root.applyPatch()
  }

  function applyPatch() {
    // Process.running = true is a no-op while the process is running, so
    // a re-apply during an in-flight write must queue instead. The patch
    // is built from live state when it actually sends, so the queued
    // write always carries the newest picks.
    if (applyProc.running) {
      root.applyQueued = true
      return
    }
    var patch = { model: root.modelSlug }
    if (root.mode === "openrouter") {
      patch.provider = "openrouter"
      if (root.keyInPatch) patch.openrouter_api_key = root.apiKey
      patch.api_base_url = ""
    } else if (root.mode === "ollama" || root.mode === "lmstudio") {
      patch.provider = "local"
      patch.api_base_url = root.mode === "lmstudio"
        ? "http://localhost:1234/v1" : "http://localhost:11434/v1"
      patch.openrouter_api_key = ""
    } else {
      patch.provider = "custom"
      patch.api_base_url = root.baseUrl
      patch.openrouter_api_key = ""
    }
    var cats = []
    var keys = Object.keys(root.catPicks)
    for (var i = 0; i < keys.length; i++) {
      if (root.catPicks[keys[i]]) cats.push(keys[i])
    }
    if (cats.length > 0) {
      patch.categories = cats.map(function(n) { return { name: n, description: n } })
    }
    // The consent step's Enable/Not now is the only path that sets this —
    // every other apply omits the key, which patchConfig preserves.
    if (root.consentApply) patch.agent_recaps = root.recapsOptIn
    applyProc.pendingPatch = JSON.stringify(patch)
    applyProc.command = ["dayflow", "config", "patch", "-"]
    applyProc.running = true
  }

  Component.onCompleted: detectProc.running = true

  DashboardCard {
    anchors.fill: parent; dayflow: root.dayflow; title: "Get started"
  Column {
    id: col
    width: parent.width
    spacing: Style.space(10)

    // ---- step 0: welcome ----
    Column {
      visible: root.step === 0
      width: parent.width
      spacing: Style.space(10)

      Text {
        width: parent.width
        text: "Welcome to Dayflow"
        color: root.dayflow ? root.dayflow.foreground : Color.foreground
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Style.font.title
        font.bold: true
      }
      Text {
        width: parent.width
        text: "Dayflow journals your screen activity locally and summarizes it with an AI model. To get started, pick where the model runs."
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body)
        wrapMode: Text.WordWrap
      }
      Row {
        spacing: Style.space(6)
        Rectangle {
          width: goText.implicitWidth + Style.space(16)
          height: goText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.accentFill(0.16) : "transparent"
          border.color: root.dayflow ? root.dayflow.accentFill(0.5) : "transparent"
          Text { id: goText; anchors.centerIn: parent; text: "Get started"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body); font.bold: true }
          MouseArea { anchors.fill: parent; onClicked: root.step = 1 }
        }
        Rectangle {
          width: skipText.implicitWidth + Style.space(16)
          height: skipText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.btnBg(skipMa.containsMouse) : "transparent"
          border.color: root.dayflow ? root.dayflow.fgFill(0.12) : "transparent"
          Text { id: skipText; anchors.centerIn: parent; text: "Skip for now"
            color: root.dayflow ? root.dayflow.dim : Color.muted
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body) }
          MouseArea { id: skipMa; anchors.fill: parent; hoverEnabled: true; onClicked: root.dismissed() }
        }
      }
    }

    // ---- step 1: provider ----
    Column {
      visible: root.step === 1
      width: parent.width
      spacing: Style.space(8)

      Text {
        width: parent.width
        text: "Where should the model run?"
        color: root.dayflow ? root.dayflow.foreground : Color.foreground
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body); font.bold: true
      }

      Flow {
        width: parent.width
        spacing: Style.space(6)
        Repeater {
          model: {
            var opts = [{ id: "openrouter", label: "OpenRouter (cloud)" }]
            if (root.detected.ollama) opts.push({ id: "ollama", label: "Ollama (detected)" })
            if (root.detected.lmstudio) opts.push({ id: "lmstudio", label: "LM Studio (detected)" })
            if (!root.detected.ollama && !root.detected.lmstudio)
              opts.push({ id: "ollama", label: "Local endpoint (Ollama default)" })
            opts.push({ id: "custom", label: "Custom endpoint" })
            return opts
          }
          delegate: Rectangle {
            width: modeLabel.implicitWidth + Style.space(14)
            height: modeLabel.implicitHeight + Style.space(8)
            radius: 8
            color: root.mode === modelData.id
              ? (root.dayflow ? root.dayflow.accentFill(0.16) : "transparent")
              : (modeMa.containsMouse && root.dayflow ? root.dayflow.accentFill(0.08) : "transparent")
            border.color: root.mode === modelData.id
              ? (root.dayflow ? root.dayflow.accentFill(0.5) : "transparent")
              : (root.dayflow ? root.dayflow.fgFill(0.12) : "transparent")
            Text { id: modeLabel; anchors.centerIn: parent; text: modelData.label
              color: root.dayflow ? root.dayflow.foreground : Color.foreground
              font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
              font.pixelSize: Math.max(12, Style.font.caption) }
            MouseArea { id: modeMa; anchors.fill: parent; hoverEnabled: true
              onClicked: root.mode = modelData.id }
          }
        }
      }

      Text {
        visible: root.mode === "openrouter"
        width: parent.width
        text: "Paste an OpenRouter key (sk-or-...). Get one at openrouter.ai/keys."
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.caption)
        wrapMode: Text.WordWrap
      }
      Rectangle {
        visible: root.mode === "openrouter"
        width: parent.width
        height: keyIn.implicitHeight + Style.space(8)
        radius: 8
        color: root.dayflow ? root.dayflow.fgFill(0.04) : "transparent"
        border.color: root.dayflow ? root.dayflow.fgFill(0.14) : "transparent"
        TextInput { id: keyIn; anchors.fill: parent; anchors.margins: Style.space(5)
          text: root.apiKey; echoMode: TextInput.Password; clip: true
          color: root.dayflow ? root.dayflow.foreground : Color.foreground
          font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
          font.pixelSize: Math.max(12, Style.font.body)
          onTextChanged: root.apiKey = text }
      }

      PagedText {
        visible: root.mode === "custom"; width: parent.width; dayflow: root.dayflow; readOnly: false; showApplyButton: false; label: "Endpoint URL"; bodyHeight: 28
        text: root.baseUrl; onEdited: function(value) { root.baseUrl = value }
      }

      Row {
        spacing: Style.space(6)
        Rectangle {
          width: nextText.implicitWidth + Style.space(16)
          height: nextText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.accentFill(0.16) : "transparent"
          border.color: root.dayflow ? root.dayflow.accentFill(0.5) : "transparent"
          opacity: (root.mode === "openrouter" && root.apiKey === "")
            || (root.mode === "custom" && root.baseUrl === "") ? 0.45 : 1
          Text { id: nextText; anchors.centerIn: parent; text: "Continue"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body); font.bold: true }
          MouseArea { anchors.fill: parent
            enabled: parent.opacity === 1
            onClicked: {
              if (root.mode !== "openrouter" && root.modelSlug.indexOf("/") >= 0)
                root.modelSlug = "gemma3:4b"
              root.step = 2
            } }
        }
        Text {
          anchors.verticalCenter: parent.verticalCenter
          text: "Skip setup"
          color: root.dayflow ? root.dayflow.dim : Color.muted
          font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
          font.pixelSize: Math.max(12, Style.font.caption)
          font.underline: true
          MouseArea { anchors.fill: parent; onClicked: root.dismissed() }
        }
      }
    }

    // ---- step 2: model + categories ----
    Column {
      visible: root.step === 2
      width: parent.width
      spacing: Style.space(8)

      Text {
        width: parent.width
        text: "Pick a vision model"
        color: root.dayflow ? root.dayflow.foreground : Color.foreground
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body); font.bold: true
      }
      Row {
        spacing: 8
        CompactButton {dayflow:root.dayflow; text:"‹"; enabled:root.presetIndex>0; onClicked:root.presetIndex--}
        Text {anchors.verticalCenter:parent.verticalCenter; text:root.presetCandidates.length ? (root.presetIndex+1)+" / "+root.presetCandidates.length : "No presets"; color:root.dayflow.dim; font.family:root.dayflow.fontFamily; font.pixelSize:Math.max(12,Style.font.caption)}
        CompactButton {dayflow:root.dayflow; text:"›"; enabled:root.presetIndex+1<root.presetCandidates.length; onClicked:root.presetIndex++}
        CompactButton {dayflow:root.dayflow; text:"Use this model"; enabled:root.presetCandidates.length>0; active:root.modelSlug===root.currentPreset.slug; onClicked:root.modelSlug=root.currentPreset.slug}
      }
      PagedText {width:parent.width; dayflow:root.dayflow; bodyHeight:28; text:root.currentPreset.name || "Configure a vision model in Settings."}

      Text {
        width: parent.width
        text: "Categories to track (optional — defaults work fine)"
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.caption)
        wrapMode: Text.WordWrap
      }
      Flow {
        width: parent.width
        spacing: Style.space(6)
        Repeater {
          model: ["coding", "browsing", "communication", "writing", "design",
                  "media", "meetings", "system", "personal"]
          delegate: Rectangle {
            width: catLabel.implicitWidth + Style.space(14)
            height: catLabel.implicitHeight + Style.space(6)
            radius: 8
            color: root.catPicks[modelData]
              ? (root.dayflow ? root.dayflow.accentFill(0.16) : "transparent")
              : "transparent"
            border.color: root.catPicks[modelData]
              ? (root.dayflow ? root.dayflow.accentFill(0.5) : "transparent")
              : (root.dayflow ? root.dayflow.fgFill(0.12) : "transparent")
            Text { id: catLabel; anchors.centerIn: parent; text: modelData
              color: root.dayflow ? root.dayflow.foreground : Color.foreground
              font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
              font.pixelSize: Math.max(12, Style.font.caption) }
            MouseArea { anchors.fill: parent
              onClicked: {
                var p = Object.assign({}, root.catPicks)
                p[modelData] = !p[modelData]
                root.catPicks = p
              } }
          }
        }
      }

      Row {
        spacing: Style.space(6)
        Rectangle {
          width: finText.implicitWidth + Style.space(16)
          height: finText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.accentFill(0.16) : "transparent"
          border.color: root.dayflow ? root.dayflow.accentFill(0.5) : "transparent"
          Text { id: finText; anchors.centerIn: parent; text: "Continue"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body); font.bold: true }
          // Continue writes the config now — dismissing on the consent
          // step must not discard provider/model/key. The patch omits
          // agent_recaps unless recapsOptIn, so writing here is not a
          // consent leak; step 3's buttons just re-apply.
          MouseArea { anchors.fill: parent; onClicked: {
            root.apply()
            var agents = root.detected.agents || {}
            var anyAgent = false
            for (var k in agents) {
              if (agents[k]) { anyAgent = true; break }
            }
            root.step = anyAgent ? 3 : 4
          } }
        }
        Text {
          anchors.verticalCenter: parent.verticalCenter
          text: "Back"
          color: root.dayflow ? root.dayflow.dim : Color.muted
          font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
          font.pixelSize: Math.max(12, Style.font.caption)
          font.underline: true
          MouseArea { anchors.fill: parent; onClicked: root.step = 1 }
        }
      }
    }

    // ---- step 3: agent-session recaps opt-in ----
    // The base config was already written by step 2's Continue (the patch
    // omits agent_recaps), so this step only decides whether a follow-up
    // patch turns recaps on: Enable applies agent_recaps=true; "Not now"
    // applies agent_recaps=false (so an existing enabled value can't
    // survive the explicit choice); Back clears the flag and returns.
    // Shown only when detect saw an agent store.
    Column {
      visible: root.step === 3
      width: parent.width
      spacing: Style.space(8)

      Text {
        width: parent.width
        text: "Agent-session recaps (optional)"
        color: root.dayflow ? root.dayflow.foreground : Color.foreground
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body); font.bold: true
      }
      Text {
        width: parent.width
        text: "Dayflow can write a one-line recap of each coding-agent session it finds in Claude Code, Codex, OpenCode, Devin, and Cursor. Transcripts are always read locally to build the session list — that happens either way, on or off."
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body)
        wrapMode: Text.WordWrap
      }
      Text {
        width: parent.width
        text: "When recaps are on, a bounded, scrubbed transcript excerpt leaves your machine — to your configured chat provider, which writes the recap, and to OpenRouter's decisions endpoint, which judges which sessions are worth summarizing. Recaps are off by default; nothing extra is sent unless you enable them here or later in Settings."
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.body)
        wrapMode: Text.WordWrap
      }

      Row {
        spacing: Style.space(6)
        Rectangle {
          width: enText.implicitWidth + Style.space(16)
          height: enText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.accentFill(0.16) : "transparent"
          border.color: root.dayflow ? root.dayflow.accentFill(0.5) : "transparent"
          Text { id: enText; anchors.centerIn: parent; text: "Enable recaps"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body); font.bold: true }
          MouseArea { anchors.fill: parent; onClicked: { root.recapsOptIn = true; root.apply(true); root.step = 4 } }
        }
        Rectangle {
          width: offText.implicitWidth + Style.space(16)
          height: offText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.btnBg(offMa.containsMouse) : "transparent"
          border.color: root.dayflow ? root.dayflow.fgFill(0.12) : "transparent"
          Text { id: offText; anchors.centerIn: parent; text: "Not now"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body) }
          MouseArea { id: offMa; anchors.fill: parent; hoverEnabled: true; onClicked: { root.recapsOptIn = false; root.apply(false); root.step = 4 } }
        }
        Text {
          anchors.verticalCenter: parent.verticalCenter
          text: "Back"
          color: root.dayflow ? root.dayflow.dim : Color.muted
          font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
          font.pixelSize: Math.max(12, Style.font.caption)
          font.underline: true
          MouseArea { anchors.fill: parent; onClicked: { root.recapsOptIn = false; root.step = 2 } }
        }
      }
    }

    // ---- step 4: result ----
    Column {
      visible: root.step === 4
      width: parent.width
      spacing: Style.space(8)

      PagedText {
        width: parent.width
        dayflow: root.dayflow
        bodyHeight: Style.space(80)
        text: root.testing ? "Checking..." : root.testResult
      }
      Text {
        visible: !root.busy
        width: parent.width
        text: "Install services to start capturing in the background, or open the panel and finish setup in Settings."
        color: root.dayflow ? root.dayflow.dim : Color.muted
        font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
        font.pixelSize: Math.max(12, Style.font.caption)
        wrapMode: Text.WordWrap
      }
      Row {
        spacing: Style.space(6)
        Rectangle {
          visible: !root.busy
          width: instText.implicitWidth + Style.space(16)
          height: instText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.accentFill(0.16) : "transparent"
          border.color: root.dayflow ? root.dayflow.accentFill(0.5) : "transparent"
          Text { id: instText; anchors.centerIn: parent; text: "Install services"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body); font.bold: true }
          MouseArea { anchors.fill: parent; onClicked: installProc.running = true }
        }
        Rectangle {
          visible: !root.busy
          width: doneText.implicitWidth + Style.space(16)
          height: doneText.implicitHeight + Style.space(8)
          radius: 8
          color: root.dayflow ? root.dayflow.btnBg(doneMa.containsMouse) : "transparent"
          border.color: root.dayflow ? root.dayflow.fgFill(0.12) : "transparent"
          Text { id: doneText; anchors.centerIn: parent; text: "Done"
            color: root.dayflow ? root.dayflow.foreground : Color.foreground
            font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
            font.pixelSize: Math.max(12, Style.font.body) }
          MouseArea { id: doneMa; anchors.fill: parent; hoverEnabled: true
            onClicked: {
              if (root.dayflow) root.dayflow.loadConfig()
              root.dismissed()
            } }
        }
      }
    }
  }
  }
}
