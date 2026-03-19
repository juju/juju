---
applyTo: "**/*"
exclude-agent: "code-review"
---

# Commit Instructions for GitHub Copilot Agent

This file provides instructions for the GitHub Copilot coding agent when making commits to this repository.

## Commit Message Format

All commits MUST follow the Conventional Commits standard:

```
<type>(<scope>): <short description>
```

The runes `<` and `>` above are indicative of a Component names only and SHALL NOT be represented in the commit 
message.

### Components

- **type**: The type of change (REQUIRED)
- **scope**: Single-word identifier for the singular affected semantic scope (OPTIONAL)
- **short description**: Brief summary of the change (REQUIRED)

Guidelines are provided for each Component.

## **type** Component

- **feat**: New feature or functional change
- **feat!**: New feature or functional change that breaks compatibility
- **fix**: Bug or performance fix in non-test code
- **fix!**: Bug or performance fix in non-test code that breaks compatibility
- **refactor**: Changes in non-test code that change the structure or algorithms used but preserves functionality. 
  **refactor** SHALL NOT be used where the commit contains an inseparable bug fix.
- **test**: Adding, deleting or updating tests
- **build**: Build system changes (e.g. to Makefile or functional changes that affect build artefacts)
- **ci**: CI configuration changes (e.g. to not test specific shell scripts in the tests directory or to GitHub actions)
- **revert**: Revert previous commit
- **docs**: Documentation changes, **docs** MUST be used where the change affects documentation files and MUST NOT 
  contain changes that are better defined by other commit types. **chore** or any other relevant commit type MAY 
  contain relevant code comment changes or minor relevant documentation file changes.
- **chore**: Maintenance tasks, code style/formatting or minor refactors to correct linting warnings (e.g. 
  `interface{}` to `any` or fixing a typo in a comment/unexported symbol name). **chore** SHALL NOT be used for any purpose defined above.

## **scope** Component

A **single word** identifying the singular affected semantic scope SHOULD be used. The semantic scope SHOULD be 
identified by the sub-package or domain that embodies it:

Common scopes: `api`, `apiserver`, `cli`, `storage`, `model`, `controller`, `agent`, `database`, `cmd`, `core`, `cloud`

The scope MUST meet the following criteria:
- The semantic scope is a singular concern
- The semantic is identified by a single-word identifier
- The identifier refers to a sub-package or domain that embodies the semantic scope

If any of the above criteria are not met, the new scope MUST be omitted.

## **short description** Component

The short description MUST be:
- Written in lowercase
- A brief summary of the change
- Free of stuttering (e.g. "fix: fix bug" is not allowed)
- Free of punctuation at the end

## Examples

```
feat(api): add user authentication endpoint
```

```
fix(storage): race condition when attaching a volume
```

```
docs: add CLA requirements to contributing guidelines
```

## Critical Requirements

- **Format correctly on first attempt** - you cannot rewrite history after pushing
- **PRs with non-compliant commits will be blocked** by commitlint in CI
- Validation runs automatically via `.github/commitlint.config.mjs`

## References

- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- [Project guidelines](../../docs/contributor/reference/conventional-commits.md)
- [Contributing guide](../../CONTRIBUTING.md)
