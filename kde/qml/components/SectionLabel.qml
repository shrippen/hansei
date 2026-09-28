import QtQuick
import QtQuick.Controls as QQC2
import org.kde.kirigami as Kirigami
import Kante

/** Small section heading: mono uppercase in Kante, a quiet small bold label in the text colour otherwise. */
QQC2.Label {
    font: KanteStyle.active ? KanteStyle.labelFont() : Qt.font({ family: Kirigami.Theme.smallFont.family, pointSize: Kirigami.Theme.smallFont.pointSize, bold: true })
    color: KanteStyle.active ? KanteStyle.mutedTextColor : Kirigami.Theme.textColor
    opacity: KanteStyle.active ? 1 : 0.85
    elide: Text.ElideRight
}
