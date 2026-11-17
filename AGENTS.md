# Juju Agent Rules Index

Apply both files below when implementing or reviewing code:

1. [Architectural rules](AGENTS.architecture-rules.md)
2. [Core domain rules](AGENTS.core-domain-rules.md)

If guidance conflicts, architectural rules take precedence.

Before modifying or reviewing code, read any applicable nested `AGENTS.md`
files between the repository root and each relevant file's directory. Nested
rules supplement the shared rules above. Where nested files conflict, the
closest applicable `AGENTS.md` to the affected file takes precedence;
architectural rules always take precedence.

Use plain `AGENTS.md` for new directory-specific guidance. Keep repository-wide
topic files (`AGENTS.<topic>.md`) at the root and reference them explicitly from
this index.

## Documentation

- [Documentation agent files](docs/agents/) — Documentation-specific agent guidance.

## Build

- `make install` — Full build including schema regeneration.
- `make go-build` — Build without schema rebuild.
- `make juju` — Build the CLI client only.
- `make jujuagentd` — Build the machine agent binary.

## Behaviour Checks

- Trace changed behaviour from data producers through conversions,
  serialisation and persistence to consumers. Check that fields and supported
  representations survive each boundary and that consumers handle them.
- Verify that intended side effects reach their destination; a method named
  to save, repair or persist something does not establish that it does so.

## Unit Test Conventions

- Always use `tc` for writing unit tests.
- Assertions:
  - Use `c.Assert(err, tc.ErrorIsNil)` for error checks.
  - Prefer `c.Check` for value assertions.
  - Use `c.Assert` for value assertions only when needed to guard subsequent assertions (e.g. prevent nil dereference).
  - The use of `tc.Must` (e.g. `tc.Must(c, NewUUID)`) should be limited to simple test setup.
- Checkers:
  - Checkers are passed to `c.Assert` and `c.Check` as the second argument.
  - Examples of checkers are `tc.IsTrue`, `tc.IsFalse`, `tc.Equals` and
    `tc.DeepEquals`.
  - Use `c.Assert(err, tc.ErrorIs, MySentinalErr)` instead of `c.Assert(errors.Is(err, MySentinalErr), tc.Equals, true)`.
  - Use `c.Check(booleanExpr, tc.IsTrue)` instead of `c.Check(booleanExpr, tc.Equals, true)`.
  - There are more Checkers, look for the most appropriate checker.
- Test suites:
  - A suite is a struct that embeds the util suites it needs, and is run
    from a `Test` function with `tc.Run`. Embed
    `internal/testing.BaseSuite` (imported as `coretesting`) unless there is
    a reason not to: it redirects the logger, isolates the user's home
    directory, scrubs the Juju environment variables and blocks outgoing
    network access.
  - Suite methods named `TestXxx` take a `*tc.C`.
  - A `SetUpTest` must call the `SetUpTest` of the embedded suites first, in
    the order they are embedded with `BaseSuite` first, and a `TearDownTest`
    must call theirs in reverse.

    ```go
    type toolsSuite struct {
        coretesting.BaseSuite

        dataDir string
    }

    func TestToolsSuite(t *testing.T) {
        tc.Run(t, &toolsSuite{})
    }

    func (s *toolsSuite) SetUpTest(c *tc.C) {
        s.BaseSuite.SetUpTest(c)
        s.dataDir = c.MkDir()
    }
    ```
- For `select` cases, use test context (`c.Context`) instead of timeouts.
- If a test event must occur, block on it and rely on the native test timeout
  instead of adding an explicit timeout branch.
- Cover materially distinct inputs, outputs and error paths, including
  representations produced by other callers. For round trips, exercise both
  directions and supported representations. A regression test for one reported
  case may not establish the whole contract.

## Running Tests

Tests must be run with the `-race` flag for code with mutexes or goroutines.
For code with goroutines, tombs or catacombs, stress must be used to ensure
robustness.

- `go test ./path/to/package` — Run package tests.
- `go test -run 'TestName' ./path/to/package` — Run specific test.
- `make pre-check` — Static analysis (golangci-lint). Run before submitting.
- For `internal/provider/kubernetes` unit tests, prefer `kubernetes.Interface`
  with `k8s.io/client-go/kubernetes/fake.NewClientset` and reactors over ad hoc
  function injection when exercising Kubernetes API interactions.

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
- Prefer British English spelling.
- Avoid `IAAS` and `CAAS` in new logs, errors and user-facing text. Use machine
  or Kubernetes terminology as appropriate. Prefer `K8s` over `CAAS` in new
  identifiers for Kubernetes functionality.
