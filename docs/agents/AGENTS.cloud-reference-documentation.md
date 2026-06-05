# Cloud Reference Documentation Pattern

This file documents the pattern for cloud-specific reference documentation based on entity-based restructuring.

**Date**: 2026-06-04
**Status**: Patterns established for both machine clouds and Kubernetes clouds

---

## Core Principle

Cloud-specific reference docs describe **the cloud entity in Juju** from the perspective of its properties and capabilities, using **descriptive (not imperative) language**, following the Diataxis framework's reference documentation pattern.

Document answers "What does this cloud require/support/create?" not "How do I use this cloud with Juju?"

---

## Two Templates: Machine Clouds vs. Kubernetes Clouds

There are **two** entity-based templates:

1. **Machine Cloud Template**: For clouds where Juju provisions or adopts infrastructure (VMs, BMs, containers)
2. **Kubernetes Cloud Template**: For clouds where Juju deploys to existing Kubernetes clusters

Both follow the same entity-based structure with key adaptations for their different provisioning models.

---

## Standard Structure for Machine Cloud Reference Docs

Each machine cloud reference doc (in `docs/reference/cloud/list-of-supported-clouds/`) should follow this entity-based structure:

```markdown
# <Cloud Name>

<Short intro describing this as a {ref}`machine cloud <machine-cloud>`>

## Cloud
  ### Definition
  ### Requirements
  ### Other (cloud-specific items like Concepts)

## Credential
  ### Supported authentication types
    #### <auth-type-1>
    #### <auth-type-2>
  ### Known issues (if any)

## Controller
  ### Bootstrap behavior
  ### Resources created at bootstrap
  ### Other (cloud-specific items like instance role integration)

## Model
  ### Cloud-specific configuration keys
    #### <config-key-1>
    #### <config-key-2>

## Machine
  ### Supported constraints
  ### Supported placement directives
  ### Resources created per machine
  ### Networking behavior (if relevant)

## Cloud-specific storage providers
  ### <provider-name>

## Appendix: <Workflow procedures>
```

**Key principles:**
- **Title**: Just cloud name (e.g., "Microsoft Azure"), rely on docs tree for context
- **Introduction**: Start with "In Juju, <Cloud> is a {ref}`machine cloud <machine-cloud>`."
- **Entity-based structure**: Organize by Juju entities (Cloud, Credential, Controller, Model, Machine, Storage)
- **Sections describe cloud properties**: "Requirements this cloud has", "Constraints this cloud understands"
- **"Other" subsections**: For cloud-specific features that don't fit standard template
- **"Supported" prefix**: Make clear when only showing supported items (e.g., "Supported constraints")
- **Anchor pattern**: Use `<cloud>-section-subsection` for uniqueness (e.g., `azure-cloud-requirements`)
- **Appendices for workflows**: Keep command sequences separate from attribute reference
- **Punctuation**: Sentential bullets end with periods. Use ` -- ` (space-hyphen-hyphen-space) for em-dashes.
- **Subsection ordering**: Within each entity section, follow a consistent order (see section templates below)

---

## Anchor Naming Pattern

All anchors should follow the pattern `(<cloud>-section-subsection)=` where:
- `<cloud>` is a short identifier (e.g., `azure`, `aws`, `gce`)
- `section` is the entity (e.g., `cloud`, `credential`, `controller`, `model`, `machine`, `storage`)
- `subsection` identifies the specific content (e.g., `requirements`, `supported-constraints`)

This ensures anchor uniqueness across all cloud documents.

**Examples:**
- `(azure-cloud-definition)=`
- `(azure-credential-managed-identity)=`
- `(azure-controller-bootstrap-behavior)=`
- `(azure-machine-supported-constraints)=`
- `(storage-provider-azure)=` (storage provider names are unique identifiers)

---

## SECTION: Cloud

**Purpose**: Document what this cloud is in Juju's terms and what it requires.

**Location**: First major section after introduction

**Structure**:

### Definition

Type and name as recognized by Juju.

**Template**:
```markdown
(<cloud>-cloud)=
## Cloud

(<cloud>-cloud-definition)=
### Definition

Type in Juju: `<cloud-type>`

Name in Juju: `<cloud-name>`
```

### Requirements

**Purpose**: Document prerequisites for using this cloud (API permissions, accounts, etc.)

**Template**:
```markdown
(<cloud>-cloud-requirements)=
### Requirements

**Required <Cloud> API permissions:**

- `permission.name` (read, write, delete)
- ...

<Additional requirements if any>
```

### Other

**Purpose**: Cloud-specific concepts, mappings, or features that don't fit standard template

**Template**:
```markdown
(<cloud>-cloud-other)=
### Other

#### Concepts

The following table shows how <Cloud>'s native abstractions map to Juju concepts:

| <Cloud> | Juju |
| - | - |
| [Resource Type](link) | {ref}`model <model>` (roughly) |
| [Resource Type](link) | {ref}`machine <machine>` |
| ...
```

---

## SECTION: Credential

**Purpose**: Document supported authentication methods and known issues.

