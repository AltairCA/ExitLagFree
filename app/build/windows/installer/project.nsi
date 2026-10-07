Unicode true

## Built by `wails build -platform windows/amd64 -nsis`, which regenerates
## wails_tools.nsh but keeps this file. The helper and wintun.dll must already
## be in <repo>/bin (override with -DELF_BIN_DIR=...).
##
## The helper service itself is first registered by the app ("Install helper"),
## not here: it records the Windows account allowed to control the tunnel, and
## an elevated installer may be running as a different (admin) account. Later
## installs only upgrade an existing service, reusing that recorded account.

!define UNINST_KEY_NAME "ExitLagFree"
!ifndef ELF_BIN_DIR
    !define ELF_BIN_DIR "..\..\..\..\bin"
!endif

!include "wails_tools.nsh"

# Version information must consist of 4 numeric parts.
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

ManifestDPIAware true

!include "MUI.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\ExitLagFree"
InstallDirRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}" "InstallLocation"
ShowInstDetails show

Function .onInit
    !insertmacro wails.checkArchitecture
FunctionEnd

Section
    !insertmacro wails.setShellContext
    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR
    !insertmacro wails.files
    File "${ELF_BIN_DIR}\exitlag-helper.exe"
    File "${ELF_BIN_DIR}\wintun.dll"

    # On upgrades, swap the running service for the new helper while keeping
    # its allowed user. Does nothing on a fresh install.
    DetailPrint "Updating the ExitLagFree helper service (if installed)"
    nsExec::ExecToLog '"$INSTDIR\exitlag-helper.exe" upgrade'
    Pop $0

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortcut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.writeUninstaller
    SetRegView 64
    WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\${UNINST_KEY_NAME}" "InstallLocation" "$INSTDIR"
SectionEnd

Section "uninstall"
    !insertmacro wails.setShellContext

    # Stops and removes the helper service, its Program Files copy and config.
    DetailPrint "Removing the ExitLagFree helper service"
    nsExec::ExecToLog '"$INSTDIR\exitlag-helper.exe" uninstall'
    Pop $0

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # WebView2 data

    # Only remove what we installed, in case INSTDIR is a shared folder.
    Delete "$INSTDIR\${PRODUCT_EXECUTABLE}"
    Delete "$INSTDIR\exitlag-helper.exe"
    Delete "$INSTDIR\wintun.dll"

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.deleteUninstaller
    RMDir $INSTDIR
SectionEnd
