# Work frontend contract

The Work section lives at /app/projects/:projectId/work. Existing shared request
transport prefixes paths with /ui-api; ingress must map /ui-api/work-items and
its subpaths to the corresponding /v1/work-items backend operations. No backend
or ingress changes are included here. Adapt api.ts if the eventual UI gateway
uses a different envelope.

Lists and attempt lists return {items: [...]}; detail and create return a work
item directly. Mutations need a successful JSON response (an empty object is
sufficient for run/cancel/review). Every read carries project_id and every
mutation includes it. Attempts must belong to the requested work_item_id.
Work items must include project_id; mismatched responses fail closed.

Artifacts are objects with optional id, name, kind and content_url. Only returned
HTTP(S) content URLs or /v1/, /ui-api/, /dashboard-api/ paths are linked. There is
no constructed artifact host/path. No environment/jobs controls are implemented
until those contracts exist.

Action availability is centralized in workActions in api.ts for parent adaptation:
draft/failed/cancelled/rejected allow run; queued/running/blocked/pending allow
cancel; awaiting_review/review/review_required/pending_review/completed allow
review. The backend remains authoritative for transitions. Run delivery retries
retain their key until success; a subsequent intentional attempt gets a new key.
Creation retries retain the key for the same form payload.

Specifications are existing workspace paths and digests, not uploaded by this UI.
Workspace links open the existing project workspace inventory and show the exact
workspace ID. Attempt evidence and task IDs remain available for diagnostics.
