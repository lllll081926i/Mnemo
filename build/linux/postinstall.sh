#!/bin/sh
# postinstall: create desktop entry
cat > /usr/share/applications/mnemo.desktop <<EOF
[Desktop Entry]
Name=Mnemo
Comment=多网盘桌面文件管理器
Exec=/usr/bin/mnemo
Icon=mnemo
Terminal=false
Type=Application
Categories=Utility;Network;FileManager;
EOF
