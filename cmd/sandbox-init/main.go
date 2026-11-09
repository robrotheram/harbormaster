// PID 1 inside the Firecracker guest. It never runs on the host.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func disk(label string) string {
	paths, e := os.ReadDir("/sys/class/block")
	must(e)
	found := ""
	for _, p := range paths {
		if !strings.HasPrefix(p.Name(), "vd") {
			continue
		}
		f, e := os.Open("/dev/" + p.Name())
		if e != nil {
			continue
		}
		var b [16]byte
		n, e := f.ReadAt(b[:], 1024+120)
		f.Close()
		if e == nil && n == 16 && strings.TrimRight(string(b[:]), "\x00") == label {
			if found != "" {
				panic("duplicate disk label")
			}
			found = "/dev/" + p.Name()
		}
	}
	if found == "" {
		panic("required guest disk missing: " + label)
	}
	return found
}
func mountedAs(mountinfo, target, fs string) bool {
	for _, line := range strings.Split(mountinfo, "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) >= 5 && len(right) >= 1 && left[4] == target && right[0] == fs {
			return true
		}
	}
	return false
}
func mountWithOptions(source, target, fs string, flags uintptr, options string) {
	fmt.Printf("SANDBOX_BOOT_STEP: mount %s (%s)\n", target, fs)
	err := unix.Mount(source, target, fs, flags, options)
	if errors.Is(err, unix.EBUSY) {
		// CONFIG_DEVTMPFS_MOUNT may have mounted /dev before PID 1. Only
		// accept that exact filesystem at that exact mount point, and apply
		// our flags. Do not ignore EBUSY for guest block devices.
		if fs == "devtmpfs" || fs == "proc" || fs == "sysfs" {
			b, e := os.ReadFile("/proc/self/mountinfo")
			if e == nil && mountedAs(string(b), target, fs) {
				err = unix.Mount("", target, "", unix.MS_REMOUNT|flags, options)
			}
		}
	}
	if err != nil {
		panic(fmt.Errorf("mount %s on %s (%s): %w", source, target, fs, err))
	}
}
func mount(source, target, fs string, flags uintptr) { mountWithOptions(source, target, fs, flags, "") }
func relay(a, b io.ReadWriteCloser) {
	defer a.Close()
	defer b.Close()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); io.Copy(a, b); a.Close(); b.Close() }()
	io.Copy(b, a)
	b.Close()
	a.Close()
	wg.Wait()
}
func vsock(port uint32) (*os.File, error) {
	fd, e := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	if e = unix.Connect(fd, &unix.SockaddrVM{CID: 2, Port: port}); e != nil {
		unix.Close(fd)
		return nil, e
	}
	unix.SetNonblock(fd, true)
	return os.NewFile(uintptr(fd), "vsock"), nil
}

func probeVsock(port uint32) error {
	fd, e := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if e != nil {
		return e
	}
	defer unix.Close(fd)
	e = unix.Connect(fd, &unix.SockaddrVM{CID: 2, Port: port})
	if e == nil {
		return nil
	}
	if !errors.Is(e, unix.EINPROGRESS) {
		return e
	}
	p := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
	n, e := unix.Poll(p, 500)
	if e != nil {
		return e
	}
	if n == 0 {
		return unix.ETIMEDOUT
	}
	code, e := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_ERROR)
	if e != nil {
		return e
	}
	if code != 0 {
		return unix.Errno(code)
	}
	return nil
}
func bridgeOut(port int) {
	l, e := net.Listen("tcp4", "127.0.0.1:"+strconv.Itoa(port))
	must(e)
	slots := make(chan struct{}, 32)
	for {
		c, e := l.Accept()
		if e != nil {
			return
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				defer c.Close()
				v, e := vsock(uint32(port))
				if e == nil {
					relay(c, v)
				}
			}()
		default:
			c.Close()
		}
	}
}
func bridgeIn() {
	fd, e := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	must(e)
	must(unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: 8080}))
	must(unix.Listen(fd, 64))
	for {
		c, _, e := unix.Accept4(fd, unix.SOCK_CLOEXEC)
		if e != nil {
			continue
		}
		unix.SetNonblock(c, true)
		file := os.NewFile(uintptr(c), "vsock-in")
		go func() {
			defer file.Close()
			tcp, e := net.DialTimeout("tcp4", "127.0.0.1:8080", time.Second)
			if e == nil {
				relay(file, tcp)
			}
		}()
	}
}

