# Juju Documentation Rules

This file defines the core philosophy and structure for Juju documentation.

## Documentation Process for New Features

When documenting a new feature, follow this process:

1. **Identify the entity** -- What is the new thing users will work with? (e.g., database, secret, offer)

2. **Identify the tools** -- What commands or interfaces let users interact with this entity?

3. **Create concept documentation** (in `docs/explanation/`) -- Explain:
   - What the entity is
   - Why it matters
   - How it fits into Juju's architecture
   - When to use it

4. **Create tool reference** (in `docs/reference/`) -- Document:
   - Complete command syntax
   - All options and flags
   - Technical details
   - Examples

5. **Create how-to guide(s)** (in `docs/howto/`) -- Show:
   - How to work with the entity through its lifecycle
   - Surface both the concept doc and tool reference through lateral references

This ensures users get the full picture: **what** it is (concept), **how** to use it (how-to), and **detailed reference** (tool docs).

## How-to Guides

### Entity-Centric Structure

How-to guides are organized around **entities** (controllers, models, databases, applications, etc.). Each guide answers: "What can I do with this entity?"

**Title pattern:** "Manage \<entity\>" (e.g., Manage controllers, Manage models, Manage databases)

### Entity Lifecycle Ordering

Sections within a how-to should follow the entity's **lifecycle logic**:

1. **Access/Create** -- How to get or create the entity
2. **List/View** -- How to see what exists
3. **Inspect** -- How to view details or configuration
4. **Modify** -- How to change the entity
5. **Query/Use** -- How to work with the entity
6. **Remove/Destroy** -- How to delete the entity

Not all entities have all stages. Order sections logically based on typical workflows.

**Example (Manage databases):**
- Access databases
- View the database cluster
- List databases
- Switch to a database
- View a database's schema
- Query a database
- Query across databases
- Modify a database
- View a database's change history

### Section Structure

Each section should follow the pattern: **"To \<accomplish task\>, \<do action\>"**

**Good:**
- "To switch to a model database by its name:"
- "To see what columns and data types a table contains:"

**Avoid:**
- Generic headings like "Prerequisites" or "Tips"
- Command-centric organization ("Run .tables")

### Context Establishment

Balance context establishment to support both first-time readers and returning users who jump to specific sections.

**Pattern:**
- **Establish once:** An early section (e.g., "Access the databases") establishes the working environment
- **Self-contained sections:** Each section should be understandable on its own for users who return directly to it
- **Clarify transitions:** Add brief context when the interaction model changes or when a command's syntax differs significantly

**Example:**
- "Access the databases" establishes you're working in the REPL
- Sections showing dot commands (`.models`, `.switch`, `.tables`) are self-contained because the dot prefix signals REPL context
- "Query a database" adds clarifying context ("type SQL statements directly at the REPL prompt") because raw SQL lacks visual cues and users may return to this section repeatedly

**Rationale:** Users often return to how-to guides to reference specific tasks. Sections should be independently usable without requiring full re-reading, while avoiding verbose repetition in every section.

### Child Entities and Bridge Sections

Guides can include subsections for **child entities** when they're tightly coupled to the parent.

**Example:** In "Manage actions", include:
- Manage action tasks
- Manage action operations

**Bridge sections** can connect to related entities:
- In "Manage clouds", include "Manage cloud credentials" that links to the full "Manage credentials" guide

## Lateral References

Link to **concept docs** at the top of how-to guides using `{ibnote}`:

