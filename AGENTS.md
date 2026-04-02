# Juju Agent Rules Index

Apply both files below for any code change:

1. [Architectural rules](AGENTS.architecture-rules.md)
2. [Core domain rules](AGENTS.core-domain-rules.md)

If guidance conflicts, architectural rules take precedence.

## Documentation

- [Documentation rules](AGENTS.documentation.rules.md) — Guidelines for writing user-facing documentation.
- [Package doc.go rules](AGENTS.doc-dot-go-rules.md) — Guidelines for writing package-level documentation.

## Build

- `make install` — Full build including schema regeneration.
- `make go-build` — Build without schema rebuild.
- `make juju` — Build the CLI client only.
- `make jujud-controller` — Build the controller binary (includes domain services, dqlite).
  - WARNING: `go build ./cmd/jujud` builds the *agent* binary, NOT the controller.
    This is a common mistake. The agent binary lacks domain services and will
    not function as a controller.

## Unit Test Conventions

- Assertions:
  - Use `c.Assert(err, tc.ErrorIsNil)` for error checks.
  - Prefer `c.Check` for value assertions.
  - Use `c.Assert` for value assertions only when needed to guard subsequent assertions (e.g. prevent nil dereference).
- For `select` cases, use test context (`c.Context`) instead of timeouts.
- If a test event must occur, block on it and rely on the native test timeout
  instead of adding an explicit timeout branch.

## Running Tests

- `go test ./path/to/package` — Run package tests.
- `go test -run 'TestName' ./path/to/package` — Run specific test.
- `make pre-check` — Static analysis (golangci-lint). Run before submitting.
- For `internal/provider/kubernetes` unit tests, prefer `kubernetes.Interface`
  with `k8s.io/client-go/kubernetes/fake.NewClientset` and reactors over ad hoc
  function injection when exercising Kubernetes API interactions.

## Integration Tests (bash-based)

- Located in `tests/suites/*/task.sh`.
- Run via: `cd tests && ./main.sh <suite>` or `./main.sh <suite> <test_name>`.
- Framework includes: `tests/includes/juju.sh` (bootstrap, ensure, destroy),
  `tests/includes/wait-for.sh` (polling), `tests/includes/check.sh` (assertions).

## General Code Structure Guidelines

- Place methods and functions below others that call them.
- Limit comment line lengths to 80 characters.
