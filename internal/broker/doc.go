/*
Package broker provides fixed, principal-authorized mTLS operations.

Configuration uses GATEWAY_MODE (fixture or upstream), GATEWAY_CONFIG (JSON file),
GATEWAY_SERVICE_TOKEN_FILE, and optional CONTROLLER_URL (default
http://runtime-controller:8080). The example JSON contains sample origins and
IP addresses. Fixture mode uses operation parameter definitions and approved
cases. Upstream mode also requires routing policy and client certificates.

POST /v1/operations/case_lookup accepts, for the example policy:

	{"case_id":"CASE-001","justification":"authorized case work","parameters":{"record_id":"123"}}

Principal, project and workspace are derived from the controller. Upstream
permissions require a nonempty projects list. Missing project, principal,
operation, or case approval fails closed. Operation and case identifiers
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
project_id. The controller supplies the last three values. The permission's
certificate name selects /run/secrets/<name>.pem. The name follows the operation
identifier rules. Mount a PEM file containing the certificate chain and private key.

The broker reads the mounted file after authorizing the request. Each request
uses a fresh TLS transport to keep client identities separate. Requests to
upstream systems time out after 20 seconds, and JSON responses are limited to
1 MiB. Certificate, network, and response failures produce UPSTREAM_UNAVAILABLE.
Use allowed_cidrs, allow_private, and ca_file to limit the destination and
configure CA trust. The broker verifies upstream TLS and disables redirects and proxies.
*/
package broker
