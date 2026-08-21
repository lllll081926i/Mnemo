package dropbox

import (
	"time"
)

// small time helpers used by the driver (kept local to avoid importing time
// everywhere).

func timeNow() time.Time { return time.Now() }

const hour = time.Hour
