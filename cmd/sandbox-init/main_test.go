package main

import "testing"

func TestMountedAsRequiresExactMountAndFilesystem(t *testing.T) {
	const info = "24 1 0:6 / /dev rw,nosuid - devtmpfs devtmpfs rw\n25 1 0:7 / /dev/pts rw - devpts devpts rw\n"
	if !mountedAs(info, "/dev", "devtmpfs") {
		t.Fatal("kernel devtmpfs mount was not recognized")
	}
	for _, v := range [][2]string{{"/dev", "tmpfs"}, {"/dev/pts", "devtmpfs"}, {"/data", "ext4"}, {"/de", "devtmpfs"}} {
		if mountedAs(info, v[0], v[1]) {
			t.Fatal("accepted wrong mount", v)
		}
	}
}
