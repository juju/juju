# Cloud Reference Documentation — Conventions

Cloud-specific reference docs describe **what a cloud is and what it supports in Juju**, in descriptive (not imperative) language (Diataxis reference style). Answer "What does this cloud require/support/create?" — not "How do I use it?"

Existing docs are the canonical examples. When in doubt, match what's already there.

---

## Design rationale

### Structure axis: primarily Juju entity, with reader-journey front matter

A purely entity-based structure would be: Cloud → Credential → Controller → Model → Machine → Storage. That ordering is stable and unambiguous, and it was the structure used in earlier iterations of these docs.

The current structure deliberately departs from pure entity ordering in two ways:

1. **Prerequisites and Concepts are promoted to top-level sections** even though they are not entities. This is a reader-experience trade-off: an SRE landing on the page needs to know requirements and limitations *before* investing time in the rest, and needs the concept mapping *before* the entity details make sense. Burying these inside entity sections (e.g. IAM requirements under `## Registration`, concepts under `## The cloud`) caused readers to miss them.

2. **The cloud and Credentials are now promoted to top-level sections**, removing the `## Registration` wrapper which had no content of its own. `## Controllers` (previously `## Bootstrap`) is named for the entity, not the act. This makes the structure purely entity-based from `## The cloud` onward, mirroring the how-to index.

The trade-off: the structure is slightly less pure but significantly more useful to the reader. The entity principle still governs the operational sections (Models → Machines → Storage) and still provides the stability guarantee for updates — new content almost always belongs to one of those entities without ambiguity.

**Why Juju entities for the operational sections?** The content is graph-shaped, not tree-shaped. Every cloud resource simultaneously belongs to multiple valid classification axes: a security group is a networking resource *and* a bootstrap artifact *and* a per-machine artifact. A tree structure forces you to pick one axis and lose the others. Juju entities are the stable axis because Juju's entity model is stable — new cloud features almost always concern how a specific entity (model, machine, storage) behaves differently on that cloud.

**Why not resource type (compute/networking/storage)?** A resource-type axis creates crossed-wire problems at every update. "Where do security groups go — Networking or Bootstrap?" The answer is *both*, which is why resource-type top-level sections always end up either duplicating content or forcing artificial splits. "Machine constraints" in Juju is also a counterexample: `root-disk`, `root-disk-source`, `spaces`, and `zones` are all machine constraints even though they touch storage and networking respectively — splitting them by resource type would fight Juju's own model.

**How resource types are still surfaced:** Compute/networking/storage sub-groupings appear **within** entity sections (as bold sub-headings inside resource lists and constraint lists) where they add clarity, but they do not define the skeleton. The entity axis provides the stable tree; the resource-type dimension is lightweight metadata within it.

### Why not resource-type sections?

Cross-cutting resources (e.g. security groups created at both bootstrap time and per machine, EBS volumes as both root disks and user-managed storage) cannot cleanly belong to one resource-type section without duplication or forced splits. The entity axis eliminates this problem.

### Why not `## Networking` or `## Storage behavior` top-level sections?

Networking and per-machine storage behavior belong under `## Machines`. VPC requirements, security group behavior, and spaces behavior go under `## Machines > ### Networking behavior`; root disk selection and AZ constraints go under `## Machines > ### Storage behavior`. Model config keys for networking or storage go under `## Models`. Dedicated top-level sections would either duplicate content or leave other sections incomplete.

`## Storage` is kept as a top-level section, but it covers only the *storage provider reference* (available backends, pool config options) — not per-machine behavior. The distinction: storage providers are a configurable surface referenced by name in constraints and deploy directives; per-machine behavior is what Juju does when provisioning.

The exception is clouds where Juju does not manage networking (Kubernetes clouds) — those simply have no `### Networking behavior` or `### Storage behavior` subsections.

---

## Structure

Two templates: **machine clouds** and **Kubernetes clouds** (see below). Both organise by Juju entity.

### Machine cloud structure