// Host-only maintenance channel. It exposes no shell, paths, environment, or
// arbitrary operation; quiesced shutdown can flush guest storage before kill.
func syncChannel() {
	fd, e := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	must(e)
	must(unix.Bind(fd, &unix.SockaddrVM{CID: unix.VMADDR_CID_ANY, Port: 8081}))
	must(unix.Listen(fd, 4))
	for {
		c, _, e := unix.Accept4(fd, unix.SOCK_CLOEXEC)
		if e != nil {
			continue
		}
		unix.SetNonblock(c, true)
		file := os.NewFile(uintptr(c), "vsock-sync")
		file.SetDeadline(time.Now().Add(2 * time.Second))
		var b [1]byte
		if n, e := file.Read(b[:]); e == nil && n == 1 && b[0] == 'S' {
			unix.Sync()
			file.Write([]byte("OK\n"))
		}
		file.Close()
	}
}
func main() {
	if os.Getpid() != 1 {
		panic("sandbox-init must run as guest PID 1")
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "SANDBOX_BOOT_REFUSED:", r)
		}
		unix.Sync()
		unix.Reboot(unix.LINUX_REBOOT_CMD_POWER_OFF)
	}()
	mount("proc", "/proc", "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC)
	mount("sysfs", "/sys", "sysfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC)
	mount("devtmpfs", "/dev", "devtmpfs", unix.MS_NOSUID)
	must(os.MkdirAll("/dev/pts", 0755))
	mount("devpts", "/dev/pts", "devpts", unix.MS_NOSUID|unix.MS_NOEXEC)
	mountWithOptions("tmpfs", "/tmp", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "size=256m,mode=1777")
	mountWithOptions("tmpfs", "/run", "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "size=32m,mode=755")
	mount(disk("awb-data"), "/data", "ext4", unix.MS_NOSUID|unix.MS_NODEV)
	mount(disk("awb-config"), "/boot-config", "ext4", unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC)
	b, e := os.ReadFile("/boot-config/env.json")
	must(e)
	var env map[string]string
	must(json.Unmarshal(b, &env))
	// The supervisor needs its grant; it receives no host/Vault/provider secret.
	if e := unix.Unmount("/boot-config", 0); e != nil {
		panic(fmt.Errorf("unmount bootstrap config: %w", e))
	}
	must(os.MkdirAll("/data/files", 0700))
	must(os.Chown("/data", 10001, 10001))
	must(os.Chown("/data/files", 10001, 10001))
	ip := exec.Command("/usr/sbin/ip", "link", "set", "lo", "up")
	must(ip.Run())
	interfaces, e := net.Interfaces()
	must(e)
	if len(interfaces) != 1 || interfaces[0].Name != "lo" {
		panic("unexpected guest network interface")
	}
	for _, target := range []string{"1.1.1.1:443", "169.254.169.254:80", "192.168.1.1:80", "[2606:4700:4700::1111]:443"} {
		c, e := net.DialTimeout("tcp", target, 300*time.Millisecond)
		if e == nil {
			c.Close()
			panic("unexpected direct network route")
		}
	}
	if file, e := os.Create("/etc/awb-write-test"); e == nil {
		file.Close()
		panic("root disk is writable")
	}
	if _, e := os.Stat("/var/run/docker.sock"); e == nil {
		panic("unexpected runtime socket")
	}
	for _, port := range []uint32{22, 53, 443, 2375, 8080, 8081, 18080} {
		if e := probeVsock(port); e == nil {
			panic(fmt.Sprintf("unexpected host vsock route on port %d", port))
		}
	}
	fmt.Println("SANDBOX_NETWORK_CHECK: no NIC; direct IPv4/IPv6/metadata/LAN and unapproved vsock ports unavailable")
	for _, p := range []int{18081, 18082, 18083, 18084} {
		go bridgeOut(p)
	}
	go bridgeIn()
	go syncChannel()
	cmd := exec.Command("/usr/local/bin/service")
	cmd.Dir = "/data/files"
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/data", "LANG=C.UTF-8", "TMPDIR=/tmp"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 10001, Gid: 10001}}
	// No console input or interactive root shell. Payload output stays in the VM.
	devnull, e := os.OpenFile("/dev/null", os.O_RDWR, 0)
	must(e)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	must(cmd.Start())
	pid := cmd.Process.Pid
	for {
		var status syscall.WaitStatus
		p, e := syscall.Wait4(-1, &status, 0, nil)
		if e == syscall.EINTR {
			continue
		}
		must(e)
		if p == pid {
			return
		}
	}
}
