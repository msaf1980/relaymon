package linuxstat

import (
	"os"
	"syscall"
	"time"
)

// StatTimes return file stat times from os.FileInfo
func StatTimes(fi os.FileInfo) (atime, mtime, ctime time.Time) {
	mtime = fi.ModTime()
	stat := fi.Sys().(*syscall.Stat_t)
	atime = time.Unix(int64(stat.Atim.Sec), int64(stat.Atim.Nsec))
	ctime = time.Unix(int64(stat.Ctim.Sec), int64(stat.Ctim.Nsec))
	return
}

// FileStatTimes return file stat times
func FileStatTimes(name string) (atime, mtime, ctime time.Time, err error) {
	fi, err := os.Stat(name)
	if err != nil {
		return
	}
	atime, mtime, ctime = StatTimes(fi)
	return
}
