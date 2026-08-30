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
; 升级时读取上次安装路径，默认装到原位置，避免自定义路径用户踩坑
InstallDirRegKey=HKCU\Software\sigpanic\Goink\InstallDir

[Files]
Source: "..\..\bin\goink.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\runtime\*"; DestDir: "{app}\runtime"; Flags: recursesubdirs

[Tasks]
Name: "desktopicon"; Description: "创建桌面快捷方式"; GroupDescription: "快捷方式:"; Flags: checkedonce

[Icons]
Name: "{autoprograms}\{#MyAppName}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{autoprograms}\{#MyAppName}\卸载 Goink"; Filename: "{uninstallexe}"
Name: "{userdesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

; 配合 [Setup] 段的 InstallDirRegKey，写入上次安装路径到注册表
; 升级时 InstallDirRegKey 读取此值作为默认目录，避免自定义路径用户踩坑
[Registry]
Root: HKCU; Subkey: "Software\sigpanic\Goink"; ValueType: string; ValueName: "InstallDir"; ValueData: "{app}"; Flags: uninsdeletevalue

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
