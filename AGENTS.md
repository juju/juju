# Juju Agent Rules Index

Apply both files below for any code change:

1. [Architectural rules](AGENTS.architecture-rules.md)
2. [Core domain rules](AGENTS.core-domain-rules.md)

If guidance conflicts, architectural rules take precedence.

## Documentation

- [Documentation rules](AGENTS.documentation.rules.md) — Guidelines for writing user-facing documentation.
- [Package doc.go rules](AGENTS.doc-dot-go-rules.md) — Guidelines for writing package-level documentation.

### Cloud Reference Style Notes

- In cloud-specific docs, place cloud requirements content under `## The cloud` -> `### Requirements`.
- For embedded requirement labels, prefer real subheadings (for example `#### VPC requirements`) so they appear in page TOCs.

## Build

- `make install` — Full build including schema regeneration.
- `make go-build` — Build without schema rebuild.
- `make juju` — Build the CLI client only.
- `make jujud` — Build the controller binary (includes domain services, dqlite).

## Unit Test Conventions

Tests use `gopkg.in/check.v1` (aliased `gc`) with checkers from
`github.com/juju/testing/checkers` (aliased `jc`). Base suites come from
`github.com/juju/juju/testing`.

- Use `c.Assert(err, jc.ErrorIsNil)` for error checks.
- Prefer `c.Check` for value assertions. Use `c.Assert` only when needed to
  guard subsequent assertions (e.g. prevent nil dereference).
- Use `c.Assert(err, jc.ErrorIs, MySentinelErr)` instead of pairing `errors.Is`
  with `jc.IsTrue`.
- Use `c.Check(booleanExpr, jc.IsTrue)` instead of `c.Check(booleanExpr, gc.Equals, true)`.

Every test package must have a `package_test.go` wired to `testing.MgoTestPackage`
or `gc.TestingT` (checked by `scripts/checktesting.bash`).

## Running Tests

Tests must be run with the `-race` flag for code with mutexes or goroutines.
For code with goroutines, tombs or catacombs, stress must be used to ensure
robustness.

- `go test ./path/to/package` — Run package tests.
- `go test -check.f 'TestName' ./path/to/package` — Run specific test.
- `make pre-check` — Static analysis (golangci-lint). Run before submitting.

Tests that touch state or domain code require CGO and dqlite/musl. Generate a
shell alias that sets the required environment:

```
make go-test-alias
```

This prints a `jt` alias. Paste it into your shell, then use `jt ./path/to/package`.

For packages that don't depend on CGO code, plain `go test` works. Otherwise
use `make run-tests` (runs all unit test packages).

### Running `stress` Tests

Compile the go package into a binary and run the test through stress.
A timeout must wrap the `stress` command to ensure that stress halts in a good
timeframe.

 - `go test ./path/to/package -c -race`
 - `timeout 31 stress -timeout 30s ./package.test`

## Code Formatting

After editing any Go file, MUST run `gci` to fix import ordering before
committing. The project uses three import stanzas: stdlib, external, then
`github.com/juju/juju` — each separated by a blank line.

```
gci write --section standard --section default --section "Prefix(github.com/juju/juju)" <file>
```

Run on every `.go` file touched in the change, excluding generated files
(mocks, `*_mock_test.go`, etc.). Failing to do so will cause the `gci`
linter to fail in CI.

## Mock Generation

Mocks use `go.uber.org/mock/mockgen` driven by `//go:generate` directives
in source files. CI checks that generated mocks are up to date (see
`.github/workflows/gen.yml`). Always regenerate mocks when changing
interfaces.

## Integration Tests (bash-based)

- Located in `tests/suites/*/task.sh`.
- Run via: `cd tests && ./main.sh <suite>` or `./main.sh <suite> <test_name>`.
- Framework includes: `tests/includes/juju.sh` (bootstrap, ensure, destroy),
  `tests/includes/wait-for.sh` (polling), `tests/includes/check.sh` (assertions).

## General Code Structure Guidelines

- Place methods and functions below others that call them.
- Limit comment line lengths to 80 characters.
- When wrapping errors across layers, add identifying context such as
  entity IDs once at the highest useful layer. Keep state-layer
  `Errorf` messages generic to avoid repeated identifiers in the final
  error chain.
