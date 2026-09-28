cmake_minimum_required(VERSION 3.20)
project(hansei-kde VERSION 0.1.0 LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_AUTOMOC ON)
set(CMAKE_AUTORCC ON)

# Demo code (screenshot runner) is internal; release builds leave it out and check that.
option(HANSEI_DEMO "Build the internal screenshot runner (never for releases)" OFF)

find_package(ECM 6.0 REQUIRED NO_MODULE)
set(CMAKE_MODULE_PATH ${ECM_MODULE_PATH})
include(KDEInstallDirs)
include(KDECMakeSettings)
include(KDECompilerSettings NO_POLICY_SCOPE)
include(ECMInstallIcons)
include(ECMFindQmlModule)

find_package(Qt6 6.6 REQUIRED COMPONENTS Core Gui Quick QuickControls2 Network Widgets)
find_package(KF6 6.8 REQUIRED COMPONENTS I18n CoreAddons Notifications)
# Runtime QML modules (not linked).
ecm_find_qmlmodule(org.kde.kirigami REQUIRED)
ecm_find_qmlmodule(org.kde.syntaxhighlighting REQUIRED)
ecm_find_qmlmodule(Qt.labs.folderlistmodel REQUIRED)

qt_add_executable(hansei-kde
    src/main.cpp
    src/client.cpp
    src/client.h
)

# QML, the vendored Kante module (scripts/sync-kante.sh) and icons, served from qrc:/.
file(GLOB_RECURSE HANSEI_QML RELATIVE ${CMAKE_CURRENT_SOURCE_DIR} CONFIGURE_DEPENDS qml/*.qml qml/*/qmldir qml/*.ttf)
qt_add_resources(hansei-kde "ui" PREFIX "/" FILES ${HANSEI_QML} icons/org.shrippen.hansei.svg icons/hatch.svg)

if(HANSEI_DEMO)
    target_sources(hansei-kde PRIVATE demo/shots.cpp demo/shots.h)
    qt_add_resources(hansei-kde "demo" PREFIX "/" FILES demo/ScreenshotRunner.qml)
    target_compile_definitions(hansei-kde PRIVATE HANSEI_DEMO=1)
endif()

target_compile_definitions(hansei-kde PRIVATE HANSEI_VERSION="${PROJECT_VERSION}")
# QML modules come from the system at runtime; nothing to link statically.
set_target_properties(hansei-kde PROPERTIES QT_QML_MODULE_NO_IMPORT_SCAN TRUE)

target_link_libraries(hansei-kde PRIVATE
    Qt6::Core Qt6::Gui Qt6::Quick Qt6::QuickControls2 Qt6::Network Qt6::Widgets
    KF6::I18n KF6::I18nQml KF6::CoreAddons KF6::Notifications
)

install(TARGETS hansei-kde ${KDE_INSTALL_TARGETS_DEFAULT_ARGS})
install(FILES org.shrippen.hansei.desktop DESTINATION ${KDE_INSTALL_APPDIR})
install(FILES org.shrippen.hansei.metainfo.xml DESTINATION ${KDE_INSTALL_METAINFODIR})
install(FILES hansei.notifyrc DESTINATION ${KDE_INSTALL_KNOTIFYRCDIR})
install(FILES icons/org.shrippen.hansei.svg DESTINATION ${KDE_INSTALL_FULL_ICONDIR}/hicolor/scalable/apps)
install(FILES dolphin/hansei-check.desktop DESTINATION ${KDE_INSTALL_DATADIR}/kio/servicemenus
        PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE WORLD_READ WORLD_EXECUTE)

ki18n_install(po)
