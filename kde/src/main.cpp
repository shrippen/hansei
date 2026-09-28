#include "client.h"
#ifdef HANSEI_DEMO
#include "../demo/shots.h"
#endif

#include <KAboutData>
#include <KLocalizedQmlContext>
#include <KLocalizedString>

#include <QApplication>
#include <QFileInfo>
#include <QIcon>
#include <QQmlApplicationEngine>
#include <QQmlContext>
#include <QQuickStyle>
#include <QUrl>

int main(int argc, char *argv[])
{
    // QApplication (not QGuiApplication): the org.kde.desktop style uses QStyle for native controls.
    QApplication app(argc, argv);
    KLocalizedString::setApplicationDomain("hansei");
    // Uninstalled builds: translations built next to the binary (build/locale).
    const QString buildLocale = QCoreApplication::applicationDirPath() + QStringLiteral("/../locale");
    if (QFileInfo::exists(buildLocale)) {
        KLocalizedString::addDomainLocaleDir("hansei", buildLocale);
    }

    KAboutData about(QStringLiteral("hansei"), i18n("Hansei"), QStringLiteral(HANSEI_VERSION),
                     i18n("Review AI edits of your Obsidian notes, batch by batch"), KAboutLicense::GPL_V3,
                     QStringLiteral("© 2026 Arian"));
    about.setDesktopFileName(QStringLiteral("org.shrippen.hansei"));
    about.setOrganizationDomain("shrippen.github.io");
    KAboutData::setApplicationData(about);
    QApplication::setWindowIcon(QIcon::fromTheme(QStringLiteral("org.shrippen.hansei"), QIcon(QStringLiteral(":/icons/org.shrippen.hansei.svg"))));

    // The platform style is the default; Kante is an opt-in style drawn by the Kante module.
    if (qEnvironmentVariableIsEmpty("QT_QUICK_CONTROLS_STYLE")) {
        QQuickStyle::setStyle(QStringLiteral("org.kde.desktop"));
    }

    QQmlApplicationEngine engine;
    KLocalization::setupLocalizedContext(&engine);
    engine.addImportPath(QStringLiteral("qrc:/qml"));

    auto *client = new Client(&engine, &app);
    engine.rootContext()->setContextProperty(QStringLiteral("Hansei"), client);
#ifdef HANSEI_DEMO
    Shots::install(engine);
#endif
    engine.load(QUrl(QStringLiteral("qrc:/qml/Main.qml")));
    if (engine.rootObjects().isEmpty()) {
        return 1;
    }
    client->start();
    return app.exec();
}
