/*
Package broker provides fixed, principal-authorized mTLS operations.

Configuration uses GATEWAY_MODE (fixture or upstream), GATEWAY_CONFIG (JSON file),
GATEWAY_SERVICE_TOKEN_FILE, and optional CONTROLLER_URL (default
http://runtime-controller:8080). Upstream mode requires VAULT_ADDR, an HTTPS
origin, and VAULT_TOKEN_FILE. The example JSON contains documentation-only
origins/IPs and must be customized before upstream use. Fixture mode needs
operations with parameter definitions and principals with approved_cases;
upstream routing, Vault and network fields are unused in fixture mode.

POST /v1/operations/case_lookup accepts, for the example policy:

	{"case_id":"CASE-001","justification":"authorized case work","parameters":{"record_id":"123"}}

Principal, project and workspace are derived from the controller. Missing
principal/operation/case approval fails closed. Operation and case identifiers
are at most 128 ASCII letters, digits, underscores or hyphens, beginning with a
letter or digit. Policy path segments have the same restrictions. Parameters
support string, number and boolean types, required, string max_length (default
1024 bytes, maximum 8192), and string enum. Unknown parameters are rejected.
At most 32 parameters, 64 KiB input and 2048 bytes justification are accepted.
Fixture responses have mode fixture and synthetic true and include only the
validated parameters and verified identity.

Real operations support POST, PUT and PATCH to an exact configured path with no
client query, headers, URL, method or credential reference. Their JSON body
contains case_id, justification, parameters, principal_id, workspace_id and
project_id; the last three are controller-derived. Vault KVv2 GET requests go
only to /v1/{mount}/data/{principal-operation vault_path}. Each secret contains
certificate (PEM certificate chain) and private_key (PEM key). Provision a Vault
token with read-only ACLs for exactly these configured paths; this service does
not provision Vault, mint credentials or require root authority. Vault errors
and private keys are never returned or logged. Keys are parsed in memory and
are never written to disk or shared with the workspace.

Each authorized request loads its certificate and creates a fresh TLS transport
and connection, preventing cross-identity reuse. Total processing is bounded to
30 seconds (Vault 10, operation 20); response JSON is limited to 1 MiB. Non-2xx,
invalid JSON, network and Vault failures produce sanitized UPSTREAM_UNAVAILABLE.
Use of CIDRs, allow_private and optional ca_file follows package gateway. Vault
and operation TLS verification is mandatory; redirects and proxies are disabled.
*/
package broker
