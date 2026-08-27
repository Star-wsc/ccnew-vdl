#define MyAppName "CCNEW Video Downloader"
#define MyAppNameCN "抖音B站视频解析工具"
#ifndef MyAppVersion
#define MyAppVersion "1.4.0"
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
Source: "build\CCNEW-VideoDownloader.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "build\ffmpeg\ffmpeg.exe"; DestDir: "{app}"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Uninstall {#MyAppNameCN}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppNameCN}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "CCNEW-VideoDownloader"; ValueData: """{app}\{#MyAppExeName}"""; Tasks: autostart; Flags: uninsdeletevalue

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppNameCN}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
Filename: "taskkill"; Parameters: "/F /IM {#MyAppExeName}"; Flags: runhidden; RunOnceId: "KillApp"

[UninstallDelete]
Type: files; Name: "{app}\{#MyAppExeName}"
Type: dirifempty; Name: "{app}"
