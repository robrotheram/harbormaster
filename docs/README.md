# Harbormaster documentation

Start with the guide for the work you need to do.

| Goal | Guide |
| --- | --- |
| Install or operate Harbormaster | [Deployment guide](deployment.md) |
| Change or test Harbormaster | [Development guide](development.md) |
| Plan, run, and review agent work | [User guide](user-guide.md) |

## Tutorials

- [Run your first synthetic task](getting-started.md) with Compose and the CLI.
- [Complete a work conversation](work-conversations.md) with a configured native agent.

## Task guides

| Task | Guide |
| --- | --- |
| Attach T3 Code, branch a worker, or copy a file | [Interactive workers](interactive-workspaces.md) |
| Import or sync GitHub and GitLab issues | [Repository connections](repository-connections.md) and [project tasks](issue-board.md) |
| Approve, close, or request changes | [Review tasks](reviews.md) |
| Read activity, export conversation history, or annotate runs | [Activity logs](workspace-logging.md) and [private history](workspace-history.md) |
| Issue an external agent credential | [Project MCP and schedules](project-workbench.md) |
| Connect Codex or Claude | [Subscription credentials](subscription-agents.md) |
| Configure Hermes or pass work between assistants | [Hermes](../deploy/hermes/README.md) and [assistant handoffs](assistant-handoffs.md) |
| Set up hosted authentication | [WorkOS AuthKit](authkit.md) and [hosted dashboard credentials](../deploy/kubernetes/README.md#configure-hosted-dashboard-access) |
| Configure inference or certificate-backed data operations | [Gateway configuration](../config/README.md) |
| Install or verify Kubernetes or Hermes on Kubernetes | [Kustomize configuration](../deploy/kubernetes/README.md), [k3s operations](k3s.md), and [Kubernetes Hermes](k3s-hermes.md) |
| Install the experimental Firecracker backend | [Firecracker host setup](firecracker.md) |
| Release, upgrade, or restore the platform | [Operations](operations.md) and [container publishing](container-publishing.md) |

## Technical reference

- [Architecture and module ownership](architecture.md) describes service boundaries, data placement, and runtime support.
- [Security boundaries and verification](security.md) describes credential handling, networking, and live acceptance requirements.
- [Service contracts](contracts.md) records selected HTTP, MCP, and runtime contracts. Check the implementation and its tests for a route's full contract.
- [Guest execution contract](workspace-factory-contract.md) describes the worker artifact and service protocol.
- [Frontend notes](../frontend/README.md) and the [Hermes integration package](../integrations/hermes/README.md) describe component-specific development.
- [Contribution guidance](../CONTRIBUTING.md) describes code review and compatibility expectations.

## Historical records

These documents describe earlier plans, implementation checkpoints, and test environments. Use the current guides above for setup and navigation.

- [Proposed architecture](architecture-direction.md)
- [Original implementation contract](implementation-contract.md) and [November 2026 orchestration contract](implementation-contract-2026-11.md)
- [Software factory plan](software-factory-plan.md) and [implementation checkpoint](software-factory-implementation.md)
- [Workbench consolidation notes](workbench-consolidation.md) and [frontend design notes](../frontend/DESIGN.md)
- [Enterprise UX review](reviews/enterprise-ux-2026-11-14.md), [release readiness review](reviews/release-readiness-2026-11-14.md), and [retained preview evidence](evidence/factory-retained-preview.md)
