import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import Kante

/** Drawer header: the mark and the name. */
RowLayout {
    property bool collapsed: false

    spacing: Kirigami.Units.smallSpacing
    Layout.fillWidth: true

    Kirigami.Icon {
        source: "qrc:/icons/org.shrippen.hansei.svg"
        Layout.preferredWidth: Kirigami.Units.iconSizes.medium
        Layout.preferredHeight: Kirigami.Units.iconSizes.medium
        Layout.margins: Kirigami.Units.smallSpacing
    }
    Kirigami.Heading {
        visible: !parent.collapsed
        text: i18n("Hansei")
        font: KanteStyle.titleFont(Kirigami.Theme.defaultFont.pointSize * 1.3)
        Layout.fillWidth: true
    }
}
