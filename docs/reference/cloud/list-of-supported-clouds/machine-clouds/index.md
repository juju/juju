---
myst:
  html_meta:
    description: "Learn how machine clouds work with Juju, including provisioning models, infrastructure resources, and deployment patterns across VMs, bare metal, and system containers."
---

(machine-clouds-and-juju)=
# Machine clouds and Juju

```{toctree}
:hidden:

Amazon EC2 <amazon-ec2>
Equinix Metal <equinix-metal>
Google GCE <google-gce>
LXD <lxd>
MAAS <maas>
Manual <manual>
Microsoft Azure <microsoft-azure>
OpenStack <openstack>
Oracle OCI <oracle-oci>
VMware vSphere <vmware-vsphere>
```

```{ibnote}
See also: {ref}`list-of-supported-clouds`
```

In Juju, a machine cloud is a {ref}`machine-cloud`. Juju provisions or adopts infrastructure resources (machines, networks, storage) and deploys {ref}`machine charms <machine-charm>` onto those resources. Unlike {ref}`Kubernetes clouds <kubernetes-cloud>`, machine clouds involve direct management of compute infrastructure.

(machine-cloud-entity)=
## Cloud

(machine-definition)=
### Definition

A machine cloud in Juju represents a substrate that can provide compute resources in the form of:

- **Virtual machines** (e.g., Amazon EC2, Google GCE, Microsoft Azure, OpenStack, Oracle OCI, VMware vSphere)
- **Bare metal machines** (e.g., MAAS)
- **System containers** (e.g., LXD)

Juju interacts with these clouds through their APIs to provision infrastructure, deploy applications, and manage the full lifecycle of workloads.

(machine-provisioning-models)=
### Provisioning models

Machine clouds vary in how Juju provisions infrastructure:

(machine-provisioning-imperative-api)=
#### Imperative API

Juju makes direct API calls to create resources (e.g., "create instance", "attach volume", "create network interface").

**Examples**: Amazon EC2, Google GCE, OpenStack, Oracle OCI

**Characteristics**:
- Juju explicitly creates each resource via API calls
- Resources created incrementally as needed
- Polling used to wait for resource readiness
- Direct control over resource creation order

(machine-provisioning-template-based)=
#### Template-based

Juju generates declarative templates that the cloud provider processes to create resources.

**Examples**: Microsoft Azure (ARM templates), VMware vSphere (VM cloning from templates)

**Characteristics**:
- Cloud provider interprets template to create resources
- Resources typically created in parallel by provider
- Abstraction layer between Juju and resource creation
- May offer better atomicity (all-or-nothing creation)

(machine-provisioning-profile-based)=
#### Profile-based

Juju applies profiles that define resource characteristics; the cloud provider creates instances matching those profiles.

**Examples**: LXD (profiles define container/VM configuration)

**Characteristics**:
- Profiles define resource constraints and configuration
- Cloud provider selects appropriate resources
- Flexible resource allocation within profile bounds
- Efficient reuse of profile definitions

(machine-provisioning-allocation)=
#### Allocation

Juju allocates pre-existing resources from an inventory rather than creating new ones.

**Examples**: MAAS (allocates machines from pool of bare metal)

**Characteristics**:
- Resources must pre-exist in the cloud
- Juju allocates from available pool
- No actual provisioning, just assignment
- Requires capacity planning upfront

(machine-provisioning-adoption)=
#### Adoption

Juju adopts existing machines via SSH without provisioning any infrastructure.

**Examples**: Manual cloud (SSH-based adoption)

**Characteristics**:
- No infrastructure provisioning
- Machines must be prepared externally
- SSH-based agent installation
- Limited control over infrastructure

(machine-requirements)=
### Requirements

Common requirements across machine clouds:

- **API access**: Credentials with sufficient permissions to provision resources
- **Network connectivity**: Juju controller must reach provisioned machines
- **SSH access**: For agent installation and management
- **Cloud quotas**: Sufficient quota for compute, network, storage resources

Specific requirements vary by cloud -- see individual cloud documentation.

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>`
```

(machine-credential)=
## Credential

(machine-credential-patterns)=
### Common authentication patterns

