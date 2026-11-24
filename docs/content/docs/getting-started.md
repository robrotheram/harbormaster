+++
title = "Verify a local installation with the CLI"
+++

Use this tutorial to create a worker, submit a task, and retrieve the same result after a restart. The fixture returns a deterministic hash. Use the [native agent tutorial](work-conversations.md) when you want an agent to create files.

Start the [local Compose deployment](deployment.md#start-the-local-compose-deployment) with its default fixture settings. Install `jq` and run these commands from the repository root in the same terminal. If you changed `CONTROL_PORT`, set `CONTROLPLANE_URL` to the matching local address.

## Create a worker

1. Open [the dashboard](http://127.0.0.1:8080/app/). Sign in as `operator` using the password in `.local/secrets/dashboard_local_password`.
2. Create a worker in the `default` project with the CLI. The CLI reads its credential from `.local/secrets/control_token`.

   ```sh
   WORKSPACE_ID=$(go run ./cmd/workbench workspace create --key getting-started-workspace | jq -er .id)
   go run ./cmd/workbench workspace get "$WORKSPACE_ID"
   ```

   A new worker shows `STOPPED`. If you rerun the tutorial, the same key returns the existing worker.

## Run the task and inspect its result

1. Submit a short instruction. Submission starts the worker if necessary.

   ```sh
   INSTRUCTION='Getting started: check that this workspace can accept a task.'
   TASK_ID=$(go run ./cmd/workbench workspace submit "$WORKSPACE_ID" "$INSTRUCTION" --key getting-started-task-v1 | jq -er .id)
   go run ./cmd/workbench workspace status "$WORKSPACE_ID" "$TASK_ID"
   ```
2. If status is `queued` or `running`, run the status command again. When it shows `succeeded`, read the result.

   ```sh
   go run ./cmd/workbench workspace result "$WORKSPACE_ID" "$TASK_ID"
   ```

   The `content` field starts with `[SYNTHETIC FIXTURE] messages_sha256=`. If status is `failed`, note the `error_code` from the status response and check the [deployment diagnostics](deployment.md#check-an-unhealthy-installation).

Reuse a mutation key for the same request. For a different instruction, use a new key. The API returns `IDEMPOTENCY_CONFLICT` when the same task key is reused with different instructions.

## Check that the result survives a restart

1. Stop the worker and start it again. The first response should show `STOPPED`.

   ```sh
   go run ./cmd/workbench workspace stop "$WORKSPACE_ID"
   go run ./cmd/workbench workspace start "$WORKSPACE_ID"
   ```
2. Read the same task result. Compare the hash with the one you recorded before stopping the worker.

   ```sh
   go run ./cmd/workbench workspace result "$WORKSPACE_ID" "$TASK_ID"
   ```

   The result is stored on the worker volume. A restart can interrupt a process, but it does not rerun the task.
3. Stop the worker when you finish.

   ```sh
   go run ./cmd/workbench workspace stop "$WORKSPACE_ID"
   ```

## Connect an external agent through MCP

Use the [project MCP guide](project-workbench.md#connect-an-external-agent) to run workspace and task operations through an external agent client.

## Workspace connection profiles

See [change a worker's network access](user-guide.md#change-a-workers-network-access) for built-in profiles, reusable profiles, and when workers apply changes.
