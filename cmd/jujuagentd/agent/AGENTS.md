# Juju Agent Wiring Rules for Coding Agents

These rules apply to `cmd/jujuagentd/agent/` and its sub-packages and supplement
the [root guidance](../../../AGENTS.md).

## Manifold Wiring

- When changing manifold declarations, worker configuration or service wiring,
  compare the machine and Kubernetes variants and the corresponding wiring in
  `machine/`, `model/`, `modeloperator/`, `safemode/` and `dbrepl/` where
  relevant.
- When changes make wiring equivalent, consolidate it in the existing common
  declaration area where available. Match dependencies, configuration and
  startup, restart and shutdown semantics before sharing declarations.
- Preserve intentional differences between deployment and operating modes.
  Identical constructor calls alone do not establish equivalent lifecycle
  requirements. Retain coverage for each affected mode when consolidating.
