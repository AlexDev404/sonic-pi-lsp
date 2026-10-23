; NSIS installer for the Sonic Pi CLI.
;
;   makensis /DVERSION=1.2.3 /DSRC=path\to\staged\sonicpi-cli /DOUT=path\to\setup.exe sonicpi.nsi
;
; SRC is the tree scripts/stage.sh makes. The components page asks whether
; to install the background service (a Windows service that starts with
; Windows); without it, sonicpi starts the service itself when needed.
; Silent installs take /SERVICE=0 to skip it.

Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "Sections.nsh"
!include "StrFunc.nsh"
!include "WordFunc.nsh"
!include "FileFunc.nsh"
!include "x64.nsh"

${StrLoc}

!ifndef VERSION
  !define VERSION "dev"
!endif
!ifndef SRC
  !error "pass /DSRC=<staged sonicpi-cli directory>"
!endif
!ifndef OUT
  !define OUT "sonicpi-cli-installer.exe"
!endif

!define APPNAME "Sonic Pi CLI"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\SonicPiCLI"
!define ENVKEY "SYSTEM\CurrentControlSet\Control\Session Manager\Environment"

Name "${APPNAME} ${VERSION}"
OutFile "${OUT}"
InstallDir "$PROGRAMFILES64\Sonic Pi CLI"
InstallDirRegKey HKLM "Software\SonicPiCLI" "InstallDir"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
BrandingText "${APPNAME} ${VERSION}"

!define MUI_ABORTWARNING
!define MUI_COMPONENTSPAGE_SMALLDESC
!define MUI_FINISHPAGE_TEXT "Open a new terminal and run:$\r$\n$\r$\n    sonicpi doctor$\r$\n    sonicpi song.rb$\r$\n    sonicpi lint --init"
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "!Sonic Pi CLI (required)" SecCore
  SectionIn RO
  ; Stop a running service before replacing its files.
  IfFileExists "$INSTDIR\sonicpi.exe" 0 +2
    nsExec::Exec '"$INSTDIR\sonicpi.exe" service stop'

  SetOutPath "$INSTDIR"
  File /r "${SRC}\*.*"

  WriteRegStr HKLM "Software\SonicPiCLI" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKLM "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKLM "${UNINSTKEY}" "Publisher" "sonic-pi-lsp"
  WriteRegStr HKLM "${UNINSTKEY}" "URLInfoAbout" "https://github.com/AlexDev404/sonic-pi-lsp"
  WriteRegStr HKLM "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKLM "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKLM "${UNINSTKEY}" "NoRepair" 1
  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKLM "${UNINSTKEY}" "EstimatedSize" "$0"
SectionEnd

Section "Add sonicpi to PATH" SecPath
  ReadRegStr $0 HKLM "${ENVKEY}" "Path"
  ${StrLoc} $1 "$0" "$INSTDIR" ">"
  StrCmp $1 "" 0 done
    StrCmp $0 "" 0 +3
      WriteRegExpandStr HKLM "${ENVKEY}" "Path" "$INSTDIR"
      Goto notify
    WriteRegExpandStr HKLM "${ENVKEY}" "Path" "$0;$INSTDIR"
  notify:
    SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  done:
SectionEnd

Section "Background service (starts with Windows)" SecService
  ; Runs as the "sonicpid" Windows service, with this user's ~/.sonic-pi
  ; configuration and logs in their %TEMP%\sonic-pi.
  DetailPrint "Installing the sonicpid service..."
  nsExec::ExecToLog '"$INSTDIR\sonicpi.exe" service install'
  Pop $0
  StrCmp $0 "0" +2
    MessageBox MB_ICONEXCLAMATION|MB_OK "The background service could not be installed (exit code $0).$\r$\nsonicpi will start the service itself when it needs it."
SectionEnd

LangString DESC_Core ${LANG_ENGLISH} "sonicpi and sonicpid, the linter, and the Sonic Pi server and audio engine."
LangString DESC_Path ${LANG_ENGLISH} "Lets you run sonicpi from any terminal."
LangString DESC_Service ${LANG_ENGLISH} "Keeps Sonic Pi running in the background as a Windows service, so sonicpi FILE plays at once. Without it, sonicpi starts Sonic Pi on demand."
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_Core)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecPath} $(DESC_Path)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecService} $(DESC_Service)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP|MB_OK "The Sonic Pi CLI needs 64-bit Windows."
    Abort
  ${EndIf}
  SetRegView 64
  ; Silent installs: /SERVICE=0 leaves the service out.
  ${GetParameters} $0
  ${GetOptions} $0 "/SERVICE=" $1
  StrCmp $1 "0" 0 +2
    !insertmacro UnselectSection ${SecService}
FunctionEnd

Function un.onInit
  SetRegView 64
FunctionEnd

Section "Uninstall"
  nsExec::ExecToLog '"$INSTDIR\sonicpi.exe" service stop'
  nsExec::ExecToLog '"$INSTDIR\sonicpi.exe" service uninstall'

  ; Take the install directory back out of PATH.
  ReadRegStr $0 HKLM "${ENVKEY}" "Path"
  ${WordReplace} "$0" ";$INSTDIR" "" "+" $0
  ${WordReplace} "$0" "$INSTDIR;" "" "+" $0
  ${WordReplace} "$0" "$INSTDIR" "" "+" $0
  WriteRegExpandStr HKLM "${ENVKEY}" "Path" "$0"
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000

  RMDir /r "$INSTDIR"
  DeleteRegKey HKLM "${UNINSTKEY}"
  DeleteRegKey HKLM "Software\SonicPiCLI"
SectionEnd
