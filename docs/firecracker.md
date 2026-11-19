# Firecracker sandbox mode

This backend assumes an agent may run arbitrary code, ignore every proxy
environment variable, and obtain root inside its own guest. Guest cooperation
is not the network boundary. Installation and real-host acceptance are required
before this mode is usable; passing Go tests alone does not establish isolation.

## Enforced topology

Each workspace gets a separate Firecracker microVM. There is **no virtual network
interface**, TAP device, Internet bridge, shared filesystem, host directory mount,
Docker socket or guest-accessible Firecracker API socket. The guest has its own
kernel, a read-only system disk, a private 8 GiB data disk and a read-only bootstrap
disk. Data disks are never mounted or filesystem-checked by the host after guest
use. Existing Docker data volumes are preserved and are not silently migrated.

The jailer uses a unique host UID/GID per generation, a chroot, a separate network
namespace, a PID namespace, Firecracker's default seccomp filter and cgroup v2
limits. Each VM has one vCPU and 1 GiB guest memory; its host cgroup permits 1.5 GiB,
no swap, 128 tasks and one CPU. The controller admits at most four simultaneous VMs.
The writable disk has a fixed size; guest console output is root-only and capped
at 64 MiB per generation. Its limit is independent of block-device writes. No root
login shell or serial input is provided.

Three guest-to-host vsock channels are always bound. Hermes workspaces configured
for Matrix get one additional, explicitly activated broker:

| Port | Fixed host destination | Authorization |
| --- | --- | --- |
| 18081 | Inference gateway on host loopback | Generation-bound inference grant |
| 18082 | Credential broker on host loopback | Generation-bound broker grant |
| 18083 | Egress gateway on host loopback | Generation-bound grant and current destination policy |
| 18084 (Hermes Matrix only) | Host Matrix broker, fixed approved homeserver | Workspace grant, operator activation, exact room/sender policy; bot token held on host |

These are fixed connections, not caller-selected addresses or a general SOCKS
proxy. Other guest-to-host vsock ports have no listening Unix socket in the jail.
The Matrix broker accepts restricted client operations for encrypted text chat.
It strips foreign room and sender events before guest processing, blocks
joins/invites and plaintext sends, and does not expose the bot token. See
[Hermes setup](../deploy/hermes/README.md).
The host initiates workspace API/console connections on guest vsock port 8080;
8081 accepts only a fixed storage-sync operation for quiesced shutdown.

Ignoring `HTTP_PROXY` leaves the guest without an external IP route. An agent can
still call the approved gateway channels directly, so those gateways authenticate
and apply their policies independently. Fully open mode is refused at start and
on policy updates. Every new generation starts Blocked until the authenticated
control plane applies an allowlist. Missing grant verification fails closed.

The controller checks the running VMM's UID, seccomp mode, chroot, network namespace,
cgroup limits and actual device configuration before reporting it ready. Reported
statuses `jailed-vm` and `no-nic-vsock` describe observed controls, not a guarantee
against unknown VMM/kernel exploits. Host administrators and the controller remain
trusted. An approved external destination or model provider can receive data;
destination filtering is not data-loss prevention.

Installed artifact hashes are checked when the host controller starts. Each VM
start then verifies that the root-owned artifacts retain their filesystem identity,
size, permissions and modification time. Changes require a controller restart and
complete hash verification. Jail verification compares directory identity across
mount namespaces, rather than the namespace-relative text of `/proc/PID/root`.

## Installation

The reference deployment uses Kubernetes/Kata. Firecracker is an experimental, unsupported backend managed by the operator; automatic host installation and image staging have been
removed. Existing installations and private disks are unaffected.

A new installation requires x86_64 Linux, KVM, cgroup v2 with cpu/memory/pids,
Docker, Go, iproute2, e2fsprogs and systemd. Provision it through your host
configuration management with these inputs:

