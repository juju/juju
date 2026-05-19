# Rules for Writing doc.go Files in Juju

## The Locality Principle

**Document everything at the smallest scope where it can be accurately documented.**

- Interface-level contracts → document on the interface
- Function-level behavior → document on the function
- Package-level patterns → document in doc.go
- Cross-package concerns → document at the package that directly embeds all related packages or in project wide documentation where directory distance between packages is greater than 1.

**Before adding anything to doc.go, ask:** "Does this apply to the package as a whole, or to a specific type/function?" If it's specific, document it on that type/function instead.

## Facts vs Interpretation

When adding documentation, distinguish between facts and interpretations:

**Safe to document:**
- Structural relationships (X combines Y + Z, A calls B)
- Observable patterns visible in the codebase (versioned formats, backward compatibility)
- Explicit constraints from existing code comments or validation code

**DO NOT document:**
- Inferred behaviors (atomic, thread-safe, protected context) unless explicitly stated in code
- Implementation mechanisms you cannot verify (internal locking, goroutine usage)
- Safety guarantees or contracts you deduce but aren't explicitly documented
- TODO comments or "should" statements from code comments
- Discoveries of security vulnerabilities or weaknesses in security design. This
  is above your pay-grade -- only divulge these if you have been directly asked
  by the user.

**When in doubt, omit the detail.** Better to under-document than to document hallucinated contracts.

## What Belongs in doc.go

Package-level patterns that span multiple types/functions:
- How multiple interfaces/types relate to each other
- Package-wide patterns (e.g., callback patterns, lifecycle patterns, state machines)
- File format specifications
- Cross-cutting constraints that affect multiple functions
- Package-wide initialization requirements

## What Does NOT Belong in doc.go

These belong on the types/functions themselves:
- Specific interface method contracts
- Individual function parameters and return values
- Type-specific constraints
- Method ordering requirements for a single type

## Structure Pattern

Every doc.go file follows this 3-paragraph structure:

### Paragraph 1: The tl;dr
State what the package does in one line:
```
// Package X manages [topic].
```

### Paragraph 2: Define the key concepts
Define what the topic *is* and what it includes. Use inline definitions (parenthetical or em-dash interpolation):
```
// Agents are Juju processes that run on behalf of specific entities (machines,
// units, applications, models, or controllers). Agent configuration provides
// persistent identity, credentials, and connection details -- API credentials,
// controller addresses, CA certificates, directory paths, and operational
// settings like logging configuration -- that allow them to authenticate to the
// controller and perform their work.
```

### Paragraph 3: Navigation
Point readers to related packages (zoom-out) and sections below (zoom-in). Use generic references to sections:
```
// See github.com/juju/juju/api for establishing API connections using agent
// configuration. See github.com/juju/juju/controller for controller-specific
// settings that agents must handle. See the sections below for package-level
// concerns that span multiple interfaces.
```

### Additional Sections (Optional)
Add sections only for package-level patterns. In doc.go files, use ASCII diagrams for state machines or workflows.

### Provider Package doc.go (internal/provider/*)
For provider packages, keep the same 3-paragraph opening, then add explicit
provider sections that document package-wide behavior boundaries.

Keep provider explanations concise:

- Prefer short, factual statements over long narrative paragraphs.
- Document behavior boundaries and invariants, not step-by-step walkthroughs.
- Keep each bullet focused on one provider-wide concern.
- Omit repeated context when a section heading already scopes the topic.
- Do NOT document provider version numbers or model-config version details.
  Version specifics belong on the types or upgrade steps that manage them,
  not in the package-level doc.

Use `internal/provider/oci/doc.go` as a concrete pattern:

- Include registry and interface context in paragraph 3 (for example links to
  `internal/provider/common`, `internal/provider`, and `environs`).
- Add a section describing how the provider differs from other providers.
  Keep these differences as observable, provider-wide facts.
- Add focused sections for domain-wide behavior that spans multiple files,
  such as configuration, networking, instances/images, storage, and
  regions/availability zones.
- In `# Configuration`, document provider-specific config as a bullet list of
  keys with concise descriptions (for example `compartment-id: ...`). When
  known, include REQUIRED/OPTIONAL, defaults, and key validation constraints.
  Include an `Auth types supported:` line where applicable (as in
  `internal/provider/openstack/doc.go`). Follow `internal/provider/oci/doc.go`
  as the formatting pattern.
- Add maintainer invariants for changes that can have broad provider impact.

Recommended heading outline:

```
// # How the <provider> provider differs from other providers
// # Configuration
// # Networking
// # Instances and images
// # Storage
// # Regions and Availability Zones
// # Maintainer notes
```

When documenting provider networking, describe provider-owned resource creation
when it is package-wide behavior (for example Juju creating a VCN in OCI).

## Writing Guidelines

1. **Maintain the red thread**: Explicitly repeat the main topic (e.g., "agent configuration") rather than using pronouns like "it" or "this". This applies to prose and to section/subsection labels (e.g., "**Secret access and grants**" instead of "**Access and grants**").

