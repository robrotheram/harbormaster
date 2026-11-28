+++
title = "Gateway configuration"
aliases = ["/docs/reference/config/readme/"]
+++

For native Codex/Claude **subscription** agents, use [subscription setup](../subscription-agents.md). The native routes use saved worker bindings.

[`deploy/config/inference.json`](../../../../deploy/config/inference.json) and [`deploy/config/broker.json`](../../../../deploy/config/broker.json) are synthetic fixture policies, safe to run without live provider credentials. `DEMO-001` is a synthetic fixture case, not a claim about real authorization. The fixture inference response is deterministic and explicitly labeled synthetic.

For real inference, use [`internal/gateway/config.example.json`](../../../../internal/gateway/config.example.json) as a policy template: choose a fixed HTTPS origin, explicit model names, and approved resolved destination CIDRs. Set the inference gateway's `GATEWAY_MODE=upstream` and mount an operator-provided key as `INFERENCE_API_KEY_FILE`. Set the controller's `INFERENCE_MODEL` to an approved model. The Chat Completions endpoint accepts nonstreaming requests with tool calls.

For a real data operation, use the [broker policy example](../../../../internal/broker/config.example.json). Set a fixed HTTPS origin, path, and method for each operation. Define the allowed parameters, principal, and cases. Each upstream permission requires a `projects` list of authorized project IDs. Set `certificate` to the name of the client certificate for that permission.

Combine the certificate chain and private key in a single PEM file. Mount it read-only on the broker at `/run/secrets/<name>.pem`, where `<name>` matches the policy. Give the broker process read access. Set its `GATEWAY_MODE` to `upstream`. If the upstream uses a private CA, mount its CA bundle and set the operation's `ca_file`. Scope `allowed_cidrs` and `allow_private` to the upstream destination.

In Compose, set `INFERENCE_GATEWAY_MODE` to configure inference separately from the shared `GATEWAY_MODE` default. Use a Compose override to supply the broker's upstream policy and mount the PEM files as Docker secrets. Keep the source files under the ignored `.local/` directory. In Kubernetes, add each `<name>.pem` file to `credential-broker-secrets`. See [platform Secrets](kubernetes.md#provision-secrets).

The gateways enforce policy for requests they receive. See [runtime network limits](../architecture/security.md#important-unverified-boundaries) for the separate host network boundary.
