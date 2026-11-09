// Package kubernetes delegates Sandbox lifecycle and persistent storage to
// Kubernetes SIGs Agent Sandbox. Cluster configuration is not enforcement
// evidence; unsupported connection-policy modes fail closed before allocation.
package kubernetes