```
# <Cloud Name>

{note} orientation note

## Limitations          ← surprising behavioral constraints (omit entirely if none)
## Requirements         ← IAM perms, version requirements (omit if none)
## Concepts             ← table mapping cloud abstractions → Juju concepts
## The cloud           ← {include} list-of-supported-clouds/reuse/machine/cloud-definition.md, then Type/Name in Juju
## Credentials
  ### Authentication types
    #### <auth-type>
## Controllers
  ### Bootstrap behavior
  ### Resources created at bootstrap
    **Compute**
    **Networking**
    **Storage**
## Models
  ### Configuration keys
    **Networking**   ← (or Compute / Storage, depending on what the cloud has)
## Machines
  ### Constraints
    **Compute**
    **Networking**
    **Storage**
  ### Placement directives
  ### Resources created per machine
    **Compute**
    **Networking**
    **Storage**
  ### Networking behavior    ← optional; VPC/subnet requirements, subnet selection, spaces, IP behavior
  ### Storage behavior       ← optional; root disk selection, AZ constraints, volume attachment behavior
## Storage
  ### Storage providers
    #### <provider>
## Appendix: Example workflows
  ### <Workflow title> (recommended)
  ### <Workflow title>
```

**Notes:**
- `## Limitations` is first — before Requirements and Concepts — so readers discover showstoppers before investing time in the rest of the page. `## Requirements` follows. Both are omitted when empty.
- From `## The cloud` onward, every top-level section is a Juju entity — mirroring the how-to index structure. There is no `## Registration` wrapper: The cloud and Credentials are entities in their own right.
- `## Controllers` (previously `## Bootstrap`) covers the controller entity on this cloud: bootstrap behavior and all resources created at bootstrap, grouped by **Compute / Networking / Storage** bold sub-headings.
- `## Models` covers cloud-specific model configuration keys, grouped by dimension (**Networking**, **Compute**, **Storage**) as bold sub-headings.
- `## Machines` covers the ongoing operational surface: what users can specify (constraints, placement) and what Juju creates per machine, grouped by dimension. `### Networking behavior` covers VPC/subnet requirements, subnet selection, spaces, IP handling, etc. `### Storage behavior` covers root disk selection, AZ constraints, volume attachment behavior, etc. Both are subsections of `## Machines`, not top-level sections.
- `## Storage` covers cloud-specific storage providers — the available backends and their configuration options. Although storage volumes ultimately attach to machines or pods (just as networking ultimately resolves at the machine/pod level), storage providers are a distinct *configurable surface* referenced by name in storage constraints and deploy directives, and warrant their own section. Per-machine storage behavior (root disk defaults, AZ constraints, volume attachment) lives under `## Machines > ### Storage behavior`, with a cross-reference to the relevant provider in `## Storage`.
- **Constraints, networking, and storage** can all be set at controller, model, or application scope, but they always take effect at the machine or pod level. This is why they all appear under `## Machines` (as constraints, resources, and behavior subsections) even when the scoping is broader.

### Kubernetes cloud structure

Kubernetes cloud docs use a similar entity-based skeleton. Because Juju does not manage Kubernetes networking (the K8s provider explicitly does not implement spaces/subnets), there is no `### Networking behavior` subsection.

Distribution-specific docs (EKS, GKE, AKS, MicroK8s, Canonical Kubernetes) are minimal stubs. Shared content is pulled via `{include}` from `docs/reference/cloud/list-of-supported-clouds/reuse/k8s/` snippets. Cloud-specific details (e.g., required services, adding the cloud, bootstrap preparation) live under the relevant entity section (typically `## The cloud` or `## Controllers`), not in a separate "Distribution-specific notes" section.

```
# <Distribution>   (stub)  OR  # Kubernetes cloud  (if a generic doc is needed)

{note} orientation note

## Concepts            ← {include} list-of-supported-clouds/reuse/k8s/concepts-table.md
## The cloud           ← {include} list-of-supported-clouds/reuse/k8s/cloud-definition.md
                         (cloud-specific requirements, adding-the-cloud notes, etc.)
## Credentials         ← {include} list-of-supported-clouds/reuse/k8s/auth-types.md
## Controllers
                       ← {include} list-of-supported-clouds/reuse/k8s/bootstrap-resources.md
                       ← {include} list-of-supported-clouds/reuse/k8s/controller-service-type.md
                         (cloud-specific bootstrap preparation, if any)
## Models
                       ← {include} list-of-supported-clouds/reuse/k8s/model-config-keys.md
## Pods                ← "Machines" equivalent for Kubernetes
  ### Constraints      ← {include} list-of-supported-clouds/reuse/k8s/constraints.md
                       ← {include} list-of-supported-clouds/reuse/k8s/pod-deployment-patterns.md
## Storage
  ### Storage providers
                       ← {include} list-of-supported-clouds/reuse/k8s/storage-provider.md
```

---

## Conventions

### Intro sentence

Every cloud doc opens with a one-sentence intro immediately after the page title:

