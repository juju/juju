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
- **body**: Detailed explanation of the change (OPTIONAL for small/trivial fixes only; REQUIRED for `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `ci`, `chore`)
- **footer**: One or more [Git trailers](https://git-scm.com/docs/git-interpret-trailers), e.g. `BREAKING CHANGE: ...` or `Fixes #999` (OPTIONAL)

Guidelines are provided for each Component.

## **body** Component

- A detailed explanation of the change; may consist of several paragraphs; states what was before, and what is after, and avoids contextual terms like "now".
- SHOULD be on the form:
  - Before this commit `<it behaves like that>`
  - After this commit `<it behaves like that>`

## **footer** Component

- The footer starts at the first occurrence of a blank line, followed by a Git trailer.
- Each trailer starts on its own line, using the format `<key><sep><value>`.
- The trailer `<key>` is either `BREAKING CHANGE` or one or more words grouped by hyphens (e.g. `Co-Authored-By`, `fixes`).
- The trailer `<sep>` is one of `:<space>` or `<space>#`, supporting both `Co-Authored-By: Name <email>` and `Fixes #999`.
- The trailer `<value>` MUST be present and can span multiple lines or paragraphs.

## **type** Component

- **feat**: New feature or functional change
- **feat!**: New feature or functional change that breaks compatibility
- **fix**: Bug or performance fix in non-test code
- **fix!**: Bug or performance fix in non-test code that breaks compatibility
- **refactor**: Changes in non-test code that change the structure or algorithms used but preserves functionality. 
  **refactor** SHALL NOT be used where the commit contains an inseparable bug fix.
- **style**: Changes that do not affect the meaning of the code (formatting, whitespace)
- **perf**: A code change that improves performance
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

Subject only:

```
feat(api): add user authentication endpoint
```

```
fix(storage): race condition when attaching a volume
```

```
docs: add CLA requirements to contributing guidelines
```

With a body and a footer:

```
feat(api): add user authentication feature

This commit adds user authentication to the API. Users can now sign up,
log in, and log out. Passwords are hashed using bcrypt. Token-based
authentication is implemented using JWT.

BREAKING CHANGE: The user authentication changes the login endpoint
from `/api/login` to `/api/v1/login`. All previous tokens are now invalid,
and users will need to reauthenticate.

Fixes #123
```

## Critical Requirements

- **Format correctly on first attempt** - you cannot rewrite history after pushing
- **PRs with non-compliant commits will be blocked** by commitlint in CI
- Validation runs automatically via `.github/commitlint.config.mjs`
- The pull request title MUST use the commit-message format of the most-significant
  commit included in the pull request: `<type>(<scope>): <short description>`

## Pull Request Description

When creating a pull request, you MUST read `PULL_REQUEST_TEMPLATE.md` at the
repository root and use it as the structure for the PR description. Fill in
each section according to the changes being made. Do not omit sections; use
strikethrough (`~text~`) for items that are not applicable.

## References

- [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/)
- [Contributing guide](../../CONTRIBUTING.md)
