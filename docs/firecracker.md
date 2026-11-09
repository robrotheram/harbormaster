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

Requires x86_64 Linux, KVM, cgroup v2 with cpu/memory/pids, Docker for building the
trusted image, Go 1.27.3, Python 3, iproute2, e2fsprogs, systemd and administrator
authentication. Firecracker/jailer are pinned to the same official v1.17.0 release
and its SHA256. The staging script pins a Firecracker CI kernel URL and SHA256.
The CI kernel is a starting artifact, not a production-hardened kernel certification.
Review kernel support, security updates, host microcode and SMT policy separately.

```sh
python3 scripts/firecracker-stage.py
pkexec python3 "$PWD/scripts/firecracker-install.py"
```

The installer copies root-owned binaries/artifacts to `/opt/agent-workbench/firecracker`,
three controller credentials to `/etc/agent-workbench/firecracker`, and a systemd
service to `/etc/systemd/system`. It provisions a dedicated cgroup subtree. It does
not flush host firewall rules, change forwarding policy, grant passwordless sudo,
or enable the service automatically at boot.

Before starting the host service, it boots a disposable real VM and checks boot
readiness, guest negative-network probes, blocked egress, rejection of Fully open,
ownership reassignment, clean stop and grant revocation. Failure leaves the
workbench stopped. These probes are acceptance checks, not a penetration test or
proof that a hostile guest cannot exploit the VMM.

The guest also contains pinned Hermes v0.23.0 and its native Matrix crypto
dependencies. Its runtime marker is covered by the root-owned artifact manifest.
Acceptance imports the real engine and encryption adapter inside the VM; each
Hermes workspace start repeats that probe before reporting readiness.

In `.env` select:

```dotenv
COMPOSE_FILE=compose.yaml:deploy/firecracker/compose.yaml
RUNTIME_BACKEND=firecracker
WORKSPACE_RUNTIME=firecracker
REQUIRE_VERIFIED_ISOLATION=true
```

The Compose overlay replaces the controller's Docker socket mount with the host
controller's root-only Unix socket. It publishes the three trusted gateways only
on host loopback. Rebuild the runtime-controller before using the new backend.
The host controller starts without starting any workspace. Do not restart the
workbench until installation and real-host acceptance have succeeded.

Unknown runtime backends refuse startup; there is no Docker fallback. Ordinary
Docker/Kata retain their existing unverified-isolation admission gate. This VM
backend always verifies its mandatory controls and cannot disable them via that flag.

## Operations and limits

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