**Location**: Second major section

**Structure**:

### Supported authentication types

List each authentication type as a subheading with description, requirements, and version notes.

**Template**:
```markdown
(<cloud>-credential)=
## Credential

(<cloud>-credential-supported-authentication-types)=
### Supported authentication types

(<cloud>-credential-<auth-type>)=
#### `<auth-type>`

**Requirements:**
- <Requirement 1>
- <Requirement 2>

**Behavior:** <How this auth type works>

**Version note:** <If applicable>

\```{ibnote}
See more: {ref}`<cloud>-appendix-workflow-X`
\```
```

### Known issues

**Purpose**: Document cloud-specific credential problems

**Template**:
```markdown
(<cloud>-credential-known-issues)=
### Known issues

<Description of known issues>
```

**Note**: Keep "Known issues" as a direct subsection, not under "Other". It's a standard documentation pattern for caveats, not a cloud-specific feature.

---

## SECTION: Controller

**Purpose**: Document controller bootstrap behavior and resources.

**Location**: Third major section

**Structure**:

### Bootstrap behavior

High-level description of what happens during bootstrap.

### Resources created at bootstrap

List shared infrastructure created once for the controller.

### Other

Cloud-specific controller features (e.g., instance role integration).

**Template**:
```markdown
(<cloud>-controller)=
## Controller

(<cloud>-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates controller and initial model on <Cloud>.

(<cloud>-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Resource type**: Description. Configuration via `config-key` or `constraint-name`.
- **Resource type**: Description.
- ...

(<cloud>-controller-other)=
### Other

#### <Cloud-specific feature>

<Description>

\```{ibnote}
See more: {ref}`constraint-xxx`
\```
```

---

## SECTION: Model

**Purpose**: Document cloud-specific model configuration keys.

**Location**: Fourth major section

**Template**:
```markdown
(<cloud>-model)=
## Model

(<cloud>-model-cloud-specific-configuration-keys)=
### Cloud-specific configuration keys

(<cloud>-model-<config-key>)=
#### `<config-key>`

<Description>

- **Type**: `<type>`
- **Default value**: `<default>` or none
- **Immutable**: `true` or `false`
- **Mandatory**: `true` or `false`
```

---

## SECTION: Machine

**Purpose**: Document constraints, placement directives, and machine resources.

**Location**: Fifth major section

**Structure** (follow this order):

1. Supported constraints
2. Supported placement directives
3. Resources created per machine
4. Networking behavior (if relevant)

### Supported constraints

List **only** supported constraints as a bullet list. Add notes using colon after constraints that need explanation, ending with a period.

**Template**:
```markdown
(<cloud>-machine-supported-constraints)=
### Supported constraints

\```{note}
<Any conflicting constraints, e.g., "The constraints `instance-type` and `[arch, cores, mem]` are mutually exclusive.">
\```

- {ref}`constraint-<name>`: <Description with punctuation at the end.>
- {ref}`constraint-<name>`
- {ref}`constraint-<name>`: <Description with punctuation at the end.>
...
```

**Important**: List only supported constraints. The section title "Supported constraints" makes it clear these are the only ones that work.

### Supported placement directives

List **only** supported directives as a bullet list.

**Template**:
```markdown
(<cloud>-machine-supported-placement-directives)=
### Supported placement directives

- {ref}`placement-directive-<name>`
- {ref}`placement-directive-<name>`: <Description with punctuation at the end.>
...
```

**Template**:
```markdown
(<cloud>-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **Resource type**: Description. Configuration via constraints/config.
- **Resource type**: Description.
- ...

**Resource tags:** All resources tagged with `juju-model` (model UUID), `juju-controller` (controller UUID), `juju-machine-name` (machine identifier).
```

### Networking behavior

**Purpose**: Document cloud-specific networking (IP addressing, subnet placement, security rules).

**Template**:
```markdown
(<cloud>-machine-networking-behavior)=
### Networking behavior

- **IP addressing**: <How IPs are allocated>
- **Subnet placement**: <Where different machine types go>
- **Security rules**: <Firewall/NSG rules>
```

---

## SECTION: Cloud-specific storage providers

**Purpose**: Document storage providers unique to this cloud.

**Location**: After Machine section

**Template**:
```markdown
(<cloud>-storage)=
## Cloud-specific storage providers

(storage-provider-<provider-name>)=
### `<provider-name>`

**Type:** <Storage type description>

**Configuration options:**

- `option-name`: Description
  - `value1`: Effect (associated with pool `<pool-name>`)
  - `value2`: Effect (associated with pool `<pool-name>`)

\```{ibnote}
See more: [Upstream Docs](link)
\```
```

**Note**: Storage provider names are unique identifiers, so anchor is `storage-provider-<name>` not `<cloud>-storage-provider-<name>`.

---

## SECTION: Appendices

**Purpose**: Provide workflow procedures for authentication and setup tasks.

**Location**: After all entity sections

