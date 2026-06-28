#define MyAppName "CCNEW Video Downloader"
#define MyAppNameCN "抖音B站视频解析工具"
#define MyAppVersion "1.2.4"
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
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "附加图标:"
Name: "autostart"; Description: "开机自动启动"; GroupDescription: "其他选项:"

[Files]
Source: "build\server.exe"; DestDir: "{app}"; DestName: "{#MyAppExeName}"; Flags: ignoreversion
Source: "build\static\*"; DestDir: "{app}\static"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "build\webview2\*"; DestDir: "{app}\webview2"; Flags: ignoreversion recursesubdirs createallsubdirs

[Dirs]
Name: "{app}\config"

[Icons]
Name: "{group}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\static\BDYD.ico"
Name: "{group}\卸载 {#MyAppNameCN}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"; IconFilename: "{app}\static\BDYD.ico"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "CCNEW-VideoDownloader"; ValueData: """{app}\{#MyAppExeName}"""; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "启动 {#MyAppNameCN}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "taskkill"; Parameters: "/F /IM {#MyAppExeName}"; Flags: runhidden

[UninstallDelete]
Type: filesandordirs; Name: "{app}\static"
Type: filesandordirs; Name: "{app}\webview2"
Type: filesandordirs; Name: "{app}\config"
Type: files; Name: "{app}\{#MyAppExeName}"
Type: files; Name: "{app}\*.log"
Type: dirifempty; Name: "{app}"