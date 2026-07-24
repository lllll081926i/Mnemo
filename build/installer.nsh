; Force a normal Windows install location for Mnemo.
; Without this, residual registry InstallLocation from older / test installs can stick.
!macro preInit
  SetRegView 64
  WriteRegExpandStr HKLM "${INSTALL_REGISTRY_KEY}" InstallLocation "$PROGRAMFILES64\Mnemo"
  WriteRegExpandStr HKCU "${INSTALL_REGISTRY_KEY}" InstallLocation "$LOCALAPPDATA\Programs\Mnemo"
  SetRegView lastused
!macroend