**Template**:
```markdown
(<cloud>-appendix-example-authentication-workflows)=
## Appendix: Example authentication workflows

(<cloud>-appendix-workflow-1)=
### Workflow 1 -- <Workflow name> (recommended)
> *Requirements:*
> - <Requirement>
> - ...

1. <Step>
2. <Step>
...

\```{tip}
<Helpful context about this workflow>
\```

(<cloud>-appendix-create-<resource>)=
## Appendix: How to create <prerequisite resource>

\```{caution}
This is just an example. For more information please see the upstream cloud documentation. See more: [<Cloud> | <Resource>](link).
\```

<Instructions>
```

---
- For infrastructure-affecting constraints: link to Cloud resources section
- For storage constraints: link to Storage section
- For instance-role: link to Authentication and Bootstrap sections

---

## SECTION: \<Cloud name\> and Juju

**Purpose**: Provide bidirectional understanding of the Cloud-Juju relationship -- how cloud abstractions map to Juju concepts, and how Juju operations manifest as cloud resources.

**Location**: Right after the introduction (section 3)

**Rationale**: Users connecting a cloud to Juju need to understand:
1. How cloud-native abstractions (VMs, disks, networks) translate into Juju's unified model
2. What cloud resources Juju creates when performing operations (bootstrap, deploy, etc.)

Combining these into one section emphasizes that this is fundamentally about the same thing -- the mapping between cloud and Juju -- just viewed from two directions.

**Structure**: Two subsections:
- **Concepts**: Cloud abstraction → Juju abstraction (VM → machine)
- **Resources**: Juju operation → Cloud resources (bootstrap → VM + disk + NIC...)

**Template**:
```markdown
## <Cloud name> and Juju

When connecting <Cloud name> to Juju, it is helpful to understand the bidirectional relationship: how <Cloud name>'s native abstractions translate into Juju's model (concepts), and how Juju operations manifest as <Cloud name> resources (resources).

### Concepts

Juju provides an abstraction layer over <Cloud name> infrastructure, allowing <Cloud name> resources to be managed through Juju's unified interface. The following table shows how <Cloud name>'s native abstractions map to Juju concepts:

| <Cloud name> | Juju |
| - | - |
| [Cloud Resource Type^](link) | {ref}`model <model>` (roughly) |
| [Cloud Resource Type^](link) | {ref}`machine <machine>` |
| Process/container within resource | {ref}`unit <unit>` |
| Collection of resources running the same workload | {ref}`application <application>` |
| [Cloud Storage Type^](link) | {ref}`storage <storage>` |
| [Cloud Network Type^](link) | Network space (roughly) |

(<cloud>-resources)=
### Resources

\```{versionadded} X.X
Resource details reflect behavior in Juju X.X+
\```

When Juju operations are performed on <Cloud name> -- such as bootstrapping a controller or deploying an application -- Juju creates specific <Cloud name> resources to support those operations. Understanding what Juju creates in <Cloud name> is helpful for cost estimation, security planning, and integration with existing <Cloud name> infrastructure.

#### Bootstrap resources

Bootstrap creates shared infrastructure that is reused by all machines in the model:

- **<Resource type>** -- <Description>. <Configuration options>.
- **<Resource type>** -- <Description>. <Configuration options>.
- ...

#### Per-machine resources

Each machine (controller or application) receives:

- **<Resource type>** -- <Description>. <Configuration options>.
- **<Resource type>** -- <Description>. <Configuration options>.
- ...

#### Resource organization

All resources are tagged with:
- `juju-model` -- Model UUID for resource grouping
- `juju-controller` -- Controller UUID for ownership tracking
- `juju-machine-name` -- Machine identifier

These tags can be used in <Cloud provider's cost management tool> for cost attribution and tracking.

\```{dropdown} Cost implications
:color: success

<Cost guidance>
\```

\```{dropdown} Security considerations
:color: warning

<Security guidance>
\```
```

**Cross-references from other sections**:
- Authentication section links here (for managed identity effects)
- Bootstrap section links here (for what gets created)
- Model configuration section links here (for resource-group and network effects)
- Constraints section links here (for allocate-public-ip, root-disk effects)
- Storage section links here (for disk types)

**Language**: Descriptive (reference style), not imperative
- ✅ "is created", "can be used", "are tagged"
- ❌ "Create...", "Use...", "Tag..."

**Example mappings by cloud**:

**Azure**:
- Resource Group → Model (roughly)
- Virtual Machine → Machine
- Process/container within a VM → Unit
- Collection of VMs running the same workload → Application
- Managed Disk → Storage
- Subnet → Network space (roughly)

**AWS EC2**:
- VPC → Model (roughly)
- EC2 Instance → Machine
- Process/container within an instance → Unit
- Collection of instances running the same workload → Application
- EBS Volume → Storage
- Subnet → Network space

**GCE**:
- GCP Project resources → Model (roughly)
- Compute Engine Instance → Machine
- Process/container within an instance → Unit
- Collection of instances running the same workload → Application
- Persistent Disk → Storage
- Subnet → Network space

**Primary cost drivers:**
- **<Resource type>** -- <Impact>. Controllable via <constraint/config>.
- **<Resource type>** -- <Impact>. Controllable via <constraint/config>.

