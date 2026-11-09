// Package filetools implements confined file operations without execution,
// task storage, credentials, or a workspace supervisor.
package filetools

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"syscall"
)

// Service borrows its filesystem root; the caller owns the root's lifetime.
type Service struct{ root *os.Root }

func New(root *os.Root) *Service { return &Service{root: root} }

func (s *Service) Call(_ context.Context, name string, raw json.RawMessage) string {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&args) != nil {
		return `{"error":"INVALID_TOOL_ARGUMENTS"}`
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return `{"error":"INVALID_TOOL_ARGUMENTS"}`
	}
	switch name {
	case "read_file":
		f, e := s.root.OpenFile(args.Path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if e != nil {
			return `{"error":"FILE_UNAVAILABLE"}`
		}
		defer f.Close()
		if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
			return `{"error":"FILE_UNAVAILABLE"}`
		}
		b, e := io.ReadAll(io.LimitReader(f, 65537))
		if e != nil || len(b) > 65536 {
			return `{"error":"FILE_TOO_LARGE"}`
		}
		out, _ := json.Marshal(map[string]string{"content": string(b)})
		return string(out)
	case "write_file":
		if len(args.Content) > 65536 || args.Path == "" {
			return `{"error":"INVALID_FILE"}`
		}
		// os.Root prevents path traversal and symlink escape for all file operations.
		if i := strings.LastIndex(args.Path, "/"); i > 0 {
			if e := s.root.MkdirAll(args.Path[:i], 0700); e != nil {
				return `{"error":"INVALID_FILE"}`
			}
		}
		f, e := s.root.OpenFile(args.Path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NONBLOCK, 0600)
		if e != nil {
			return `{"error":"WRITE_FAILED"}`
		}
		defer f.Close()
		if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
			return `{"error":"INVALID_FILE"}`
		}
		if _, e = f.Write([]byte(args.Content)); e != nil {
			return `{"error":"WRITE_FAILED"}`
		}
		return `{"written":true}`
	case "list_files":
		path := args.Path
		if path == "" {
			path = "."
		}
		f, e := s.root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if e != nil {
			return `{"error":"DIRECTORY_UNAVAILABLE"}`
		}
		defer f.Close()
		if info, err := f.Stat(); err != nil || !info.IsDir() {
			return `{"error":"DIRECTORY_UNAVAILABLE"}`
		}
		entries, e := f.ReadDir(1000)
		if e != nil && e != io.EOF {
			return `{"error":"DIRECTORY_UNAVAILABLE"}`
		}
		names := []string{}
		for _, entry := range entries {
			n := entry.Name()
			if entry.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
		b, _ := json.Marshal(map[string]any{"entries": names})
		return string(b)
	default:
		return `{"error":"UNKNOWN_TOOL"}`
	}
}
