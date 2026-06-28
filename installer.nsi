!include "MUI2.nsh"
!include "FileFunc.nsh"

; ===== 基本信息 =====
Name "CCNEW Video Downloader"
OutFile "ccnew-vdl-setup.exe"
InstallDir "$LOCALAPPDATA\CCNEW-VideoDownloader"
InstallDirRegKey HKCU "Software\CCNEW-VideoDownloader" "InstallDir"
RequestExecutionLevel admin

; ===== 版本信息 =====
VIProductVersion "1.2.1.0"
VIAddVersionKey "ProductName" "CCNEW Video Downloader"
VIAddVersionKey "FileVersion" "1.2.1"
VIAddVersionKey "FileDescription" "抖音/B站视频下载工具"
VIAddVersionKey "LegalCopyright" "Star-wsc"

; ===== 界面 =====
!define MUI_ABORTWARNING
; !define MUI_ICON "static\favicon.ico"
; !define MUI_UNICON "static\favicon.ico"

; ===== 页面 =====
!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "SimpChinese"

; ===== 安装 =====
Section "安装主程序" SecMain
    SetOutPath "$INSTDIR"
    
    ; 写入注册表
    WriteRegStr HKCU "Software\CCNEW-VideoDownloader" "InstallDir" "$INSTDIR"
    WriteRegStr HKCU "Software\CCNEW-VideoDownloader" "Version" "1.2.1"
    
    ; 写入卸载信息
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "DisplayName" "CCNEW Video Downloader"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "DisplayVersion" "1.2.1"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "Publisher" "Star-wsc"
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "NoModify" 1
    WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader" "NoRepair" 1
    
    ; 复制文件
    File "server.exe"
    File /r "static"
    File "start-desktop.bat"
    File "config.json.example"
    
    ; 创建默认配置（如果不存在）
    IfFileExists "$INSTDIR\config.json" skip_config
        FileOpen $0 "$INSTDIR\config.json" w
        FileWrite $0 '{"port":"18000","download_dir":"","bilibili_cookie":"","douyin_cookie":"","proxy":""}'
        FileClose $0
    skip_config:
    
    ; 创建下载目录
    CreateDirectory "$DOCUMENTS\video-downloader"
    
    ; 创建快捷方式
    CreateDirectory "$SMPROGRAMS\CCNEW Video Downloader"
    CreateShortCut "$SMPROGRAMS\CCNEW Video Downloader\CCNEW Video Downloader.lnk" "$INSTDIR\server.exe" "" "$INSTDIR\server.exe" 0
    CreateShortCut "$SMPROGRAMS\CCNEW Video Downloader\卸载.lnk" "$INSTDIR\uninstall.exe" "" "$INSTDIR\uninstall.exe" 0
    CreateShortCut "$DESKTOP\CCNEW Video Downloader.lnk" "$INSTDIR\server.exe" "" "$INSTDIR\server.exe" 0
    
    ; 创建卸载程序
    WriteUninstaller "$INSTDIR\uninstall.exe"
    
    ; 询问是否开机自启
    MessageBox MB_YESNO "是否设置开机自动启动？" IDYES autostart IDNO skip_autostart
    autostart:
        WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader" '"$INSTDIR\server.exe"'
        Goto done_autostart
    skip_autostart:
        DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader"
    done_autostart:
SectionEnd

; ===== 卸载 =====
Section "Uninstall"
    ; 停止进程
    nsExec::ExecToLog 'taskkill /F /IM server.exe'
    Sleep 1000
    
    ; 删除文件
    RMDir /r "$INSTDIR\static"
    Delete "$INSTDIR\server.exe"
    Delete "$INSTDIR\start-desktop.bat"
    Delete "$INSTDIR\config.json.example"
    Delete "$INSTDIR\config.json"
    Delete "$INSTDIR\uninstall.exe"
    Delete "$INSTDIR\*.log"
    RMDir "$INSTDIR"
    
    ; 删除快捷方式
    Delete "$DESKTOP\CCNEW Video Downloader.lnk"
    RMDir /r "$SMPROGRAMS\CCNEW Video Downloader"
    
    ; 删除注册表
    DeleteRegKey HKCU "Software\CCNEW-VideoDownloader"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\CCNEW-VideoDownloader"
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "CCNEW-VideoDownloader"
SectionEnd