**Example:** <Realistic cost estimate with context>.
\```

\```{dropdown} Network architecture
:color: info

<Detailed network architecture description, including:>
- Subnet/network separation rationale
- Security group/firewall rules
- IP address allocation patterns
- Security recommendations

\```

\```{dropdown} Integration with existing infrastructure
:color: info

<Examples of how to use existing cloud resources>

\```{caution}
<Any important warnings about resource lifecycle>
\```

<Code examples showing configuration options>

\```

\```{ibnote}
See more: {ref}`constraint-xxx`, {ref}`storage-provider-xxx`
\```
```

---

## SECTION: Storage

**Purpose**: Document cloud-specific storage providers and options.

**Location**: After Placement directives (section 11)

**Change from previous**: Add cross-reference to Cloud resources subsection of Azure and Juju section, add anchor.

**Template**:
```markdown
## Storage

\```{ibnote}
See first: {ref}`storage-provider`
\```

(storage-provider-<cloud>)=
### `<provider-name>`

Configuration options:
<Options>

\```{ibnote}
See more: {ref}`<cloud>-resources` (for OS disk and additional storage details)
\```
```

---

## Cross-Referencing Pattern

**Purpose**: Enable progressive discovery -- users can navigate to related information without forcing everything into linear order.

**Implementation**: Add `{ibnote}` blocks with "See more:" links after relevant content.

**Key cross-reference points**:

1. **Authentication → Bootstrap, Constraints, Cloud Resources**
   - Instance-role integration links to Bootstrap and Constraints
   - Managed identity links to what gets created in Cloud Resources

2. **Bootstrap → Model Configuration, Constraints, Cloud Resources**
   - Links to configs usable during bootstrap
   - Links to constraints usable during bootstrap
   - Links to what bootstrap creates

3. **Model Configuration → Cloud Resources**
   - resource-group-name links to what gets created in resource group
   - network links to network architecture in Cloud Resources

4. **Constraints → Cloud Resources, Storage, Authentication**
   - allocate-public-ip links to public IP behavior
   - root-disk/root-disk-source link to disk types
   - instance-role links to Authentication

5. **Storage → Cloud Resources**
   - Links to detailed disk and volume information

**Example patterns**:
```markdown
\```{ibnote}
See more: {ref}`<cloud>-resources` (for what gets created)
\```

\```{ibnote}
See more: {ref}`<cloud>-bootstrap`, {ref}`constraint-instance-role`
\```

\```{ibnote}
See more: {ref}`storage-provider-<cloud>`, {ref}`<cloud>-resources` (for disk types)
\```
```

**Benefits**:
- Non-linear navigation suits reference material
- Context-aware links show relationships between attributes
- Cloud Resources section discoverable via multiple paths
- Users can explore based on their needs

---

## What NOT to Include

Following strict Diataxis separation:

**DO NOT include**:
- ❌ Step-by-step how-to instructions (belong in how-to guides)
- ❌ Imperative language ("Run this command...", "To configure X, do Y...")
- ❌ Troubleshooting procedures (use dropdowns if needed, or link to how-to)
- ❌ Detailed cost optimization strategies (can mention key cost drivers)
- ❌ Operational workflows (those belong in generic how-to guides like manage-clouds.md)

**DO include**:
- ✅ What resources ARE
- ✅ What resources are created when
- ✅ Resource characteristics and attributes
- ✅ Configuration options that affect resource creation
- ✅ Cost implications (informational, not prescriptive)
- ✅ Security characteristics (informational)
- ✅ Integration points with existing infrastructure

---

## Documentation Rules Compliance

### From AGENTS.documentation.rules.md:

1. **Descriptive language**: "is created", "can be configured", not "create", "configure"
2. **Dropdown patterns** for supplementary content:
   - `:color: success` for tips (e.g., cost implications)
   - `:color: info` for contextual details (e.g., network architecture)
   - `:color: warning` for safety/caution information
3. **Em-dash**: Use ` -- ` (space-hyphen-hyphen-space) in prose
4. **Version markers**: Use `{versionadded}` for version-specific features
5. **Cross-references**: Use `{ibnote}` blocks with "See more:" for tool references
6. **External links**: Use `^` suffix for upstream links: `[Azure Portal^](url)`
7. **Colon in definitions**: `**Term**: Description.` with period at end

---

## Assessment: Need to Know vs. Nice to Know

From Juju paradigm perspective, users primarily work through Juju abstractions and shouldn't need deep cloud-specific knowledge. Categorization:

### Need to Know (Core Juju User Knowledge)
- Resource organization (what contains what)
- That bootstrap creates shared networking infrastructure
- Storage types available and how to specify them via constraints
- Public IP cost implications and how to control them
- How to integrate with existing infrastructure
- Basic resource tagging for cost tracking
- What resources are created automatically vs. optionally

### Nice to Know (Advanced/Troubleshooting)
- Exact subnet IP ranges
- Security group rule priorities
- Availability set details
- Specific cloud resource names
- Detailed cloud console views
- Cloud-specific resource SKU naming