```markdown
In Juju, [<Cloud>](<upstream-url>) is a {ref}`machine cloud <machine-cloud>` and works as described below.
```

For Kubernetes:

```markdown
In Juju, [<Distribution>](<url>) is a {ref}`Kubernetes cloud <kubernetes-cloud>` and works as described below.
```

### Orientation note

Opening `{note}` block immediately after the intro sentence:

```markdown
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to and/or consult the {ref}`example workflows <cloud-appendix-example-workflows>`.
```

Omit the appendix clause for docs with no appendix.

### Limitations and Requirements sections

`## Limitations` comes first — before `## Requirements` and `## Concepts` — so readers discover showstoppers before investing time in the rest of the page. If the cloud has no limitations worth calling out, omit the section entirely (absence is informative; do not add a "None" placeholder).

`## Requirements` follows immediately after Limitations (or comes first if there are no limitations). It contains IAM/API permissions Juju needs and version requirements (e.g. LXD 5.x, MAAS 2+).

**What qualifies as a Limitation?** Surprising behavioral constraints that would block or change a deployment design — things a reader might not expect from a general Juju cloud. The bar is: *would a reasonable person be surprised or blocked by this?* Example: EC2's single-NIC behavior when using multiple spaces is a limitation (users expect multiple NICs as on OpenStack/Azure). Missing features that aren't surprising (e.g. a cloud not supporting a specific auth type) are not limitations.

**Language principle for Limitations, Requirements, and Concepts:** Start from Juju-external language (cloud/infrastructure terms the reader already knows) and only progress to Juju-internal terms (spaces, endpoint bindings, models, units) at the end — specifically in the Concepts table where the mapping is made explicit. This eases onboarding: the reader encounters familiar ground first, then learns how Juju names it.

Example — limitation phrasing (infrastructure-first, Juju consequence second):
```markdown
- **Single NIC per machine**: EC2 machines are provisioned with one network interface. If your deployment relies on network isolation across multiple subnets, be aware that Juju will connect a machine to one subnet only. See {ref}`ec2-machine-networking-behavior`.
```

Not:
```markdown
- **Spaces resolve to a single NIC**: When you specify multiple spaces or endpoint bindings, Juju selects one intersecting space rather than adding multiple network interfaces.
```

### Lateral links (ibnote)

Beneath the `##` heading for Bootstrap, Models, Machines, and Storage (and their `###` sub-headings where appropriate), add an ibnote linking to related reference and how-to pages:

```markdown
## Controllers

\```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
\```
```

The ibnote goes **first** in the section (before the intro sentence), so it appears right under the heading.

### Reuse snippets

Machine cloud docs include two shared snippets:
- `{include} ../../../list-of-supported-clouds/reuse/machine/cloud-definition.md` — under `## The cloud`.
- `{include} ../../../list-of-supported-clouds/reuse/machine/credential-definition.md` — under `## Credentials`.

Kubernetes cloud docs include snippets from `docs/reference/cloud/list-of-supported-clouds/list-of-supported-clouds/reuse/k8s/` for all shared content (see K8s structure above). Each snippet is self-contained: it opens with its own ibnote and an "As for all Kubernetes/machine clouds, …" intro sentence.

### Compute/Networking/Storage groupings within sections

Where a section lists resources or constraints across multiple dimensions, use **bold inline headings** to group them — do not create `###` or `####` sub-headings for this purpose:

```markdown
### Resources created

**Compute**
- **EC2 instance**: ...
- **IAM Role/Instance Profile** (optional): ...

**Networking**
- **Security groups**: ...
- **Network interfaces**: ...

**Storage**
- **EBS root volume**: ...
```

The same pattern applies to `### Constraints` and `### Configuration keys`.

### Controller / machine resource cross-references

The controller runs on a machine provisioned via the same mechanisms as workload machines. The `## Controllers > ### Resources created` section therefore cross-references `## Machines > ### Resources created per machine` (and vice versa), explaining that controller resources are a special case with different defaults:

```markdown
### Resources created at bootstrap

The controller runs on an EC2 instance provisioned using the same mechanisms as workload machines — see {ref}`<cloud>-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.
```

```markdown
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults are documented in {ref}`<cloud>-controller-resources-created-at-bootstrap`.
```

### Clarifying intros

Each subsection that lists cloud-specific items needs an intro sentence:

- Auth types: `<Cloud> supports the following authentication types:`
- Config keys: `<Cloud> supports the following {ref}\`cloud-specific model configuration keys <model-config-cloud-specific-key>\`:`
- Constraints: `<Cloud> supports the following {ref}\`constraints <constraint>\`:`
- Placement directives: `<Cloud> supports the following {ref}\`placement directives <placement-directive>\`:`
- Storage providers: `In addition to generic storage providers, <Cloud> provides the following {ref}\`cloud-specific storage providers <storage-provider-cloud-specific>\`:`

