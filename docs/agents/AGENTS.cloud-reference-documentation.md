# Cloud Reference Documentation — Conventions

Cloud-specific reference docs describe **what a cloud is and what it supports in Juju**, in descriptive (not imperative) language (Diataxis reference style). Answer "What does this cloud require/support/create?" — not "How do I use it?"

Existing docs are the canonical examples. When in doubt, match what's already there.

---

## Structure

Two templates: **machine clouds** and **Kubernetes clouds** (see below). Both organise by Juju entity.

### Machine cloud structure

```
# <Cloud Name>

{note} orientation note (see Orientation note below)

## Requirements
## Concepts          ← table mapping cloud abstractions → Juju concepts
## The cloud         ← Type/Name in Juju
## Credentials
  ### Authentication types
    #### <auth-type>   ← #### heading per type; content varies (see Auth types below)
## Controllers
  ### Bootstrap behavior
  ### Resources created at bootstrap
  ### Other           ← cloud-specific subsections only if needed
## Models
  ### Configuration keys
## Machines
  ### Constraints
  ### Placement directives
  ### Resources created per machine
  ### Networking behavior  ← optional
## Storage
  ### Storage providers
    #### <provider>
## Appendix: Example workflows   ← or "Example quickstart" for simpler clouds
  ### Authenticate with <method> (recommended)
  ### Authenticate with <method>
  ...
```

### Kubernetes cloud structure

```
# Kubernetes cloud  (generic doc)  OR  # <Distribution> (stub doc)

## Requirements
## Concepts
## The cloud
## Credentials
  ### Authentication types
## Controllers
  ### Bootstrap behavior
  ### Resources created at bootstrap
  ### Controller service type
## Models
  ### Configuration keys (Model configuration keys)
## Pods                ← "Machines" equivalent for K8s
  ### Constraints
  ### Placement directives
  ### Resources created per application
## Storage
  ### Storage providers
```

Distribution-specific docs (EKS, GKE, AKS, MicroK8s, Canonical K8s) are minimal stubs that note cloud-specific differences and link to the generic Kubernetes cloud doc.

---

## Conventions

### Orientation note

Opening `{note}` block. For docs with an appendix:

```markdown
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`tutorial`, then use this page together with the generic materials it links to and/or consult the {ref}`example workflows <cloud-appendix-example-workflows>`.
```

Use `example quickstart` (with corresponding anchor) for docs that use that appendix name. For docs with no appendix, omit the second clause.

### Lateral links (ibnote)

Beneath each `##` section heading, add an ibnote block linking to related how-tos:

```markdown
## Credentials

\```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
\```
```

### Clarifying intros

Each subsection that lists cloud-specific items needs an intro sentence making scope explicit:

- Auth types: `<Cloud> supports the following authentication types:`
- Config keys: `<Cloud> supports the following {ref}\`cloud-specific model configuration keys <model-config-cloud-specific-key>\`:`
- Constraints: `<Cloud> supports the following {ref}\`constraints <constraint>\`:`
- Placement directives: `<Cloud> supports the following {ref}\`placement directives <placement-directive>\`:`
- Storage providers: `In addition to generic storage providers, <Cloud> provides the following {ref}\`cloud-specific storage providers <storage-provider-cloud-specific>\`:`

### Anchors

Pattern: `(<cloud>-section-subsection)=`

Examples: `(azure-credential-managed-identity)=`, `(ec2-controller-bootstrap-behavior)=`

Exception: storage provider anchors are `(storage-provider-<name>)=` (globally unique by name).

### Authentication types

Use `####` headings — content per type varies too much for bullets (requirements lists, version notes, ibnote blocks).

Env vars used for auto-detection (e.g. `GOOGLE_APPLICATION_CREDENTIALS`) belong under the specific auth type they affect as an **Auto-detection:** note, scoped to `juju autoload-credentials`. Do not float them before `### Authentication types`.

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
- Use `none` (not `""` or `(omitted)`) when there is no default.
- Because list-item anchors have no implicit title, `{ref}` to these anchors must always use explicit display text: `` {ref}`vpc-id <ec2-model-vpc-id>` ``

### Constraints and placement directives

Bullet lists. Append cloud-specific notes inline (e.g. `. Valid values: \`amd64\`.`). Add a `{note}` block for mutual-exclusion rules. No `####` headings.

### Storage providers

Use `####` headings — providers have their own config options and behavior notes that warrant navigable structure.

### Appendix sections

Workflow heading style: `### Authenticate with <method>` (verb phrase). Mark the recommended workflow with `(recommended)`.

Deferred quickstart sections (`### Add cloud, add credential, bootstrap`, anchor `<cloud>-appendix-quickstart`) were removed in PR `3.6-update-cloud-ref` and preserved in `misc/cloud-playbook-debrief-template.md` (sections 14–15). Restore with improvements in the follow-up PR.

### Language and punctuation

- Descriptive, not imperative: "is created", "can be configured" — not "Create...", "Configure..."
- Sentential bullets end with periods.
- Em-dash: ` -- ` (space-hyphen-hyphen-space).

### What not to include

- Step-by-step how-to instructions (belong in how-to guides)
- Troubleshooting procedures
- Operational workflows

---

## Adding a new cloud or updating an existing one

1. Study `internal/provider/<cloud>/` — especially `credentials.go` (auth types/attributes), `config.go` (config keys + defaults), `environ_policy.go` (supported constraints), `environ.go` / `environ_broker.go` (bootstrap/machine resources).
2. Follow the structure and conventions above.
3. Match the style of the closest existing doc.

---

## Changelog

| Date | Change |
|------|--------|
| 2026-06-04 | Initial pattern documented based on Azure analysis |
| 2026-06-04 | Entity-based restructuring: Cloud/Credential/Controller/Model/Machine/Storage with anchor pattern `<cloud>-section-subsection` |
| 2026-06-04 | Added Kubernetes cloud template |
| 2026-06-17 | PR `3.6-update-cloud-ref`: Removed dropdown quickstart sections from all 15 docs (deferred). Renamed appendix headings to verb phrases. Renamed `### Supported constraints` → `### Constraints` with `{ref}` link. |
| 2026-06-18 | Compacted config key format: anchored bullets, inline Type/Default, Immutable/Mandatory only when true. Placement directive intros use `{ref}\`placement directives <placement-directive>\``. Env vars moved under specific auth type. Orientation note appendix link folded into existing sentence. File slimmed to conventions only. |