### Belongs Elsewhere (Not in Reference)
- Step-by-step troubleshooting → Discourse/how-to guides
- Detailed cost optimization strategies → Blog posts/Discourse
- Operational workflows → Generic how-to guides

---

## Pattern Rationale

### Why Concept Mapping?
- Users asked: "How does Juju model clouds?"
- Kubernetes doc already has this pattern
- Makes abstraction layer explicit
- Helps users understand what's happening "under the hood"
- Establishes relationship between Juju terms and cloud-native terms

### Why Provisioned Resources?
- Users need to understand what they're paying for
- Security teams need to know what resources exist
- DevOps needs to integrate with existing infrastructure
- Cost estimation requires resource knowledge
- Troubleshooting requires understanding resource lifecycle

### Why Dropdowns?
- Progressive disclosure: basic info visible, details hidden
- Keeps main content focused on "what IS"
- Follows documentation rules for supplementary content
- Allows including helpful context without overwhelming

### Why Descriptive Language?
- Reference docs describe attributes, not operations
- Operations belong in how-to guides
- "What it IS" vs. "What you DO with it"
- Maintains Diataxis framework separation

---

## Implementation Notes

### For Each New Cloud:

1. **Research phase**:
   - **Study provider code**: Examine `/home/dora/git/juju/internal/provider/<cloud>/` to understand:
     - What resources Juju provisions during bootstrap vs machine creation
     - How networking is set up (spaces, subnets, IP allocation)
     - How storage is allocated and configured
     - What cloud-specific tags or metadata Juju uses
     - Key differences between bootstrap and regular machine provisioning
   - Understand what resources the cloud provides vs what Juju provisions
   - Identify cloud-native equivalents to Juju concepts
   - Determine cost drivers (for public clouds)
   - Note security implications
   - Find integration points

2. **Concept mapping**:
   - Fill in the mapping table
   - Link to upstream cloud documentation
   - Use "(roughly)" for inexact mappings

3. **Bootstrap resources**:
   - List shared infrastructure created once
   - Note configuration options (model configs, constraints)
   - Specify default values and names

4. **Per-machine resources**:
   - List resources created for each machine
   - Note configuration options
   - Specify defaults

5. **Dropdowns**:
   - Cost implications: real numbers, practical examples
   - Network architecture: detailed technical info
   - Integration: code examples with existing infrastructure

6. **Cross-references**:
   - Link to relevant constraints
   - Link to storage providers
   - Link to related reference docs

### Quality Checklist:

- [ ] Concept mapping table complete with upstream links
- [ ] Bootstrap resources listed with defaults
- [ ] Per-machine resources listed with configuration options
- [ ] Resource tagging documented
- [ ] Cost implications dropdown with realistic estimates
- [ ] Network architecture dropdown with technical details
- [ ] Integration dropdown with examples
- [ ] All language is descriptive, not imperative
- [ ] Em-dashes use ` -- ` format
- [ ] Version markers added where appropriate
- [ ] Cross-references use `{ibnote}` blocks
- [ ] External links use `^` suffix
- [ ] Tested that dropdowns render correctly

---

## Examples to Reference

- **Good example**: `/home/dora/git/juju/docs/reference/cloud/list-of-supported-clouds/the-microsoft-azure-cloud-and-juju.md` (after this update)
- **Concept mapping pattern**: `/home/dora/git/juju/docs/reference/cloud/list-of-supported-clouds/kubernetes-clouds/index.md`
- **Reference language**: Any reference doc following Diataxis

---

## Future Improvements

Potential enhancements to consider:

1. **Automated resource inventory**: Tool to query cloud and generate resource list
2. **Cost calculator integration**: Link to cloud provider cost calculators with pre-filled values
3. **Diagram generation**: Visual representation of network architecture
4. **Validation tooling**: Ensure all clouds have consistent sections
5. **Resource lifecycle diagrams**: Show what happens during bootstrap vs. deploy vs. destroy

---

## Related Documentation

- `agents/AGENTS.documentation.rules.md` -- Overall documentation framework and rules
- `agents/AGENTS.documentation-landing-pages.md` -- Landing page patterns
- `docs/reference/cloud/list-of-supported-clouds/kubernetes-clouds/index.md` -- Concept mapping example
- `docs/howto/manage-clouds.md` -- Generic cloud operations (cloud-agnostic)

---

## Lessons Learned from Azure Restructuring

### What Worked

1. **Noun-based section titles** ("Authentication", "Bootstrap", "Cloud definition") instead of operational framing ("Notes on `juju add-credential`")
   - Clearer scope
   - Diataxis-compliant
   - Better for reference material

2. **Bootstrap as dedicated section** between Authentication and Model configuration
   - Reflects distinct operational phase
   - Natural home for instance-role integration context
   - Links forward to configs/constraints/resources that bootstrap uses

3. **Cross-references for progressive discovery**
   - Non-linear navigation suits reference docs
   - Solves tension between reference purity and operational context
   - Enables "need to know" vs "nice to know" balance without sharp divisions
   - Users can explore based on their questions

