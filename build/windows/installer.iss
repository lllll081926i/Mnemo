; Inno Setup Script for Mnemo
; Build: wails build -platform windows/amd64, then ISCC this script.
; Pass /DMyArchSuffix=Mnemo-windows-x64 to set the output filename.

#ifndef MyArchSuffix
  #define MyArchSuffix "MnemoSetup"
#endif

#define MyAppName "Mnemo"
#define MyAppVersion "0.1.0"
#define MyAppExeName "mnemo-go.exe"

[Setup]
AppId={{8F4E3D2A-1B5C-4E7F-9A2D-6C8B1F3E5D7A}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppName}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
OutputDir=..\installer
OutputBaseFilename={#MyArchSuffix}
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
ArchitecturesAllowed=x64compatible
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
Source: "..\bin\mnemo-go.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\bin\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#MyAppName}}"; Flags: nowait postinstall skipifsilent
