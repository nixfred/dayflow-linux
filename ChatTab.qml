import QtQuick
import Quickshell.Io
import qs.Commons

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  width: parent ? parent.width : 0
  height: parent ? parent.height : implicitHeight
  implicitHeight: 360
  property var chatMessages: []
  property int chatConversation: 0
  property bool chatLoading: false
  property var conversations: []
  property string chatInput: ""
  property string lastSent: ""
  property string chatAttribution: ""

  // Instant local recap from the already-loaded timeline spans — no LLM call.
  function recapText() {
    var spans = (dayflow && dayflow.spans) ? dayflow.spans : []
    if (spans.length === 0) return ""
    var byCat = {}
    var total = 0
    for (var i = 0; i < spans.length; i++) {
      var m = Number(spans[i].minutes || 0)
      total += m
      var c = spans[i].category || "other"
      byCat[c] = (byCat[c] || 0) + m
    }
    if (total <= 0) return ""
    var cats = Object.keys(byCat).sort(function(a, b) { return byCat[b] - byCat[a] }).slice(0, 3)
    var parts = []
    for (i = 0; i < cats.length; i++)
      parts.push(dayflow.catDisplay(cats[i]) + " " + dayflow.fmtDur(byCat[cats[i]]))
    var last = spans[spans.length - 1]
    var head = (dayflow.dateLabel || "Today") + " · " + dayflow.fmtDur(total) + " tracked — " + parts.join(" · ")
    if (last && last.title) head += "\nLatest: " + last.title
    return head
  }

  function applyChat(raw) {
    root.chatLoading = false
    try {
      var d = JSON.parse(raw)
      root.chatMessages = d.messages || []
      root.chatConversation = Number(d.conversation_id || 0)
      root.chatAttribution = d.provider ? d.provider + " · " + (d.model || "") : ""
      if (dayflow) dayflow.notice = ""
      conversationsProc.running = true // a new conversation may have been created
    } catch (e) {
      root.chatInput = root.lastSent
      if (dayflow) dayflow.notice = "chat failed"
    }
  }

  function applyConversations(raw) {
    try {
      root.conversations = JSON.parse(raw)
    } catch (e) {
      root.conversations = []
    }
  }

  function applyConversation(raw) {
    root.chatLoading = false
    try {
      var d = JSON.parse(raw)
      root.chatMessages = d.messages || []
      if (dayflow) dayflow.notice = ""
    } catch (e) {
      if (dayflow) dayflow.notice = "could not load conversation"
    }
  }

  function loadConversation(id) {
    if (dayflow) dayflow.uilog("chat load conv " + id)
    // chatProc and convProc both write chatConversation/chatMessages —
    // switching while either is active lets completions land out of order.
    if (chatProc.running || convProc.running) return
    root.chatConversation = id
    root.chatMessages = []
    root.chatAttribution = ""
    if (id <= 0) return
    root.chatLoading = true
    convProc.command = ["dayflow", "conversation", String(id), "--json"]
    convProc.running = true
  }

  function newConversation() {
    if (dayflow) dayflow.uilog("chat new")
    if (chatProc.running || convProc.running) return
    root.chatConversation = 0
    root.chatMessages = []
    root.chatAttribution = ""
  }

  function sendChat() {
    if (root.chatInput.trim() === "" || root.chatLoading || chatProc.running || convProc.running) return
    if (dayflow) dayflow.uilog("chat send")
    root.chatLoading = true
    root.lastSent = root.chatInput
    var cmd = ["dayflow", "chat", root.chatInput, "--json"]
    if (root.chatConversation > 0) {
      cmd = ["dayflow", "chat", root.chatInput, "--conversation-id", String(root.chatConversation), "--json"]
    }
    chatProc.command = cmd
    chatProc.running = true
    root.chatInput = ""
  }


  Row {
    y: 0; width: parent.width; height: parent.height; spacing: 12
    DashboardCard {
      id: compose; width: (parent.width-parent.spacing)*0.36; height: parent.height; dayflow: root.dayflow; title: "Ask about your work"
      Row {
        spacing: 8
        CompactButton { dayflow: root.dayflow; text: "New"; enabled: !root.chatLoading; onClicked: root.newConversation() }
        CompactButton { dayflow: root.dayflow; text: root.chatLoading ? "Working…" : "Send"; active: true; enabled: !root.chatLoading && root.chatInput.trim() !== ""; onClicked: root.sendChat() }
      }
      PagedText { width: parent.width; dayflow: root.dayflow; label: "Question"; readOnly: false; acceptOnBlur: false; showApplyButton: false; bodyHeight: Math.max(28,(compose.height-220)/2); text: root.chatInput; onEdited: function(value) { root.chatInput=value }
      onAccepted: if (!root.chatLoading) root.sendChat() }
      Flow {
        width: parent.width; spacing: 5
        Repeater {
          model: ["Summarize today", "Draft my standup", "Where did I lose focus this week?"]
          CompactButton { required property string modelData; dayflow: root.dayflow; text: modelData; enabled: !root.chatLoading; onClicked: { root.chatInput=modelData; root.sendChat() } }
        }
      }
      PagedText { width: parent.width; dayflow: root.dayflow; bodyHeight: Math.max(28,(compose.height-220)/2); text: root.recapText() || "Ask a question to get started. Timeline facts are available locally; model answers use your configured chat provider." }
    }
    Column {
      width: (parent.width-parent.spacing)*0.64; height: parent.height; spacing: 10
      DashboardCard {
        id: history; width: parent.width; height: 114; dayflow: root.dayflow; title: "Saved conversations"
        Row {
          width: parent.width; spacing: 6
          CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.conversationIndex>0; onClicked: root.conversationIndex-- }
          PagedText { width: parent.width-210; bodyHeight: 28; dayflow: root.dayflow; text: root.conversations.length ? root.conversations[root.conversationIndex].title || "Untitled conversation" : "No saved conversations" }
          CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.conversationIndex+1<root.conversations.length; onClicked: root.conversationIndex++ }
          CompactButton { dayflow: root.dayflow; text: "Open"; enabled: root.conversations.length>0 && !root.chatLoading; onClicked: root.loadConversation(Number(root.conversations[root.conversationIndex].id)) }
        }
      }
      RecordCard {
        id: messages; width: parent.width; height: parent.height-history.height-parent.spacing; dayflow: root.dayflow; title: "Conversation"; followLatest: true
        records: root.chatMessages.filter(function(msg) {return msg.role !== "tool" && !(msg.role === "assistant" && msg.tool_calls && msg.tool_calls.length>0)})
        emptyText: root.chatLoading ? "Waiting for the configured provider…" : "Your conversation will appear here."
        formatRecord: function(record) { return (record.role === "user" ? "You" : "Assistant") + (root.chatAttribution ? " · "+root.chatAttribution : "") + "\n\n" + (record.content || "") }
      }
    }
  }
  property int conversationIndex: 0
  onConversationsChanged: conversationIndex = Math.max(0,Math.min(conversationIndex,conversations.length-1))
  // Provider calls are triggered only by an explicit Send or question action.
  Process {
    id: chatProc
    command: ["dayflow", "chat", "hello", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyChat(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) {
        root.chatLoading = false
        root.chatInput = root.lastSent // restore for retry
        if (dayflow) dayflow.notice = "chat error"
      }
    }
    // FailedToStart emits neither exited nor streamFinished — clear the
    // flag so the busy bar can't stick.
    onRunningChanged: {
      if (!chatProc.running) root.chatLoading = false
    }
  }

  Process {
    id: convProc
    command: ["dayflow", "conversation", "0", "--json"]
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyConversation(text)
    }
    onExited: function(exitCode) {
      if (exitCode !== 0) {
        root.chatLoading = false
        if (dayflow) dayflow.notice = "could not load conversation"
      }
    }
    onRunningChanged: {
      if (!convProc.running) root.chatLoading = false
    }
  }

  Process {
    id: conversationsProc
    command: ["dayflow", "conversations", "--json"]
    running: true
    stdout: StdioCollector {
      waitForEnd: true
      onStreamFinished: root.applyConversations(text)
    }
  }
}
