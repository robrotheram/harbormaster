package workspace

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"agent-workbench/internal/shared"
	"golang.org/x/sys/unix"
)

func (m *Manager) listReviewFiles(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), gitSnapshotTimeout)
	defer cancel()
	const limit = 1000
	files := shared.WorkspaceFiles{Paths: make([]string, 0)}
	err := fs.WalkDir(m.root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if name == "." {
				return err
			}
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if !reviewableProjectPath(name) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			if len(files.Paths) == limit {
				files.Truncated = true
				return fs.SkipAll
			}
			files.Paths = append(files.Paths, name)
		}
		return nil
	})
	if err != nil {
		shared.Error(w, 503, "WORKSPACE_FILES_UNAVAILABLE")
		return
	}
	slices.Sort(files.Paths)
	shared.JSON(w, 200, files)
}

func reviewableProjectPath(name string) bool {
	return utf8.ValidString(name) && shared.ValidProjectFile(name) && name != "work-requests" && !strings.HasPrefix(name, "work-requests/")
}

func (m *Manager) inspectReviewFile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	if !shared.Decode(w, r, &input, 2048) {
		return
	}
	if !reviewableProjectPath(input.Path) {
		shared.Error(w, 400, "INVALID_PROJECT_FILE")
		return
	}
	f, err := openReviewFile(m.root, input.Path)
	if err != nil {
		shared.Error(w, 404, "FILE_UNAVAILABLE")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		shared.Error(w, 404, "FILE_UNAVAILABLE")
		return
	}
	if info.Size() > shared.HandoffLimit {
		shared.Error(w, 413, "WORKSPACE_FILE_TOO_LARGE")
		return
	}
	content, err := io.ReadAll(io.LimitReader(f, shared.HandoffLimit+1))
	if err != nil {
		shared.Error(w, 503, "FILE_UNAVAILABLE")
		return
	}
	if len(content) > shared.HandoffLimit {
		shared.Error(w, 413, "WORKSPACE_FILE_TOO_LARGE")
		return
	}
	shared.JSON(w, 200, shared.FileHandoff{Path: input.Path, Content: content})
}

// Refuse links in every component, including links to private files inside the root.
func openReviewFile(root *os.Root, name string) (*os.File, error) {
	dir, err := openReviewDirectory(root, path.Dir(name))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return openReviewChild(dir, path.Base(name))
}

func openReviewDirectory(root *os.Root, name string) (*os.File, error) {
	return openProjectDirectory(root, name, false)
}

func openProjectDirectory(root *os.Root, name string, create bool) (*os.File, error) {
	dir, err := root.Open(".")
	if err != nil || name == "." {
		return dir, err
	}
	for segment := range strings.SplitSeq(name, "/") {
		next, err := unix.Openat(int(dir.Fd()), segment, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if create && err == unix.ENOENT {
			err = unix.Mkdirat(int(dir.Fd()), segment, 0700)
			if err == nil || err == unix.EEXIST {
				next, err = unix.Openat(int(dir.Fd()), segment, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		dir.Close()
		if err != nil {
			return nil, err
		}
		dir = os.NewFile(uintptr(next), segment)
	}
	return dir, nil
}

func openReviewChild(dir *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