- When wrapping errors across layers, add identifying context such as
  entity UUIDs once at the highest useful layer. Keep state-layer
  `Errorf` messages generic to avoid repeated identifiers in the final
  error chain.

## Commit Messages

Commit messages and PR titles MUST follow
[Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/) with
the Juju rules below. CI runs commitlint (`.github/commitlint.config.mjs`)
on every commit in a PR and blocks non-compliant commits.

```
<type>(<scope>): <short description>

<body>

<footer>
```

- **type**: One of `build`, `chore`, `ci`, `docs`, `feat`, `fix`, `perf`,
  `refactor`, `revert`, `style` or `test`; commitlint rejects others. Add
  `!` for breaking changes (`feat!`, `fix!`). Juju specifics:
  - `fix` covers bug and performance fixes in non-test code; `test` covers
    test-only changes.
  - `refactor` preserves behaviour and MUST NOT contain an inseparable bug
    fix.
  - `docs` MUST be used when documentation files change and MUST NOT carry
    changes better described by another type. Minor comment or
    documentation edits MAY ride along in the commit type they relate to.
  - `chore` is for maintenance, formatting and minor lint-driven changes
    (e.g. `interface{}` to `any`, a typo in a comment or unexported name).
    It MUST NOT be used where another type applies.
- **scope**: SHOULD be a single word naming the one sub-package or domain
  affected (e.g. `api`, `apiserver`, `cli`, `storage`, `model`,
  `controller`, `agent`, `database`, `cmd`, `core`, `cloud`). Omit it if
  no single such word fits.
- **short description**: Lowercase, no trailing punctuation, no stuttering
  (`fix: fix bug` is not allowed).
- **body**: REQUIRED for `feat`, `fix`, `docs`, `refactor`, `test`, `build`,
  `ci` and `chore`, except for trivial changes. It SHOULD explain why the
  change is needed: the problem or symptom, its cause and its impact.
  Describe behaviour rather than listing the edits. When the change alters
  behaviour, the body SHOULD contrast it before and after, avoiding words
  like "now", e.g. "Before this commit …" and "After this commit …". Wrap
  body lines at 72 columns.
- **footer**: Git trailers after a blank line, e.g. `BREAKING CHANGE: …`,
  `Fixes #999`.

```
fix(storage): race condition when attaching a volume

Before this commit, concurrent attach requests could both observe the
volume as unattached and both proceed, recording the volume as attached
to two machines.

After this commit, the attachment is claimed in a single transaction, so
the second request fails with an already-attached error.

Fixes #123
```

## Pull Requests

The PR title MUST use the commit-message format of the most significant
commit in the PR.

When creating a PR, you MUST read `PULL_REQUEST_TEMPLATE.md` and use it as
the structure of the description. Fill in every section; do not omit any.
Mark checklist items that do not apply with strikethrough (`~text~`). The
template's HTML comments SHOULD be removed. In Links, the `Fixes #19267.`
and `[JUJU-](…)` placeholders SHOULD be replaced with real references, or
with `None` when there are none.

The PR description becomes the body of the merge commit, so treat it as the
prospective merge commit message:

- It SHOULD describe the final change, its important behaviour and
  tradeoffs, and its rationale, explaining the problem or motivation rather
  than restating the diff.
- It SHOULD mention an alternative or earlier approach when that helps
  explain the final design or its tradeoffs.
- It SHOULD include context that helps reviewers and be kept current as the
  change evolves. Before merging, remove context that would not be useful
  in the commit history.
- QA steps SHOULD be reproducible, functional instructions for the reviewer
  that exercise the changed behaviour, for example with `juju` commands,
  including setup commands and expected results. Put commands in fenced
  code blocks. Prefer a step whose result differs without the change. For
  documentation changes, build and view the changed pages and, where
  practical, check the behaviour they describe. When no functional path
  exists, say so and why; only then list unit test commands, if relevant.
  QA steps are not a record of checks already run.
- Review responses, review chronology and follow-up validation results
  SHOULD go in comments. Decisions needed to understand the final change
  SHOULD stay in the description.
