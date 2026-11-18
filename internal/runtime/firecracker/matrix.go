package firecracker

import (
	"agent-workbench/internal/gateway"
	"agent-workbench/internal/shared"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const matrixSecrets = "/etc/agent-workbench/firecracker/matrix"

func (f *Firecracker) enableMatrixRoute(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	v := f.vms[r.PathValue("id")]
	running := v != nil && v.running && !v.stopping
	f.mu.Unlock()
	if !running || v.Spec.HermesMatrix == nil {
		shared.Error(w, 409, "MATRIX_RESTART_REQUIRED")
		return
	}
	var config shared.HermesMatrix
	if !shared.Decode(w, r, &config, 16384) {
		return
	}
	a, _ := json.Marshal(config)
	b, _ := json.Marshal(v.Spec.HermesMatrix)
	if string(a) != string(b) {
		shared.Error(w, 409, "MATRIX_RESTART_REQUIRED")
		return
	}
	var secret matrixCredential
	body, err := os.ReadFile(filepath.Join(matrixSecrets, v.Spec.ProjectID+".json"))
	if err != nil || json.Unmarshal(body, &secret) != nil || secret.Config.Homeserver != config.Homeserver || secret.Config.BotUserID != config.BotUserID {
		shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if validateMatrixCredential(ctx, config, secret.Token) != nil {
		shared.Error(w, 400, "MATRIX_CREDENTIAL_REJECTED")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.matrixOwners == nil {
		f.matrixOwners = map[string]string{}
	}
	prior := f.matrixOwners[v.Spec.ProjectID]
	if prior != "" && prior != v.Generation {
		for _, other := range f.vms {
			if other.running && other.Generation == prior {
				shared.Error(w, 409, "MATRIX_ALREADY_CONNECTED")
				return
			}
		}
	}
	f.matrixOwners[v.Spec.ProjectID] = v.Generation
	shared.JSON(w, 200, map[string]bool{"enabled": true})
}

type matrixCredential struct {
	Config shared.HermesMatrix `json:"config"`
	Token  string              `json:"access_token"`
}

func matrixTransport(config shared.HermesMatrix) (*http.Client, *url.URL, error) {
	u, err := url.Parse(config.Homeserver)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, nil, errors.New("invalid Matrix origin")
	}
	t, err := (gateway.NetworkPolicy{AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}).Transport(nil)
	if err != nil {
		return nil, nil, err
	}
	return &http.Client{Transport: t, Timeout: 40 * time.Second, CheckRedirect: gateway.NoRedirect}, u, nil
}
func matrixOriginRequest(ctx context.Context, c *http.Client, u *url.URL, token, method, path, query string, body io.Reader) (*http.Response, error) {
	v := *u
	v.Path = strings.TrimRight(u.Path, "/") + path
	v.RawPath = ""
	v.RawQuery = query
	r, err := http.NewRequestWithContext(ctx, method, v.String(), body)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	return c.Do(r)
}
func validateMatrixCredential(ctx context.Context, config shared.HermesMatrix, token string) error {
	if token == "" || len(token) > 8192 || strings.ContainsAny(token, " \r\n\t") {
		return errors.New("invalid credential")
	}
	c, u, err := matrixTransport(config)
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	read := func(path string, out any) error {
		r, e := matrixOriginRequest(ctx, c, u, token, "GET", path, "", nil)
		if e != nil {
			return e
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			return errors.New("Matrix credential or membership rejected")
		}
		return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(out)
	}
	var who struct {
		ID string `json:"user_id"`
	}
	if read("/_matrix/client/v3/account/whoami", &who) != nil || who.ID != config.BotUserID {
		return errors.New("Matrix identity mismatch")
	}
	var joined struct {
		Rooms []string `json:"joined_rooms"`
	}
	if err = read("/_matrix/client/v3/joined_rooms", &joined); err != nil {
		return err
	}
	allowed := map[string]bool{}
	membership := map[string]bool{}
	for _, id := range config.RoomIDs {
		allowed[id] = true
	}
	for _, id := range joined.Rooms {
		membership[id] = true
		if !allowed[id] {
			return errors.New("bot belongs to a room outside this project")
		}
	}
	for _, room := range config.RoomIDs {
		if !membership[room] {
			return errors.New("bot must already belong to each approved room")
		}
		var encryption struct {
			Algorithm string `json:"algorithm"`
		}
		if read("/_matrix/client/v3/rooms/"+room+"/state/m.room.encryption", &encryption) != nil || encryption.Algorithm != "m.megolm.v1.aes-sha2" {
			return errors.New("room encryption required")
		}
	}
	return nil
}

func matrixRouteAllowed(method, path string, config shared.HermesMatrix) bool {
	if strings.ContainsAny(path, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	if path == "/_matrix/client/versions" {
		return method == "GET"
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "_matrix" || parts[1] != "client" || (parts[2] != "v3" && parts[2] != "r0") {
		return false
	}
	base := strings.Join(parts[3:], "/")
	if method == "GET" && (base == "sync" || base == "account/whoami" || base == "capabilities" || base == "devices" || base == "joined_rooms" || base == "pushrules") {
		return true
	}
	if method == "POST" && (base == "keys/upload" || base == "keys/query" || base == "keys/claim" || base == "keys/signatures/upload" || base == "keys/device_signing/upload") {
		return true
	}
	if method == "GET" && base == "keys/changes" {
		return true
	}
	if parts[3] == "sendToDevice" && len(parts) == 6 && method == "PUT" {
		return parts[4] == "m.room.encrypted" || parts[4] == "m.room_key_request" || parts[4] == "m.room_key.withheld"
	}
	if parts[3] == "filter" {
		return false
	}
	if parts[3] == "user" && len(parts) >= 6 && parts[4] == config.BotUserID && parts[5] == "filter" {
		return method == "GET" || method == "POST"
	}
	if parts[3] != "rooms" || len(parts) < 6 {
		return false
	}
	if !slices.Contains(config.RoomIDs, parts[4]) {
		return false
	}
	operation := parts[5]
	if method == "GET" && (operation == "state" || operation == "members" || operation == "joined_members") {
		return true
	}
	if method == "PUT" && operation == "send" && len(parts) == 8 {
		return parts[6] == "m.room.encrypted"
	}
	return false
}

// Apply the project boundary before any Matrix event reaches guest model,
// session or memory processing. Invites/knocks never reach auto-join handlers.
func filterMatrixSync(body []byte, config shared.HermesMatrix) ([]byte, error) {
	var data map[string]any
	if json.Unmarshal(body, &data) != nil {
		return nil, errors.New("invalid sync response")
	}
	allowed := map[string]bool{}
	senders := map[string]bool{config.BotUserID: true}
	for _, s := range config.SenderIDs {
		senders[s] = true
	}
	for _, r := range config.RoomIDs {
		allowed[r] = true
	}
	rooms, _ := data["rooms"].(map[string]any)
	if rooms != nil {
		delete(rooms, "invite")
		delete(rooms, "knock")
		for _, section := range []string{"join", "leave"} {
			entries, _ := rooms[section].(map[string]any)
			for id, value := range entries {
				if !allowed[id] {
					delete(entries, id)
					continue
				}
				room, _ := value.(map[string]any)
				timeline, _ := room["timeline"].(map[string]any)
				events, _ := timeline["events"].([]any)
				filtered := []any{}
				for _, raw := range events {
					event, _ := raw.(map[string]any)
					sender, _ := event["sender"].(string)
					if !senders[sender] {
						continue
					}
					filtered = append(filtered, raw)
				}
				if timeline != nil {
					timeline["events"] = filtered
				}
			}
		}
	}
	delete(data, "presence")
	return json.Marshal(data)
}

func (f *Firecracker) storeMatrixCredential(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !shared.ValidID(id) {
		shared.Error(w, 400, "INVALID_REQUEST")
		return
	}
	var c matrixCredential
	if !shared.Decode(w, r, &c, 32768) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if validateMatrixCredential(ctx, c.Config, c.Token) != nil {
		shared.Error(w, 400, "MATRIX_CREDENTIAL_REJECTED")
		return
	}
	if os.MkdirAll(matrixSecrets, 0700) != nil {
		shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
		return
	}
	fp, err := os.CreateTemp(matrixSecrets, "credential-")
	if err != nil {
		shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
		return
	}
	name := fp.Name()
	defer os.Remove(name)
	b, _ := json.Marshal(c)
	_, err = fp.Write(b)
	if err == nil {
		err = fp.Sync()
	}
	fp.Close()
	if err == nil {
		err = os.Rename(name, filepath.Join(matrixSecrets, id+".json"))
	}
	if err != nil {
		shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
		return
	}
	shared.JSON(w, 200, map[string]bool{"connected": true})
}

func (f *Firecracker) matrixProxy(v *fcVM) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		running := v.running
		owned := v.Generation != "" && f.matrixOwners[v.Spec.ProjectID] == v.Generation
		f.mu.Unlock()
		if !owned || !running || v.Spec.HermesMatrix == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+f.grant(v))) != 1 || f.verifyVM(v) != nil {
			shared.Error(w, 403, "NOT_FOUND_OR_FORBIDDEN")
			return
		}
		config := *v.Spec.HermesMatrix
		if !matrixRouteAllowed(r.Method, r.URL.Path, config) {
			shared.Error(w, 403, "MATRIX_OPERATION_DENIED")
			return
		}
		b, err := os.ReadFile(filepath.Join(matrixSecrets, v.Spec.ProjectID+".json"))
		var credential matrixCredential
		if err != nil || json.Unmarshal(b, &credential) != nil || credential.Config.Homeserver != config.Homeserver || credential.Config.BotUserID != config.BotUserID {
			shared.Error(w, 503, "CREDENTIAL_UNAVAILABLE")
			return
		}
		c, u, err := matrixTransport(config)
		if err != nil {
			shared.Error(w, 503, "MATRIX_UNAVAILABLE")
			return
		}
		defer c.CloseIdleConnections()
		query := r.URL.Query()
		query.Del("access_token")
		if strings.HasSuffix(r.URL.Path, "/sync") {
			filter, _ := json.Marshal(map[string]any{"room": map[string]any{"rooms": config.RoomIDs}})
			query.Set("filter", string(filter))
			query.Set("timeout", "25000")
		}
		var body io.Reader = http.MaxBytesReader(w, r.Body, 2<<20)
		if strings.Contains(r.URL.Path, "/sendToDevice/") {
			b, err := io.ReadAll(body)
			var payload struct {
				Messages map[string]json.RawMessage `json:"messages"`
			}
			if err != nil || json.Unmarshal(b, &payload) != nil {
				shared.Error(w, 400, "INVALID_REQUEST")
				return
			}
			allowed := map[string]bool{config.BotUserID: true}
			for _, id := range config.SenderIDs {
				allowed[id] = true
			}
			for recipient := range payload.Messages {
				if !allowed[recipient] {
					shared.Error(w, 403, "MATRIX_OPERATION_DENIED")
					return
				}
			}
			body = bytes.NewReader(b)
		}
		res, err := matrixOriginRequest(r.Context(), c, u, credential.Token, r.Method, r.URL.Path, query.Encode(), body)
		if err != nil {
			shared.Error(w, 503, "MATRIX_UNAVAILABLE")
			return
		}
		defer res.Body.Close()
		response, err := io.ReadAll(io.LimitReader(res.Body, 16<<20+1))
		if err != nil || len(response) > 16<<20 {
			shared.Error(w, 503, "MATRIX_UNAVAILABLE")
			return
		}
		if res.StatusCode == 200 && strings.HasSuffix(r.URL.Path, "/sync") {
			response, err = filterMatrixSync(response, config)
			if err != nil {
				shared.Error(w, 503, "MATRIX_UNAVAILABLE")
				return
			}
		}
		if res.StatusCode == 200 && strings.HasSuffix(r.URL.Path, "/joined_rooms") {
			// Membership may have changed since activation. Never expose another
			// project's room even if the bot was joined outside this application.
			var joined struct {
				Rooms []string `json:"joined_rooms"`
			}
			if json.Unmarshal(response, &joined) != nil {
				shared.Error(w, 503, "MATRIX_UNAVAILABLE")
				return
			}
			rooms := []string{}
			for _, room := range joined.Rooms {
				for _, approved := range config.RoomIDs {
					if room == approved {
						rooms = append(rooms, room)
						break
					}
				}
			}
			response, _ = json.Marshal(map[string]any{"joined_rooms": rooms})
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(res.StatusCode)
		w.Write(response)
	})
}
