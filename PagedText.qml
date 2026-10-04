import QtQuick
import qs.Commons

// Complete text in measured pages. Each page fits at the active theme's
// readable font size; neither long results nor multiline prompts are clipped.
Column {
  id: root
  property var dayflow: null
  property string text: ""
  property bool readOnly: true
  property bool showApplyButton: true
  property int bodyHeight: Style.space(54)
  readonly property int effectiveBodyHeight: Math.max(bodyHeight, Math.ceil(Math.max(12, Style.font.body) * 1.5) + Style.space(10))
  property string label: ""
  property int page: 0
  property var ranges: [{start: 0, end: 0}]
  property bool syncing: false
  property int desiredCursor: -1
  readonly property int pageCount: ranges.length
  readonly property string displayedText: editor.text
  signal edited(string value)
  signal accepted(string value)
  spacing: Style.space(3)

  function rebuild() {
    if (width <= 0) return
    var result = [], start = 0
    while (start < text.length) {
      var lo = 1, hi = Math.min(text.length - start, 4096), count = 1
      while (lo <= hi) {
        var mid = Math.floor((lo + hi) / 2)
        measure.text = text.substring(start, start + mid)
        measure.forceLayout()
        if (measure.implicitHeight <= effectiveBodyHeight - Style.space(10)) {
          count = mid; lo = mid + 1
        } else hi = mid - 1
      }
      // Do not split a UTF-16 surrogate pair across pages.
      var end = start + count
      if (end < text.length && end > start && text.charCodeAt(end - 1) >= 0xD800 && text.charCodeAt(end - 1) <= 0xDBFF) end--
      if (end <= start) end = Math.min(text.length, start + 2)
      result.push({start: start, end: end})
      start = end
    }
    ranges = result.length ? result : [{start: 0, end: 0}]
    if (desiredCursor >= 0) {
      for (var i = 0; i < ranges.length; i++) {
        if (desiredCursor <= ranges[i].end) { page = i; break }
      }
    }
    page = Math.max(0, Math.min(page, ranges.length - 1))
    syncEditor()
  }
  function syncEditor() {
    syncing = true
    var r = ranges[page]
    editor.text = text.substring(r.start, r.end)
    if (desiredCursor >= 0) editor.cursorPosition = Math.max(0, Math.min(editor.length, desiredCursor - r.start))
    syncing = false
    desiredCursor = -1
  }
  onTextChanged: rebuild()
  onWidthChanged: Qt.callLater(rebuild)
  onEffectiveBodyHeightChanged: Qt.callLater(rebuild)
  onPageChanged: syncEditor()
  Component.onCompleted: Qt.callLater(rebuild)

  Text {
    id: measure
    visible: false
    width: Math.max(1, root.width - Style.space(10))
    font: editor.font
    textFormat: Text.PlainText
    wrapMode: Text.WrapAnywhere
  }
  Rectangle {
    width: parent.width
    height: root.effectiveBodyHeight
    radius: Style.cornerRadius
    color: root.dayflow ? root.dayflow.fgFill(0.04) : "transparent"
    border.color: root.dayflow ? root.dayflow.fgFill(0.12) : Color.muted
    TextEdit {
      id: editor
      objectName: "pageEditor"
      anchors.fill: parent
      anchors.margins: Style.space(5)
      readOnly: root.readOnly
      selectByMouse: true
      textFormat: TextEdit.PlainText
      wrapMode: TextEdit.WrapAnywhere
      color: root.dayflow ? root.dayflow.foreground : Color.foreground
      font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
      font.pixelSize: Math.max(12, Style.font.body)
      onTextChanged: {
        if (root.syncing || root.readOnly) return
        var r = root.ranges[root.page]
        root.desiredCursor = r.start + cursorPosition
        root.edited(root.text.substring(0, r.start) + text + root.text.substring(r.end))
      }
      onActiveFocusChanged: if (!activeFocus && !root.readOnly) root.accepted(root.text)
      Keys.onPressed: function(event) {
        if ((event.modifiers & Qt.ControlModifier) && (event.key === Qt.Key_Return || event.key === Qt.Key_Enter)) {
          root.accepted(root.text); event.accepted = true
        }
      }
    }
  }
  Row {
    spacing: Style.space(5)
    visible: root.pageCount > 1 || !root.readOnly
    Text {
      anchors.verticalCenter: parent.verticalCenter
      text: root.label
      visible: text !== ""
      color: root.dayflow ? root.dayflow.dim : Color.muted
      font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
      font.pixelSize: Math.max(12, Style.font.caption)
    }
    CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.page > 0; onClicked: root.page-- }
    Text {
      anchors.verticalCenter: parent.verticalCenter
      text: (root.page + 1) + " / " + root.pageCount
      color: root.dayflow ? root.dayflow.dim : Color.muted
      font.family: root.dayflow ? root.dayflow.fontFamily : Style.font.family
      font.pixelSize: Math.max(12, Style.font.caption)
    }
    CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.page + 1 < root.pageCount; onClicked: root.page++ }
    CompactButton { visible: !root.readOnly && root.showApplyButton; dayflow: root.dayflow; text: "Apply"; onClicked: root.accepted(root.text) }
  }
}
