package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

func hostOperation(operation string) error {
	if os.Geteuid() != 0 {
		return errors.New("host operations require root")
	}
	if operation == "--prepare-host" {
		info, err := os.Stat("/dev/kvm")
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return errors.New("KVM character device required")
		}
		if _, err = os.Stat("/sys/fs/cgroup/cgroup.controllers"); err != nil {
			return errors.New("cgroup v2 required")
		}
		if err = os.MkdirAll("/sys/fs/cgroup/awb-firecracker", 0755); err != nil {
			return err
		}
		for _, controller := range []string{"cpu", "memory", "pids"} {
			for _, dir := range []string{"/sys/fs/cgroup", "/sys/fs/cgroup/awb-firecracker"} {
				if err = os.WriteFile(filepath.Join(dir, "cgroup.subtree_control"), []byte("+"+controller), 0644); err != nil {
					return err
				}
			}
		}
		for _, dir := range []string{"/var/lib/agent-workbench/firecracker", "/srv/agent-workbench-jailer", "/run/agent-workbench"} {
			if err = os.MkdirAll(dir, 0700); err != nil {
				return err
			}
			if err = os.Chmod(dir, 0700); err != nil {
				return err
			}
		}
		return nil
	}
	groups, err := filepath.Glob("/sys/fs/cgroup/awb-firecracker/fc-*/cgroup.kill")
	if err != nil {
		return err
	}
	var failures []error
	for _, group := range groups {
		if err := os.WriteFile(group, []byte("1"), 0644); err != nil && !os.IsNotExist(err) {
			failures = append(failures, err)
		}
	}
	namespaces, _ := filepath.Glob("/run/netns/awb-fc-*")
	for _, ns := range namespaces {
		if err := exec.Command("ip", "netns", "delete", filepath.Base(ns)).Run(); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
