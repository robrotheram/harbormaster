package kubernetes

// Kubernetes delegates Pod/PVC lifecycle to Kubernetes SIGs Agent Sandbox.
// The runtime controller remains the only holder of Kubernetes credentials.
import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

const SandboxAPIVersion = "agents.x-k8s.io/v1beta1"
const AgentSandboxRelease = "v1.0.9"

type KubernetesConfig struct {
	APIURL, APIToken, CAFile                            string
	Token, GatewayServiceToken, GrantSecret, Deployment string
	Image, RuntimeClass, StorageClass, ServiceAccount   string
	InferenceURL, BrokerURL, CodexModel, ClaudeModel    string
	ProjectNamespaces                                   map[string]string
	ProvisionProjectNamespaces                          bool
	ProvisionWorkspaceNamespaces                        bool
	SharedGatewayProject                                string
	CoreNamespace                                       string
	ProjectAllowlists                                   map[string][]string
	EgressURL                                           string
	MemoryLimit                                         int64
	CPURequestMillis                                    int
	RequireVerified                                     bool
	HermesApprovedImage, HermesModel, HermesMCPIP       string
	DiscoverGateways                                    bool
}

type Kubernetes struct {
	cfg                KubernetesConfig
	apiHTTP, agentHTTP *http.Client
	mu                 sync.Mutex
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var pinnedImage = regexp.MustCompile(`^[a-z0-9][a-z0-9._:/-]*@sha256:[a-f0-9]{64}$`)

func NewKubernetes(c KubernetesConfig) (*Kubernetes, error) {
	if c.CPURequestMillis == 0 {
		c.CPURequestMillis = 1000
	}
	if c.CPURequestMillis < 1 || c.CPURequestMillis > 1000 {
		return nil, errors.New("invalid Kubernetes worker CPU request")
	}
	u, err := url.Parse(c.APIURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || c.APIToken == "" || len(c.Token) < 32 || len(c.GatewayServiceToken) < 32 || len(c.GrantSecret) < 32 || !dnsLabel.MatchString(c.Deployment) || len(c.Deployment) > 20 || !pinnedImage.MatchString(c.Image) || c.RuntimeClass != "kata" || !dnsLabel.MatchString(c.ServiceAccount) || c.StorageClass == "" || len(c.ProjectNamespaces) == 0 {
		return nil, errors.New("invalid Kubernetes runtime configuration")
	}
	if c.CoreNamespace == "" {
		c.CoreNamespace = "harbormaster"
	}
	if !dnsLabel.MatchString(c.CoreNamespace) {
		return nil, errors.New("invalid core namespace")
	}
	if c.SharedGatewayProject != "" && !shared.ValidID(c.SharedGatewayProject) {
		return nil, errors.New("invalid shared gateway project")
	}
	if c.HermesMCPIP != "" {
		addr, err := netip.ParseAddr(c.HermesMCPIP)
		if err != nil || !addr.IsGlobalUnicast() || addr.Zone() != "" {
			return nil, errors.New("invalid Hermes MCP service IP")
		}
	}
	for service, origin := range map[string]*string{
		"inference-gateway": &c.InferenceURL,
		"credential-broker": &c.BrokerURL,
		"egress-gateway":    &c.EgressURL,
	} {
		if *origin == "" {
			*origin = "http://" + service + "." + c.CoreNamespace + ".svc:8080"
		}
	}
	for project, destinations := range c.ProjectAllowlists {
		if _, ok := c.ProjectNamespaces[project]; !ok {
			return nil, errors.New("allowlist project not bound")
		}
		if !(shared.ConnectionPolicy{Mode: "allowlist", Destinations: destinations}).Valid() {
			return nil, errors.New("invalid project allowlist")
		}
	}
	seen := map[string]bool{}
	for project, namespace := range c.ProjectNamespaces {
		if !shared.ValidID(project) || !dnsLabel.MatchString(namespace) || seen[namespace] {
			return nil, errors.New("invalid project namespace binding")
		}
		seen[namespace] = true
	}
	ca, err := os.ReadFile(c.CAFile)
	if err != nil {
		return nil, errors.New("Kubernetes CA unavailable")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid Kubernetes CA")
	}
	if c.MemoryLimit == 0 {
		c.MemoryLimit = 1 << 30
	}
	for _, origin := range []string{c.InferenceURL, c.BrokerURL, c.EgressURL} {
		u, err := url.Parse(origin)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Path != "" || u.RawQuery != "" {
			return nil, errors.New("invalid internal gateway origin")
		}
	}
	denyRedirect := func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	k := &Kubernetes{cfg: c, apiHTTP: &http.Client{Timeout: 30 * time.Second, CheckRedirect: denyRedirect, Transport: &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}}, agentHTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: denyRedirect, Transport: &http.Transport{Proxy: nil}}}
	if c.DiscoverGateways {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := k.discoverGateways(ctx); err != nil {
			return nil, err
		}
	}
	return k, nil
}

