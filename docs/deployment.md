# Deploy Harbormaster

## Choose a runtime

| Runtime | Use it for | Deployment boundary |
| --- | --- | --- |
| Docker Compose | Local development and synthetic task tests on Linux | Dashboard, management API, and MCP bind to loopback. Workers use runc. VM isolation and full host egress containment are unverified. |
| Kubernetes with Agent Sandbox and Kata | A reference installation on operator-managed infrastructure | The default verified-isolation requirement blocks worker starts. Run the live checks for your cluster before relying on isolation. |
| Firecracker | Experimental microVM testing on a dedicated Linux host | Requires KVM, pinned host and guest artifacts, and a host controller. See [Firecracker installation](firecracker.md). |

## Start the local Compose deployment

1. Clone or download the repository. Install Docker Engine, its Compose plugin, Make, and the Go version in [go.mod](../go.mod). Use a Linux Docker host with `/var/run/docker.sock` and an account that can access it. Run `docker info` and `docker compose version` to check access.
2. From the repository root, run `make up`.

   ```sh
   make up
   docker compose ps
   ```

   The command creates missing local credentials, builds platform and worker images, and waits for the services to start. Bootstrap preserves existing credentials and copies [.env.example](../.env.example) to `.env` when needed. If a port is already in use, change the corresponding port in `.env` before retrying.
3. Open [the dashboard](http://127.0.0.1:8080/app/). Sign in as `operator` using the password in `.local/secrets/dashboard_local_password`.
4. With the default fixture configuration, run `make smoke`. The check submits a synthetic task, retries the submission, and reads the same result after a worker restart. It stops the smoke-test worker at the end of a successful run.

| Endpoint | Default address | Access |
| --- | --- | --- |
| Dashboard | `http://127.0.0.1:8080/app/` | Local Basic authentication |
| Management API | `http://127.0.0.1:8090` | Control plane bearer credential |
| MCP | `http://127.0.0.1:8082/mcp` | Project credential or the bootstrap MCP credential for `default` |

PostgreSQL, Vault, and the runtime controller use private Compose networks. Keep the default host publications on loopback. If you change `CONTROL_PORT`, set `CONTROLPLANE_URL` to the new URL when you run the operator CLI or `make smoke`.

## Configure inference and credentials

- Keep `GATEWAY_MODE=fixture` and `INFERENCE_MODEL=fixture` for synthetic tests. Vault can stay sealed or uninitialized in this mode.
- For Codex or Claude with a subscription, configure the provider, model, and credential path, then follow [subscription setup](subscription-agents.md). A saved personal subscription needs a worker binding. The default Compose policy does not enable personal subscriptions.
- For API-key inference or certificate-backed data requests, configure each gateway, mount its scoped credentials, and verify its allowed destinations. Follow [gateway configuration](../config/README.md).
- For hosted sign-in, configure [WorkOS AuthKit](authkit.md) or OAuth2 Proxy with scoped dashboard credentials. The stock local Compose service mounts the control token. Current dashboard binaries reject that token when hosted authentication is enabled.

Keep `.env`, `.local/secrets`, and Vault recovery material private. Back up the credential encryption key with the database. Retain the original control token while any credentials written with the legacy key remain in use.

## Install the Kubernetes reference deployment

1. Prepare a cluster with ValidatingAdmissionPolicy, NetworkPolicy enforcement, a working Kata handler, and a CSI StorageClass that supports ReadWriteOncePod. The application base includes Agent Sandbox. Follow [cluster dependency setup](../deploy/kubernetes/infrastructure/README.md) for Kata.
2. Create an environment overlay based on [the example overlay](../deploy/kubernetes/overlays/example/kustomization.yaml). Pin the platform and worker images to digests from the same release. Configure the StorageClass, project bindings and destination policy, ingress class, selectors, DNS, and TLS.
3. Provision a PostgreSQL database and the Secrets required by the rendered workloads. Update the dashboard patch to remove its legacy control token and supply the scoped credentials. Use the [Kubernetes configuration guide](../deploy/kubernetes/README.md) for the credential mapping and gateway mode.
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

For Compose, let active work finish or cancel it. Stop the workers in **Workers**, then run `make down`. Dynamically created workers have a separate lifecycle from the Compose services. `make down` keeps the named volumes. The `--volumes` option deletes Compose volumes, including PostgreSQL and Vault data.

Before you upgrade, record the current image digests and stop new work. Capture one recovery point for PostgreSQL, workspace volumes, Vault, and the corresponding secrets. Test the restore on a separate installation. See [backup, upgrade, and recovery procedures](operations.md#upgrade-and-recovery) for migration and rollback details.

## Check an unhealthy installation

| Symptom | Check |
| --- | --- |
| Local dashboard returns 401 | Use `operator` and the password from the same checkout's `.local/secrets/dashboard_local_password`. |
| Dashboard exits after hosted auth is configured | Check for a legacy control-token mount. Verify the scoped read token and human signing key on both dashboard and control plane. |
| Workspace remains stopped | Open **Workers**, then **Check & start**. Read the failed policy, runtime, or quota check before retrying. |
| Fixture task works, but native inference fails | Check the provider model, worker subscription binding or Vault record, and gateway destination policy. |
| Kubernetes pods are pending | Check pod events, the Kata RuntimeClass, and PVC binding. Keep existing PVCs attached to their recorded owners. |
| Preview fails after hosted sign-in | Check wildcard DNS and TLS, ingress routing, and whether the preview service is still active. |

For Compose diagnostics, start with `docker compose ps` and `make logs`. Service health checks report whether the service is reachable. Use the fixture test or the relevant live acceptance procedure to check actual work execution.
