import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons

Item {
  id: root
  property var dayflow: parent && parent.panel ? parent.panel : null
  width: parent.width
  implicitHeight: col.implicitHeight
  height: implicitHeight
  property int cardIndex: 0
  readonly property int cardCount: dayflow ? dayflow.spans.length : 0
  onCardCountChanged: cardIndex = Math.max(0,Math.min(cardIndex,cardCount-1))

  Column {
    id: col
    width: parent.width
    spacing: Style.space(8)

    BusyBar {
      width: parent.width
      pal: dayflow
      active: dayflow.timelineLoading
    }

    CompletionFeed { width: parent.width; dayflow: root.dayflow; records: root.dayflow.completions || []; visible: records.length > 0 }

    // ---- day switcher ----
    Row {
      width: parent.width
      spacing: Style.space(4)

      Rectangle {
        height: Style.space(28)
        width: Style.space(28)
        radius: Style.cornerRadius
        color: dPrev.containsMouse ? dayflow.accentFill(0.12) : dayflow.fgFill(0.04)
        border.color: dayflow.accentFill(0.4)
        Text {
          anchors.centerIn: parent
          text: "‹"
          textFormat: Text.PlainText
          color: dayflow.foreground
          font.family: dayflow.fontFamily
          font.pixelSize: Style.font.body
        }
        MouseArea {
          id: dPrev
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: dayflow.goDay(-1)
        }
      }

      Text {
        text: dayflow.viewDateLabel()
        textFormat: Text.PlainText
        color: dayflow.foreground
        font.family: dayflow.fontFamily
        font.pixelSize: Style.font.body
        font.bold: true
        anchors.verticalCenter: parent.verticalCenter
      }

      Rectangle {
        height: Style.space(28)
        width: Style.space(28)
        radius: Style.cornerRadius
        color: dNext.containsMouse ? dayflow.accentFill(0.12) : dayflow.fgFill(0.04)
        border.color: dayflow.accentFill(0.4)
        opacity: dayflow.dayOffset < 0 ? 1 : 0.4
        Text {
          anchors.centerIn: parent
          text: "›"
          textFormat: Text.PlainText
          color: dayflow.foreground
          font.family: dayflow.fontFamily
          font.pixelSize: Style.font.body
        }
        MouseArea {
          id: dNext
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          enabled: dayflow.dayOffset < 0
          onClicked: dayflow.goDay(1)
        }
      }

      Text {
        visible: dayflow.dayOffset !== 0
        text: "back to today"
        textFormat: Text.PlainText
        color: backToday.containsMouse ? dayflow.foreground : dayflow.dim
        font.family: dayflow.fontFamily
        font.pixelSize: Style.font.caption
        font.underline: backToday.containsMouse
        anchors.verticalCenter: parent.verticalCenter
        MouseArea {
          id: backToday
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: { dayflow.uilog("back to today"); dayflow.dayOffset = 0; dayflow.loadTimeline() }
        }
      }

      Rectangle {
        height: Style.space(28)
        width: calLbl.implicitWidth + Style.space(14)
        radius: Style.cornerRadius
        color: calPicker.visible ? dayflow.accentFill(0.15) : dayflow.fgFill(0.04)
        border.color: dayflow.accentFill(0.4)
        Text {
          id: calLbl
          anchors.centerIn: parent
          text: "Cal"
          textFormat: Text.PlainText
          color: dayflow.foreground
          font.family: dayflow.fontFamily
          font.pixelSize: Style.font.caption
        }
        MouseArea {
          anchors.fill: parent
          hoverEnabled: true
          cursorShape: Qt.PointingHandCursor
          onClicked: { calPicker.visible = !calPicker.visible; dayflow.uilog("calendar " + (calPicker.visible ? "open" : "close")) }
        }
      }
    }

    // ---- copy actions for the viewed day ----
    Flow {
      width: parent.width
      spacing: Style.space(4)

      Repeater {
        // Copies the currently viewed day, not always today.
        model: [
          { label: "Copy day · md", proc: "copyProc", logName: "copy day md", extra: "" },
          { label: "Copy day · mini", proc: "copyMiniProc", logName: "copy day mini", extra: " --brief" }
        ]
        delegate: CopyButton {
          required property var modelData
          dayflow: root.dayflow
          label: modelData.label
          proc: root.dayflow.procByName(modelData.proc)
          onActivated: {
            root.dayflow.uilog(modelData.logName + " " + root.dayflow.viewDateStr())
            proc.command = ["bash", "-c",
              "dayflow export " + root.dayflow.viewDateStr() + modelData.extra + " | wl-copy"]
            proc.running = true
          }
        }
      }
    }

    // ---- calendar picker ----
    CalendarPicker {
      id: calPicker
      visible: false
      width: parent.width
      dayflow: root.dayflow
    }

    // ---- this-week day jump ----
    Flow {
      width: parent.width
      spacing: Style.space(3)

      Repeater {
        model: 7
        delegate: Rectangle {
          property int offset: index - dayflow.todayIndex()
          height: Style.space(22)
          width: chipLbl.implicitWidth + Style.space(10)
          radius: Style.cornerRadius
          opacity: offset > 0 ? 0.4 : 1
          color: dayflow.dayOffset === offset
            ? dayflow.accentFill(0.18)
            : (chipMa.containsMouse ? dayflow.accentFill(0.08) : "transparent")
          border.color: dayflow.dayOffset === offset
            ? dayflow.accentFill(0.5) : dayflow.fgFill(0.12)

          Text {
            id: chipLbl
            anchors.centerIn: parent
            text: dayflow.dayName(index)
            textFormat: Text.PlainText
            color: dayflow.dayOffset === offset ? dayflow.foreground : dayflow.dim
            font.family: dayflow.fontFamily
            font.pixelSize: Style.font.caption
          }
          MouseArea {
            id: chipMa
            anchors.fill: parent
            hoverEnabled: true
            enabled: offset <= 0
            cursorShape: Qt.PointingHandCursor
            onClicked: { dayflow.uilog("week chip " + modelData); dayflow.dayOffset = offset; dayflow.loadTimeline() }
          }
        }
      }
    }

    Text {
      visible: dayflow.spans.length === 0 && dayflow.completions.length === 0 && dayflow.errorText === "" && dayflow.configured
      width: parent.width
      text: "Nothing summarized yet — blocks land once per block interval (15 min by default)."
      textFormat: Text.PlainText
      color: dayflow.dim
      font.family: dayflow.fontFamily
      font.pixelSize: Style.font.body
      wrapMode: Text.WordWrap
    }

    Row {
      visible: root.cardCount > 0
      spacing: Style.space(5)
      CompactButton { dayflow: root.dayflow; text: "‹"; enabled: root.cardIndex > 0; onClicked: root.cardIndex-- }
      Text { anchors.verticalCenter: parent.verticalCenter; text: "Screen summaries · " + (root.cardIndex+1) + " / " + root.cardCount; color: root.dayflow.dim; font.family: root.dayflow.fontFamily; font.pixelSize: Math.max(12,Style.font.caption) }
      CompactButton { dayflow: root.dayflow; text: "›"; enabled: root.cardIndex+1 < root.cardCount; onClicked: root.cardIndex++ }
    }
    Repeater {
      model: dayflow.spans.slice(root.cardIndex, root.cardIndex+1)

      delegate: Rectangle {
        id: cardRoot
        width: col.width
        height: cardCol.implicitHeight + Style.space(14)
        radius: Style.cornerRadius
        color: dayflow.fgFill(0.04)
        border.color: dayflow.fgFill(0.08)
        clip: true

        property bool editing: false
        property string editTitle: ""
        property string editCategory: ""
        property bool editProd: false

        function beginEdit() {
          editTitle = modelData.title || ""
          editCategory = modelData.category || ""
          editProd = modelData.productive === true
          editing = true
        }

        // Category-colored edge so spans scan by activity type.
        Rectangle {
          anchors.left: parent.left
          anchors.top: parent.top
          anchors.bottom: parent.bottom
          width: Style.space(3)
          color: dayflow.categoryColor(modelData.category)
        }

        Column {
          id: cardCol
          width: parent.width - Style.space(20)
          anchors.centerIn: parent
          anchors.horizontalCenterOffset: Style.space(3)
          spacing: Style.space(3)

          Row {
            width: parent.width
            spacing: Style.space(8)

            Image {
              id: appIcon
              width: Style.space(14)
              height: Style.space(14)
              source: dayflow.appIcon(modelData.app)
              visible: status === Image.Ready
              anchors.verticalCenter: parent.verticalCenter
            }

            Text {
              width: parent.width - appIcon.width - categoryPill.width - (productiveMark.visible ? productiveMark.width : 0) - (lowConfMark.visible ? lowConfMark.width : 0) - editText.implicitWidth - parent.spacing * ((productiveMark.visible ? 1 : 0) + (lowConfMark.visible ? 1 : 0) + 3)
              text: modelData.start + "–" + modelData.end +
                    " · " + dayflow.fmtDur(modelData.minutes) +
                    (modelData.count > 1 ? " · " + modelData.count + " blocks" : "") +
                    (modelData.appName ? " · " + modelData.appName : "")
              textFormat: Text.PlainText
              color: dayflow.dim
              font.family: dayflow.fontFamily
              font.pixelSize: Style.font.caption
              elide: Text.ElideRight
              anchors.verticalCenter: parent.verticalCenter
            }

            Rectangle {
              id: categoryPill
              height: catText.implicitHeight + Style.space(4)
              width: catText.implicitWidth + Style.space(10)
              radius: height / 2
              color: dayflow.pillBgColor(modelData.category)

              Text {
                id: catText
                anchors.centerIn: parent
                text: dayflow.catDisplay(modelData.category)
                textFormat: Text.PlainText
                color: dayflow.foreground
                font.family: dayflow.fontFamily
                font.pixelSize: Style.font.caption
              }
            }

            Text {
              id: productiveMark
              visible: modelData.productive === true
              text: "⚡"
              textFormat: Text.PlainText
              color: dayflow.dim
              font.family: dayflow.fontFamily
              font.pixelSize: Style.font.caption
              anchors.verticalCenter: parent.verticalCenter
            }

            Text {
              // Jev scored a judgment on this card below the confidence
              // threshold — treat the label/summary as suspect.
              id: lowConfMark
              visible: modelData.low_confidence === true
              text: "?"
              textFormat: Text.PlainText
              color: Qt.rgba(0.95, 0.70, 0.15, 1.0)
              font.family: dayflow.fontFamily
              font.pixelSize: Style.font.caption
              font.bold: true
              anchors.verticalCenter: parent.verticalCenter
            }

            Text {
              id: editText
              text: cardRoot.editing ? "close" : "edit"
              textFormat: Text.PlainText
              color: editLink.containsMouse ? dayflow.foreground : dayflow.dim
              font.family: dayflow.fontFamily
              font.pixelSize: Style.font.caption
              font.underline: editLink.containsMouse
              anchors.verticalCenter: parent.verticalCenter
              MouseArea {
                id: editLink
                anchors.fill: parent
                hoverEnabled: true
                cursorShape: Qt.PointingHandCursor
                onClicked: {
                  if (cardRoot.editing) cardRoot.editing = false
                  else cardRoot.beginEdit()
                }
              }
            }
          }

          PagedText {
            width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(54)
            text: modelData.title + "\n" + modelData.summary
          }

          // ---- inline edit form (title, category, productive) ----
          Column {
            visible: cardRoot.editing
            width: parent.width
            spacing: Style.space(6)

            Rectangle {
              width: parent.width
              height: titleEdit.implicitHeight + Style.space(8)
              radius: Style.cornerRadius
              color: dayflow.fgFill(0.06)
              border.color: dayflow.fgFill(0.12)
              clip: true
              TextEdit {
                id: titleEdit
                anchors.fill: parent
                anchors.margins: Style.space(4)
                text: cardRoot.editTitle
                color: dayflow.foreground
                font.family: dayflow.fontFamily
                font.pixelSize: Style.font.body
                wrapMode: TextEdit.Wrap
                onTextChanged: { if (activeFocus) cardRoot.editTitle = text }
              }
            }

            Row {
              width: parent.width
              spacing: Style.space(6)

              Rectangle {
                width: parent.width - prodPill.width - Style.space(6)
                height: catEdit.implicitHeight + Style.space(8)
                radius: Style.cornerRadius
                color: dayflow.fgFill(0.06)
                border.color: dayflow.fgFill(0.12)
                clip: true
                TextEdit {
                  id: catEdit
                  anchors.fill: parent
                  anchors.margins: Style.space(4)
                  text: cardRoot.editCategory
                  color: dayflow.foreground
                  font.family: dayflow.fontFamily
                  font.pixelSize: Style.font.body
                  onTextChanged: { if (activeFocus) cardRoot.editCategory = text }
                }
              }

              Rectangle {
                id: prodPill
                height: catEdit.implicitHeight + Style.space(8)
                width: prodPillText.implicitWidth + Style.space(12)
                radius: height / 2
                color: cardRoot.editProd ? dayflow.accentFill(0.15) : dayflow.fgFill(0.06)
                border.color: cardRoot.editProd ? dayflow.accentFill(0.5) : dayflow.fgFill(0.12)
                Text {
                  id: prodPillText
                  anchors.centerIn: parent
                  text: cardRoot.editProd ? "⚡ productive" : "not productive"
                  textFormat: Text.PlainText
                  color: dayflow.foreground
                  font.family: dayflow.fontFamily
                  font.pixelSize: Style.font.caption
                }
                MouseArea {
                  anchors.fill: parent
                  hoverEnabled: true
                  cursorShape: Qt.PointingHandCursor
                  onClicked: cardRoot.editProd = !cardRoot.editProd
                }
              }
            }

            Row {
              spacing: Style.space(8)

              Rectangle {
                height: Style.space(24)
                width: saveEditText.implicitWidth + Style.space(14)
                radius: Style.cornerRadius
                color: mSaveEdit.containsMouse ? dayflow.accentFill(0.12) : "transparent"
                border.color: dayflow.accentFill(0.5)
                Text {
                  id: saveEditText
                  anchors.centerIn: parent
                  text: "Save"
                  textFormat: Text.PlainText
                  color: dayflow.foreground
                  font.family: dayflow.fontFamily
                  font.pixelSize: Style.font.caption
                }
                MouseArea {
                  id: mSaveEdit
                  anchors.fill: parent
                  hoverEnabled: true
                  cursorShape: Qt.PointingHandCursor
                  onClicked: {
                    cardRoot.editing = false
                    dayflow.saveBlockEdits(modelData.start_ts,
                      cardRoot.editTitle, cardRoot.editCategory,
                      cardRoot.editProd, modelData)
                  }
                }
              }

              Rectangle {
                height: Style.space(24)
                width: cancelEditText.implicitWidth + Style.space(14)
                radius: Style.cornerRadius
                color: mCancelEdit.containsMouse ? dayflow.fgFill(0.08) : "transparent"
                border.color: dayflow.fgFill(0.15)
                Text {
                  id: cancelEditText
                  anchors.centerIn: parent
                  text: "Cancel"
                  textFormat: Text.PlainText
                  color: dayflow.dim
                  font.family: dayflow.fontFamily
                  font.pixelSize: Style.font.caption
                }
                MouseArea {
                  id: mCancelEdit
                  anchors.fill: parent
                  hoverEnabled: true
                  cursorShape: Qt.PointingHandCursor
                  onClicked: cardRoot.editing = false
                }
              }
            }
          }

          PagedText {
            width: parent.width; dayflow: root.dayflow; bodyHeight: Style.space(26)
            text: modelData.count > 1
              ? (modelData.children || []).map(function(c) { return c.start + " " + c.title }).join("\n")
              : (((modelData.children || [])[0] || {}).activities || []).map(function(a) { return root.dayflow.appDisplayName(a.app) + " · " + a.title }).join("\n")
            visible: text !== ""
          }

        }
      }
    }
  }
}
