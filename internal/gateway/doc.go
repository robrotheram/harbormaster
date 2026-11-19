/*
Package gateway implements the nonstreaming OpenAI-compatible inference endpoint.

Configuration is loaded by ConfigFromEnv. Required environment variables are
GATEWAY_MODE (fixture or upstream), GATEWAY_CONFIG (JSON file), and
GATEWAY_SERVICE_TOKEN_FILE. CONTROLLER_URL defaults to
http://runtime-controller:8080. Upstream mode additionally requires
INFERENCE_API_KEY_FILE. Secrets are read from files only. Both binaries listen
on :8080 and expose public GET /healthz.

config.example.json shows the JSON policy shape. Its example.invalid hosts and
RFC 5737 IPs must be replaced for real use. Fixture mode requires only models;
it still verifies every workspace token. Send model fixture if using that
example. Responses are deterministic hashes of messages, visibly synthetic,
and always report model fixture. They do not emulate actual tool execution or
perform token accounting. Real requests use only /v1/chat/completions at the
configured HTTPS origin; successful JSON responses are returned unchanged.
stream:true is explicitly rejected with STREAMING_UNSUPPORTED. Unsupported
request extensions are rejected; routing and identity fields are never accepted.

Requests are limited to 1 MiB, responses to 4 MiB, verification to five seconds,
and upstream requests to thirty seconds. No prompts, tokens, keys, upstream
errors, or responses are logged. Non-success upstream responses are replaced
with sanitized 503 errors.

Network policies require allowed_cidrs or exact allowed_hosts. Private, loopback and link-local IPs
additionally require allow_private:true. DNS answers outside the policy are skipped;
the transport connects only to a checked IP while retaining original TLS hostname
verification. Redirects and environment proxies are disabled. ca_file optionally
selects a PEM CA bundle; TLS verification cannot be disabled. For a private
Vault or test upstream, set both its narrow CIDR and allow_private:true.
*/
package gateway