type kubeResource struct {
	Metadata struct {
		Name, Namespace string
		Labels          map[string]string
		Generation      int64  `json:"generation"`
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
	Spec struct {
		OperatingMode string `json:"operatingMode"`
		PodTemplate   struct {
			Spec struct {
				RuntimeClassName string `json:"runtimeClassName"`
				Containers       []struct {
					Image string `json:"image"`
				} `json:"containers"`
			} `json:"spec"`
		} `json:"podTemplate"`
	} `json:"spec"`
	Status struct {
		Conditions []struct {
			Type, Status, Reason string
			ObservedGeneration   int64 `json:"observedGeneration"`
		} `json:"conditions"`
	} `json:"status"`
}

func (k *Kubernetes) call(ctx context.Context, method, path string, body any, out any) error {
	var b []byte
	var err error
	if body != nil {
		b, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, k.cfg.APIURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+k.cfg.APIToken)
	r.Header.Set("Content-Type", "application/json")
	if method == "PATCH" {
		r.Header.Set("Content-Type", "application/merge-patch+json")
	}
	res, err := k.apiHTTP.Do(r)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return protocol.APIError{Status: res.StatusCode}
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
	}
	return nil
}

func (k *Kubernetes) collection(namespace string) string {
	return "/apis/" + SandboxAPIVersion + "/namespaces/" + namespace + "/sandboxes"
}
func (k *Kubernetes) name(id string) string {
	return strings.ReplaceAll(k.cfg.Deployment+"-"+strings.ToLower(id), "_", "-")
}
func (k *Kubernetes) token(id, generation string) string {
	return protocol.Grant(k.cfg.GrantSecret, id, generation)
}
func (k *Kubernetes) namespaces() []string {
	ns := []string{}
	for _, n := range k.cfg.ProjectNamespaces {
		ns = append(ns, n)
	}
	sort.Strings(ns)
	return ns
}
func (k *Kubernetes) owned(resource kubeResource, id string) bool {
	l := resource.Metadata.Labels
	return protocol.HasOwnershipLabels(l) && l["workbench.deployment"] == k.cfg.Deployment && l["workbench.workspace"] == id && k.workspaceNamespace(l["workbench.project"]) == resource.Metadata.Namespace && resource.Metadata.Name == k.name(id)
}
func (k *Kubernetes) find(ctx context.Context, id string) (kubeResource, error) {
	if !shared.ValidID(id) || !dnsLabel.MatchString(k.name(id)) {
		return kubeResource{}, errors.New("invalid workspace")
	}
	var found *kubeResource
	namespaces, err := k.runtimeNamespaces(ctx)
	if err != nil {
		return kubeResource{}, err
	}
	for _, namespace := range namespaces {
		var list struct {
			Items []kubeResource `json:"items"`
		}
		selector := url.QueryEscape("workbench.deployment=" + k.cfg.Deployment + ",workbench.workspace=" + id)
		if err := k.call(ctx, "GET", k.collection(namespace)+"?labelSelector="+selector, nil, &list); err != nil {
			return kubeResource{}, err
		}
		for _, item := range list.Items {
			if !k.owned(item, id) || found != nil {
				return kubeResource{}, errors.New("ambiguous or invalid workspace ownership")
			}
			copy := item
			found = &copy
		}
	}
	if found == nil {
		return kubeResource{}, protocol.APIError{Status: 404}
	}
	return *found, nil
}
func (k *Kubernetes) endpoint(resource kubeResource) string {
	return "http://" + resource.Metadata.Name + "." + resource.Metadata.Namespace + ".svc:8080"
}
func (k *Kubernetes) agent(ctx context.Context, resource kubeResource, method, path string, body []byte) (int, []byte, error) {
	r, err := http.NewRequestWithContext(ctx, method, k.endpoint(resource)+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	r.Header.Set("Authorization", "Bearer "+k.token(resource.Metadata.Labels["workbench.workspace"], resource.Metadata.Labels["workbench.generation"]))
	r.Header.Set("Content-Type", "application/json")
	res, err := k.agentHTTP.Do(r)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, protocol.AgentResponseLimit(path)+1))
	if int64(len(b)) > protocol.AgentResponseLimit(path) {
		return 0, nil, errors.New("workspace response too large")
	}
	return res.StatusCode, b, err
}
func sandboxReady(r kubeResource) bool {
	if r.Spec.OperatingMode == "Suspended" {
		return false
	}
	for _, c := range r.Status.Conditions {
		if c.Type == "Ready" && c.Status == "True" && c.ObservedGeneration == r.Metadata.Generation {
			return true
		}
	}
	return false
}
func sandboxSuspended(r kubeResource) bool {
	if r.Spec.OperatingMode != "Suspended" {
		return false
	}
	for _, condition := range r.Status.Conditions {
		if condition.Type == "Suspended" && condition.Status == "True" && condition.ObservedGeneration == r.Metadata.Generation {
			return true
		}
	}
	return false
}

