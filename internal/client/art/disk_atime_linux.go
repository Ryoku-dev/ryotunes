package art

import (
	"os"
	"syscall"
	"time"
)

func accessTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Atim.Unix())
	}
	return info.ModTime()
}