1. Build `cmd/firecracker-controller` as a static binary. Build the guest from the
   repository root with the `firecracker-guest` target. It uses the workspace
   binaries and desktop packages from the same source revision.

   ```sh
   docker build --target firecracker-guest \
     --build-arg HERMES_COMMIT="$(jq -r .commit deploy/hermes/runtime.json)" \
     --build-arg HERMES_SOURCE_SHA256="$(jq -r .source_sha256 deploy/hermes/runtime.json)" \
     -t harbormaster-firecracker-guest:local .
   ```
2. Obtain matching Firecracker/jailer releases and a supported guest kernel from
   trusted sources. Verify their approved SHA256 values before installation.
3. Export the trusted guest image, extract it into an empty staging directory and
   create a 6 GiB ext4 image using `mkfs.ext4 -d`. Never use a guest-modified data
   disk as the source. Install `firecracker`, `jailer`, `controller`, `vmlinux`,
   `rootfs.ext4` and `hermes-runtime.json` beneath `/opt/agent-workbench/firecracker`.
   Binaries are root-owned mode 0755; image, kernel and runtime marker are mode 0444.
4. Create root-owned mode 0444 `manifest.json`, mapping each of `firecracker`,
   `jailer`, `vmlinux`, `rootfs.ext4` and `hermes-runtime.json` to its SHA256 hex digest.
5. Install the existing `controller_token`, `gateway_service_token` and
   `grant_secret` as root-owned mode 0600 files in mode 0700
   `/etc/agent-workbench/firecracker`. Copy the existing control-plane authority
   (`.local/secrets/control_token`) there as `project_mcp_signing_key`; do not
   generate a different signing key.
6. Install `deploy/firecracker/agent-workbench-firecracker.service` into systemd
   and reload units. Run `controller --prepare-host`, then `controller --self-test`
   with the three `*_FILE` environment variables shown in the unit. Start the unit
   only after real KVM acceptance succeeds. The systemd unit expects the host
   preparation step to have created `/srv/agent-workbench-jailer`. Its compiled hooks prepare
   its cgroup subtree and clean up its own stale guests/namespaces.

Quiesce workspaces before replacing any installed artifacts. The self-test boots
and stops a disposable VM and checks isolation, grants and protocol compatibility.
Unit tests alone do not establish real-host isolation. Host lifecycle management
is separate from Kustomize and must not be run on the reference k3s node implicitly.

In `.env` select:

```dotenv
COMPOSE_FILE=compose.yaml:deploy/firecracker/compose.yaml
RUNTIME_BACKEND=firecracker
REQUIRE_VERIFIED_ISOLATION=true
```

The Compose overlay replaces the controller's Docker socket mount with the host
controller's root-only Unix socket. It publishes the inference gateway, credential broker, egress gateway and MCP service on host loopback. Rebuild the runtime-controller before using the new backend.
The host controller starts without starting any workspace. Do not restart the
workbench until installation and real-host acceptance have succeeded.

Unknown runtime backends refuse startup; there is no Docker fallback. Ordinary
Docker retains its existing unverified-isolation admission gate. This VM
backend always verifies its mandatory controls and cannot disable them via that flag.

## Operations and limits

The host controller is a separate installed binary: `docker compose build` does
not update it. If startup reports `RUNTIME_CAPABILITIES_INCOMPATIBLE`, build the
matching controller with `CGO_ENABLED=0 go build -trimpath -o
.local/firecracker/controller ./cmd/firecracker-controller`. Pause Matrix
listeners, wait for active work to finish, and stop workers through the API so
their storage is synchronized. Only then stop the host service and run
the provisioning procedure above for the controller and signing authority only.
Retain the installed guest image, run real KVM acceptance, and start the host service.
Recreate the Compose `runtime-controller` bridge with the Firecracker overlay
and `--no-deps --force-recreate` to refresh its bind mount of the recreated
systemd runtime directory. Then restart the previously running workers and
reconnect their Matrix listeners.
Never restart the host service over active guests to resolve a version mismatch.

