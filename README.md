<img src="docs/static/icon.svg" width="64" height="64" alt="">

# Harbormaster

Harbormaster is an agent workbench for software and research projects. Give an agent a task, follow its work in a browser, and review the files or application it produces. Agents run in managed workers that keep their files and conversation history between stops.

Use the project overview to plan work, the workspace to talk to an agent, and the task workspace to approve a result or request changes. You can also open a worker in T3 Code or connect an external agent through MCP.

## Try it locally

On Linux with Docker Engine, Docker Compose, Make, and the Go version in [go.mod](go.mod), run this from the repository root.

```sh
make up
```

Open [the dashboard](http://127.0.0.1:8080/app/) and sign in as `operator`. The password is in `.local/secrets/dashboard_local_password`. The default Compose setup is for local development and uses synthetic responses. Follow the [deployment guide](docs/content/docs/deployment.md#configure-inference-and-credentials) to connect a provider.

## Documentation

- [Use the dashboard](docs/content/docs/user-guide.md) to open a project and [run your first agent task](docs/content/docs/work-conversations.md).
- [Architecture](docs/content/docs/architecture/_index.md) covers system context, service boundaries, and deployment choices.
- [Development](docs/content/docs/development/_index.md) covers setup, code ownership, and checks. Coding agents can start with [AGENTS.md](AGENTS.md).
- [Deployment](docs/content/docs/deployment.md) covers local and Kubernetes installation.

Browse the [published documentation](https://robrotheram.github.io/harbormaster/) or run `make docs` to preview it locally. See [how to build and serve the docs](docs/content/docs/development/_index.md#build-and-serve-the-docs).

[MIT license](LICENSE)
