+++
title = "Deploy Harbormaster"
+++

## Choose a runtime

- Use [Docker Compose](#start-the-local-compose-deployment) for local development and synthetic tests.
- Use [Kubernetes with Agent Sandbox and Kata](#install-the-kubernetes-reference-deployment) for the reference deployment.
- Use [Firecracker](firecracker.md) for experimental microVM testing on a dedicated host.

See [backend support](architecture/_index.md#backend-support) for network policies and isolation requirements.

## Start the local Compose deployment

1. Clone or download the repository. Install Docker Engine, its Compose plugin, Make, and the Go version in [go.mod](../../../go.mod). Use a Linux Docker host with `/var/run/docker.sock` and an account that can access it. Run `docker info` and `docker compose version` to check access.
2. From the repository root, run `make up`.

   ```sh
   make up
   docker compose ps
   ```

   The command creates missing local credentials, builds platform and worker images, and waits for the services to start. Bootstrap preserves existing credentials and copies [.env.example](../../../.env.example) to `.env` when needed. If a port is already in use, change the corresponding port in `.env` before retrying.
3. Open [the dashboard](http://127.0.0.1:8080/app/). Sign in as `operator` using the password in `.local/secrets/dashboard_local_password`.
4. With the default fixture configuration, run `make smoke`. The check submits a synthetic task, retries the submission, and reads the same result after a worker restart. It stops the smoke-test worker at the end of a successful run.

| Endpoint | Default address | Access |
| --- | --- | --- |
| Dashboard | `http://127.0.0.1:8080/app/` | Local Basic authentication |
| Management API | `http://127.0.0.1:8090` | Control plane bearer credential |
| MCP | `http://127.0.0.1:8082/mcp` | Project credential or the bootstrap MCP credential for `default` |

PostgreSQL and the runtime controller use private Compose networks. Keep the default host publications on loopback. If you change `CONTROL_PORT`, set `CONTROLPLANE_URL` to the new URL when you run the operator CLI or `make smoke`.

## Configure inference and credentials

- Keep `GATEWAY_MODE=fixture` and `INFERENCE_MODEL=fixture` for synthetic tests.
- For Codex or Claude with a subscription, configure the provider and model, then follow [subscription setup](subscription-agents.md). A saved personal subscription needs a worker binding. The default Compose policy does not enable personal subscriptions.
- For API-key inference or certificate-backed data requests, configure each gateway, mount its scoped credentials, and verify its allowed destinations. Follow [gateway configuration](operations/gateway-configuration.md).
- For hosted sign-in, configure WorkOS AuthKit or OAuth2 Proxy with a human signing key. If you use Compose, remove the dashboard's local operator credential and mount. [Configure hosted sign-in](authkit.md)

Keep `.env` and `.local/secrets` private. Back up the credential encryption keys with the database. [Upgrade deployments with legacy credentials](operations/_index.md#security-release-upgrade)

## Install the Kubernetes reference deployment

1. Prepare a cluster with ValidatingAdmissionPolicy, NetworkPolicy enforcement, a working Kata handler, and a CSI StorageClass that supports ReadWriteOncePod. The application base includes Agent Sandbox. Follow [cluster dependency setup](operations/cluster-dependencies.md) for Kata.
2. Create an environment overlay based on [the example overlay](../../../deploy/kubernetes/overlays/example/kustomization.yaml). Pin the platform and worker images to digests from the same release. Configure the StorageClass, project bindings and destination policy, ingress class, selectors, DNS, and TLS.
3. Provision a PostgreSQL database and the Secrets required by the rendered workloads. [Kubernetes configuration guide](operations/kubernetes.md)
4. Create any required namespaces and cluster prerequisites through your environment's provisioning process. Render the overlay, check it against the cluster API, then review the diff. Replace the context and overlay path before running these commands.

   ```sh
   kubectl kustomize /path/to/environment
   kubectl --context YOUR_CONTEXT apply --server-side --dry-run=server -k /path/to/environment
   kubectl --context YOUR_CONTEXT diff --server-side -k /path/to/environment
   ```

   `kubectl diff` exits with status 1 when it finds differences.
5. Apply the reviewed overlay. Check rollout and pod status in the platform namespace.

   ```sh
   kubectl --context YOUR_CONTEXT apply --server-side -k /path/to/environment
   kubectl --context YOUR_CONTEXT -n harbormaster get deployments,pods
   kubectl --context YOUR_CONTEXT -n harbormaster rollout status deployment/dashboard
   ```
6. Sign in over HTTPS and check access with an account that has project membership. For hosted previews, configure wildcard DNS, wildcard TLS, and ingress to the dashboard. Run the [live runtime, network, and restore checks](k3s.md) on a disposable installation.

Use the [release inventory](container-publishing.md) for image names and digests. Keep the control plane at one replica. Keep MCP and the control plane private. Use the [Kubernetes Hermes guide](k3s-hermes.md) if your environment needs Hermes.

## Stop, upgrade, and recover

For Compose, let active work finish or cancel it. Stop the workers in **Workers**, then run `make down`. Dynamically created workers have a separate lifecycle from the Compose services. `make down` keeps the named volumes. The `--volumes` option deletes Compose volumes, including PostgreSQL data.

Before you upgrade, record the current image digests and stop new work. Capture one recovery point for PostgreSQL, workspace volumes, and the corresponding secrets. Test the restore on a separate installation. See [backup, upgrade, and recovery procedures](operations/_index.md#upgrade-and-recovery) for migration and rollback details.

If your previous deployment used Vault, follow the [credential migration steps](operations/_index.md#upgrade-from-a-deployment-that-uses-vault).

## Check an unhealthy installation

| Symptom | Check |
| --- | --- |
| Local dashboard returns 401 | Use `operator` and the password from the same checkout's `.local/secrets/dashboard_local_password`. |
| Dashboard exits after hosted auth is configured | Remove the local operator credential from the dashboard. Verify the read token and human signing key on both services. |
| Workspace remains stopped | Open **Workers**, then **Check & start**. Read the failed policy, runtime, or quota check before retrying. |
| Fixture task works, but native inference fails | Check the provider model, worker subscription binding, and gateway destination policy. |
| Kubernetes pods are pending | Check pod events, the Kata RuntimeClass, and PVC binding. Keep existing PVCs attached to their recorded owners. |
| Preview fails after hosted sign-in | Check wildcard DNS and TLS, ingress routing, and whether the preview service is still active. |

For Compose diagnostics, start with `docker compose ps` and `make logs`. Service health checks report whether the service is reachable. Use the fixture test or the relevant live acceptance procedure to check actual work execution.
