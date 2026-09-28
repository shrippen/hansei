import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/**
 * Header above a hunk: number, heading, the AI's reason, state and the actions.
 * Accept is the primary action of the current change; rarer actions sit in the ⋯ menu.
 * Decided changes fold to one line; neighbours with the same decision share it.
 */
QQC2.Control {
    id: header

    property var hunk: ({})
    property int total: 0
    property bool current: false
    property bool folded: false
    property bool editable: true
    property int group: 1          // folded neighbours shown by this header
    property bool compact: width < Kirigami.Units.gridUnit * 30

    signal decide(string decision)
    signal reject()
    signal edit()
    signal feedback()
    signal history()
    signal regenerate()
    signal toggle()
    signal picked()
    signal rule(string ref)

    readonly property bool pending: hunk.state === "pending"
    readonly property color stateColor: hunk.state === "accepted" ? KanteStyle.positiveTextColor
        : hunk.state === "rejected" ? KanteStyle.negativeTextColor
        : (current ? KanteStyle.accentColor : KanteStyle.frameColor)
    readonly property string stateText: hunk.state === "accepted" ? i18ncp("hunk state", "accepted", "%1 accepted", group)
        : hunk.state === "rejected" ? i18ncp("hunk state", "rejected", "%1 rejected", group) : i18n("open")

    topPadding: folded ? Kirigami.Units.smallSpacing : Kirigami.Units.largeSpacing
    bottomPadding: Kirigami.Units.smallSpacing
    leftPadding: Kirigami.Units.smallSpacing
    rightPadding: Kirigami.Units.smallSpacing

    background: Item {
        Rectangle {
            anchors { fill: parent; topMargin: Kirigami.Units.smallSpacing / 2 }
            color: header.current ? Qt.alpha(KanteStyle.accentColor, 0.08) : KanteStyle.sunkenColor
        }
        Rectangle {
            anchors { left: parent.left; top: parent.top; bottom: parent.bottom; topMargin: Kirigami.Units.smallSpacing / 2 }
            width: 3
            color: header.stateColor
        }
        TapHandler { onTapped: header.folded ? header.toggle() : header.picked() }
    }

    contentItem: ColumnLayout {
        spacing: Kirigami.Units.smallSpacing

        RowLayout {
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true

            KanteToolButton {
                icon.name: header.folded ? "go-next" : "go-down"
                visible: !header.pending
                onClicked: header.toggle()
                QQC2.ToolTip.text: header.folded ? i18n("Show lines") : i18n("Hide lines")
                QQC2.ToolTip.visible: hovered
            }
            QQC2.Label {
                text: header.group > 1 ? i18n("Changes %1–%2 of %3", header.hunk.index + 1, header.hunk.index + header.group, header.total)
                                       : i18n("Change %1 of %2", header.hunk.index + 1, header.total)
                // A label, not a title: small uppercase mono in Kante, small bold otherwise.
                font: KanteStyle.active ? KanteStyle.labelFont() : Qt.font({ family: Kirigami.Theme.smallFont.family, pointSize: Kirigami.Theme.smallFont.pointSize, bold: true })
                color: header.current ? KanteStyle.textColor : KanteStyle.mutedTextColor
            }
            QQC2.Label {
                text: header.hunk.heading === "---" ? i18n("Frontmatter") : (header.hunk.heading || "").replace(/^#+\s*/, "")
                visible: text !== "" && header.group === 1
                font.weight: Font.DemiBold
                color: header.current ? KanteStyle.strongTextColor : KanteStyle.mutedTextColor
                elide: Text.ElideRight
                Layout.maximumWidth: Kirigami.Units.gridUnit * 14
            }
            Item { Layout.fillWidth: true }
            Chip {
                visible: !!header.hunk.rule && !header.folded
                plain: true
                interactive: true
                checkable: false
                text: header.hunk.rule || ""
                tone: KanteStyle.infoColor
                onClicked: header.rule(header.hunk.rule)
                QQC2.ToolTip.visible: hovered
                QQC2.ToolTip.text: i18n("Show this rule")
            }
            Chip {
                visible: header.hunk.feedback > 0
                text: i18np("1 comment", "%1 comments", header.hunk.feedback)
                tone: KanteStyle.infoColor
            }
            Chip {
                visible: !header.pending
                text: header.stateText
                tone: header.stateColor
            }
        }

        QQC2.Label {
            visible: !!header.hunk.reason && !header.folded
            text: header.hunk.reason || ""
            wrapMode: Text.Wrap
            Layout.fillWidth: true
            Layout.leftMargin: Kirigami.Units.smallSpacing
        }

        QQC2.Label {
            visible: !!header.hunk.rejected && !header.folded
            text: i18n("Rejected: %1", header.hunk.rejected || "")
            color: KanteStyle.negativeTextColor
            wrapMode: Text.Wrap
            Layout.fillWidth: true
        }

        RowLayout {
            visible: header.editable && !header.folded
            spacing: Kirigami.Units.smallSpacing
            Layout.fillWidth: true

            KanteButton {
                visible: header.pending
                text: i18n("Accept")
                icon.name: "dialog-ok-apply"
                emphasis: header.current ? KanteButton.Emphasis.Primary : KanteButton.Emphasis.Normal
                onClicked: header.decide("accepted")
                QQC2.ToolTip.text: i18n("Accept (A)")
                QQC2.ToolTip.visible: hovered
            }
            KanteToolButton {
                visible: header.pending
                text: i18n("Reject")
                icon.name: "dialog-cancel"
                display: header.compact ? QQC2.AbstractButton.IconOnly : QQC2.AbstractButton.TextBesideIcon
                onClicked: header.reject()
                QQC2.ToolTip.text: i18n("Reject (R)")
                QQC2.ToolTip.visible: hovered
            }
            KanteToolButton {
                text: i18n("Edit")
                icon.name: "document-edit"
                display: header.compact ? QQC2.AbstractButton.IconOnly : QQC2.AbstractButton.TextBesideIcon
                onClicked: header.edit()
                QQC2.ToolTip.text: i18n("Edit (E)")
                QQC2.ToolTip.visible: hovered
            }
            KanteToolButton {
                text: i18n("Feedback")
                icon.name: "mail-reply-sender"
                display: header.compact ? QQC2.AbstractButton.IconOnly : QQC2.AbstractButton.TextBesideIcon
                onClicked: header.feedback()
                QQC2.ToolTip.text: i18n("Feedback to the AI (F)")
                QQC2.ToolTip.visible: hovered
            }
            Item { Layout.fillWidth: true }
            KanteToolButton {
                icon.name: "overflow-menu"
                onClicked: more.popup()
                QQC2.ToolTip.text: i18n("More")
                QQC2.ToolTip.visible: hovered
                QQC2.Menu {
                    KantePopupSkin { popup: more }
                    id: more
                    QQC2.MenuItem {
                        text: i18n("History of this change")
                        icon.name: "view-history"
                        onTriggered: header.history()
                    }
                    QQC2.MenuItem {
                        text: i18n("Propose again")
                        icon.name: "view-refresh"
                        onTriggered: header.regenerate()
                    }
                    QQC2.MenuItem {
                        visible: !header.pending
                        height: visible ? implicitHeight : 0
                        text: i18n("Undecide")
                        icon.name: "edit-undo"
                        onTriggered: header.decide("pending")
                    }
                }
            }
        }
    }
}
