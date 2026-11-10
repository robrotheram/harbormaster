package workspace

import (
	"agent-workbench/internal/shared"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"

	"syscall"
	"time"
)

//go:embed recipes/experiment.py
var researchProgram string

type recipeJob struct {
	ID        string `json:"id"`
	Template  string `json:"template"`
	State     string `json:"state"`
	ErrorCode string `json:"error_code"`
	CreatedAt string `json:"created_at"`
	Timeout   int    `json:"timeout_seconds"`
}

func (m *Manager) recipe(id string) (recipeJob, error) {
	var j recipeJob
	err := m.db.QueryRow(`SELECT id,template,state,error_code,created_at,timeout_seconds FROM recipe_jobs WHERE id=?`, id).Scan(&j.ID, &j.Template, &j.State, &j.ErrorCode, &j.CreatedAt, &j.Timeout)
	return j, err
}
func (m *Manager) getRecipe(w http.ResponseWriter, r *http.Request) {
	j, err := m.recipe(r.PathValue("id"))
	if err != nil {
		shared.Error(w, 404, "JOB_NOT_FOUND")
		return
	}
	shared.JSON(w, 200, j)
}
func (m *Manager) cancelRecipe(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := r.PathValue("id")
	j, err := m.recipe(id)
	if err != nil {
		shared.Error(w, 404, "JOB_NOT_FOUND")
		return
	}
	if c := m.cancels[id]; c != nil {
		_, err = m.db.Exec(`UPDATE recipe_jobs SET state='cancel_requested' WHERE id=?`, id)
		if err != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		c()
		j.State = "cancel_requested"
	}
	shared.JSON(w, 200, j)
}
func (m *Manager) startRecipe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Template string `json:"template"`
		Key      string `json:"idempotency_key"`
		Timeout  int    `json:"timeout_seconds"`
		Resource string `json:"resource_class"`
	}
	if !shared.Decode(w, r, &in, 2048) {
		return
	}
	if in.Timeout == 0 {
		in.Timeout = 600
		if in.Template == "gogallery-v1" {
			in.Timeout = 1800
		}
	}
	if in.Resource == "" {
		in.Resource = "cpu-small"
	}
	if !shared.ValidKey(in.Key) || in.Timeout < 1 || in.Timeout > 3600 || in.Resource != "cpu-small" {
		shared.Error(w, 400, "INVALID_JOB_BUDGET")
		return
	}
	// Only built-in executable recipes are admitted; repository JSON is never authority.
	if in.Template != "research-v1" && in.Template != "gogallery-v1" {
		shared.Error(w, 422, "UNSUPPORTED_APPROVED_TEMPLATE")
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var id string
	err := m.db.QueryRow(`SELECT id FROM recipe_jobs WHERE idem_key=?`, in.Key).Scan(&id)
	if err == nil {
		j, e := m.recipe(id)
		if e != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		if j.Template != in.Template || j.Timeout != in.Timeout {
			shared.Error(w, 409, "IDEMPOTENCY_CONFLICT")
			return
		}
		shared.JSON(w, 200, j)
		return
	}
	if err != sql.ErrNoRows {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	a := m.activityLocked()
	if m.closed || m.quiescing || !a.Known || a.ActiveRuns > 0 || a.BackgroundJobs > 0 || len(m.cancels) > 0 {
		shared.Error(w, 409, "WORKSPACE_BUSY")
		return
	}
	limit, err := exec.LookPath("prlimit")
	if err != nil {
		shared.Error(w, 422, "RESOURCE_LIMITS_UNAVAILABLE")
		return
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		shared.Error(w, 422, "PYTHON_UNAVAILABLE")
		return
	}
	if in.Template == "gogallery-v1" {
		if _, err := os.Stat("/opt/workbench/gogallery/run"); err != nil {
			shared.Error(w, 422, "APPROVED_GOGALLERY_IMAGE_REQUIRED")
			return
		}
	}
	id = shared.NewID("job_")

	if err = m.root.MkdirAll("outputs/"+id, 0700); err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	outputRoot, err := m.root.OpenRoot("outputs/" + id)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer outputRoot.Close()
	directory, err := outputRoot.Open(".")
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	transferred := false
	defer func() {
		if !transferred {
			directory.Close()
		}
	}()
	// Fresh output directory and fixed argv; no user command, host mount, or proxy.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(in.Timeout)*time.Second)
	cmd := exec.CommandContext(ctx, limit, "--as=1073741824", "--cpu=3600", "--nproc=64", "--fsize=33554432", "--", python, "-c", researchProgram+"\nimport json\nfrom pathlib import Path\nPath('variation.json').write_text(json.dumps(experiment(42, 10)))\n", "--output", "baseline.json")
	if in.Template == "gogallery-v1" {
		cmd = exec.CommandContext(ctx, limit, "--as=4294967296", "--cpu=3600", "--nproc=256", "--fsize=134217728", "--", "/opt/workbench/gogallery/run")
	}
	cmd.Env = []string{"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "OPENBLAS_NUM_THREADS=1", "HOME=/tmp", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "GOMAXPROCS=1"}
	cmd.Dir = fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), directory.Fd())
	cmd.ExtraFiles = []*os.File{directory}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	log, err := outputRoot.OpenFile("execution.log", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		cancel()
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	cmd.Stdout = log
	cmd.Stderr = log
	j := recipeJob{id, in.Template, "running", "", time.Now().UTC().Format(time.RFC3339Nano), in.Timeout}
	if _, err = m.db.Exec(`INSERT INTO recipe_jobs(id,idem_key,template,state,created_at,timeout_seconds) VALUES(?,?,?,'running',?,?)`, id, in.Key, in.Template, j.CreatedAt, j.Timeout); err != nil {
		cancel()
		log.Close()
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	m.cancels[id] = cancel
	m.wg.Add(1)
	transferred = true
	go func() {
		defer m.wg.Done()
		defer directory.Close()
		defer cancel()
		err := cmd.Run()
		log.Close()
		m.mu.Lock()
		defer m.mu.Unlock()
		state, code := "succeeded", ""
		if err != nil {
			state, code = "failed", "EXECUTION_FAILED"
		}
		if ctx.Err() != nil {
			state, code = "cancelled", "CANCELLED"
			if ctx.Err() == context.DeadlineExceeded {
				state, code = "failed", "DEADLINE_EXCEEDED"
			}
		}
		if archiveErr := m.archiveRecipeEvidence(id, in.Template, state == "succeeded"); archiveErr != nil {
			state, code = "failed", "EVIDENCE_PUBLICATION_FAILED"
		}
		m.db.Exec(`UPDATE recipe_jobs SET state=?,error_code=? WHERE id=?`, state, code, id)
		delete(m.cancels, id)
	}()
	shared.JSON(w, 202, j)
}

func (m *Manager) listRecipes(w http.ResponseWriter, r *http.Request) {
	rows, err := m.db.Query(`SELECT id,template,state,error_code,created_at,timeout_seconds FROM recipe_jobs ORDER BY created_at DESC,id LIMIT 200`)
	if err != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	defer rows.Close()
	items := []recipeJob{}
	for rows.Next() {
		var j recipeJob
		if rows.Scan(&j.ID, &j.Template, &j.State, &j.ErrorCode, &j.CreatedAt, &j.Timeout) != nil {
			shared.Error(w, 503, "STORAGE_UNAVAILABLE")
			return
		}
		items = append(items, j)
	}
	if rows.Err() != nil {
		shared.Error(w, 503, "STORAGE_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]any{"items": items})
}
