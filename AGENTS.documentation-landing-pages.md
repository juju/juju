---
customInstructions:
  role: >-
    Documentation standards for homepage and landing pages. When asked to update landing pages,
    FIRST assess the current state against these standards, identify gaps, and create a todo list
    grouped by format and content issues. ALWAYS present assessment to user before making changes.
applyTo:
  - pattern: "**/index.md"
    reason: Homepage follows specific structure and content patterns
  - pattern: "**/tutorial/index.md"
    reason: Tutorial landing page should group and explain content flow
  - pattern: "**/howto/index.md"
    reason: How-to landing page should organize by lifecycle/workflow
  - pattern: "**/reference/index.md"
    reason: Reference landing page should organize by logical groupings
  - pattern: "**/explanation/index.md"
    reason: Explanation landing page should group related conceptual topics
---

# Landing Pages Documentation Standards

## WORKFLOW: Assess Before Changing

**When asked to update landing pages, always start with assessment:**

### Step 1: Check Current State

Read the current versions of all landing pages:
- `docs/index.md` (homepage)
- `docs/tutorial/index.md` (if exists as landing page)
- `docs/howto/index.md`
- `docs/reference/index.md`
- `docs/explanation/index.md`

If working in a git repository with a feature branch, check the original state before any changes:
```bash
# Find the baseline commit (first commit in the branch)
git log --oneline [branch-name] --format="%h %s" | tail -n 20
# Extract original files
git show [commit-before-changes]:docs/index.md
```

### Step 2: Identify Gaps

Compare current state against the standards in this document. Create two lists:

**FORMAT GAPS** - Structural/presentation issues:
- Missing required sections
- Wrong formatting (e.g., commas instead of bullet separators •)
- Missing explanatory text for groups
- Incorrect section structure

**CONTENT GAPS** - Logical organization issues:
- Topics grouped alphabetically instead of by user journey/workflow
- Domains of concern that are architecture-facing instead of user-facing
- Missing representation of key documentation areas
- Unclear progression or relationships between topics

### Step 3: Create Todo List

Organize gaps into actionable todos grouped by:
1. **Format todos**: Update to match structure shown in standards
2. **Content todos**: Verify and improve the logic of how topics are surfaced and grouped

### Step 4: Discuss Before Implementing

Present the assessment to the user showing:
- What's already good (no todos needed)
- What needs format fixes
- What needs content discussion

Only proceed with changes after getting user agreement on the approach.

### Example Assessment Output

```markdown
## ASSESSMENT: Current Landing Pages

### Homepage (docs/index.md)

#### FORMAT GAPS ✗
1. **TODO**: Replace commas with bullet separators (•) in "In this documentation" navigation
2. **TODO**: Add "How this documentation is organised" section explaining Diátaxis
3. **TODO**: Restructure "Project and community" into four subsections

#### CONTENT GAPS ✓
No issues - domains are user-facing and follow user journey

### How-to Landing Page (docs/howto/index.md)

#### FORMAT GAPS ✓ Perfect!
Has opening sentence, subsection headings, and explanatory text for all groups

#### CONTENT GAPS ✓ Excellent
Organized by workflow/lifecycle, not alphabetically

### Reference Landing Page (docs/reference/index.md)

#### FORMAT GAPS ✗
1. **TODO**: Add explanatory text for "Tools" group
2. **TODO**: Add explanatory text for "Entities" group
3. **TODO**: Add explanatory text for "Processes" group

#### CONTENT GAPS → NEEDS DISCUSSION
Current type-based grouping (Tools/Entities/Processes) may be acceptable for reference material.
Discuss: Should we reorganize by user domains instead?

---

## SUMMARY
- **9 Format todos** (structural fixes)
- **1 Content question** (requires discussion)
- **How-to page is the model** - already follows standards perfectly
```

### What to Check in Each Landing Page

