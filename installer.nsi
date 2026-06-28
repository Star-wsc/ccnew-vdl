!include "MUI2.nsh"
!include "FileFunc.nsh"

Name "CCNEW Video Downloader"
OutFile "ccnew-vdl-setup.exe"
InstallDir "$LOCALAPPDATA\CCNEW-VideoDownloader"
InstallDirRegKey HKCU "Software\CCNEW-VideoDownloader" "InstallDir"
RequestExecutionLevel admin

VIProductVersion "1.2.2.0"
VIAddVersionKey "ProductName" "CCNEW Video Downloader"
VIAddVersionKey "FileVersion" "1.2.2"
VIAddVersionKey "FileDescription" "CCNEW Video Downloader"
VIAddVersionKey "LegalCopyright" "Star-wsc"

!define MUI_ABORTWARNING
!define MUI_ICON "static\BDYD.ico"
!define MUI_UNICON "static\BDYD.ico"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"

Section "Install" SecMain
    SetOutPath "$INSTDIR"

    WriteRegStr HKCU "Software\CCNEW-VideoDownloader" "InstallDir" "$INSTDIR"
    WriteRegStr HKCU "Software\CCNEW-VideoDownloader" "Version" "1.2.2"

    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "DisplayName" "CCNEW Video Downloader"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "DisplayVersion" "1.2.2"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "Publisher" "Star-wsc"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "DisplayIcon" "$INSTDIR\static\BDYD.ico"
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "NoRepair" 1

    File "server.exe"
    File /r "static"
    File /r "webview2"
    File "start-desktop.bat"

    IfFileExists "$INSTDIR\config.json" skip_config
        FileOpen $0 "$INSTDIR\config.json" w
        FileWrite $0 '{"port":"18000","download_dir":"","bilibili_cookie":"","douyin_cookie":"","proxy":""}'
        FileClose $0
    skip_config:

    CreateDirectory "$DOCUMENTS\video-downloader"

    CreateDirectory "$SMPROGRAMS\CCNEW Video Downloader"
    CreateShortCut "$SMPROGRAMS\CCNEW Video Downloader\CCNEW Video Downloader.lnk" "$INSTDIR\server.exe" "" "$INSTDIR\static\BDYD.ico" 0
    CreateShortCut "$SMPROGRAMS\CCNEW Video Downloader\Uninstall.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\uninstall.exe" 0
    CreateShortCut "$DESKTOP\CCNEW Video Downloader.lnk" "$INSTDIR\server.exe" "" "$INSTDIR\static\BDYD.ico" 0

    WriteUninstaller "$INSTDIR\uninstall.exe"

    MessageBox MB_YESNO "Start on Windows boot?" IDYES autostart IDNO skip_autostart
    autostart:
        WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader" '"$INSTDIR\server.exe"'
        Goto done
    skip_autostart:
        DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader"
    done:
SectionEnd

Section "Uninstall"
    nsExec::ExecToLog 'taskkill /F /IM server.exe'
    Sleep 1000

    RMDir /r "$INSTDIR\static"
    RMDir /r "$INSTDIR\webview2"
    Delete "$INSTDIR\server.exe"
    Delete "$INSTDIR\start-desktop.bat"
    Delete "$INSTDIR\config.json"
    Delete "$INSTDIR\config.json.example"
    Delete "$INSTDIR\uninstall.exe"
    Delete "$INSTDIR\*.log"
    RMDir "$INSTDIR"

    Delete "$DESKTOP\CCNEW Video Downloader.lnk"
    RMDir /r "$SMPROGRAMS\CCNEW Video Downloader"

    DeleteRegKey HKCU "Software\CCNEW-VideoDownloader"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader"
SectionEnd