```markdown
```{ibnote}
See more: {ref}`entity-concept`, [External docs](https://...)
```
```

> **Note:** Consider placing a brief intro sentence before lateral reference links for better AI processing. Starting documents with links may cause AI systems to prioritize link-following over content understanding. This pattern is under review.

Link to **tool references** at the end of relevant sections using "See more" `{ibnote}` (zoom-in principle):

```markdown
```{ibnote}
See more: {ref}`command-reference`
```
```

## Content Hierarchy: At-Issue vs Not-At-Issue

Use **dropdowns** for supplementary content readers can choose to view. Label clearly:

- **`Tip:`** -- Workflow advice, best practices
- **`Reminder:`** -- Conceptual background
- **`Troubleshooting:`** -- Problem-solving help
- **`About...` with `:color: warning`** -- Critical safety information

**Placement:** Immediately before where most relevant (after intro paragraph, before the command).

**Example:**
```markdown
```{dropdown} Reminder: What's a view?
A view is a virtual table defined by a SQL query.
```
```

## Writing Style

- Use **entity names** consistently (controller model database, not "controller database")
- Use **em-dash** as ` -- ` (two hyphens with spaces)
- Use **active voice**: "To do X:" not "X can be done by:"
- Provide **context**: "In a model database, to view applications:"
- Use `text` for code blocks (not `bash` or `sh`)
- Keep code blocks clean: empty line before and after triple backticks
- Include sample output after commands when helpful
- Use realistic UUIDs and names in examples

## Upstream Documentation

**Point to upstream; don't duplicate.**

When Juju documentation references technologies maintained by external projects (SQL, YAML, cloud APIs, etc.):

- **DO** link to upstream documentation as the source of truth
- **DO** provide brief guidance on how to use that technology with Juju (e.g., "To modify data, use standard SQL write operations")
- **DON'T** duplicate upstream syntax references, tutorials, or specifications

**Good:**
```markdown
To modify data, use standard SQL write operations.

```{ibnote}
See more: [SQL documentation](https://www.sqlite.org/lang.html)
```
```

**Avoid:**
```markdown
To modify data, use standard SQL write operations:

INSERT INTO table_name (column1, column2) VALUES (value1, value2);
UPDATE table_name SET column1 = value1 WHERE condition;
DELETE FROM table_name WHERE condition;
```

**Rationale:** This keeps our documentation coherent while avoiding maintenance burden and version drift. We maintain Juju-specific guidance; upstream maintains their specifications.

## Version Information

Mark version-specific features immediately after the title:

```markdown
```{versionadded} 4.0
```
```

## What to Avoid

- Metalanguage sections ("Prerequisites", "Tips") -- integrate or use dropdowns
- Command-first organization -- organize by entity, not command
- Generic troubleshooting sections -- place contextually as dropdowns

## Concept Documentation

Concept docs (in `docs/explanation/`) explain **what** an entity is, **why** it matters, and **how** it fits into Juju's architecture.

**Pattern:** A concept doc should link **down** to related tool references and how-to guides using lateral references at the end or in relevant sections.

**Example:** A database concept doc should link to:
- {ref}`juju_db_repl` (tool reference)
- {ref}`manage-the-databases` (how-to guide)

This provides readers with a complete path: concept → reference details → practical usage.

> **Future enhancement:** Consider using sphinx tags to enforce and validate these cross-reference patterns automatically.

## Reference Documentation

Reference docs (in `docs/reference/`) define **what things are** in Juju -- entities, tools, processes, and concepts. Think of an IKEA manual: when you open the package, you first get an inventory that defines each part. That's your reference documentation.

**What belongs in reference:**
- Entity definitions (controller, model, database, application, unit, etc.)
- Tool specifications (juju CLI, juju_db_repl, juju-dashboard, etc.)
- Process references (scaling, upgrading, removing things, etc.)
- Technical specifications and APIs

**Key principle:** If you're defining what something **is**, that's reference. If you're explaining why it **matters** or how it **fits** into architecture, that's explanation/concept.

**Example:** The database entity reference defines what a database is in Juju, its types (controller database, model databases), and technical characteristics. The explanation would cover broader architectural implications and design decisions.

## Tool Reference Documentation

Tool reference docs (in `docs/reference/`) provide complete technical details about commands, options, and usage.

### File Naming

Name reference files after the **tool name users invoke**, not the underlying technology:

- **Good:** `juju-cli.md`, `juju-dashboard.md`, `juju-db-repl.md`
- **Avoid:** `client.md`, `dqlite-repl.md`

Exception: Entity references use the entity name directly (e.g., `model.md`, `controller.md`).

### Anchor Naming

The main anchor should match the tool name, using underscores for separators (MyST convention):

```markdown
(juju_db_repl)=
# `juju_db_repl`
```

Command-specific anchors should follow the pattern `<tool>-<command>`:

```markdown
(juju_db_repl-models)=
### `.models`
```

### Section Organization

Organize tool references by **tool capabilities or attributes**, not by metalanguage categories:

**Good (by capability):**
- Overview
- Database architecture
- Basic commands
- Navigation
- Schema introspection
- Query execution
- Cluster management

**Avoid (metalanguage):**
- Commands
- General commands
- Database navigation commands

This pattern applies to CLI tools, REPLs, and other interactive tools.

### Command Documentation

For commands that are alternatives (not aliases), list them together in the heading:

```markdown
### `.exit`, `.quit`
```

Use **Usage:** for syntax specifications, not **Aliases:** unless one is truly an alias of another.

## Cross-Documentation Patterns

When documenting a feature that spans multiple documentation types (concept, reference, how-to):

1. **Concept doc** links down to tool reference and how-to guide
2. **How-to guide** links up to concept doc (top) and over to tool reference (per section)
3. **Tool reference** links to how-to guide for practical usage

This creates a documentation triangle where users can move between understanding (concept), reference (details), and practice (how-to).

> **Future enhancement:** Consider sphinx tags or other tooling to validate these cross-reference patterns and ensure documentation completeness.