4. **Unified "\<Cloud name\> and Juju" section** with bidirectional mapping
   - Combines Concepts (Cloud→Juju) and Resources (Juju→Cloud) into one section
   - Emphasizes that both are about the same fundamental relationship
   - Table direction Cloud→Juju reflects that users are bringing cloud INTO Juju's abstraction layer
   - Explicit section intros explain each direction's purpose
   - Early placement (right after intro) gives users complete mental model before operations

5. **Authentication workflows inline** (not appendices)
   - Reduces mental distance between auth type and workflow
   - Keeps authentication section self-contained
   - Appendices feel disconnected; inline subsections feel integrated

### What to Avoid

1. **Gerund titles** ("Working with", "Understanding")
   - Invites Diataxis categorization debates
   - Ambiguous scope
   - Conflicts with reference doc purity

2. **Operational framing** ("Notes on `juju command`")
   - Implies procedural content
   - Conflicts with attribute-based reference style
   - Command mentions belong in content, not titles

3. **Forced linear order** when attributes have complex interdependencies
   - Bootstrap uses configs/constraints, but configs/constraints are attributes too
   - Cross-references enable "see also" pattern without breaking flow

4. **"Provisioned resources" title**
   - Ambiguous: provisioned by whom? for what?
   - "Cloud resources" or "\<Cloud name\> resources" clearer

5. **Separate Concepts and Resources sections**
   - Originally placed concepts early and resources at end
   - User insight: both are about the same thing (the Cloud-Juju mapping) viewed from different directions
   - Unified section emphasizes this relationship and provides complete mental model upfront

---

## Changelog

| Date | Change |
|------|--------|
| 2026-06-04 | Initial pattern documented based on Azure analysis |
| 2026-06-04 | Moved concept mapping to right after intro (matches Kubernetes doc pattern); moved provisioned resources to end (keeps operational sections together) |
| 2026-06-04 | Major restructuring after Azure implementation: Added Bootstrap section, renamed sections with noun-based titles, added cross-referencing pattern, renamed "Provisioned resources" to "Cloud resources", moved auth workflows inline, documented lessons learned |
| 2026-06-04 | Merged Concepts and Resources into unified "\<Cloud name\> and Juju" section with bidirectional mapping (Cloud→Juju concepts, Juju→Cloud resources); updated title to just cloud name; documented rationale for bidirectional approach |
| 2026-06-04 | Entity-based restructuring: Adopted Cloud/Credential/Controller/Model/Machine/Storage structure with "Other" subsections for cloud-specific features; removed former Concepts/Resources unified section in favor of integrating Concepts under Cloud > Other; documented anchor naming pattern |
| 2026-06-04 | Simplified cross-referencing: Link to constraint/placement directive sections rather than individual rows, keeping users in cloud-specific context |
| 2026-06-04 | Added Kubernetes cloud template: Adapted entity-based structure with Application section instead of Machine section for K8s-specific deployment patterns |

---

## Standard Structure for Kubernetes Cloud Reference Docs

The Kubernetes cloud template follows the same entity-based approach as machine clouds, but with key adaptations for Kubernetes' fundamentally different architecture.

### Template Structure

```markdown
# Kubernetes clouds and Juju

<Short intro describing this as a {ref}`kubernetes-cloud`>

## Cloud
  ### Definition
  ### Requirements
  ### Juju-to-Kubernetes concept mapping

## Credential
  ### Supported authentication types
    #### <auth-type-1>
    #### <auth-type-2>

## Controller
  ### Bootstrap behavior
  ### Resources created at bootstrap
  ### Service type by cloud (if relevant)

## Model
  ### Model configuration keys

## Application
  ### Supported constraints
  ### Placement directives
  ### Resources created per application
  ### Pod deployment patterns

## Cloud-specific storage providers
  ### <provider-name>
```

### Key Differences from Machine Cloud Template

1. **Section 1: Cloud**
   - **Definition**: Similar to machine clouds
   - **Requirements**: K8s-specific (running cluster, kubectl, RBAC)
   - **Juju-to-Kubernetes concept mapping**: Instead of "Other", this is a standard subsection showing model→namespace, unit→pod, etc.

2. **Section 2: Credential**
   - Similar structure but K8s auth types (certificate, clientcertificate, oauth2, oauth2withcert, userpass)
   - Note about `juju add-k8s` adding both cloud and credentials

3. **Section 3: Controller**
   - **Bootstrap behavior**: Describes namespace creation and StatefulSet deployment
   - **Resources created**: K8s-specific (Namespace, Service, ServiceAccount, ClusterRoleBinding, StatefulSet, Secrets, ConfigMaps, PVC, Proxy resources)
   - **Service type by cloud**: Documents LoadBalancer vs ClusterIP patterns

4. **Section 5: Application (not Machine)**
   - This is the critical adaptation
   - **Supported constraints**: cpu-power, mem, tags (much more limited than machine clouds)
   - **Placement directives**: Typically "not supported" (Kubernetes handles scheduling)
   - **Resources created per application**: Deployment/StatefulSet/DaemonSet, Pod, Service, ConfigMap, Secret, PVC
   - **Pod deployment patterns**: Init container, operator container, workload containers