2. **Use sentence case for section labels**: Bold subsection labels within `# Package patterns` and `# Package constraints` should use sentence case, not title case.

3. **Document contracts, not implementation**: State what callers can rely on and what constraints they must respect. Never describe internal mechanisms (locks, goroutines, data structures).

4. **Use " -- " for em dashes**: Not "-" or "—", but " -- " with spaces.

5. **Use " -> " for right arrows**: Not "→" or "⮕" or "➡" or "⇨" or "🡒" or "⟶", but " -> " with spaces.

6. **Use ASCII diagrams for workflows**: In doc.go files, when documenting state transitions, sequences, or data flow, add ASCII diagrams.

   Example:
   ```
   //	New Agent                 First Connect              After Connect
   //	+-----------------+       +-----------------+       +-----------------+
   //	| old: set        |       | old: set        |       | current: NEW    |
   //	| current: empty  | ----> | current: empty  | ----> | old: set        |
   //	+-----------------+       |                 |       +-----------------+
   ```

## Guard-Rails for LLM Consumption

Beyond describing what a package does, doc.go should explicitly state behavioral boundaries that prevent misuse. These guard-rails help LLMs (and developers) make safe decisions when using or modifying the package.

**The insight**: Implicit constraints are invisible to LLMs. Humans might learn through experience or tribal knowledge that "you always check grants before accessing secrets" or "domain packages never import from apiserver," but an LLM has no such experience. Making these explicit transforms doc.go from a map (here's what exists) into a rulebook (here's how to use this safely).

### When to Add Guard-Rails

Add guard-rail sections when the package has:
- Transaction or concurrency requirements
- Security-sensitive operations with specific handling rules
- Immutability constraints on types
- Required validation steps before operations
- Layer boundary restrictions (what can/cannot be imported)
- Initialization or cleanup requirements
- Error handling patterns that must be followed
- Performance-critical patterns that must be maintained

### Guard-Rail Section Format

**Both sections document contract-level information** from the caller's perspective:

- `# How this package works` - Descriptive contract information (realis mood): what concepts exist, how they relate, what flows are available. This explains what the package **is**.
- `# How to use this package correctly` - Prescriptive contract information (irrealis mood): constraints callers must respect. This states what callers **must do**.

Neither section documents implementation details. Both focus on the interface contract.

In the prescriptive section (`# How to use this package correctly`), organize by category using bold labels with sentence case. Write guard-rails using RFC 2119 keywords:
- **MUST** statements for mandatory requirements
- **MUST NOT** statements for forbidden actions
- **MAY** statements for optional behaviors
- **SHOULD** statements for recommended but not mandatory practices

The RFC 2119 keywords signal the prescriptive nature of this section.

### Example Structure

```go
// # How this package works
//
// **Secret access and grants**: Access to a secret is managed through grants,
// which define the role (view or manage) and the scope (unit, application,
// model, or relation) of the permissions. [Continue with descriptive content...]
//
// **Secret rotation and expiry**: Secrets can be configured with rotation
// policies (hourly, daily, weekly, etc.) that determine when a secret should be
// updated. [Continue with descriptive content...]

// # How to use this package correctly
//
// **Architecture**: This package MUST NOT import from apiserver or
// internal/worker packages. Domain logic must remain transport-agnostic. Only
// core/* and domain/* packages may be imported for type definitions.
//
// **Immutability**: Secret URIs MUST NOT be modified after creation. Secret
// revisions are append-only -- existing revisions cannot be modified or deleted,
// only obsoleted through expiry.
//
// **Security**: Secret values MUST NOT be logged or included in error messages.
// Secret values MUST NOT be accessed without verifying read permissions.
//
// **Transactions**: Callers MUST NOT hold transactions across multiple service
// calls to avoid deadlocks. Write operations MUST occur within database
// transactions.
//
// **Validation**: Secret URIs MUST be validated using secret.ParseURI() before
// storage. Rotation policies MUST be one of the enumerated constants
// (HourlyRotation, DailyRotation, etc.). Grant scopes MUST match the secret's
// ownership scope.
//
// **Lifecycle**: Services MUST be initialized with a valid backend provider
// before use. Secret watches MUST be closed explicitly to prevent goroutine
// leaks.
//
// **Error Handling**: All errors from the state layer MUST be wrapped with
// context using errors.Annotatef(). NotFound errors MUST use errors.NotFoundf()
// for consistent handling. Security-sensitive errors MUST NOT expose the secret
// URI in the error message.
//
// **Concurrency**: The service layer is concurrency-safe for reads. Write
// operations acquire exclusive locks on the affected entities. Callers MAY call
// read methods concurrently but MUST serialize write operations to the same
// entity.
```

### Section Naming Conventions

**Top-level sections**: Use action-oriented headings that distinguish descriptive (realis) from prescriptive (irrealis) content:

- `# How this package works` - Descriptive contract information: concepts, relationships, flows
- `# How to use this package correctly` - Prescriptive contract information: constraints, requirements, boundaries

The linguistic mood distinction (realis vs. irrealis) is inherent in the verb forms. The RFC 2119 keywords (MUST, MUST NOT, MAY, SHOULD) in the second section reinforce the prescriptive nature.

**Subsection labels**: Use bold labels with sentence case (not title case).
Apply the red thread pattern by repeating the main package topic:
- Good: `**Secret access and grants**`, `**Secret rotation and expiry**`
- Avoid: `**Access and grants**`, `**Rotation and expiry**` (loses context)

### Categories to Consider

When documenting constraints, consider including (where applicable). Categories
MUST be ordered alphabetically in doc.go files:
- **Architecture** - layer boundaries and import restrictions
- **Concurrency** - thread-safety guarantees (only what's explicitly guaranteed)
- **Error handling** - required error handling patterns
- **Immutability** - structural invariants and append-only constraints
- **Lifecycle** - initialization and cleanup requirements
- **Security** - handling of sensitive data, access control requirements
- **Transactions** - database transaction boundaries (critical for domain packages)
- **Validation** - mandatory validation steps before operations

### Balancing Comprehensiveness with Maintainability

To avoid doc.go bloat:
- Focus on **package-wide constraints** that affect multiple functions
- Skip constraints that are **enforced by type systems** (e.g., if a function signature requires `*sql.Tx`, the transaction requirement is obvious from the types)
- Document only what you can **verify in code** (stick to the "Facts vs Interpretation" principle)
- Prioritize **security and correctness** constraints over performance or style preferences
- Omit constraints that are **obvious from function names** (e.g., ValidateURI clearly validates)

### Verification for Guard-Rails

Verify each guard-rail statement by:
1. **Searching for validation code** that enforces the constraint
2. **Finding code comments** that explicitly state the requirement
3. **Locating tests** that verify the boundary condition
4. **Checking error messages** that indicate violation of the constraint

If a constraint cannot be verified through code inspection, grep searches, or explicit comments, do not document it. Unverifiable guard-rails are interpretations, not facts.

### AI-Assisted Guard-Rail Development

LLMs can accelerate guard-rail documentation, but human oversight is critical to distinguish **interface contracts** from **implementation details**.

**What LLMs can reasonably infer:**
- Import patterns and architecture boundaries (what packages are imported/forbidden)
- Function signatures and type relationships (what goes in and out)
- Explicit error types and validation patterns visible in code
- Structural patterns (e.g., State/Service layer separation)

**What LLMs struggle to infer:**
- Implicit behavioral rules ("always check grants before accessing secret values")
- Required operation ordering ("must validate before persisting")
- Transaction boundaries in domain packages (service calls `RunAtomic`, but state methods don't show `*sql.Tx` parameters)
- Immutability constraints (types may be mutable in Go but conceptually immutable)
- Security requirements that span multiple functions
- Concurrency safety guarantees not enforced by type system

**Critical distinction: Contract vs. Implementation**

Guard-rails document **interface contracts** (what callers must respect), not **implementation details** (how the package enforces those contracts internally).

Good guard-rail (contract):
```
**Security**: Secret values MUST NOT be accessed without verifying
grants through GetSecretAccess.
```

Bad guard-rail (implementation detail):
```
**Security**: GetSecretAccess queries the secret_permission table
with a JOIN on secret_metadata...
```

The first states what callers must do; the second reveals internal database structure.

**Recommended workflow for AI-assisted guard-rails:**

1. **LLM analyzes code** and proposes constraint statements based on patterns
2. **Human reviews proposals** to identify which are real contracts vs. current implementation
3. **LLM drafts `# How to use this package correctly` section** with validated constraints in RFC 2119 language
4. **Human verifies** each constraint is observable in code (per "Verification for Guard-Rails")
5. **Iterate** until all statements are contract-level and verifiable

This workflow leverages LLM pattern recognition while maintaining human architectural authority on what constitutes a package contract.

## Verification Process

Before finalizing a doc.go file:

1. **Check locality**: Every sentence describes a package-level pattern, not a type-specific or function-specific detail.

2. **Verify claims against code**: For each factual claim (backwards compatibility, state transitions, file formats, behavior patterns):
   - **State transitions/behaviors**: Use `grep_search` to find code comments, implementations, or tests that confirm the pattern
   - **File formats**: Read format-related files (format.go, parse.go) to confirm version support and compatibility
   - **Type relationships**: Read the actual interface/type definitions to confirm structure
   - **Constraints**: Search for validation code, error messages, or explicit code comments
   - **If unsupported**: Remove the claim or add qualifying language ("typically", "generally")

3. **Check sections**: Each section describes patterns spanning multiple types/functions, not individual type behavior.

4. **Consider ASCII diagrams**: For state transitions, sequences, or workflows, add ASCII diagrams.
