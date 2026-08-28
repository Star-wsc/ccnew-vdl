#define MyAppName "CCNEW Video Downloader"
#define MyAppNameCN "抖音B站视频解析工具"
#ifndef MyAppVersion
#define MyAppVersion "1.3.5"
#endif
#define MyAppPublisher "Star-wsc"
#define MyAppExeName "CCNEW-VideoDownloader.exe"

[Setup]
AppId={{CCNEW-VideoDownloader-2026}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\{#MyAppNameCN}
DefaultGroupName={#MyAppNameCN}
OutputDir=.
OutputBaseFilename=ccnew-vdl-setup
SetupIconFile=static\BDYD.ico
UninstallDisplayIcon={app}\{#MyAppExeName}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesInstallIn64BitMode=x64compatible

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create desktop shortcut"; GroupDescription: "Additional icons:"
Name: "autostart"; Description: "Start on Windows boot"; GroupDescription: "Other options:"

[Files]
Source: "build\server.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion
Source: "build\static\*"; DestDir: "{app}\static"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "build\webview2\*"; DestDir: "{app}\webview2"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "build\ffmpeg\ffmpeg-windows-amd64.exe"; DestDir: "{app}\ffmpeg"; DestName: "ffmpeg.exe"; Flags: ignoreversion

[Dirs]
Name: "{app}\config"

[Icons]
Name: "{group}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\static\BDYD.ico"
Name: "{group}\Uninstall {#MyAppNameCN}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\static\BDYD.ico"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "CCNEW-VideoDownloader"; ValueData: """{app}\{#MyAppExeName}"""; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppNameCN}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "taskkill"; Parameters: "/F /IM {#MyAppExeName}"; Flags: runhidden; RunOnceId: "KillApp"

[UninstallDelete]
Type: filesandordirs; Name: "{app}\static"
Type: filesandordirs; Name: "{app}\webview2"
Type: filesandordirs; Name: "{app}\ffmpeg"
Type: filesandordirs; Name: "{app}\config"
Type: files; Name: "{app}\{#MyAppExeName}"
Type: files; Name: "{app}\*.log"
Type: dirifempty; Name: "{app}"