const workspaceUnschedulable = "WORKSPACE_UNSCHEDULABLE"

func sandboxUnschedulable(r kubeResource) bool {
	if r.Spec.OperatingMode == "Suspended" || sandboxReady(r) {
		return false
	}
	for _, c := range r.Status.Conditions {
		if c.Type == "PodScheduled" && c.Status == "False" && c.Reason == "Unschedulable" && c.ObservedGeneration == r.Metadata.Generation {
			return true
		}
	}
	return false
}

func (k *Kubernetes) observe(ctx context.Context, r kubeResource) (shared.Observation, error) {
	o := shared.Observation{RuntimeType: "kata", IsolationStatus: "unverified", NetworkStatus: "unverified", Running: true}
	if r.Spec.OperatingMode == "Suspended" {
		o.Running = !sandboxSuspended(r)
		return o, nil
	}
	if !sandboxReady(r) || k.cfg.RequireVerified {
		if sandboxUnschedulable(r) {
			o.StartupReason = workspaceUnschedulable
		}
		return o, nil
	}
	status, b, err := k.agent(ctx, r, "GET", "/v1/activity", nil)
	if err == nil && status == 200 && json.Unmarshal(b, &o.Activity) == nil {
		o.Ready = true
		if r.Metadata.Labels["workbench.agent"] == "hermes" {
			status, body, e := k.agent(ctx, r, "GET", "/v1/runtime-capabilities", nil)
			var caps struct {
				Hermes bool `json:"hermes"`
			}
			o.Ready = k.hermesEnabled() && e == nil && status == 200 && json.Unmarshal(body, &caps) == nil && caps.Hermes
		}
	}
	return o, nil
}
func (k *Kubernetes) StartConfigured(ctx context.Context, id string, spec protocol.WorkspaceSpec) (shared.Observation, error) {
	if err := protocol.CheckResources(spec.Resources, protocol.ResourceLimits{CPUCores: 1, MemoryBytes: k.cfg.MemoryLimit}); err != nil {
		return shared.Observation{}, err
	}
	spec, err := protocol.NormalizeSpec(spec)
	if err != nil {
		return shared.Observation{}, err
	}
	if spec.AgentKind == "hermes" && (!k.hermesEnabled() || spec.HermesMatrix != nil) {
		return shared.Observation{}, shared.CodedError{Code: "HERMES_RUNTIME_NOT_APPROVED_OR_MATRIX_UNSUPPORTED"}
	}
	if k.cfg.RequireVerified {
		return shared.Observation{}, shared.CodedError{Code: "VERIFIED_ISOLATION_UNAVAILABLE"}
	}
	if spec.ConnectionMode != "" && spec.ConnectionMode != "blocked" && spec.ConnectionMode != "allowlist" {
		return shared.Observation{}, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "NETWORK_POLICY_UNAVAILABLE"}
	}
	if spec.ParentID != "" {
		return shared.Observation{}, shared.CodedError{Code: "KUBERNETES_BRANCH_SEED_UNAVAILABLE"}
	}
	namespace := k.workspaceNamespace(spec.ProjectID)
	if namespace == "" || !shared.ValidID(id) || !dnsLabel.MatchString(k.name(id)) {
		return shared.Observation{}, errors.New("project runtime not provisioned")
	}
	if _, bound := k.cfg.ProjectNamespaces[spec.ProjectID]; !bound {
		if err := k.ensureWorkspaceNamespace(ctx, spec.ProjectID); err != nil {
			return shared.Observation{}, err
		}
	}
	if err := k.networkEvidence(ctx, namespace, spec.ProjectID); err != nil {
		return shared.Observation{}, err
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	resource, err := k.find(ctx, id)
	var absent protocol.APIError
	if err != nil && (!errors.As(err, &absent) || absent.Status != 404) {
		return shared.Observation{}, err
	}
	if err == nil {
		if err := k.storageEvidence(ctx, resource); err != nil {
			return shared.Observation{}, err
		}
		if protocol.CheckOwnership(resource.Metadata.Labels, spec) != nil || resource.Metadata.Labels["workbench.agent"] != spec.AgentKind || resource.Spec.PodTemplate.Spec.RuntimeClassName != k.cfg.RuntimeClass || len(resource.Spec.PodTemplate.Spec.Containers) != 1 || resource.Spec.PodTemplate.Spec.Containers[0].Image != k.cfg.Image {
			return shared.Observation{}, errors.New("sandbox ownership or policy drift")
		}
		if err = k.call(ctx, "PATCH", k.collection(namespace)+"/"+resource.Metadata.Name, map[string]any{"spec": map[string]string{"operatingMode": "Running"}}, nil); err != nil {
			return shared.Observation{}, err
		}
	} else {
		labels := map[string]string{"workbench.deployment": k.cfg.Deployment, "workbench.workspace": id, "workbench.project": spec.ProjectID, "workbench.home": spec.HomeID, "workbench.agent": spec.AgentKind, "workbench.template": spec.TemplateID, "workbench.generation": shared.NewID("gen_")}
		secretName := k.name(id) + "-grant"
		secretPath := "/api/v1/namespaces/" + namespace + "/secrets/" + secretName
		var secret kubeResource
		secretErr := k.call(ctx, "GET", secretPath, nil, &secret)
		if secretErr == nil {
			if protocol.CheckOwnership(secret.Metadata.Labels, spec) != nil || secret.Metadata.Labels["workbench.workspace"] != id || secret.Metadata.Labels["workbench.deployment"] != k.cfg.Deployment {
				return shared.Observation{}, errors.New("grant ownership mismatch")
			}
			labels["workbench.generation"] = secret.Metadata.Labels["workbench.generation"]
			if labels["workbench.generation"] == "" {
				return shared.Observation{}, errors.New("invalid grant generation")
			}
		} else {
			var missing protocol.APIError
			if !errors.As(secretErr, &missing) || missing.Status != 404 {
				return shared.Observation{}, secretErr
			}
			body := map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": secretName, "namespace": namespace, "labels": labels}, "type": "Opaque", "data": map[string][]byte{"token": []byte(k.token(id, labels["workbench.generation"]))}}
			if err = k.call(ctx, "POST", "/api/v1/namespaces/"+namespace+"/secrets", body, nil); err != nil {
				return shared.Observation{}, err
			}
		}
		if err = k.call(ctx, "POST", k.collection(namespace), k.sandbox(id, spec, namespace, labels, secretName), nil); err != nil {
			return shared.Observation{}, err
		}
	}
	wait, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for wait.Err() == nil {
		current, err := k.find(wait, id)
		if err != nil {
			if wait.Err() != nil {
				break
			}
			return shared.Observation{}, err
		}
		resource = current
		if sandboxReady(resource) {
			status, _, e := k.agent(wait, resource, "POST", "/v1/resume", nil)
			if e == nil && status == 200 {
				return k.observe(wait, resource)
			}
		}
		select {
		case <-wait.Done():
		case <-time.After(200 * time.Millisecond):
		}
	}
	code := "NOT_READY"
	if sandboxUnschedulable(resource) {
		code = workspaceUnschedulable
	}
	return shared.Observation{}, shared.CodedError{Status: http.StatusServiceUnavailable, Code: code}
}
func (k *Kubernetes) sandbox(id string, spec protocol.WorkspaceSpec, namespace string, labels map[string]string, secretName string) map[string]any {
	env := []map[string]any{}
	provider := "codex"
	if spec.AgentKind == "hermes" {
		provider = "hermes"
	}
	for name, value := range map[string]string{"WORKSPACE_ID": id, "PROJECT_ID": spec.ProjectID, "AGENT_HOME_ID": spec.HomeID, "AGENT_KIND": spec.AgentKind, "WORKSPACE_NAME": spec.Name, "DATA_DIR": "/data", "MONITOR_PROCESSES": "true", "WORKSPACE_T3": fmt.Sprint(spec.AgentKind == "t3" || spec.ConsoleEnabled), "WORKSPACE_AGENT": provider, "CODEX_MODEL": k.cfg.CodexModel, "CLAUDE_MODEL": k.cfg.ClaudeModel, "INFERENCE_URL": k.cfg.InferenceURL, "BROKER_URL": k.cfg.BrokerURL, "EGRESS_URL": k.cfg.EgressURL} {
		env = append(env, map[string]any{"name": name, "value": value})
	}
	if spec.AgentKind == "hermes" {
		env = append(env, map[string]any{"name": "HERMES_MODEL", "value": k.cfg.HermesModel}, map[string]any{"name": "HERMES_MCP_TRANSPORT", "value": "kubernetes-generation"})
	}
	env = append(env, map[string]any{"name": "WORKSPACE_TOKEN", "valueFrom": map[string]any{"secretKeyRef": map[string]string{"name": secretName, "key": "token"}}})
	container := map[string]any{"name": "workspace", "image": k.cfg.Image, "imagePullPolicy": "IfNotPresent", "env": env, "ports": []any{map[string]any{"containerPort": 8080, "name": "workspace"}}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": map[string]any{"drop": []string{"ALL"}}}, "resources": map[string]any{"requests": map[string]string{"cpu": fmt.Sprintf("%dm", k.cfg.CPURequestMillis), "memory": fmt.Sprint(k.cfg.MemoryLimit)}, "limits": map[string]string{"cpu": "1", "memory": fmt.Sprint(k.cfg.MemoryLimit)}}, "volumeMounts": []any{map[string]string{"name": "data", "mountPath": "/data"}, map[string]string{"name": "tmp", "mountPath": "/tmp"}}, "readinessProbe": map[string]any{"httpGet": map[string]any{"path": "/healthz", "port": 8080}, "periodSeconds": 2}}
	pod := map[string]any{"runtimeClassName": k.cfg.RuntimeClass, "priorityClassName": "harbormaster-workspace", "serviceAccountName": k.cfg.ServiceAccount, "automountServiceAccountToken": false, "restartPolicy": "Never", "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001, "seccompProfile": map[string]string{"type": "RuntimeDefault"}}, "containers": []any{container}, "volumes": []any{map[string]any{"name": "tmp", "emptyDir": map[string]string{"medium": "Memory", "sizeLimit": "64Mi"}}}}
	if spec.AgentKind == "hermes" && k.cfg.HermesMCPIP != "" {
		pod["hostAliases"] = []map[string]any{{"ip": k.cfg.HermesMCPIP, "hostnames": []string{"hermes-mcp.harbormaster.svc"}}}
	}
	return map[string]any{"apiVersion": SandboxAPIVersion, "kind": "Sandbox", "metadata": map[string]any{"name": k.name(id), "namespace": namespace, "labels": labels}, "spec": map[string]any{"operatingMode": "Running", "service": true, "failedPodPolicy": "Retain", "podTemplate": map[string]any{"metadata": map[string]any{"labels": labels}, "spec": pod}, "volumeClaimTemplates": []any{map[string]any{"metadata": map[string]any{"name": "data", "labels": labels}, "spec": map[string]any{"accessModes": []string{"ReadWriteOncePod"}, "storageClassName": k.cfg.StorageClass, "resources": map[string]any{"requests": map[string]string{"storage": "10Gi"}}}}}}}
}
func (k *Kubernetes) Inspect(ctx context.Context, id string) (shared.Observation, error) {
	r, err := k.find(ctx, id)
	var missing protocol.APIError
	if errors.As(err, &missing) && missing.Status == 404 {
		return shared.Observation{RuntimeType: "kata", IsolationStatus: "unverified", NetworkStatus: "unverified"}, nil
	}
	if err != nil {
		return shared.Observation{}, err
	}
	return k.observe(ctx, r)
}
func (k *Kubernetes) Stop(ctx context.Context, id string) error {
	return k.stop(ctx, id, false)
}

func (k *Kubernetes) stop(ctx context.Context, id string, force bool) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	r, err := k.find(ctx, id)
	if err != nil {
		if force && isNotFound(err) {
			return nil
		}
		return err
	}
	if r.Spec.OperatingMode == "Suspended" && !force {
		return nil
	}
	if !force {
		status, _, err := k.agent(ctx, r, "POST", "/v1/quiesce", nil)
		if err != nil || status != 200 {
			return shared.CodedError{Status: http.StatusConflict, Code: "WORKSPACE_BUSY"}
		}
	}
	if err = k.call(ctx, "PATCH", k.collection(r.Metadata.Namespace)+"/"+r.Metadata.Name, map[string]any{"spec": map[string]string{"operatingMode": "Suspended"}}, nil); err != nil {
		if !force {
			resume, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _, _ = k.agent(resume, r, "POST", "/v1/resume", nil)
		}
		return err
	}
	wait, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	for {
		r, err = k.find(wait, id)
		if err != nil {
			return err
		}
		if sandboxSuspended(r) {
			return nil
		}
		select {
		case <-wait.Done():
			return shared.CodedError{Code: "STOP_UNCONFIRMED"}
		case <-time.After(200 * time.Millisecond):
		}
	}
}
func (k *Kubernetes) Reconcile(ctx context.Context, id string) error {
	r, err := k.find(ctx, id)
	if err != nil {
		return err
	}
	return k.networkEvidence(ctx, r.Metadata.Namespace, r.Metadata.Labels["workbench.project"])
}
func (k *Kubernetes) Agent(ctx context.Context, id, method, path string, b []byte) (int, []byte, error) {
	if k.cfg.RequireVerified {
		return 0, nil, shared.CodedError{Code: "VERIFIED_ISOLATION_UNAVAILABLE"}
	}
	clean := strings.SplitN(path, "?", 2)[0]
	if !protocol.AllowedAgentPath(method, clean) {
		return 0, nil, errors.New("workspace route denied")
	}
	r, err := k.find(ctx, id)
	if err != nil {
		return 0, nil, err
	}
	if !sandboxReady(r) {
		return 0, nil, shared.CodedError{Status: http.StatusServiceUnavailable, Code: "NOT_READY"}
	}
	return k.agent(ctx, r, method, path, b)
}
func (k *Kubernetes) proxy(w http.ResponseWriter, r *http.Request, id, path, route string) {
	resource, err := k.find(r.Context(), id)
	if err != nil || !sandboxReady(resource) || k.cfg.RequireVerified {
		shared.Error(w, 503, "CONSOLE_UNAVAILABLE")
		return
	}
	shared.Proxy(w, r, k.endpoint(resource), route+strings.TrimPrefix(path, "/"), k.token(id, resource.Metadata.Labels["workbench.generation"]))
}
func (k *Kubernetes) Console(w http.ResponseWriter, r *http.Request, id, path string) {
	k.proxy(w, r, id, path, "/v1/t3/")
}
func (k *Kubernetes) Desktop(w http.ResponseWriter, r *http.Request, id, path string) {
	k.proxy(w, r, id, path, "/v1/t3-desktop/")
}

// Approval binds the exact digest to the pinned v0.23 runtime contract.
func (k *Kubernetes) hermesEnabled() bool {
	return pinnedImage.MatchString(k.cfg.Image) && k.cfg.HermesApprovedImage == k.cfg.Image && k.cfg.HermesModel != ""
}

func (k *Kubernetes) Application(w http.ResponseWriter, r *http.Request, id, path string) {
	k.proxy(w, r, id, path, "/v1/application-proxy/")
}
