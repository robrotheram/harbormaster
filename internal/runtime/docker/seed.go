package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (c *Controller) archive(ctx context.Context, method, id, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Docker.Base+"/containers/"+url.PathEscape(id)+"/archive?path="+url.QueryEscape(path), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-tar")
	}
	return c.Docker.HTTP.Do(req)
}

// Copy only project files, never the parent's task database, T3 sessions, agent
// homes or grants. The source is frozen during the bounded archive transfer.
func (c *Controller) seedFromParent(ctx context.Context, target containerInfo, parentID string) (err error) {
	if target.Config.Labels["workbench.parent"] != parentID {
		return errors.New("seed ownership mismatch")
	}
	source, e := c.inspect(ctx, parentID)
	if e != nil || source.Config.Labels["workbench.project"] != target.Config.Labels["workbench.project"] {
		return errors.New("cross-project seed denied")
	}
	marker, e := c.archive(ctx, "GET", target.ID, "/data/.workbench-seeded", nil)
	if e == nil {
		defer marker.Body.Close()
		if marker.StatusCode == 200 {
			reader := tar.NewReader(marker.Body)
			if _, e = reader.Next(); e == nil {
				b, _ := io.ReadAll(io.LimitReader(reader, 129))
				if string(b) == parentID {
					return nil
				}
			}
		}
	}
	if target.State.Running {
		return errors.New("refusing to reseed a running workspace")
	}
	if !source.State.Running {
		return errors.New("source unavailable")
	}
	observed, e := c.observe(ctx, parentID)
	if e != nil || !observed.Ready || !observed.Activity.Known || observed.Activity.ActiveRuns > 0 || observed.Activity.BackgroundJobs > 0 {
		return errors.New("source busy")
	}
	if e = c.Docker.call(ctx, "POST", "/containers/"+source.ID+"/pause", nil, nil); e != nil {
		return e
	}
	defer func() {
		resume, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := c.Docker.call(resume, "POST", "/containers/"+source.ID+"/unpause", nil, nil); e != nil {
			err = e
		}
	}()
	res, e := c.archive(ctx, "GET", source.ID, "/data/files", nil)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || res.ContentLength > 256<<20 {
		return errors.New("source archive unavailable or too large")
	}
	output, e := c.archive(ctx, "PUT", target.ID, "/data", io.LimitReader(res.Body, (256<<20)+1))
	if e != nil {
		return e
	}
	output.Body.Close()
	if output.StatusCode != 200 {
		return errors.New("seed transfer failed")
	}
	var data bytes.Buffer
	writer := tar.NewWriter(&data)
	if e = writer.WriteHeader(&tar.Header{Name: ".workbench-seeded", Mode: 0600, Uid: 10001, Gid: 10001, Size: int64(len(parentID))}); e != nil {
		return e
	}
	_, _ = writer.Write([]byte(parentID))
	_ = writer.Close()
	output, e = c.archive(ctx, "PUT", target.ID, "/data", &data)
	if e != nil {
		return e
	}
	output.Body.Close()
	if output.StatusCode != 200 {
		return errors.New("seed marker failed")
	}
	return nil
}
