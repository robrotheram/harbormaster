# Security boundaries and verification

This first deployment is a **functional development environment for synthetic data**. It does not yet establish the strong isolation required for sensitive investigations.

The optional [Firecracker backend](firecracker.md) replaces workspace containers
with jailed, no-NIC microVMs and fixed vsock gateway channels. It requires host
installation and real KVM acceptance; its mechanism statuses do not establish
completion of the release gates below. The Docker-specific limitations in this
document continue to apply to the development Docker backend.

## Implemented controls

- Only the trusted runtime controller receives the Docker socket. It exposes fixed lifecycle operations and a bounded workspace API forwarder, not arbitrary Docker requests, images, mounts, runtime flags or host paths.
- Each workspace receives its own container, internal network and persistent volume; it runs as a non-root user with a read-only root filesystem, dropped capabilities, no new privileges and CPU/memory/PID bounds.
- Workspace grants are derived from a controller-held secret and a container generation. Gateways validate them against currently running, deployment-owned containers. A stopped container's grant is rejected for new requests.
- Separate inference and certificate-broker processes; no provider certificate private key or Vault credential is intentionally given to the workspace. Gateways accept named destinations/operations rather than arbitrary URLs.
- Native Codex/Claude subscription routes fetch expiring access tokens from dedicated Vault KV v2 paths, enforce provider/model/destination allowlists, and send only workspace grants into native agent processes. Vault uses TLS. Root/unseal recovery material remains operator-only; the gateway has read-only access to the two local-operator subscription paths. Open native inference streams recheck workspace grants and are cancelled after revocation. See [subscription agents](subscription-agents.md).
- Platform metadata excludes prompts, files, retrieved bodies and task results. The unauthenticated dashboard has an explicit read-only projection and no runtime authority.
- MCP and the management API require provisioned credentials. The initial identity model is one local operator and one project, not multi-user OIDC.
- The local launcher uses workspace-specific browser origins and scoped, one-use connection tickets. It forwards native T3 HTTP/WebSocket traffic to that workspace, with native credentials held by the workspace supervisor. The launcher is trusted and requires no local account login; it carries local operator authority.
- Desktop attachment uses a separate `desktop-ws-….localhost` origin with native T3 pairing, token exchange and client authorization. Management-hop credentials never substitute for desktop authority. Foreign web Origins and cross-workspace native tokens are rejected. The loopback launcher may issue a pairing link under its existing operator authority; keep the link private.
- File handoffs authorize both workspaces, accept only bounded project-relative paths, read confined regular files and atomically create a new destination without replacement. Content stays out of platform metadata/logs. Copies are explicit handoffs, not a cross-workspace mount or credential-sharing channel.
- Stop uses a workspace quiesce handshake, atomically refusing new tasks before stopping an idle runtime. Unknown activity blocks automatic stop. Transport disconnect does not cancel accepted tasks.

## Important unverified boundaries

Docker internal networks are not a complete host firewall. Host listeners, DNS, IPv6, gateway interfaces and multihomed platform services require adversarial testing and host enforcement. An explicit loopback-only DNS forwarder configuration removes the default public DNS fallback from the workspace execution profile, but its behavior still requires Docker-level verification.

The development controller therefore reports `network_status=unverified`. It does not claim that network segmentation proves allowlist-only containment against a compromised guest.

The Docker controller requires runc when starting a workspace. It rejects legacy Kata containers at startup and rejects a legacy `WORKSPACE_RUNTIME=kata` setting. Kata deployment uses the Kubernetes backend.

`REQUIRE_VERIFIED_ISOLATION=true` refuses workspace startup. This is deliberately fail-closed: there is no configuration boolean that turns an untested runtime into verified isolation. Unlocking a secure mode requires implementing an evidence-producing verifier and repeating network/storage/guest-root tests on the selected host stack.

The Docker socket grants root-equivalent host authority to the runtime controller. Host administrators are trusted. Do not expose the controller on the LAN or mount its socket/credential into untrusted components.

## Remaining release gates for sensitive data

1. Pin and verify the selected Kubernetes Kata/VMM/storage combination, one VM per workspace, guest-root containment, resource bounds and host backend confinement.
2. Enforce and test host-side deny rules covering forwarding, host INPUT, DNS and IPv6; gateway loss, policy failure and daemon restarts must preserve denial.
3. Introduce verified per-user/project identities, narrowly scoped credential grants and defined revocation behavior, beyond the single-operator development principal.
4. Provision real approved inference/provider credentials and scoped Vault access; review which workspace data may leave through inference/results.
5. Revalidate native T3 browser/HTTP/WebSocket access, disconnect/reconnect, cold starts and provider compatibility under the chosen microVM and host policies. Native browser access and desktop pairing/token exchange/WebSocket calls have functional Docker checks; remote exposure remains a separate gate.

No test involving mocks or ordinary containers establishes these gates. Review [Docker daemon security](https://docs.docker.com/engine/security/) and the [Kata threat model](https://github.com/kata-containers/kata-containers/blob/main/docs/threat-model/threat-model.md) before changing this boundary.

## Workspace connection profiles

Workspace Settings select Blocked, GitHub only, Custom allowlist or Fully open.
GitHub only allows `github.com:443`. Custom rules contain exact
public hostname/IP and TCP port pairs (maximum 64); no wildcard, URL, or guest DNS
fallback. HTTP and HTTPS use the trusted egress proxy, which resolves destinations,
rejects private/loopback/link-local addresses and verifies a generation-bound grant.
HTTP redirects are returned to clients; each subsequent request is checked again.
CONNECT approves destinations, not encrypted paths, methods or payloads. It is
not data-loss prevention. Private/LAN targets are excluded from custom allowlists.

Fully open is an explicit per-workspace opt-out from destination restrictions. It
adds a separate owned non-internal Docker bridge and normal external DNS; native
tools use direct networking, including TCP/UDP, SSH, Internet and reachable LAN/host
services. No runtime socket, Vault token or provider credential is added. Private
homes and volumes retain project ownership, but unrestricted networking is not a
network confidentiality boundary between projects or against the host.

The workspace must be stopped before changing into or out of fully open mode. On
start, a stopped container is recreated as needed using the same validated data
volume and home. Runtime configuration reports the actual mode separately from
the desired profile. Applying restricted rules while an open network remains
attached is refused. Fully open intentionally continues direct egress if platform
services fail; proxy modes fail closed when their grant verifier is unavailable.

Fully open is never enabled by default or silently selected.

Workspace supervisors run alongside agent code. Treat their activity, results,
and artifact metadata as untrusted. The control plane checks work-item download
bytes against the SHA-256 digest recorded for the artifact before serving them.
