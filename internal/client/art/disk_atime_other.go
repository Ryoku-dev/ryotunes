//go:build !linux

package art

import (
	"os"
	"time"
)

// accessTime falls back to the fetch time where atime is not read.
func accessTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
