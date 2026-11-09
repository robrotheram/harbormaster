package firecracker

import (
	"os"
	"syscall"
)

func rootOwned(i os.FileInfo) bool { s, ok := i.Sys().(*syscall.Stat_t); return ok && s.Uid == 0 }
