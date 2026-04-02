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

- Always use `tc` for writing unit tests.
- Assertions:
  - Use `c.Assert(err, tc.ErrorIsNil)` for error checks.
  - Prefer `c.Check` for value assertions.
  - Use `c.Assert` for value assertions only when needed to guard subsequent assertions (e.g. prevent nil dereference).
  - The use of `tc.Must` (e.g. `tc.Must(c, NewUUID)`) should be limited to simple test setup.
- Checkers:
  - Checkers are passed to `c.Assert` and `c.Check` as the second argument.
  - Examples of checkers arr `tc.IsTrue`, `tc.IsFalse`, `tc.Equals` and `tc.DeepEquals`.
  - Use `c.Assert(err, tc.ErrorIs, MySentinalErr)` instead of `c.Assert(errors.Is(err, MySentinalErr), tc.Equals, true)`.
  - Use `c.Check(booleanExpr, tc.IsTrue)` instead of `c.Check(booleanExpr, tc.Equals, true)`.
  - There are more Checkers, look for the most appropriate checker.
- For `select` cases, use test context (`c.Context`) instead of timeouts.
- If a test event must occur, block on it and rely on the native test timeout
  instead of adding an explicit timeout branch.

## Running Tests

Tests must be run with the `-race` flag for code with mutexes or goroutines.
For code with goroutines, tombs or catacombs, stress must be used to ensure
robustness.

- `go test ./path/to/package` — Run package tests.
- `go test -run 'TestName' ./path/to/package` — Run specific test.
- `make pre-check` — Static analysis (golangci-lint). Run before submitting.

### Running `stress` Tests

Compile the go package into a binary and run the test through stress.
A timeout must wrap the `stress` command to ensure that stress halts in a good
timeframe.

 - `go test ./path/to/package -c -race`
 - `timeout 31 stress -timeout 30s ./package.test`

## Integration Tests (bash-based)

- Located in `tests/suites/*/task.sh`.
- Run via: `cd tests && ./main.sh <suite>` or `./main.sh <suite> <test_name>`.
- Framework includes: `tests/includes/juju.sh` (bootstrap, ensure, destroy),
  `tests/includes/wait-for.sh` (polling), `tests/includes/check.sh` (assertions).

## General Code Structure Guidelines

- Place methods and functions below others that call them.
- Limit comment line lengths to 80 characters.
- When wrapping errors across layers, add identifying context such as
  entity UUIDs once at the highest useful layer. Keep state-layer
  `Errorf` messages generic to avoid repeated identifiers in the final
  error chain.