### Anchors

Every section and subsection must have an anchor. Pattern: `(<cloud>-<section>-<subsection>)=`, matching the heading title with the cloud prefix.

Examples:

| Heading | Anchor |
|---|---|
| `## Limitations` | `(<cloud>-limitations)=` |
| `## Requirements` | `(<cloud>-cloud-requirements-iam)=` (add a qualifier if the requirements are domain-specific) |
| `## The cloud` | `(<cloud>-cloud)=` |
| `## Credentials` | `(<cloud>-credential)=` |
| `### Authentication types` | `(<cloud>-credential-authentication-types)=` |
| `#### \`instance-role\`` | `(<cloud>-credential-instance-role)=` |
| `## Controllers` | `(<cloud>-controller)=` |
| `### Bootstrap behavior` | `(<cloud>-controller-bootstrap-behavior)=` |
| `### Resources created at bootstrap` | `(<cloud>-controller-resources-created-at-bootstrap)=` |
| `## Models` | `(<cloud>-model)=` |
| `### Configuration keys` | `(<cloud>-model-configuration-keys)=` |
| `## Machines` | `(<cloud>-machine)=` |
| `### Constraints` | `(<cloud>-machine-constraints)=` |
| `### Placement directives` | `(<cloud>-machine-placement-directives)=` |
| `### Resources created per machine` | `(<cloud>-machine-resources-created-per-machine)=` |
| `### Networking behavior` | `(<cloud>-machine-networking-behavior)=` |
| `### Storage behavior` | `(<cloud>-machine-storage-behavior)=` |
| `## Storage` | `(<cloud>-storage)=` |
| `### Storage providers` | `(<cloud>-storage-providers)=` |

Exception: storage provider anchors are `(storage-provider-<name>)=` (globally unique by name, not prefixed with the cloud name).

### Authentication types

Use `####` headings (one level below `### Authentication types` which is itself under `## Credentials`). Content per type varies (attributes lists, version notes, requirements, ibnote blocks).

Env vars used for auto-detection (e.g. `GOOGLE_APPLICATION_CREDENTIALS`) belong under the specific auth type they affect as an **Auto-detection:** note, scoped to `juju autoload-credentials`.

### Configuration keys

Use **anchored bullet items**, not `####` headings. Format:

```markdown
(<cloud>-model-<key>)=
- **`<key>`**: <Description>. Type: `<type>`. Default: `<default>` (or `none`). Immutable. Mandatory.
```

Rules:
- Always include Type and Default.
- Add `Immutable.` only when true; silence = mutable.
- Add `Mandatory.` only when true; silence = optional.
- Use `none` (not `""`) when there is no default.
- Because list-item anchors have no implicit title, `{ref}` to these anchors must always use explicit display text: `` {ref}`vpc-id <ec2-model-vpc-id>` ``

### Constraints and placement directives

Bullet lists. Append cloud-specific notes inline (e.g. `. Valid values: \`amd64\`.`). Add a `{note}` block for mutual-exclusion rules. No `####` headings.

Constraints are a Juju provisioning spec, not a pure compute concept — `root-disk`, `root-disk-source`, `spaces`, and `zones` are all constraints even though they touch storage and networking. Group them by dimension (**Compute / Networking / Storage**) using bold headings, but keep them in one flat list conceptually.

### Storage providers

Use `####` headings — providers have their own config options and behavior notes.

### Appendix sections

Workflow heading style: `### <Verb phrase>` (e.g. `### Authenticate with managed identity (recommended)`). Mark the recommended workflow explicitly.

### Language and punctuation

- Descriptive, not imperative: "is created", "can be configured" — not "Create...", "Configure..."
- Sentential bullets end with periods.
- Em-dash: ` -- ` (space-hyphen-hyphen-space).
- "As for all machine/Kubernetes clouds, …" — intro sentence pattern for reuse snippet sections.

### What not to include

- Step-by-step how-to instructions (belong in how-to guides).
- Troubleshooting procedures.
- Operational workflows.

---

## Adding a new cloud or updating an existing one

