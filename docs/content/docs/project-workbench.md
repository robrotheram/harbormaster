+++
title = "Connect an external agent and schedule work"
+++

See the [user guide](user-guide.md) for the browser workflow. Use these procedures when a client or integration needs its own project credential.

## Connect an external agent

1. From the local operator terminal, list the projects. Copy the ID for the project the client should use.

   ```sh
   go run ./cmd/workbench project list
   ```
2. Replace `PROJECT_ID` with the selected ID. Issue the project credential.

   ```sh
   go run ./cmd/workbench project access PROJECT_ID
   ```

   The command writes a private configuration file in `.local/project-access/` and prints its path. The credential expires after 24 hours.
3. Configure your client for Streamable HTTP using the file's endpoint and private Authorization header. The default endpoint is `http://127.0.0.1:8082/mcp`. If you changed `MCP_PORT`, change the URL in the client configuration. Use a client that supports bearer headers.
4. Use the project's workspace list to verify the connection. The credential authorizes operations within the selected project. Keep it in the client's private configuration.

The local bootstrap MCP token in `.local/secrets/mcp_token` is scoped to `default`. The CLI uses the separate control token. For a client on another machine, use an authenticated tunnel to the local MCP endpoint.

## Schedule work

1. Put the instruction file in the target worker's project files. Configure native execution for that worker.
2. Use the project credential with `schedule_create` in MCP or `POST /v1/schedules` in the management API. Supply the worker, instruction path, and trigger. See the [Hermes schedule example](../../../workspace/hermes/examples/digest-schedule.json) for a file-based request.
3. Check the stored schedule through `schedule_list`. Use `schedule_run_now` with a retry key to exercise one occurrence. Use `run_list` to inspect scheduled run records.
4. Use `schedule_disable` to pause future automatic occurrences. Cancel accepted work separately if needed.

The scheduler wakes the target worker and deduplicates retries of the same occurrence. Missed recurring intervals are coalesced. The browser has no Schedules or Runs page. See [schedule management](architecture/contracts.md#schedule-management) for update and deletion requests.
