# Juju Removal Domain Rules for Coding Agents

These rules apply to `domain/removal` and its sub-packages and supplement the
[domain guidance](../AGENTS.md).

## Removal Job Rules

- Never schedule a new removal job from inside another removal job. Instead,
  identify the parent entity whose removal cascades the dying process, and
  schedule the job when that cascade reaches the entity. Removal jobs must
  react to a dying entity, not recruit one another.
- An entity taking part in a removal must only enact on itself. Do not
  advance, schedule or progress other entities' removals from within a job;
  each entity owns its own lifecycle. Keep cross-cutting concerns to a
  minimum, and let the cascade drive ordering between entities.