**Homepage** (`docs/index.md`):
- [ ] Four opening paragraphs (what/does/need/whom)?
- [ ] "In this documentation" section exists?
- [ ] Uses bullet separators (•) not commas?
- [ ] Organized by user-facing domains (not product architecture)?
- [ ] "How this documentation is organised" section explaining Diátaxis?
- [ ] "Project and community" with four subsections?

**How-to landing page**:
- [ ] Opening sentence explaining section purpose?
- [ ] Groups with subsection headings (## level)?
- [ ] Each group has 1-3 sentences of explanatory text?
- [ ] Organized by workflow/lifecycle (not alphabetically)?
- [ ] Shows progression through user journey?

**Reference landing page**:
- [ ] Groups with subsection headings?
- [ ] Each group has explanatory text (what/why/when)?
- [ ] Grouping serves user needs (can be type-based for reference)?

**Explanation landing page**:
- [ ] Groups by theme/layer (not alphabetically)?
- [ ] Each group has explanatory text?
- [ ] Indicates what's foundational vs. advanced?

**Tutorial landing page** (if multiple tutorials exist):
- [ ] Opening explaining progression?
- [ ] Groups by learning path (beginner → advanced)?
- [ ] Prerequisites and time estimates noted?

### Key Distinction: Interpreting "Domains of Concern"

When assessing "In this documentation" organization, consider:

**Which axis (or axes) of "domains of concern" fits this product?**

The standard lists multiple domain types, representing different ways to slice concerns:

- **Point of entry**: Tutorial or essential starting point
- **Conceptual/stack layers**: Hardware, control, software, abstract layer (e.g., mathematics)
- **Features**: Product-specific capabilities
- **Resources and interfaces**: Compute, networking, storage, database, UI
- **Quality**: Security, performance, accessibility, energy efficiency
- **Lifecycle**: Installation, using, deployment, maintenance, scaling, upgrading, development
- **Customer/industry use-cases**: Product-dependent scenarios

These aren't a single system - they're different organizational axes you might choose. The examples in the standard (Mathematics, Hardware and design, Image processing) mix several of these axes.

**The key principle is "rationality"**: users should be able to "follow and work with the structures because they fit with how they think about the product."

For assessing existing documentation, ask:
- **What axis/axes are currently used?** Are domains organized by lifecycle phases, architectural layers, features, quality attributes, or something else?
- **Does this axis match how users conceptualize working with the product?**
- **Is each domain actually a coherent "concern" from the user's perspective?**

Common patterns:
- **Lifecycle-dominant**: "Set up [Product]" → "Deploy applications" → "Monitor and maintain"
- **Layer-dominant**: "Hardware layer" → "Software layer" → "Abstract/mathematical layer"
- **Mixed**: One domain per major axis ("Getting started" / "Security" / "Advanced features")

The test: Can users predict where to find information based on the domain names and their understanding of what they're trying to accomplish? If domains feel arbitrary or unclear, they may not be the right axis for this product and audience.

---

## Homepage Structure (index.md)

The homepage introduces the product and orients users. Follow this exact structure:

### Opening Paragraphs

1. **First paragraph**: A single sentence that says what the product is, succinctly and memorably
2. **Second paragraph**: One to three short sentences that describe what the product does
3. **Third paragraph**: Similar length, explaining what need the product meets
4. **Fourth paragraph**: Describes whom the product is useful for

**Rule**: Keep these paragraphs concise. Each serves a specific purpose in the user's understanding journey.

### In this documentation

This section systematically exposes the documentation's contents organized by **domains of concern**—thematic slices representing different aspects of the product (stack layers, lifecycle stages, features, quality attributes, etc.).

The pattern is designed to be: **systematic, rational, complete, compact, and exposing**.

**Two acceptable patterns**:

**Pattern 1: With subsection headings** (preferred for larger documentation):
```markdown
## In this documentation

### [Domain of concern - e.g., "Set up [Product]", "Quality", "Lifecycle"]

[Brief explanatory sentence about this domain]

* **[Category]**: [Link](#) • [Link](#) • [Link](#)
* **[Category]**: [Link](#) • [Link](#)

### [Domain of concern]

[Brief explanatory sentence]

* **[Category]**: [Link](#) • [Link](#)
```

**Pattern 2: Flat with category labels** (for smaller/simpler documentation):
```markdown
## In this documentation

* **[Domain/category]**: [Link](#) • [Link](#) • [Link](#)
* **[Domain/category]**: [Link](#) • [Link](#)
* **[Domain/category]**: [Link](#) • [Link](#)
```

**Domains of concern** include:
- **Point of entry**: Tutorial (or essential how-to guide if tutorial isn't appropriate)
- **Conceptual/stack layers**: Hardware, control, software, abstract layer (e.g., mathematics)
- **Features**: Product-specific capabilities
- **Resources and interfaces**: Compute, networking, storage, database, UI
- **Quality**: Security, performance, accessibility, energy efficiency
- **Lifecycle**: Installation, using, deployment, maintenance, scaling, upgrading, development
- **Customer/industry use-cases**: Product-specific scenarios

**Rules**:
- Organize by domains of concern, not alphabetically
- Use bullet separators (•) between links
- Pattern 1: Each domain gets a heading and explanatory sentence
- Pattern 2: Use bold category labels for each domain
- Provide complete (though not exhaustive) coverage—every documentation page should be represented
- Order logically (by user journey, stack layers, or importance)
- For large documentation, can add a third layer by grouping multiple domains under a heading
- Dense and compact—present large directory of information in small space

### How this documentation is organised

**Required section** explaining the Diátaxis structure:
```markdown
## How this documentation is organised

This documentation uses the [Diátaxis documentation structure](https://diataxis.fr/).
The [Tutorial](#) takes you step-by-step through [key workflow]. [How-to guides](#)
assume you have basic familiarity with [Product]. [Reference](#) provides a guide to
APIs, key classes and functions. [Explanation](#) includes topic overviews, background
and context and detailed discussion.
```

### Project and community

**Required section** with standard subsections:

```markdown
## Project and community

[Product] is a member of the Ubuntu family. It's an open source project that warmly
welcomes community contributions, suggestions, fixes and constructive feedback.

### Get involved

* [Support](#)
* [Online chat](#)
* [Contribute](#)

### Releases

* [Release notes](#)
* [Roadmap](#)

### Governance and policies

* [Code of conduct](#)

### Commercial support

Thinking about using [Product] for your next project? [Get in touch!](#)
```

**Rule**: Update links and product name, but keep structure and tone.

---

## Key Principles for "In this documentation"

The "In this documentation" section follows these principles:

### System
Uses a reusable system based on division and management of ideas into domains of concern. The system is in the conceptual organization, not the visual design.

### Rationality
Users can follow and work with the structures because they fit with how they think about the product. The domains of concern assert a conceptual model of the product in an ordered layout.

### Completeness
Provides complete (though not exhaustive) coverage—every page except the homepage and landing pages should be represented.

### Density/Compactness
Dense and compact, presenting a potentially large directory of information in a small space. This has a forcing function on documentation architecture quality.

### Pragmatism
Designed to be bent, adapted, extended, and truncated for different scales while retaining a recognizable shape.

### Exposition
Not just showing *what is the case*, but also:
- Exposes the documentation's thinking and ideas
- Brings patterns to the surface for users and maintainers
- Has a forcing function—exposes defects, gaps, contradictions, excess, and lack of systematic thinking

**Note**: This pattern is orthogonal to Diátaxis—it represents the thematic space of the documentation without rearranging Diátaxis content types.

---

## Landing Page Structure (Section Index Pages)

Landing pages guide users through a section's content by organizing links into logical groups with explanatory text.

### Bad Pattern (Never Do This)

```markdown
# How-to guides

* [Install](#)
* [Deploy to ABC](#)
* [Deploy to PQR](#)
* [Deploy to XYZ](#)
* [Manage resources](#)
* [Add instances](#)
* [Monitor performance](#)
* [Diagnose performance issues](#)
* [Configure and manage logging](#)
* [Troubleshooting](#)
```

**Problems**:
- Flat list with no organization
- No context or explanation
- User must guess relationships between topics
- Doesn't convey workflow or lifecycle

### Good Pattern (Always Do This)

```markdown
# How-to guides

These guides accompany you through the complete [Product] operations lifecycle.

## Installation and deployment

Installation follows a broadly similar pattern on all platforms, but due to
differences in the platforms, configuration and deployment must be approached
differently in each case.

* [Install](#)
* [Deploy to ABC](#)
* [Deploy to PQR](#)
* [Deploy to XYZ](#)

## Scaling

As your needs grow, a deployment can be scaled to meet increased traffic needs,
either by allocating additional resources (CPU, RAM, etc) or by adding entire
application instances. See [Approaches to scaling](#) for more discussion of
which strategy to adopt.

* [Manage resources](#)
* [Add instances](#)

## Monitoring and troubleshooting

* [Monitor performance](#)
* [Diagnose performance issues](#)
* [Configure and manage logging](#)
* [Troubleshooting](#)
```

**What makes this good**:
- Opening sentence establishes section purpose
- Content grouped by lifecycle stage or workflow
- Each group has explanatory text providing context
- Cross-references to related conceptual content where helpful
- Shows relationships between topics
- Guides user through logical progression

### Landing Page Rules

1. **Start with orientation**: One sentence explaining what this section contains and its purpose
2. **Group by workflow/lifecycle**: Not alphabetically or by topic in isolation
3. **Add explanatory text**: Each group needs 1-3 sentences explaining:
   - What this group of topics covers
   - When/why users need these guides
   - How topics relate to each other
   - Links to related conceptual material (Explanation section)
4. **Show progression**: Order groups to reflect user journey or operational lifecycle
5. **Cross-reference thoughtfully**: Link to Explanation topics that provide context for decisions
6. **Keep scannable**: Use clear headings, bullets, and whitespace

### Section-Specific Guidance

**Tutorial landing pages**:
- Organize by learning progression (beginner → advanced)
- Explain prerequisites and what each tutorial teaches
- Indicate estimated time or difficulty

**How-to landing pages**:
- Organize by operational lifecycle or workflow stages
- Group related operations together
- Link to conceptual content that helps users choose between approaches

**Reference landing pages**:
- Organize by logical categories (API types, command groups, etc.)
- Provide brief explanation of each category's purpose
- Can be more list-like than other sections, but still grouped

**Explanation landing pages**:
- Organize by theme or architectural layer
- Group related conceptual topics
- Indicate which topics are foundational vs. advanced

---

## Quality Checklist

Before committing homepage or landing page changes:

**Homepage**:
- [ ] Homepage has all four required sections (opening, in this documentation, how organised, project & community)
- [ ] Opening paragraphs each serve their specific purpose (what is it, what does it do, what need, for whom)
- [ ] "In this documentation" organized by domains of concern (not alphabetically)
- [ ] Domains of concern are rational and match how users think about the product
- [ ] Coverage is complete—all pages represented (except homepage and landing pages)
- [ ] Presentation is compact and dense
- [ ] Links use bullet separators (•) in navigation
- [ ] Pattern chosen (subsection headings vs. flat categories) fits documentation scale

**Landing pages**:
- [ ] Landing pages group content logically (not alphabetically)
- [ ] Each group has explanatory text (not just a heading)
- [ ] Cross-references to related content where helpful
- [ ] Content follows user journey or lifecycle progression
- [ ] Tone is welcoming and informative
- [ ] No flat lists without context
