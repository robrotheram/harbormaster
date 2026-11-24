package kubernetes

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/url"
	"time"

	"agent-workbench/internal/runtime/protocol"
	"agent-workbench/internal/shared"
)

type workspaceRouteKey struct {
	id    string
	grant [32]byte
}

type workspaceRoute struct {
	id        string
	namespace string
	expires   time.Time
}

func (k *Kubernetes) find(ctx context.Context, id string) (kubeResource, error) {
	if !shared.ValidID(id) || !dnsLabel.MatchString(k.name(id)) {
		return kubeResource{}, errors.New("invalid workspace")
	}
	key := workspaceRouteKey{id: id}
	item, err := k.findCachedWorkspace(ctx, key)
	if !isNotFound(err) {
		return item, err
	}
	return k.discoverWorkspace(ctx, id)
}

func (k *Kubernetes) findCachedWorkspace(ctx context.Context, key workspaceRouteKey) (kubeResource, error) {
	k.routeMu.Lock()
	route := k.routes[key]
	k.routeMu.Unlock()
	if !time.Now().Before(route.expires) {
		return kubeResource{}, protocol.APIError{Status: 404}
	}
	item, err := k.findInNamespace(ctx, route.id, route.namespace)
	if err == nil {
		err = k.checkWorkspaceRoute(ctx, route.namespace, item.Metadata.Labels["workbench.project"])
	}
	if isNotFound(err) {
		k.routeMu.Lock()
		if k.routes[key] == route {
			delete(k.routes, key)
		}
		k.routeMu.Unlock()
	}
	return item, err
}

// Discover without a cached route on cold lookups and before creation. This
// keeps recovery stateless and detects duplicate IDs across namespaces.
func (k *Kubernetes) discoverWorkspace(ctx context.Context, id string) (kubeResource, error) {
	namespaces, err := k.runtimeNamespaces(ctx)
	if err != nil {
		return kubeResource{}, err
	}
	var found kubeResource
	for _, namespace := range namespaces {
		item, err := k.findInNamespace(ctx, id, namespace)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return kubeResource{}, err
		}
		if found.Metadata.Name != "" {
			return kubeResource{}, errors.New("ambiguous workspace ownership")
		}
		found = item
	}
	if found.Metadata.Name == "" {
		return kubeResource{}, protocol.APIError{Status: 404}
	}
	k.rememberWorkspaceRoute(workspaceRouteKey{id: id}, found)
	return found, nil
}

func (k *Kubernetes) rememberWorkspaceRoute(key workspaceRouteKey, resource kubeResource) {
	k.routeMu.Lock()
	defer k.routeMu.Unlock()
	now := time.Now()
	if len(k.routes) >= 1024 {
		for key, route := range k.routes {
			if !now.Before(route.expires) {
				delete(k.routes, key)
			}
		}
	}
	if k.routes == nil {
		k.routes = make(map[workspaceRouteKey]workspaceRoute)
	}
	if _, existing := k.routes[key]; existing || len(k.routes) < 1024 {
		k.routes[key] = workspaceRoute{id: resource.Metadata.Labels["workbench.workspace"], namespace: resource.Metadata.Namespace, expires: now.Add(15 * time.Second)}
	}
}

func (k *Kubernetes) findInNamespace(ctx context.Context, id, namespace string) (kubeResource, error) {
	var list struct {
		Items []kubeResource `json:"items"`
	}
	selector := url.QueryEscape("workbench.deployment=" + k.cfg.Deployment + ",workbench.workspace=" + id)
	if err := k.call(ctx, "GET", k.collection(namespace)+"?labelSelector="+selector, nil, &list); err != nil {
		// A missing collection is an API failure. Only an empty successful
		// result establishes that this namespace has no such sandbox.
		if isNotFound(err) {
			return kubeResource{}, fmt.Errorf("sandbox collection missing in namespace %s", namespace)
		}
		return kubeResource{}, err
	}
	if len(list.Items) == 0 {
		return kubeResource{}, protocol.APIError{Status: 404}
	}
	item := list.Items[0]
	if len(list.Items) != 1 || item.Metadata.Namespace != namespace || !k.owned(item, id) {
		return kubeResource{}, errors.New("ambiguous or invalid workspace ownership")
	}
	return item, nil
}
func (k *Kubernetes) findGrantWorkspace(ctx context.Context, token string) (kubeResource, error) {
	key := workspaceRouteKey{grant: sha256.Sum256([]byte(token))}
	matches := func(item kubeResource) bool {
		id := item.Metadata.Labels["workbench.workspace"]
		generation := grantGeneration(item)
		return generation != "" && k.owned(item, id) && sandboxReady(item) &&
			subtle.ConstantTimeCompare([]byte(token), []byte(k.token(id, generation))) == 1
	}
	// A cached grant identifies a namespace. The current generation, readiness,
	// ownership and policy are verified again before returning any identity.
	if item, err := k.findCachedWorkspace(ctx, key); err == nil && matches(item) {
		return item, nil
	}
	namespaces, err := k.runtimeNamespaces(ctx)
	if err != nil {
		return kubeResource{}, err
	}
	for _, namespace := range namespaces {
		var list struct {
			Items []kubeResource `json:"items"`
		}
		if err := k.call(ctx, "GET", k.collection(namespace), nil, &list); err != nil {
			return kubeResource{}, errors.New("workspace grant lookup unavailable")
		}
		for _, item := range list.Items {
			if item.Metadata.Namespace == namespace && matches(item) {
				k.rememberWorkspaceRoute(key, item)
				return item, nil
			}
		}
	}
	return kubeResource{}, protocol.APIError{Status: 404}
}
