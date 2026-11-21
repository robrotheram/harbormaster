# Project-isolated agent workbench: proposed direction

> Historical record. See the [documentation index](README.md) for current setup and workflows.

Status: architecture proposal, 1 November 2026. This records requirements and proposed decisions; it does not change deployment behavior or establish compliance certification.

The [software factory and research delivery plan](software-factory-plan.md), dated
9 November 2026, replaces this proposal's rollout sequence and EKS/k3s target with
Compose development and k3s production. It adds GoGallery, experimental research,
and personal/project assistants while preserving the isolation principles below.

## Confirmed requirements

- Each **project** is a mandatory data and agent-memory isolation boundary.
- The human may use the same desktop client and provider subscription account across projects.
- Workspace data remains separate. File movement within a project is explicit; conversations, memories and native agent state are not copied by default.
- The target is dynamic workspace provisioning on a custom EKS or k3s cluster, with strong VM isolation, externally enforced allowlists, controlled DNS and dependency registries.
- Compose remains useful for orchestration development with synthetic data. Infrastructure restrictions may be relaxed there; project ownership and authorization semantics must be exercised from the beginning.
- Existing Codex/Claude subscriptions are preferred. Never silently fall back to API billing or overage spending.
- T3 desktop and Hermes/Matrix are supported experiences to investigate and integrate, rather than agent engines to reimplement.

## 1. Separate persistent identity from replaceable compute

Use these concepts, with immutable project ownership:

| Object | Purpose and boundary |
| --- | --- |
| Project | Isolation root: members, approved providers, retention, network policy, credential bindings, export policy and security tier. |
| WorkspaceTemplate | Administrator-approved, versioned image digest, agent support, MCP manifest, resource defaults and required isolation/network policy. A template is not an isolation boundary. |
| Worker private storage | Persistent agent identity, memories, tool state and conversations. Bound to one project and worker through immutable storage ownership; no separate AgentHome catalog or UI. |
| Workspace | Stable identity and private storage for interactive or task execution. A pod/container is an instance of this workspace, not its identity. |
| Run | Durable execution identity bound to one project, agent home and workspace, including native thread/session reference, state, cancellation and activity. |
| Schedule | Durable trigger, owner, project, concurrency policy and a project-local reference to instructions. It must work while execution pods are stopped. |
| Artifact handoff | Explicit source/destination authorization and provenance. Same-project file copies are separate from branch baselines. Cross-project transfers are denied until a separate approved export process is specified. |

Never reuse a thread or assistant memory across projects, even when its model account and human viewer are the same. Different workspaces inside a project have separate homes and storage; sharing project-level memory would require an explicit feature and policy.

The existing `general/research/banking/gallery` profiles are tool presets, not projects. Every existing workspace currently belongs to the prototype's `default` project. A migration must explicitly classify it. Likely assignments are research, banking and gallery projects, with the gallery branch staying in the gallery project; names alone must not authorize migration or relabel content.

## 2. Keep our control plane small and independent of agent engines

The Go control plane owns project authorization, admission of workspace claims, quotas, schedules, run metadata and connection preparation. It does not act as a global conversational agent, persist task bodies or aggregate project memories.

Keep task instructions, results, chat history, Matrix plaintext, T3 databases and Hermes memory in project-owned storage. A shared scheduler or queue carries identifiers, leases and payload references, not prompts or results. Shared infrastructure is still trusted and can carry traffic; metadata-only persistence does not mean it cannot observe content in transit. Stronger administrator separation requires a different deployment tier.

Use two independent adapter boundaries:

- **Runtime provider:** provision, start, inspect, stop, attach and retire a managed workspace from a validated template. Compose/Docker is the development implementation; Kubernetes is another implementation. No caller-selected privileged mounts, host paths or runtime flags.
- **Agent adapter:** discover capabilities, launch/resume a supported native run, observe events, cancel and prepare a client connection. T3/Codex, native Claude Code and Hermes retain their own protocols and durable content. Unsupported capabilities are reported explicitly.

