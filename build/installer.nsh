; Default install dirs only when no previous InstallLocation is recorded.
; This preserves custom paths from prior installs / upgrades.
!macro preInit
  SetRegView 64
  ReadRegStr $0 HKLM "${INSTALL_REGISTRY_KEY}" InstallLocation
  ${If} $0 == ""
    WriteRegExpandStr HKLM "${INSTALL_REGISTRY_KEY}" InstallLocation "$PROGRAMFILES64\Mnemo"
  ${EndIf}
  ReadRegStr $0 HKCU "${INSTALL_REGISTRY_KEY}" InstallLocation
  ${If} $0 == ""
    WriteRegExpandStr HKCU "${INSTALL_REGISTRY_KEY}" InstallLocation "$LOCALAPPDATA\Programs\Mnemo"
  ${EndIf}
  SetRegView lastused
!macroend
