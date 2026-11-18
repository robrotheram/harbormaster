// Package forge provides bounded, server-authenticated issue operations.
package forge

import (
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"encoding/json"

	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Host struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIURL   string `json:"api_url"`
	Token    string `json:"token,omitempty"`
}

// Hosts are operator-provisioned: a browser cannot redirect a credential to a new host.
func Hosts() ([]Host, error) {
	path := os.Getenv("WORKBENCH_FORGE_CONFIG")
	if path == "" {
		return []Host{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CONFIG_UNAVAILABLE"}
	}
	var hosts []Host
	if json.Unmarshal(b, &hosts) != nil {
		return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CONFIG_INVALID"}
	}
	seen := map[string]bool{}
	for _, h := range hosts {
		u, e := url.Parse(h.APIURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || h.Token == "" || strings.ContainsAny(h.Token, "\r\n") || h.Name == "" || seen[h.Name] || (h.Provider != "github" && h.Provider != "gitlab") {
			return nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "FORGE_CONFIG_INVALID"}
		}
		seen[h.Name] = true
	}
	return hosts, nil
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

func ValidRepository(provider, repository string) bool {
	parts := strings.Split(repository, "/")
	if len(parts) < 2 || (provider == "github" && len(parts) != 2) || len(repository) > 500 {
		return false
	}
	for _, p := range parts {
		if !repoPart.MatchString(p) {
			return false
		}
	}
	return true
}

type Issue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	URL       string   `json:"url"`
	State     string   `json:"state"`
	Labels    []string `json:"labels"`
	Milestone string   `json:"milestone"`
}
type Client struct {
	Host Host
	HTTP *http.Client
}

func (c Client) path(repo string) string {
	if c.Host.Provider == "github" {
		return "/repos/" + repo + "/issues"
	}
	return "/projects/" + url.PathEscape(repo) + "/issues"
}
func (c Client) call(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Host.APIURL, "/")+path, body)
	if err != nil {
		return shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_UNAVAILABLE"}
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Host.Provider == "github" {
		req.Header.Set("Authorization", "Bearer "+c.Host.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("PRIVATE-TOKEN", c.Host.Token)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	if c.HTTP != nil {
		*client = *c.HTTP
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_UNAVAILABLE"}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return shared.CodedError{Status: http.StatusBadGateway, Code: fmt.Sprintf("FORGE_HTTP_%d", resp.StatusCode)}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if err != nil || len(b) > 4<<20 {
		return shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_RESPONSE_TOO_LARGE"}
	}
	if output != nil && json.Unmarshal(b, output) != nil {
		return shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_RESPONSE_INVALID"}
	}
	return nil
}

type rawIssue struct {
	Number      int               `json:"number"`
	IID         int               `json:"iid"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	Description string            `json:"description"`
	HTMLURL     string            `json:"html_url"`
	WebURL      string            `json:"web_url"`
	State       string            `json:"state"`
	PR          json.RawMessage   `json:"pull_request"`
	Labels      []json.RawMessage `json:"labels"`
	Milestone   *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

func (r rawIssue) issue(provider string) Issue {
	i := Issue{Number: r.Number, Title: r.Title, Body: r.Body, URL: r.HTMLURL, State: r.State, Labels: []string{}}
	if provider == "gitlab" {
		i.Number = r.IID
		i.Body = r.Description
		i.URL = r.WebURL
		if i.State == "opened" {
			i.State = "open"
		}
	}
	for _, l := range r.Labels {
		var name string
		if json.Unmarshal(l, &name) != nil {
			var v struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(l, &v)
			name = v.Name
		}
		if name != "" {
			i.Labels = append(i.Labels, name)
		}
	}
	if r.Milestone != nil {
		i.Milestone = r.Milestone.Title
	}
	return i
}
func (c Client) List(ctx context.Context, repo string, page int) ([]Issue, error) {
	var raw []rawIssue
	err := c.call(ctx, "GET", fmt.Sprintf("%s?state=all&per_page=50&page=%d", c.path(repo), page), nil, &raw)
	items := []Issue{}
	for _, r := range raw {
		if len(r.PR) == 0 {
			items = append(items, r.issue(c.Host.Provider))
		}
	}
	return items, err
}
func (c Client) Get(ctx context.Context, repo string, number int) (Issue, error) {
	var raw rawIssue
	err := c.call(ctx, "GET", fmt.Sprintf("%s/%d", c.path(repo), number), nil, &raw)
	if len(raw.PR) > 0 {
		return Issue{}, shared.CodedError{Status: http.StatusBadGateway, Code: "FORGE_PULL_REQUEST_NOT_ISSUE"}
	}
	return raw.issue(c.Host.Provider), err
}
func (c Client) Save(ctx context.Context, repo string, number int, title, body, state string) (Issue, error) {
	var description *string
	if number == 0 {
		description = &body
	}
	return c.SaveDescription(ctx, repo, number, title, description, state)
}

// A nil body preserves the remote description; an empty string clears it.
func (c Client) SaveDescription(ctx context.Context, repo string, number int, title string, body *string, state string) (Issue, error) {
	input := map[string]any{}
	method := "POST"
	path := c.path(repo)
	if number > 0 {
		path += fmt.Sprintf("/%d", number)
		method = "PATCH"
		if c.Host.Provider == "gitlab" {
			method = "PUT"
		}
	}
	if title != "" {
		input["title"] = title
	}
	if body != nil {
		if c.Host.Provider == "github" {
			input["body"] = *body
		} else {
			input["description"] = *body
		}
	}
	if state != "" {
		if c.Host.Provider == "github" {
			input["state"] = state
		} else {
			event := "reopen"
			if state == "closed" {
				event = "close"
			}
			input["state_event"] = event
		}
	}
	var raw rawIssue
	err := c.call(ctx, method, path, input, &raw)
	return raw.issue(c.Host.Provider), err
}
