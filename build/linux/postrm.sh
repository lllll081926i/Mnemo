#!/bin/sh
# postrm: remove desktop entry (keep user data)
rm -f /usr/share/applications/mnemo.desktop 2>/dev/null || true
