# Contribute to Harbormaster

Use the [development guide](docs/development.md) to install tools, run the app, and choose checks. Read [architecture and module ownership](docs/architecture.md) before you change a service boundary.

## Make a change

- Keep policy in the application package that owns the operation. Let transports adapt requests and runtime or database packages implement the owning package's interfaces.
- Add coverage for changed ownership, concurrency, recovery, or protocol behavior. If a change spans HTTP, runtime clients, and controllers, check the operation across those boundaries.
- Preserve existing workspace ownership and storage during migrations. Record any compatibility path, its consumer, and the condition for removing it.
- Use synthetic fixtures. Keep `.local`, credentials, private history, workspace files, and browser artifacts out of commits.

## Open a pull request

1. Run the [checks for your change](docs/development.md#check-your-changes).
2. Describe the problem, the resulting behavior, and how you verified it.
3. Check the [CI workflow](.github/workflows/check.yml) before merging. For deployment changes, follow the [release acceptance checks](docs/operations.md#release-gates).

Harbormaster uses the [MIT license](LICENSE). Third-party runtimes, fonts, and integrations retain their respective licenses.
