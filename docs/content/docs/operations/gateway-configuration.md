+++
title = "Gateway configuration"
aliases = ["/docs/reference/config/readme/"]
+++

For native Codex/Claude **subscription** agents, use [subscription setup](../subscription-agents.md). The native routes use saved worker bindings or scoped Vault records.

[`config/inference.json`](../../../../config/inference.json) and [`config/broker.json`](../../../../config/broker.json) are synthetic fixture policies, safe to run without live provider credentials. `DEMO-001` is a synthetic fixture case, not a claim about real authorization. The fixture inference response is deterministic and explicitly labeled synthetic.

For real inference, use [`internal/gateway/config.example.json`](../../../../internal/gateway/config.example.json) as a policy template: choose a fixed HTTPS origin, explicit model names, and approved resolved destination CIDRs. Set the inference gateway's `GATEWAY_MODE=upstream` and mount an operator-provided key as `INFERENCE_API_KEY_FILE`. Set the controller's `INFERENCE_MODEL` to an approved model. The Chat Completions endpoint accepts nonstreaming requests with tool calls.

For a real data operation, use [`internal/broker/config.example.json`](../../../../internal/broker/config.example.json): map each named operation to a fixed HTTPS origin/path/method, bounded parameter schema, explicit principal/case permissions and a fixed Vault KVv2 path. The broker expects PEM fields `certificate` and `private_key` in that KVv2 record.

Set the broker's `GATEWAY_MODE=upstream`, provide `VAULT_TOKEN_FILE` with access only to those records, and use a TLS-enabled `VAULT_ADDR` with a trusted server certificate. For a private CA, set the network policy's `ca_file` to its mounted PEM CA bundle; scope `allowed_cidrs` and `allow_private` explicitly. The built-in Compose Vault listener uses TLS with bootstrap-generated material. The certificate broker still needs its own mounted CA, scoped token and destination policy before upstream mode is enabled. Subscription setup provisions a separate read-only token solely for the inference gateway; it does not authorize certificate operations.

Gateway modes can be configured independently through a Compose override; the shared `.env` `GATEWAY_MODE` is only a convenience for the fixture deployment. Do not enable upstream mode globally without supplying both integrations' policies and credentials.

Use Docker secrets in a local Compose override for these additional files. Keep secret values under the ignored `.local/` directory, never in policy JSON, git, logs or URLs.

The gateways enforce policy for requests they receive. See [runtime network limits](../architecture/security.md#important-unverified-boundaries) for the separate host network boundary.
