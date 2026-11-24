package workspace

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"agent-workbench/internal/shared"
	"golang.org/x/sys/unix"
)

func (m *Manager) writeReport(w http.ResponseWriter, r *http.Request) {
	var report shared.ReportWrite
	if !shared.Decode(w, r, &report, 4<<20) {
		return
	}
	if !report.Valid() {
		shared.Error(w, 400, "INVALID_REPORT")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	activity := m.workActivityLocked()
	if m.closed || m.quiescing || !activity.Known || activity.ActiveRuns > 0 || activity.BackgroundJobs > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	if report.Previous == nil {
		if err := m.root.Mkdir("reports", 0700); err != nil && !errors.Is(err, os.ErrExist) {
			shared.Error(w, 503, "REPORT_SAVE_FAILED")
			return
		}
	}
	dir, err := openReviewDirectory(m.root, path.Dir(report.Path))
	if err != nil {
		shared.Error(w, 409, "REPORT_CHANGED")
		return
	}
	defer dir.Close()
	if err := replaceReport(dir, path.Base(report.Path), report); err != nil {
		if errors.Is(err, os.ErrExist) {
			shared.Error(w, 409, "REPORT_CHANGED")
		} else {
			shared.Error(w, 503, "REPORT_SAVE_FAILED")
		}
		return
	}
	m.lastActivity = time.Now().UTC()
	shared.JSON(w, 200, map[string]string{"path": report.Path})
}

func replaceReport(dir *os.File, name string, report shared.ReportWrite) error {
	if report.Previous != nil {
		previous, err := readReport(dir, name)
		if err != nil {
			return os.ErrExist
		}
		if previous == report.Markdown {
			return nil
		}
		if previous != *report.Previous {
			return os.ErrExist
		}
	}
	fd := int(dir.Fd())
	temp := "." + shared.NewID("report")
	tempFD, err := unix.Openat(fd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(fd, temp, 0)
	file := os.NewFile(uintptr(tempFD), temp)
	_, err = file.WriteString(report.Markdown)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	if report.Previous == nil {
		err := unix.Linkat(fd, temp, fd, name, 0)
		if errors.Is(err, os.ErrExist) {
			if previous, err := readReport(dir, name); err == nil && previous == report.Markdown {
				return nil
			}
		}
		return err
	}
	return unix.Renameat(fd, temp, fd, name)
}

func readReport(dir *os.File, name string) (string, error) {
	source, err := openReviewChild(dir, name)
	if err != nil {
		return "", err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > shared.ReportLimit {
		return "", os.ErrInvalid
	}
	content, err := io.ReadAll(io.LimitReader(source, shared.ReportLimit+1))
	if len(content) > shared.ReportLimit {
		return "", os.ErrInvalid
	}
	return string(content), err
}
