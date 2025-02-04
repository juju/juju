(conventional-commits)=
# Conventional commits

The [Conventional Commits standard](https://www.conventionalcommits.org/en/v1.0.0/) is adopted for the Juju project with the
following structure:

```
<type>(optional <scope>): <description>

[optional body]

[optional footer(s)]
```

- **type**: Describes the kind of change (e.g., feat, fix, docs, style, refactor, test, chore).

|Type|Kind of change|Description|
|---|---|---|
|feat|Features|A new feature|
|fix|Bug fixes|A bug fix|
|docs|Documentation|Documentation only changes|
|style|Styles|Changes that do not affect the meaning of the code|
|refactor|Code refactoring|A code change that neither fixes a bug nor adds a feature|
|perf|Performance improvements|A code change that improves performance|
|test|Tests|Adding missing tests or correcting existing tests|
|build|Builds|Changes that affect the build system or external dependencies|
|ci|Continuous integration|Changes to our CI configuration files and scripts|
|chore|Chores|Necessary yet mundane/trivial changes (updating dependencies, merging through …)|
|revert|Reverts|Reverts a previous commit|

- **scope**: A scope indicating the part of the codebase affected (e.g., model, api, cli).
    - Can be Optional is it’s hard to define the scope
- **description**: A brief summary of the change.
    - Must be provided after the BREAKING CHANGE:, describing what has changed about the API, e.g., BREAKING CHANGE: environment variables now take precedence over config files.
    - Should use lowercase.
    - Should not end in any punctuation.
- **body**: Detailed explanation of the change.
    - Can be Optional for small/trivial `fix`, but NOT for other types.
    - Could consist of several paragraphs.
    - Explanation of change should details what was before, and what is after, and avoid contextual terms like 'now'
    - Good body should be on the form:
    * Before this commit `<it behaves like that>`
    * After this commit `<it behaves like that>`
- **footer**: (Optional) Information about breaking changes, issues closed, etc.
    - A commit can contain a footer which must consist of one or more [Git trailers](https://git-scm.com/docs/git-interpret-trailers).
    - The footer must start at the first occurrence of a blank line, followed by a Git trailer.
    - Each trailer must start on its own line, using the format `<key><sep><value>`.
    - The trailer `<key>` must be either BREAKING CHANGE or be one or more words, grouped by hyphens (e.g. Co-Authored-By, fixes, etc.).
    - The trailer `<sep>` must be one of `:<space>` or `<space>#`, supporting both `Co-Authored-By: Thomas Miller <thomas.miller@canonical.com>` and `Fixes #999`.
    - The trailer `<value>` must be present and can contain any characters, either on a single line, split over multiple lines, or split over multiple paragraphs.


Here is the reference example of a conventional commit message:

```
feat(api): add user authentication feature

# scope: Indicates the part of the codebase affected
# Here, 'api' is specified as the scope

# description: A brief summary of the change in lower-case
# "add user authentication feature" summarizes the change concisely

This commit adds user authentication to the API. Users can now sign up,
log in, and log out. Passwords are hashed using bcrypt. Token-based 
authentication is implemented using JWT.

# body: Detailed explanation of the change
# A more detailed explanation of what has been changed and why

BREAKING CHANGE: The user authentication changes the login endpoint
from `/api/login` to `/api/v1/login`. All previous tokens are now invalid,
and users will need to reauthenticate.

# footer: Information about breaking changes
# Here, the footer indicates a breaking change, describing what has changed
# about the API and how it impacts users

Authored-By: Thomas Miller <[thomas.miller@canonical.com](mailto:thomas.miller@canonical.com)>

Fixes #123

# footer: Issues closed or other information
# This footer references to and author and an issue that this commit closes.
```

The PR title, where a conventional commit is introduced, should be the same format as a first line of a commit description: `<type>(optional <scope>): <description>`. In case this commit is core/base/most significant for this PR.

### Footer and Multi-paragraph Body
### Juju-Specific Scopes

Define specific scopes relevant to the Juju project to enhance clarity. Examples include:

- model: Logical grouping of applications, representing an environment.
- controller: Manages and oversees multiple models, providing centralized management.
- charm: Encapsulated operational code for applications, defining how they are deployed and managed.
- application: Deployed instance of a charm, representing a running service.
- unit: Individual instance of an application, running on a machine.
- relation: Connection between different applications, enabling them to communicate and share data.
- bundle: Collection of applications and relations, defining a complex deployment setup.
- action: Scripted operation on an application or unit, allowing for specific tasks to be performed.
- machine: Physical or virtual host running units, part of the infrastructure.
- storage: Persistent data storage for units, ensuring data remains available across restarts.
- offer: Exposed application endpoint to other models, allowing cross-model relations.
- endpoint: Specific interaction point for a relation within an application.
- constraint: Resource specifications for machines and applications, such as CPU and memory requirements.
- user: Individual with access to the Juju environment, assigned roles and permissions.
- space: Logical network segmentation within models, aiding in network isolation and security.
- subnet: IP address range within a space, used for assigning IPs to machines and units.
- backup: Process of saving the state of a model or controller for recovery purposes.
- migration: Moving a model or application from one controller to another.
- resource: Files or data required by charms during deployment and operation.
- upgrade: Process of updating charms, applications, or Juju itself to newer versions.
- cloud: Infrastructure provider where models and applications are deployed, such as AWS, Azure, or OpenStack.
- credential: Authentication details for accessing cloud providers and resources.

This list just illustrates the idea and suggestion of which single words should be used for scope definition. It is NOT complete and is NOT limited by these scopes.

### Other Examples
```
feat(networking): add juju networking support for all provider

<optional body for feat(networking)>
```
---
```
feat!: remove ticket list endpoint

refers to JIRA-666

BREAKING CHANGES: ticket enpoints no longer supports list all entites.
```
---
```
fix(migration): fix the universal migration to any major/minor version in future

<optional body for fix(migration)>
```
---
```
docs: add support for multi-version + tips windows

<optional body for docs>
```
---
```
refactor(cmr): remove all uclear and understandle code from CMR implementation

<optional body for refactor(cmr)>
```
---
```
fix(backup): fix backup db state issue

<optional body for fix(backup)>
```

See Specification [JU098](https://docs.google.com/document/d/1SYUo9G7qZ_jdoVXpUVamS5VCgHmtZ0QA-wZxKoMS-C0) for context.
