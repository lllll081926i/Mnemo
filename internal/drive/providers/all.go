// Package providers aggregates every drive plugin via blank imports.
// Importing this package registers all providers into the drive registry.
package providers

import (
	_ "mnemo-go/internal/drive/providers/aliopen"
	_ "mnemo-go/internal/drive/providers/dropbox"
	_ "mnemo-go/internal/drive/providers/onedrive"
	_ "mnemo-go/internal/drive/providers/s3"
	_ "mnemo-go/internal/drive/providers/webdav"
)
