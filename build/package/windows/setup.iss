#define MyAppName "Goink"
#define MyAppVersion GetEnv("VERSION")
#define MyAppExeName "goink.exe"

[Setup]
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=sigpanic
AppPublisherURL=https://github.com/sigpanic/goink
AppSupportURL=https://github.com/sigpanic/goink/issues
AppUpdatesURL=https://github.com/sigpanic/goink/releases
AppId={{9288ae33-8307-4a08-ac6b-3d3c83521f86}
DefaultDirName={code:GetDefaultDir}
DefaultGroupName={#MyAppName}
OutputDir=..\..\dist
OutputBaseFilename=goink-v{#MyAppVersion}-windows-amd64
Compression=lzma2
SolidCompression=yes
UninstallDisplayName={#MyAppName}
UninstallDisplayIcon={app}\{#MyAppExeName}
ArchitecturesInstallIn64BitMode=x64compatible
DirExistsWarning=no
WizardStyle=modern
; UsePreviousAppDir 默认 yes，AppId 稳定后升级时自动读取上次安装目录

[Files]
Source: "..\..\bin\goink.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\runtime\*"; DestDir: "{app}\runtime"; Flags: ignoreversion recursesubdirs

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "快捷方式:"; Flags: checkedonce

[Icons]
Name: "{autoprograms}\{#MyAppName}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autoprograms}\{#MyAppName}\卸载 Goink"; Filename: "{uninstallexe}"
Name: "{userdesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

; 安装完成后可选启动 Goink（postinstall 显示复选框，skipifsilent 避免静默安装时弹窗）
[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "启动 Goink"; Flags: postinstall skipifsilent

[Code]
function GetDefaultDir(Param: string): string;
begin
  if DirExists('D:\') then Result := 'D:\Goink'
  else if DirExists('E:\') then Result := 'E:\Goink'
  else Result := 'C:\Goink';
end;
