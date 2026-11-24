# Juju Scriptlet Rules for Coding Agents

These rules apply to `internal/scriptlet/` and to packages contributing to the
scriptlet environment, and supplement the [root guidance](../../AGENTS.md).

## Environment Registration

- By convention, packages that help define the scriptlet environment define a
  `RegisterEnv(*scriptlet.EnvRegistry)` function.
- `RegisterEnv` functions are called when constructing the global scriptlet
  environment, in `internal/worker/unitless/environment.go`. Adding a
  contributing package requires adding a call there.
- Registration must complete before the first `App()` call; registration
  after that panics.
- Builtins and values must be independent of any particular Juju model.
