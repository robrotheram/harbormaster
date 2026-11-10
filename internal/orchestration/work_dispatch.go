package orchestration

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// TickWork participates in the existing scheduler and lifecycle locks. A lost
// acknowledgement always retries the same native acceptance key.
func (r Repository) TickWork(ctx context.Context, s *Scheduler) error {
	rows, err := r.DB.QueryContext(ctx, `SELECT project_id,metadata FROM work_attempts WHERE active_workspace IS NOT NULL ORDER BY id`)
	if err != nil {
		return err
	}
	type pending struct {
		project string
		a       shared.WorkAttempt
	}
	var list []pending
	for rows.Next() {
		var p pending
		var b string
		if err = rows.Scan(&p.project, &b); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal([]byte(b), &p.a); err != nil {
			rows.Close()
			return err
		}
		list = append(list, p)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures error
	for _, p := range list {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		func() {
			unlock := s.Lock("work:"+p.a.WorkItemID, p.a.WorkspaceID)
			defer unlock()
			// Reload after obtaining the same lock used by API transitions.
			attempts, e := r.Attempts(ctx, p.project, p.a.WorkItemID)
			if e != nil {
				failures = errors.Join(failures, e)
				return
			}
			a := p.a
			for _, fresh := range attempts {
				if fresh.ID == a.ID {
					a = fresh
				}
			}
			if a.State == "failed" || a.State == "cancelled" || a.State == "awaiting_review" {
				return
			}
			save := func() {
				a.UpdatedAt = s.Now().UTC()
				failures = errors.Join(failures, r.saveAttempt(ctx, p.project, a))
			}
			ws, e := s.Workspaces.Workspace(ctx, a.WorkspaceID)
			if e != nil {
				if errors.Is(e, sql.ErrNoRows) {
					a.State = "failed"
					a.ErrorCode = "NOT_FOUND_OR_FORBIDDEN"
					save()
				} else {
					failures = errors.Join(failures, e)
				}
				return
			}
			if ws.ProjectID != p.project {
				a.State = "failed"
				a.ErrorCode = "NOT_FOUND_OR_FORBIDDEN"
				save()
				return
			}
			item, e := r.Work(ctx, p.project, a.WorkItemID)
			if e != nil {
				return
			}
			if a.TaskID != "" {
				// A stopped guest must be woken to observe its durable receipt.
				// Read-like observation avoids renewing an explicit user-open.
				if observer, ok := s.Lifecycle.(interface {
					ObserveTask(context.Context, *shared.Workspace) error
				}); ok {
					if e = observer.ObserveTask(ctx, &ws); e != nil {
						return
					}
				}
				if a.State == "cancel_requested" {
					status, _, e := s.Agent.Agent(ctx, ws.ID, "POST", "/v1/tasks/"+a.TaskID+"/cancel", []byte(`{}`))
					if e != nil || status >= 300 {
						return
					}
				}
				status, b, e := s.Agent.Agent(ctx, ws.ID, "GET", "/v1/tasks/"+a.TaskID, nil)
				var task shared.Task
				if e != nil || status != 200 || json.Unmarshal(b, &task) != nil || !shared.ValidTask(task, ws.ID) || task.ID != a.TaskID {
					return
				}
				cancelling := a.State == "cancel_requested"
				a.State = task.State
				a.ErrorCode = shared.SanitizedTaskError(task.ErrorCode)
				if cancelling && shared.Active(task.State) {
					a.State = "cancel_requested"
				}
				if task.State == "completed" || task.State == "succeeded" {
					if cancelling {
						a.State = "cancelled"
					} else {
						a.State = "awaiting_review"
						a.Artifacts = []shared.WorkArtifact{}
						for index, path := range item.ArtifactPaths {
							body, _ := json.Marshal(map[string]string{"source_path": path, "idempotency_key": fmt.Sprintf("work:%s:artifact:%d", a.ID, index)})
							status, bytes, err := s.Agent.Agent(ctx, ws.ID, "POST", "/v1/artifacts", body)
							var artifact shared.WorkArtifact
							if err != nil || (status != 200 && status != 201) || json.Unmarshal(bytes, &artifact) != nil || !shared.ValidID(artifact.ID) {
								a.State = "failed"
								a.ErrorCode = "ARTIFACT_PUBLICATION_FAILED"
								break
							}
							artifact.ContentURL = "/v1/work-items/" + item.ID + "/artifacts/" + artifact.ID + "/content?project_id=" + url.QueryEscape(p.project)
							a.Artifacts = append(a.Artifacts, artifact)
						}

					}
				}
				save()
				return
			}
			if a.State == "cancel_requested" { // Resolve uncertain dispatch before cancelling; never abandon a possibly accepted task.
				if !a.DispatchStarted {
					a.State = "cancelled"
					save()
					return
				}
			}
			if e = s.Lifecycle.Refresh(ctx, &ws); e != nil || (ws.State == "BUSY" && !a.DispatchStarted) || ws.State == "UNKNOWN" {
				return
			}
			if e = s.Lifecycle.Start(ctx, &ws); e != nil {
				a.ErrorCode = "WORKSPACE_UNAVAILABLE"
				save()
				return
			}
			cancelling := a.State == "cancel_requested"
			a.DispatchStarted = true
			if !cancelling {
				a.State = "dispatching"
			}
			a.UpdatedAt = s.Now().UTC()
			if e = r.saveAttempt(ctx, p.project, a); e != nil {
				failures = errors.Join(failures, e)
				return
			}
			b, _ := json.Marshal(map[string]string{"instruction_path": item.SpecificationPath, "specification_digest": item.SpecificationDigest, "provider": "", "idempotency_key": "work:" + a.ID})
			status, reply, e := s.Agent.Agent(ctx, ws.ID, "POST", "/v1/tasks/from-file", b)
			if e != nil {
				return
			}
			var task shared.Task
			if (status == 200 || status == 202) && json.Unmarshal(reply, &task) == nil && shared.ValidTask(task, ws.ID) {
				a.TaskID = task.ID
				a.State = task.State
				if cancelling {
					a.State = "cancel_requested"
				}
				a.ErrorCode = ""
				if e = s.Workspaces.SaveTask(ctx, task); e != nil {
					failures = errors.Join(failures, e)
				}
			} else if status >= 400 && status < 500 {
				a.State = "failed"
				a.ErrorCode = "SPECIFICATION_ACCEPTANCE_FAILED"
			}
			save()
		}()
	}
	return failures
}
