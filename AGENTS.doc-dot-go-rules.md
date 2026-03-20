# Rules for Writing doc.go Files in Juju

## The Locality Principle

**Document everything at the smallest scope where it can be accurately documented.**

- Interface-level contracts → document on the interface
- Function-level behavior → document on the function
- Package-level patterns → document in doc.go
- Cross-package concerns → document at the package that embeds all related packages

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
Add sections only for package-level patterns. Use ASCII diagrams for state machines or workflows (70 character width max).

## Writing Guidelines

1. **Maintain the red thread**: Explicitly repeat the main topic (e.g., "agent configuration") rather than using pronouns like "it" or "this".

2. **Document contracts, not implementation**: State what callers can rely on and what constraints they must respect. Never describe internal mechanisms (locks, goroutines, data structures).

3. **Use " -- " for em dashes**: Not "-" or "—", but " -- " with spaces.

4. **Use ASCII diagrams for workflows**: When documenting state transitions, sequences, or data flow, add ASCII diagrams.

   Example:
   ```
   //	New Agent                 First Connect              After Connect
   //	+-----------------+       +-----------------+       +-----------------+
   //	| old: set        |       | old: set        |       | current: NEW    |
   //	| current: empty  | ----> | current: empty  | ----> | old: set        |
   //	+-----------------+       |                 |       +-----------------+
   ```

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

4. **Consider ASCII diagrams**: For state transitions, sequences, or workflows, add ASCII diagrams (70 char width max).