1. Study `internal/provider/<cloud>/` — especially `credentials.go` (auth types/attributes), `config.go` (config keys + defaults), `environ_policy.go` (supported constraints), `environ.go` / `environ_broker.go` (bootstrap/machine resources), and the provider's networking implementation for spaces/subnet support.
2. Check `docs/reference/space.md` for cloud-specific spaces behavior (single NIC vs multiple NICs, inherited vs discovered subnets) and add any limitations to `## Limitations`.
3. Follow the structure and conventions above. The canonical example for machine clouds is `amazon-ec2.md`; for Kubernetes clouds, use `microk8s.md` (stub) alongside the `docs/reference/cloud/list-of-supported-clouds/list-of-supported-clouds/reuse/k8s/` snippets.

### Verifying constraint accuracy against code

The constraint tables are the most error-prone section. Before committing, cross-check each documented constraint against the provider code:

**Step 1 — Check `var unsupportedConstraints`** in `environ_policy.go` (or `environ.go`, `constraints.go` — search with `grep -rn "var unsupportedConstraints" internal/provider/<cloud>/`). Any constraint in this list must **not** appear in the doc's constraint table.

**Step 2 — Check `validator.RegisterConflicts`** calls. The conflict note in the doc must exactly match what is registered. Common mistake: listing a constraint in the conflict group when it is only conditionally conflicting (e.g. OpenStack `root-disk` is validated in `PrecheckInstance`, not `RegisterConflicts`).

**Step 3 — Check `validator.RegisterVocabulary`** for `constraints.Arch` and other vocabulary-restricted constraints. If arch is registered as `[amd64, arm64]`, the doc must say `Valid values: amd64, arm64`.

**Step 4 — Check `RegisterConflictResolver`**. If a conflict has a resolver (e.g. EC2 and Azure allow `instance-type` + `arch` together when they are compatible), the conflict note must mention the exception.

**Step 5 — Check that documented constraints are actually used.** If a constraint is not in `unsupportedConstraints` but is also never referenced in `StartInstances`/`environ.go`, Juju will accept it but silently ignore it. Such constraints should be omitted from the doc (e.g. OCI `cpu-power`).

The storage provider section likewise needs checking against `storage.go`: verify `DefaultPools()` for pool names, `ConfigSchema()` for `account-type` defaults and valid values, and the storage provider type name.

---

## Changelog

| Date | Change |
|------|--------|
| 2026-06-04 | Initial pattern documented based on Azure analysis |
| 2026-06-04 | Entity-based restructuring: Cloud/Credential/Controller/Model/Machine/Storage with anchor pattern `<cloud>-section-subsection` |
| 2026-06-04 | Added Kubernetes cloud template |
| 2026-06-17 | PR `3.6-update-cloud-ref`: Removed dropdown quickstart sections (deferred). Renamed appendix headings to verb phrases. Renamed `### Supported constraints` → `### Constraints`. |
| 2026-06-18 | Compacted config key format: anchored bullets, inline Type/Default. Env vars moved under specific auth type. |
| 2026-06-24 | Major restructure session: entity axis confirmed as primary skeleton. `## Security` → `## Registration` → `## The cloud` + `## Credentials` (promoted); `## Bootstrap` → `## Controllers`; `## Compute`/`## Networking` top-level sections removed; Compute/Networking/Storage as bold sub-groupings within entity sections. `## Prerequisites` split into `## Limitations` (first) + `## Requirements`; both omitted when empty. Spaces behavior incorporated. `### Storage behavior` added parallel to `### Networking behavior` under Machines. Rationale documented. |
| 2026-06-25 | Removed `## Distribution-specific notes` from K8s cloud template. Cloud-specific details (requirements, adding the cloud, bootstrap prep) moved into the relevant entity section (`## The cloud` or `## Controllers`). Fixed snippet path typo. |
| 2026-06-30 | Added constraint-accuracy audit procedure to "Adding a new cloud" section. Corrections applied to all clouds: EC2 (`allocate-public-ip`, `virt-type` removed; `arch` added to `instance-type` conflict with resolver note); GCE (`arch` removed from `instance-type` conflict); Azure (`arm64` added to arch vocab; `StandardSSD_LRS` added as default storage account type); OpenStack (`access-key` auth type added; `instance-type` conflict corrected from `[mem, root-disk, cores]` to `[mem, cores]` with `root-disk` conditional note); OCI (`cpu-power` removed -- silently ignored by provider; `region` credential marked optional not required); vSphere (disk-provisioning-type corrected: `thin`, `thick` (default), `thick-lazy-zero` -- `thickEagerZero` does not exist in code). |