5. **Section 6: Storage**
   - kubernetes storage provider using StorageClasses and PVCs
   - Different configuration options (storage-class, storage-provisioner, parameters)

### Anchor Pattern for Kubernetes Docs

Follow the same `<cloud>-section-subsection` pattern:
- `(kubernetes-cloud-definition)=`
- `(kubernetes-credential-certificate)=`
- `(kubernetes-controller-bootstrap-behavior)=`
- `(kubernetes-model-config-operator-storage)=`
- `(kubernetes-application-supported-constraints)=`
- `(storage-provider-kubernetes)=`

### Kubernetes-Specific Templates

#### SECTION: Cloud

```markdown
(kubernetes-cloud)=
## Cloud

(kubernetes-definition)=
### Definition

A Kubernetes cloud in Juju represents an existing Kubernetes cluster. Juju connects to the cluster via the Kubernetes API and manages application deployments within namespaces.

(kubernetes-requirements)=
### Requirements

- A running Kubernetes cluster (any conformant distribution: EKS, GKE, AKS, MicroK8s, Canonical Kubernetes, etc.)
- kubectl configured with cluster access
- Sufficient RBAC permissions to create namespaces, deployments, services, and other resources

(kubernetes-concept-mapping)=
### Juju-to-Kubernetes concept mapping

If you are familiar with Kubernetes, the following maps Juju concepts to their Kubernetes equivalents:

| Juju | Kubernetes |
| - | - |
| {ref}`model <model>` | [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) |
| {ref}`machine <machine>` (not managed by Juju) | [node](https://kubernetes.io/docs/concepts/architecture/nodes/) |
| {ref}`unit <unit>` | [pod](https://kubernetes.io/docs/concepts/workloads/pods/) |
| process in a unit | container |
| {ref}`application <application>` | [service](https://kubernetes.io/docs/concepts/services-networking/service/) |
```

#### SECTION: Credential

```markdown
(kubernetes-credential)=
## Credential

On Kubernetes clouds, both the cloud definition and the credentials are added through `juju add-k8s`, which reads from your kubeconfig file.

(kubernetes-supported-authentication-types)=
### Supported authentication types

(kubernetes-auth-certificate)=
- **`certificate`**: Kubernetes service account token with certificate.
  - ClientCertificateData: The kubernetes certificate data (required).
  - Token: The kubernetes service account bearer token (required).
  - rbac-id: The unique ID key name of the rbac resources (optional).

<...other auth types...>
```

#### SECTION: Controller

```markdown
(kubernetes-controller)=
## Controller

(kubernetes-bootstrap-behavior)=
### Bootstrap behavior

When bootstrapping a controller on a Kubernetes cloud, Juju creates a namespace for the controller and deploys the controller as a StatefulSet with associated resources. The controller manages the Juju state database (MongoDB) and API server within Kubernetes pods.

(kubernetes-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Namespace**: A dedicated namespace for the controller (named `controller-<controller-name>`).
- **Service**: A Kubernetes Service to expose the controller API (type depends on the cloud: LoadBalancer for public clouds, ClusterIP for localhost clouds).
- **ServiceAccount**: A service account for the controller with cluster-admin privileges.
- **ClusterRoleBinding**: Binds the controller service account to the cluster-admin ClusterRole.
- **StatefulSet**: A StatefulSet with the controller pod containing two containers: `mongodb` (Juju's state database) and `api-server` (Juju API server).
- **Secrets**: Multiple secrets for TLS certificates (`server.pem`), shared secrets, and optionally docker registry credentials for private image registries.
- **ConfigMaps**: Configuration maps for bootstrap parameters and agent configuration.
- **PersistentVolumeClaim**: Storage for the controller's operator-storage (MongoDB data and API server state).
- **Proxy resources** (if using ClusterIP service): Additional ConfigMap, Role, RoleBinding, and ServiceAccount for cluster IP proxy access.

(kubernetes-bootstrap-service-type)=
### Service type by cloud

The controller Service type varies by cloud:

- **Public clouds** (EKS, GKE, AKS, OpenStack): `LoadBalancer` -- creates a cloud load balancer with a public IP.
- **Localhost clouds** (MicroK8s, LXD): `ClusterIP` -- uses internal cluster networking with optional proxy.
- **MAAS**: `LoadBalancer` (experimental).
- **Other**: `ClusterIP` (default).
```

#### SECTION: Model

```markdown
(kubernetes-model)=
## Model

(kubernetes-model-configuration-keys)=
### Model configuration keys

(kubernetes-model-config-operator-storage)=
- **`operator-storage`**: The storage class used to provision operator storage. Type: string. Default: "" (uses cluster default storage class). Immutable: true. Mandatory: false.

(kubernetes-model-config-workload-storage)=
- **`workload-storage`**: The preferred storage class used to provision workload storage. Type: string. Default: "" (uses cluster default storage class). Immutable: false. Mandatory: false.
```

