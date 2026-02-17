# Juju Agent Rules Index

Apply both files below for any code change:

1. [Architectural rules](AGENTS.architecture-rules.md)
2. [Core domain rules](AGENTS.core-domain-rules.md)

If guidance conflicts, architectural rules take precedence.

## Build

- `make install` — Full build including schema regeneration.
- `make go-build` — Build without schema rebuild.
- `make juju` — Build the CLI client only.
- `make jujud-controller` — Build the controller binary (includes domain services, dqlite).
    - WARNING: `go build ./cmd/jujud` builds the *agent* binary, NOT the controller.
      This is a common mistake. The agent binary lacks domain services and will
      not function as a controller.

## Test

- `go test ./path/to/package` — Run package tests.
- `go test -run 'TestName' ./path/to/package` — Run specific test.
- `make pre-check` — Static analysis (golangci-lint). Run before submitting.

## Integration Tests (bash-based)

- Located in `tests/suites/*/task.sh`.
- Run via: `cd tests && ./main.sh <suite>` or `./main.sh <suite> <test_name>`.
- Framework includes: `tests/includes/juju.sh` (bootstrap, ensure, destroy),
  `tests/includes/wait-for.sh` (polling), `tests/includes/check.sh` (assertions).