The control plane tracks operational state from the actual engine. A native T3 turn and a separate CLI task must not silently become two competing authorities for one run. Activity includes detached work, terminals, pending approvals and domain-local scheduled execution. A disconnected client does not cancel a run; restarting compute does not automatically replay an interrupted side effect.

## 3. Reuse a sandbox lifecycle implementation

Evaluate **Kubernetes SIGs Agent Sandbox first** for the cluster backend. Its `Sandbox`, `SandboxTemplate`, `SandboxClaim` and `SandboxWarmPool` resources fit stable workspace identity and dynamic allocation. It supports runtime selection including Kata; it does not supply our project authorization or prove our deployment is secure. [Agent Sandbox documentation](https://agent-sandbox.sigs.k8s.io/docs/)

Use **OpenSandbox** as the main comparison candidate for a unified Docker/Kubernetes execution service. Select one lifecycle owner after a bounded integration spike; do not operate two controllers over the same pods or volumes. [OpenSandbox repository](https://github.com/opensandbox-group/OpenSandbox)

Evaluate **OpenShell** separately for reusable network, filesystem, process and inference policy enforcement. Its published controls are useful reference material, but adopting it does not by itself establish the requested Kata VM boundary or authorize subscription token brokerage. [OpenShell security controls](https://docs.nvidia.com/openshell/latest/security/best-practices)

The platform remains responsible for project isolation, approved templates, authenticated users and integrations. Avoid building a custom VM manager, generic agent loop or another MCP protocol implementation.

## 4. Production execution and egress boundary

Target one workspace per pod and one Kata guest boundary per workspace pod, with persistent state mounted only for its owner and a generation fence preventing simultaneous writers. Multiple containers in a Kata pod share that guest boundary; a sidecar is not independently isolated from a compromised guest.

Use project namespaces as organization and policy scope, plus enforced runtime selection, restricted pod specifications, scoped service accounts, default-deny networking, storage isolation and admission policy. Workspace users and agents cannot change RuntimeClass, networking, privileges, host mounts or enforcement. Do not grant direct pod creation, pod/exec, service-account token or Docker access merely to make agent tooling convenient.

Project-bound services have explicit routes for:

1. Controlled DNS: only approved names, no recursive forwarding for arbitrary labels. Block direct IP egress, alternate DNS/DoH/DoT, IPv6 bypasses, host services and metadata endpoints.
2. Dependency retrieval through Nexus: approved package/version promotion, pinned inputs and read-only workspace consumers. Public immutable dependency caches may be shared if policy allows; private artifacts, writable caches, repository credentials and sensitive access logs are project-separated. Nexus's hosted/proxy/group repositories are building blocks, not package approval by themselves. [Nexus repository types](https://help.sonatype.com/en/repository-types.html)
3. Explicit approved external services: operation-aware brokers where credentials or allowed operations matter; constrained destination proxies where they suffice. CONNECT is a tunnel and does not validate encrypted HTTP operations.
4. Approved inference: provider-specific authentication and data handling. Reaching an allowed model still sends project data to that provider; DNS policy cannot prevent that data flow.

Cilium is a candidate for deny policies and DNS-aware egress, alongside an authoritative restricted resolver and proxies. Its DNS policy features must be tested with the actual Kata/CNI combination; an FQDN allowlist is not a substitute for the other controls. [Cilium DNS policy](https://docs.cilium.io/en/stable/security/dns/)

Warm capacity must be unassigned and contain no user data or credentials. Bind a project before attaching state or granting access. Destroy used guests on release rather than recycling dirty memory, volumes or agent homes into another project. Shared read-only base images must contain only approved common dependencies.

Kubernetes namespaces alone do not protect against privileged cluster/host administrators. If project requirements exclude a common administration or control-plane trust boundary, use dedicated clusters/accounts, nodes and key custody as required. [Kubernetes multi-tenancy](https://kubernetes.io/docs/concepts/security/multi-tenancy/)

EKS needs a hardware/runtime feasibility spike before choosing node pools. EC2 documents nested virtualization on selected virtual instance families, but availability is not an EKS/Kata compatibility certification. Validate instance type, enablement, `/dev/kvm`, node OS, containerd/Kata integration, CNI and performance; consider eligible bare-metal nodes where appropriate. k3s likewise needs validated host virtualization support. [EC2 nested virtualization](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/amazon-ec2-nested-virtualization.html)

## 5. Subscription access needs provider-specific designs

Preserve the no-API-billing requirement, but expose a tested provider/engine/authentication compatibility matrix rather than one universal subscription gateway. Scaling pods does not increase an account's plan allowance. Queue work under account-level concurrency/usage budgets while keeping each project's content and sessions separate.

**Codex:** use the native client/app-server and an authorized ChatGPT sign-in path. Official documentation describes subscription login and a separate integration flow for ChatGPT plan usage; select the applicable path for personal/internal versus hosted use and verify entitlement with real turns. Token renewal needs a single supported owner; cloning a rotating refresh credential into many pods is not a renewal strategy. The prototype's expiring token snapshots are not the final account-lifecycle design. [Codex authentication](https://learn.chatgpt.com/docs/auth), [ChatGPT-plan app-server integration](https://developers.openai.com/siwc/token-sharing-open-source/codex-app-server)

**Claude:** prefer the unmodified Claude Code binary with the user's own supported sign-in. Anthropic distinguishes this from third-party collection or intermediation of Claude.ai credentials. The prototype's planned Vault-extracted Claude OAuth gateway must not become the production design without a permitted arrangement. Do not assume a custom harness or Hermes can consume the base subscription. Vault remains useful for approved certificates, Matrix credentials and platform secrets. [Anthropic authentication conditions](https://code.claude.com/docs/en/legal-and-compliance)

**Hermes:** its current provider documentation lists Codex OAuth, but leaves some plan-quota behavior undocumented. Its Anthropic route describes purchased extra usage rather than the base Max allowance. Therefore Hermes directly using Claude is not an accepted no-additional-spend path. A project assistant dispatching to native coding agents is a separate integration to validate; it does not automatically solve how Hermes's own reasoning is funded. [Hermes provider documentation](https://github.com/NousResearch/hermes-agent/blob/main/website/docs/integrations/providers.md)

Subscription credentials are not automatically protected from an agent's own commands when the native process, shell tools and auth files share a guest/user. A Vault origin does not fix that exposure. Define whether per-project native credential possession is acceptable or validate a supported separation between the credential-holding harness and execution. Do not claim the present gateway's credential guarantee for a different native login design.

## 6. T3 desktop schedules through project-scoped capabilities

Keep native T3 protocol/client behavior. The same human desktop can connect to multiple project environments as requested. Its connection catalog and local caches become a trusted human access surface; no agent gets a combined cross-project transcript or result stream.

Upstream T3 documents remote environments and linked-environment delegation, while explicitly treating links as routing rather than isolation. Reuse links only within a project, under narrowed grants. Our pinned build needs a capability/version spike before relying on newer upstream delegation behavior. Native client pairing alone does not provision a cluster workspace. [T3 remote architecture](https://raw.githubusercontent.com/pingdotgg/t3code/main/docs/internals/remote.md)

A proposed platform MCP operation requests a workspace/run using an authorized project, approved template, project-local instruction reference and idempotency key. The controller claims compute, waits for native readiness and returns a stable connection/run reference. Desktop connection preparation and background scheduling both use this policy path. A broker authenticates before wake-up; requests do not widen permissions by supplying a project ID.

Choose one durable trigger owner. The workbench scheduler owns infrastructure claims and recurrence; native T3/Hermes scheduling is bridged or constrained rather than independently duplicating the same schedule. Trigger delivery is at least once, with occurrence IDs and explicit recovery; do not promise exactly-once external actions.

## 7. Hermes and Matrix remain inside one project boundary

Deploy a Hermes gateway and private `HERMES_HOME` per project assistant, with separate memory, session database, skills, cron state and Matrix crypto store. Do not put legally separate projects into profiles inside one shared Hermes container. Hermes documents both running the agent in Docker and using Docker only for shell execution; the latter alone does not confine its memory, messaging or model traffic. [Hermes Docker deployment](https://hermes-agent.nousresearch.com/docs/user-guide/docker)

Bind each Matrix bot credential/device to one project and allowlisted immutable room IDs and senders. Explicitly reject DMs, auto-joins and room switching that escape the binding. The human may join multiple project rooms; the bots cannot read each other's rooms or memory. Keep user/room authorization outside model interpretation.

Hermes already provides a Matrix adapter with E2EE support, so reuse it after a pinned-version integration test. Encryption terminates at the bot: plaintext, attachments and model calls still belong to that project. Required E2EE, membership handling, media, federation and retention need explicit settings; encryption alone is not a project boundary. [Hermes Matrix integration](https://hermes-agent.nousresearch.com/docs/user-guide/messaging/matrix)

An always-on project assistant may wake short-lived coding workspaces through project-scoped MCP. It receives no cluster-admin, Docker socket, other-project credential or global orchestration grant.

## 8. Changes to the current prototype

| Current implementation | Proposed change |
| --- | --- |
| One `local-operator` and `default` project | Real project records and immutable project binding on workspaces, agent homes, runs, credentials and schedules. Human identity may have multiple project memberships. |
| Labels/tool profiles stand in for use cases | Separate approved templates from project ownership and agent choice. |
| Handoffs authorize both sides in one global project | Enforce same-project handoffs; deny cross-project branch seeding, native links and memory access. A privileged human token is not a blanket data-export policy. |
| Supervisor owns task execution plus a concrete T3 child | Separate runtime lifecycle, native agent adapters and a common operational run projection. |
| Docker-specific workspace controller | Runtime provider interface with one selected Kubernetes lifecycle implementation. |
| Shared subscription snapshots/gateway | Provider-specific supported sign-in, renewal ownership, per-project sessions and no-spend fallback rules. |
| No durable scheduling or Hermes/Matrix integration | Project-bound schedules and a project-local messaging assistant. |
| Loopback single-operator launcher | Retain local development convenience; authenticate users/devices and authorize projects for remote/cluster use. |

The existing transport proxies, durable workspace storage, native T3 integration, grants, cancellation/activity work and tests remain useful. Current behavior is an orchestration prototype, not the production trust model.

## 9. Staged delivery and decision gates

1. **Agree the contracts:** project ownership, workspace privacy, same-project handoffs, approved templates, provider authentication model, account usage budgets and schedule ownership. Classify existing workspaces before migrating them.
2. **Compose orchestration slice:** two real projects; persistent agent homes; on-demand workspace and PR claims; narrow MCP; one durable schedule; a project-local Matrix/Hermes experiment using a verified no-additional-spend provider path. Use synthetic data and explicitly unverified VM/network status.
3. **Runtime reuse spike:** compare Agent Sandbox and OpenSandbox on identity/storage, Kata support, streaming/native connections, warm capacity, cleanup, restart reconciliation and project policy. Select one backend and pin releases.
4. **k3s enforcement slice:** validate actual Kata guests, admission, network/DNS/registry rules, scoped storage/keys, client authentication and quota/capacity management. Admission failure blocks readiness; no silent runc fallback.
5. **EKS production slice:** validate eligible node infrastructure, autoscaling, storage, recovery, network policy and the required administration boundary; complete project-specific retention and access review before sensitive data.

Minimum acceptance tests: a project A agent cannot enumerate/read/attach/seed/transfer from project B; Matrix events cannot switch projects; desktop requests cannot enlarge an agent grant; native histories and Hermes memories stay private; a lost trigger acknowledgement does not create a duplicate native run; quota exhaustion never selects paid API fallback; used warm guests never change owner; disconnects do not terminate work; unknown activity blocks unsafe stop; required enforcement failures block startup.

Open decisions after the confirmed project boundary: which projects need dedicated infrastructure; permitted provider sign-in/credential possession model; who may approve same-project transfers; retention and legal holds; Matrix hosting/federation; whether Hermes is a persistent project assistant or only a messaging interface; and the selected sandbox lifecycle backend. These are decisions to resolve before implementation, not permissions requested by this proposal.
