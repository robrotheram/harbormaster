package coordination

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"strings"
	"syscall"
)

// Files owns a project-specific export directory, never a cross-project mount.
type Files struct{ Root *os.Root }

func (f Files) Put(_ context.Context, key string, b []byte) error {
	if !validBlobKey(key) {
		return ErrInvalid
	}

	temp := "pending-" + rand.Text()
	file, e := f.Root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Root.Remove(temp)
	_, e = file.Write(b)
	if e == nil {
		e = file.Sync()
	}
	ce := file.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	e = f.Root.Link(temp, key)
	if errors.Is(e, os.ErrExist) {
		old, readErr := readBounded(f.Root, key)
		if readErr != nil {
			return readErr
		}
		if Digest(old) != Digest(b) {
			return ErrConflict
		}
		return nil
	}
	if e != nil {
		return e
	}
	dir, e := f.Root.Open(".")
	if e == nil {
		e = dir.Sync()
		dir.Close()
	}
	return e
}

func (f Files) Get(_ context.Context, key string) ([]byte, error) {
	if !validBlobKey(key) {
		return nil, ErrDenied
	}
	return readBounded(f.Root, key)
}

type NoteWrite struct {
	Path           string   `json:"path"`
	Body           string   `json:"body"`
	ExpectedDigest string   `json:"expected_digest"`
	Occurrence     string   `json:"occurrence"`
	Sources        []string `json:"sources"`
}
type NoteReceipt struct {
	Path           string `json:"path"`
	Digest         string `json:"digest"`
	Occurrence     string `json:"occurrence"`
	PreviousDigest string `json:"previous_digest"`
	RequestDigest  string `json:"request_digest"`
}

// Notes is explicitly provisioned per project/workspace. The root must be a
// dedicated bounded notes directory, not a home or inherited personal vault.
// .awb-* is reserved for durable journals. flock coordinates service processes;
// external editors must honor the same lock for atomic compare-and-write.
type NotePublisher interface {
	Publish(NoteWrite) (NoteReceipt, error)
}

type Notes struct{ Root *os.Root }

func (n Notes) Publish(in NoteWrite) (NoteReceipt, error) {
	var out NoteReceipt
	if !valid(in.Occurrence) || !valid(strings.TrimSuffix(in.Path, ".md")) || !strings.HasSuffix(in.Path, ".md") || len(in.Body) > 65536 || len(in.Sources) == 0 || len(in.Sources) > 32 {
		return out, ErrInvalid
	}
	for _, link := range in.Sources {
		u, e := url.Parse(link)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(link, "\r\n") {
			return out, ErrInvalid
		}
	}
	body := in.Body + "\n\nSources:\n"
	for _, link := range in.Sources {
		body += "- " + link + "\n"
	}
	if len(body) > 65536 {
		return out, ErrInvalid
	}
	request, _ := json.Marshal(in)
	out = NoteReceipt{in.Path, Digest([]byte(body)), in.Occurrence, in.ExpectedDigest, Digest(request)}
	lock, e := n.Root.OpenFile(".awb-lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return out, e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); e != nil {
		return out, e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	journal := ".awb-" + in.Occurrence + ".json"
	prior, e := readBounded(n.Root, journal)
	if e == nil {
		var receipt NoteReceipt
		if json.Unmarshal(prior, &receipt) != nil || receipt != out {
			return out, ErrConflict
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	current, e := readBounded(n.Root, in.Path)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	digest := ""
	if e == nil {
		digest = Digest(current)
	}
	if prior != nil && digest == out.Digest {
		return out, nil
	}
	if digest != in.ExpectedDigest {
		return out, ErrConflict
	}
	if prior == nil {
		b, _ := json.Marshal(out)
		f, e := n.Root.OpenFile(journal, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return out, e
		}
		_, e = f.Write(b)
		if e == nil {
			e = f.Sync()
		}
		f.Close()
		if e != nil {
			return out, e
		}
	}
	temp := ".awb-" + in.Occurrence + ".tmp"
	f, e := n.Root.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return out, e
	}
	_, e = f.WriteString(body)
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	if e != nil {
		return out, e
	}
	// Recheck after preparing the replacement to detect edits made during IO.
	latest, e := readBounded(n.Root, in.Path)
	if e != nil && !errors.Is(e, os.ErrNotExist) {
		return out, e
	}
	latestDigest := ""
	if e == nil {
		latestDigest = Digest(latest)
	}
	if latestDigest != digest {
		return out, ErrConflict
	}
	if e = n.Root.Rename(temp, in.Path); e != nil {
		return out, e
	}
	dir, e := n.Root.Open(".")
	if e == nil {
		e = dir.Sync()
		dir.Close()
	}
	return out, e
}

func validBlobKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}
func readBounded(root *os.Root, path string) ([]byte, error) {
	file, err := root.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, ErrInvalid
	}
	body, err := io.ReadAll(io.LimitReader(file, 65537))
	if len(body) > 65536 {
		return nil, ErrInvalid
	}
	return body, err
}