Machine clouds support various authentication methods:

(machine-credential-api-key)=
#### API key-based

Uses static API keys or access keys with secrets.

**Examples**: Amazon EC2 (`access-key`), OpenStack (`userpass`)

**Characteristics**:
- Static credentials stored by Juju
- Requires careful secret management
- May support key rotation policies

(machine-credential-oauth)=
#### OAuth/service principal

Uses OAuth2 flows or service principal identities.

**Examples**: Microsoft Azure (`service-principal-secret`), Google GCE (`oauth2`, `jsonfile`)

**Characteristics**:
- Time-limited tokens with refresh capability
- May support interactive flows
- Often recommended for production use

(machine-credential-instance-identity)=
#### Instance identity

Uses cloud-native instance identity mechanisms (no static credentials).

**Examples**: Amazon EC2 (`instance-role`), Microsoft Azure (`managed-identity`)

**Characteristics**:
- No credentials stored by Juju
- Cloud provider validates instance identity
- Most secure option when available
- Requires controller running on same cloud

(machine-credential-certificate)=
#### Certificate-based

Uses PKI certificates for authentication.

**Examples**: LXD (`certificate`), VMware vSphere (`userpass` with certificate validation)

**Characteristics**:
- Certificate-based trust model
- May require certificate management
- Often used for private clouds

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>` > Credential
```

(machine-controller)=
## Controller

(machine-bootstrap-behavior)=
### Bootstrap behavior

When bootstrapping a controller on a machine cloud, Juju:

1. Provisions a machine (or allocates/adopts one)
2. Installs the Juju agent and MongoDB
3. Creates initial model infrastructure
4. Establishes API server endpoint

The bootstrap process creates shared infrastructure (networks, security rules, etc.) that subsequent models may reuse.

(machine-bootstrap-patterns)=
### Common bootstrap patterns

(machine-bootstrap-network-setup)=
#### Network setup

Most machine clouds require Juju to create or configure networking:

- **Virtual networks/VPCs**: Isolated network spaces for the controller and models
- **Subnets**: Separate subnets for different machine types or purposes
- **Security groups/firewalls**: Ingress/egress rules for API access and inter-machine communication
- **Public IPs**: Optional public IP allocation for controller accessibility

(machine-bootstrap-storage-setup)=
#### Storage setup

Controllers require persistent storage for the MongoDB state database:

- **Boot/root disk**: Operating system and Juju agent installation
- **Data disk**: Optional separate disk for MongoDB data (higher-end deployments)
- **Storage provisioning**: Via cloud-specific storage systems (EBS, Persistent Disk, Managed Disk, Cinder, etc.)

(machine-bootstrap-instance-creation)=
#### Instance creation

The controller machine itself is provisioned with:

- **Instance type/size**: Constrained by `bootstrap-constraints` or defaults
- **Operating system**: Ubuntu (Juju's supported OS)
- **Cloud-init**: Initial configuration and agent installation
- **Metadata/tags**: Identification and organization (model UUID, controller UUID, etc.)

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>` > Controller
```

(machine-model)=
## Model

(machine-model-configuration)=
### Common model configuration patterns

Machine clouds support various model-level configurations that affect resource provisioning:

(machine-model-config-network)=
#### Network configuration

- **VPC/network selection**: Which virtual network to use for machines
- **Subnet selection**: Which subnets to place machines in
- **DNS settings**: Custom DNS servers or search domains
- **Proxy settings**: HTTP/HTTPS proxy configuration for agent communication

(machine-model-config-resource)=
#### Resource configuration

- **Default constraints**: Constraints applied to all machines in the model
- **Storage defaults**: Default storage pool or size
- **Image streams**: Which OS image stream to use (released vs. daily)

(machine-model-config-behavior)=
#### Behavioral configuration

- **Automatic retries**: Whether to retry failed operations
- **Update behavior**: How to handle charm and agent updates
- **Container networking**: Configuration for nested containers (LXD)

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>` > Model
```

(machine-machine)=
## Machine

(machine-supported-constraints)=
### Common constraints

Machine clouds support a rich set of constraints for machine provisioning:

(machine-constraint-compute)=
#### Compute constraints

- {ref}`constraint-arch`: CPU architecture (amd64, arm64, etc.)
- {ref}`constraint-cores`: Number of CPU cores
- {ref}`constraint-cpu-power`: CPU performance benchmark score
- {ref}`constraint-mem`: Memory size in MB/GB
- {ref}`constraint-instance-type`: Cloud-specific instance type/size

```{ibnote}
The constraints `instance-type` and `[arch, cores, cpu-power, mem]` are typically mutually exclusive -- use one or the other.
```

(machine-constraint-storage)=
#### Storage constraints

- {ref}`constraint-root-disk`: Root disk size in MB/GB
- {ref}`constraint-root-disk-source`: Storage backend or volume type

(machine-constraint-network)=
#### Network constraints

- {ref}`constraint-allocate-public-ip`: Whether to allocate a public IP address
- {ref}`constraint-spaces`: Network spaces for network isolation
- {ref}`constraint-zones`: Availability zones for high availability

(machine-constraint-cloud-specific)=
#### Cloud-specific constraints

- {ref}`constraint-instance-role`: IAM instance role (AWS, Azure)
- {ref}`constraint-virt-type`: Virtualization type (e.g., HVM vs. PV on AWS)

```{ibnote}
Not all constraints are supported on every cloud. See individual cloud documentation for supported constraints.
```

(machine-supported-placement)=
### Common placement directives

Placement directives control where machines are created:

- {ref}`placement-directive-zone`: Specific availability zone
- {ref}`placement-directive-machine`: Co-locate with existing machine (subordinate charms)
- {ref}`placement-directive-lxd`: Place in LXD container on machine (nested containers)
- {ref}`placement-directive-kvm`: Place in KVM container on machine (nested virtualization)

```{ibnote}
Placement directive support varies by cloud. See individual cloud documentation.
```

(machine-resources-per-machine)=
### Resources created per machine

When Juju provisions a machine, typical resources include:

(machine-resource-compute)=
#### Compute resources

- **Instance/VM/container**: The primary compute resource
- **Network interfaces**: NICs attached to appropriate subnets
- **IP addresses**: Private IPs (always), public IPs (if configured)
- **Security groups/firewall rules**: Access control for the machine

(machine-resource-storage)=
#### Storage resources

- **Root disk**: Boot volume containing the operating system
- **Additional disks**: Optional data volumes for charms requiring storage
- **Volume attachments**: Associations between volumes and instances

(machine-resource-metadata)=
#### Metadata and organization

All resources are typically tagged with:

- **Model UUID**: Associates resources with their model
- **Controller UUID**: Identifies the managing controller
- **Machine identifier**: Unique machine name or ID within the model
- **Additional tags**: Cloud-specific organizational metadata

```{ibnote}
Specific resources created vary by cloud. See individual cloud documentation for details.
```

(machine-networking-behavior)=
### Common networking patterns

(machine-networking-ip-allocation)=
#### IP address allocation

- **Private IPs**: Allocated automatically from subnet ranges
- **Public IPs**: Allocated on demand (via constraints or configuration)
- **Elastic IPs**: Some clouds support persistent public IPs that survive machine recreation

(machine-networking-security)=
#### Security and access control

- **Ingress rules**: Allow traffic to machines (typically SSH, application ports)
- **Egress rules**: Control outbound traffic (typically permissive)
- **Inter-machine communication**: Rules allowing machines in same model to communicate
- **Controller access**: Rules allowing machines to reach controller API

(machine-networking-dns)=
#### DNS and service discovery

- **Private DNS**: Internal name resolution within the cloud
- **Public DNS**: Optional DNS records for publicly accessible machines
- **Service discovery**: Juju-managed address resolution between units

(machine-storage)=
## Cloud-specific storage providers

```{ibnote}
See first: {ref}`storage-provider`
```

Machine clouds provide storage through cloud-native storage systems:

(machine-storage-block)=
### Block storage

Block storage provides volumes that can be attached to machines:

- **Amazon EC2**: `ebs` (Elastic Block Store) -- gp2, gp3, io1, io2, st1, sc1
- **Google GCE**: `gce` -- pd-standard, pd-ssd, pd-balanced
- **Microsoft Azure**: `azure` -- Standard_LRS, Premium_LRS, StandardSSD_LRS
- **OpenStack**: `cinder` -- depends on Cinder configuration
- **Oracle OCI**: `oracle` -- iSCSI block volumes
- **LXD**: `lxd` -- zfs, btrfs, lvm, ceph, dir

Block storage volumes:
- Can be attached and detached from machines
- Persist independently of machine lifecycle
- Support snapshotting and backup
- Performance varies by storage tier

(machine-storage-filesystem)=
### Filesystem storage

Some clouds support shared filesystem storage:

- **MAAS**: `maas` (static filesystems only -- no dynamic provisioning)
- **Manual**: No storage provider (rely on existing filesystems)

(machine-storage-configuration)=
### Storage configuration patterns

When requesting storage, charms specify:

- **Pool**: Storage backend/type (maps to cloud-specific storage class)
- **Size**: Volume size in MB/GB
- **Count**: Number of volumes (for RAID configurations, etc.)

Storage is typically created on-demand when deploying charms that require it, with volumes attached to machines and mounted at charm-specified paths.

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>` > Cloud-specific storage providers
```

(machine-differences)=
## Differences between machine clouds

While all machine clouds follow the entity-based pattern documented here, significant differences exist:

(machine-differences-provisioning)=
### Provisioning model differences

- **Creation vs. allocation**: Some clouds create new resources (EC2, Azure), others allocate existing ones (MAAS, Manual)
- **Template vs. imperative**: Some use declarative templates (Azure ARM), others use imperative APIs (EC2, GCE)
- **Synchronous vs. asynchronous**: Resource creation may be immediate (LXD) or require polling for readiness (EC2, OCI)

(machine-differences-networking)=
### Networking differences

- **VPC/network management**: Some require explicit VPC creation (EC2, Azure), others use shared networks (OpenStack)
- **Public IP assignment**: Methods vary (elastic IPs, floating IPs, load balancers, direct assignment)
- **Security model**: Security groups (EC2, OpenStack), network security groups (Azure), firewall rules (GCE)

(machine-differences-storage)=
### Storage differences

- **Provisioning support**: Some clouds support dynamic provisioning (EC2 EBS, GCE PD), others require pre-provisioning (MAAS)
- **Storage types**: Different performance tiers, replication options, and pricing models
- **Attachment model**: How volumes attach to machines varies by cloud

(machine-differences-capabilities)=
### Capability differences

- **Availability zones**: Not all clouds support zones
- **Instance roles**: Only some clouds support instance identity-based authentication
- **Spot/preemptible instances**: Cost optimization features vary by cloud
- **Nested virtualization**: Support for containers or VMs within machines varies

```{ibnote}
For cloud-specific details on these differences, see: {ref}`list-of-supported-clouds` > `<cloud name>`
```

(machine-best-practices)=
## Best practices for machine clouds

(machine-best-practices-security)=
### Security

- **Use instance identity authentication** when available (reduces credential exposure)
- **Minimize public IP allocation** (use private networks with NAT/bastion when possible)
- **Apply principle of least privilege** to API credentials
- **Use network spaces** to isolate sensitive workloads

(machine-best-practices-cost)=
### Cost optimization

- **Right-size machines** using constraints (avoid over-provisioning)
- **Use appropriate storage tiers** (standard for non-critical data, premium for databases)
- **Leverage spot/preemptible instances** for fault-tolerant workloads (when available)
- **Monitor and tag resources** for cost attribution and analysis

(machine-best-practices-reliability)=
### Reliability

- **Use availability zones** for high availability when available
- **Enable controller HA** (`enable-ha`) for production deployments
- **Size root disks appropriately** to avoid disk exhaustion
- **Use persistent storage** for stateful workloads

(machine-best-practices-operations)=
### Operations

- **Test in development clouds** (LXD localhost) before deploying to production
- **Use consistent constraints** across models for predictable sizing
- **Document cloud-specific configurations** for reproducibility
- **Monitor quota limits** to avoid provisioning failures

```{ibnote}
See more: {ref}`manage-clouds`, {ref}`manage-models`, {ref}`manage-machines`
```