#### SECTION: Application

```markdown
(kubernetes-application)=
## Application

(kubernetes-supported-constraints)=
### Supported constraints

Kubernetes clouds support a limited subset of constraints compared to machine clouds:

- {ref}`constraint-cpu-power`: CPU resource request/limit for pods.
- {ref}`constraint-mem`: Memory resource request/limit for pods.
- {ref}`constraint-tags`: Used for pod affinity and anti-affinity rules.

\```{ibnote}
Constraints like `arch`, `cores`, `instance-type`, `root-disk`, `zones`, and others are not supported on Kubernetes clouds. Kubernetes manages node resources and pod scheduling.
\```

(kubernetes-placement-directives)=
### Placement directives

Placement directives are not supported on Kubernetes clouds. Pod placement is controlled by Kubernetes scheduling, node selectors, and affinity rules (configured via constraints).

(kubernetes-resources-per-application)=
### Resources created per application

When deploying an application to a Kubernetes model, Juju creates:

- **Deployment, StatefulSet, or DaemonSet**: Depending on the charm specification and application type. StatefulSets are used for applications requiring stable network identities and persistent storage. Deployments are used for stateless applications. DaemonSets run one pod per node.
- **Pod**: One or more pods containing the application's charm containers. Each pod typically includes an init container (`juju-init`) and a main container (`juju-operator`).
- **Service**: A Kubernetes Service to expose the application within the cluster or externally.
- **ConfigMap**: Configuration data for the application.
- **Secret**: Sensitive data like credentials.
- **PersistentVolumeClaim**: If the charm requires storage, one PVC per unit is created based on the configured storage class.

(kubernetes-pod-deployment-patterns)=
### Pod deployment patterns

Kubernetes application pods in Juju follow these patterns:

- **Init container** (`juju-init`): Prepares the pod environment before the main container starts.
- **Operator container** (`juju-operator`): Runs the charm logic and manages the application lifecycle.
- **Workload containers**: Additional containers defined by the charm (e.g., database, web server).
```

#### SECTION: Storage

```markdown
(kubernetes-storage)=
## Cloud-specific storage providers

\```{ibnote}
See first: {ref}`storage-provider`
\```

Kubernetes-based models have access to the `kubernetes` storage provider.

(storage-provider-kubernetes)=
### `kubernetes`

\```{ibnote}
See also: [Persistent storage and Kubernetes](https://discourse.charmhub.io/t/topic/1078)
\```

The `kubernetes` storage provider provisions storage using Kubernetes PersistentVolumeClaims (PVCs). The underlying storage is provided by the cluster's configured storage classes.

Configuration options:

- **`storage-class`**: The storage class for the Kubernetes cluster to use. It can be any storage class defined in your cluster, for example: `juju-unit-storage`, `juju-charm-storage`, `microk8s-hostpath`, `gp2`, `standard`, etc.

- **`storage-provisioner`**: The Kubernetes storage provisioner. For example: `kubernetes.io/no-provisioner`, `kubernetes.io/aws-ebs`, `kubernetes.io/gce-pd`, `microk8s.io/hostpath`, etc.

- **`parameters.type`**: Extra parameters passed to the storage provisioner. For example: `gp2`, `pd-standard`, etc.
```

### Implementation Notes for Kubernetes-Specific Cloud Docs

Kubernetes-specific cloud docs (EKS, GKE, AKS, MicroK8s, Canonical Kubernetes) should be **minimal stubs** that:

1. **Identify cloud type**: State "X is a {ref}`kubernetes-cloud`"
2. **Reference generic doc**: "For complete documentation, see {ref}`kubernetes-clouds`"
3. **Document cloud-specific notes only**:
   - EKS/GKE/AKS: `add-k8s` with 'raw' client due to snap confinement
   - MicroK8s: Required services (dns, hostpath-storage)
   - Canonical Kubernetes: Required services, containerd path, /run resizing
4. **Reference storage**: Link to {ref}`storage-provider-kubernetes`

**Example minimal stub template**:
```markdown
(cloud-kubernetes-<cloud>)=
# The <Cloud Name> cloud and Juju

This document describes details specific to using your existing <Cloud Name> cloud with Juju.

\```{ibnote}
See more: [<Cloud Name>](upstream-link)
\```

In Juju, <Cloud Name> is a {ref}`kubernetes-cloud`.

\```{ibnote}
See more: {ref}`kubernetes-clouds` (for complete Kubernetes cloud documentation)
\```

## Cloud-specific notes

### Notes on `add-k8s`

<Cloud-specific add-k8s notes if any>

### Requirements

<Cloud-specific requirements if any>

## Cloud-specific storage providers

\```{ibnote}
See first: {ref}`storage-provider`
\```

As for all Kubernetes clouds. See {ref}`storage-provider-kubernetes`.
```

**Rationale**: All Kubernetes clouds use the same single provider (`internal/provider/kubernetes/`). Cloud-specific differences are minimal configuration variations, not fundamental architectural differences.

---
