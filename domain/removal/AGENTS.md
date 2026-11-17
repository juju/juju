# Juju Removal Domain Rules for Coding Agents

These rules apply to `domain/removal` and its sub-packages and supplement the
[domain guidance](../AGENTS.md).

## Removal Job Rules

- Do not create removal jobs from within a removal job. Schedule each job
  when the parent entity's removal cascade reaches the target entity.
- Each removal job must manage only its target entity's lifecycle. Let the
  cascade determine removal order.