If a work attempt fails with `SPECIFICATION_ACCEPTANCE_FAILED` before a task ID
appears, check that the installed **guest image**, not just the host controller,
matches the control plane. An older guest can report ready and accept file
uploads while rejecting the `specification_digest` field used by work dispatch.
Rebuilding Compose services or doing a controller-only installation does not
update that guest. Stage the current image, quiesce workers and Matrix listeners,
and perform the full installation above. The VM acceptance check now verifies
digest-aware task submission without executing a task or contacting a provider.
After restoring the workers, use **Retry with new attempt** on the existing work
item; its original specification and failed attempt remain intact.

```sh
systemctl status agent-workbench-firecracker.service
journalctl -u agent-workbench-firecracker.service
```

Stopping the host service terminates all its guests. A daemon restart kills stale
generation cgroups rather than adopting unverified processes. The systemd stop
hook kills VMM cgroups even if the controller crashes. Workspace stop first requires
quiescence and a guest storage sync; operator service shutdown remains an emergency
stop. Grant revocation and proxy stream rechecks are inherited from the gateways.

Root-only VM state is under `/var/lib/agent-workbench/firecracker`; jails are under
`/srv/agent-workbench-jailer`. Each workspace owner record fences project/home
reassignment. Private data disks persist across VM generations. Failed generations
retain their logs/jails for investigation; clean these only after verifying that
the corresponding cgroup has no running processes. Parent-workspace seeding is
explicitly refused by this backend until a VM-safe handoff is implemented.

## Local acceptance record — 6 November 2026

### Controller upgrade — 12 November 2026

Upgraded the separately installed host controller to the current schema-1
capability contract, retaining the installed guest image and private disks.
The controller-only install passed real KVM acceptance. The runtime bridge was
recreated for the new host socket; Hermes restarted with observed `jailed-vm`
and `no-nic-vsock` status, and its Matrix listener reconnected. The stopped T3
test worker passed all five deployed startup checks (state, compatibility,
allocation, connections, and capacity). No provider conversation was used as
an acceptance test.

### Hermes extension — 8 November 2026

The updated guest passed real KVM acceptance, including native Hermes and Matrix
crypto imports. A Hermes workspace using Blocked networking completed a real
subscription-gateway task (`HERMES_RUNTIME_OK`) and used its terminal to write and
read a proof file. Its direct public-IP and metadata-IP connection attempts failed.
The proof file was independently read through the authenticated workspace API;
the host did not mount the guest disk. Existing workspaces were quiesced before
installation and restored afterwards.

Go race tests and vet, frontend build/tests, Matrix route/sync boundary tests and
an offline native-adapter callback/status check passed. A live Matrix conversation
has not been tested: no homeserver, bot credential or room/sender IDs were supplied.

### Original installation

On the x86_64 Omarchy host (Linux 7.2.15-1-omarchy), Firecracker/jailer v1.17.0
and the pinned 6.1.188 guest kernel passed a real KVM acceptance run:

- Guest boot and authenticated workspace readiness through host-to-guest vsock.
- Observed VMM UID, seccomp, jail directory identity, namespace, device and cgroup controls.
- Guest-root direct TCP probes to public IPv4, IPv6, LAN and cloud metadata targets denied.
- Guest-root host-vsock probes on ports 22, 53, 443, 2375, 8080, 8081 and 18080 denied.
- Guest system-disk write probe denied and no guest Docker socket present.
- Blocked egress, Fully open rejection and project-owner reassignment rejection.
- Quiesced guest storage sync, VM stop and inference-grant revocation.
- Compose controller bridge health through the read-only Unix-socket directory;
  inspection confirmed that no Docker socket was mounted.
- Full `go test -race ./...` and `go vet ./...` passed.

The workbench containers and all agent guests were left stopped. The host
controller is running without guests and is not enabled automatically at boot.
No existing Docker workspace data was migrated or deleted. The native provider
and end-to-end release gates listed below remain separate from this acceptance.

Provider sign-in, native T3 attachment, allowlisted Internet requests, cross-project
attacks, guest-root vsock abuse, exhaustion, host crashes and storage recovery need
end-to-end validation on the selected host before sensitive workloads. Keep the
existing synthetic-data policy until those release gates pass.

Upstream references: [jailer](https://github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md),
[vsock](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md),
[production host setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md).
