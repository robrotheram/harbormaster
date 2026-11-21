<img src="assets/icon.svg" width="64" height="64" alt="">

# Harbormaster

Harbormaster is an agent workbench for software and research projects. Give an agent a task, follow its work in a browser, and review the files or application it produces. Agents run in managed workers that keep their files and conversation history between stops.

Use the project overview to plan work, the workspace to talk to an agent, and the review queue to approve a result or request changes. You can also open a worker in T3 Code or connect an external agent through MCP.

## Try it locally

On Linux with Docker Engine, Docker Compose, Make, and the Go version in [go.mod](go.mod), run this from the repository root.

```sh
make up
```

Open [the dashboard](http://127.0.0.1:8080/app/) and sign in as `operator`. The password is in `.local/secrets/dashboard_local_password`. The default Compose setup is for local development and uses synthetic responses. See the deployment guide before connecting a real provider.

## Documentation

- [Deployment guide](docs/deployment.md) for Compose, Kubernetes, configuration, and recovery.
- [Development guide](docs/development.md) for building, testing, and changing the code.
- [User guide](docs/user-guide.md) for projects, agent work, reviews, and workers.
- [Documentation index](docs/README.md) for tutorials, integrations, and technical reference.

Browse the [published documentation](https://robrotheram.github.io/harbormaster/) or run `make docs-serve` to preview it locally. See [how to build and serve the docs](docs/development.md#build-and-serve-the-docs).

[MIT license](LICENSE)
