# Firecracker acceptance, November 2026

These checks describe the installed controller and guest images at the time. For current setup, see the [Firecracker guide](../../content/docs/firecracker.md).

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
