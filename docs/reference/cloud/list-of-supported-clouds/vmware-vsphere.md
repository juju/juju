---
myst:
  html_meta:
    description: "Configure VMware vSphere cloud with Juju, including ESXi requirements, vSAN support, and authentication for vSphere deployments."
---

(cloud-vsphere)=
# VMware vSphere

In Juju, [VMware vSphere](https://www.vmware.com/products/vsphere.html) is a {ref}`machine cloud <machine-cloud>` and works as described below.

```{note}
This reference assumes basic familiarity with Juju. If you are new to Juju, start with the {ref}`Tutorial <tutorial>`, then use this page together with the generic materials it links to.
```

(vsphere-requirements)=
## Requirements

In order to add a vSphere cloud you will need an existing vSphere installation which supports, or has access to, the following:

- VMware Hardware Version 8 or greater.
- ESXi 5.0 or greater.
- Internet access.
- DNS and DHCP.

Juju supports both high-availability vSAN deployments and standard deployments.

(vsphere-concepts)=
## Concepts

The following table shows how vSphere abstractions map to Juju concepts:

| vSphere | Juju |
| - | - |
| Virtual machine | {ref}`machine <machine>` |
| Process inside a VM | {ref}`unit <unit>` |
| Group of units for one workload | {ref}`application <application>` |
| Datastore disk | {ref}`storage <storage>` |
| Port group / network | Network spaces and placement targets (roughly) |
| Cluster/resource pool | Placement target (`zones`) |

(vsphere-cloud)=
## The cloud

```{ibnote}
See also: {ref}`cloud`, {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

As for all machine clouds, the cloud is registered in Juju via a cloud definition, stored in `clouds.yaml` on the client (on Linux: `~/.local/share/juju/clouds.yaml`) and following this schema:

```yaml
clouds:
  <cloud-name>:  # User-defined name
    type: vsphere
    auth-types:
      - <auth-type>                # See Authentication types below
    endpoint: <vsphere-vcenter-url>  # vCenter API endpoint
    config:                        # Optional: model config defaults
      <config-key>: <value>        # See Configuration keys below
```


(vsphere-credential)=
## Credentials

```{ibnote}
See also: {ref}`credential`, {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

As for all machine clouds, credentials are stored in `credentials.yaml` on the client and follow this schema:

```yaml
credentials:
  <your-vsphere-cloud>        # Cloud name as defined above
    <credential-name>:             # User-defined credential name
      auth-type: <auth-type>       # userpass (the only type)
      <attribute>: <value>         # Auth-type-specific attributes (see below)
```


(vsphere-credential-authentication-types)=
### Authentication types

VMware vSphere supports the following authentication types:

(vsphere-credential-userpass)=
#### `userpass`

Attributes:

- `user`: The username to authenticate with (required).
- `password`: The password to authenticate with (required).
- `vmfolder`: The folder to add VMs from the model (optional).

(vsphere-controller)=
## Controllers

```{ibnote}
See also: {ref}`controller`, {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(vsphere-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller VM on vSphere by cloning from a template and waiting for provisioning tasks to complete.

Bootstrap downloads a cloud image to the client, uploads it to the ESX host, and creates a template. This process can be slow depending on network connection. Using pre-created templates speeds up bootstrap and machine deployment.

```{tip}
Bootstrap with cloud-specific model-configuration keys `datastore` and `primary-network` to avoid ambiguity.
```

(vsphere-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

The controller runs on a vSphere VM provisioned using the same mechanisms as workload machines — see {ref}`vsphere-machine-resources-created-per-machine` for the full per-machine resource model. Controller-specific differences are noted below.

**Compute**

- **VM folder hierarchy**: Creates folder `Juju Controller (<controller-uuid>)` with nested structure `<vm-folder>/Juju Controller (UUID)/Model "name" (UUID)/`. Folders enable cleanup by controller/model.
- **Template cache**: Creates `Juju Controller (<uuid>)/templates/<os>_<track>/` folder. Templates named `juju-template-<sha256>` with architecture tag in extra config.
- **Controller VM**: Created by cloning from a template VM. Disk extended if needed. Hardware upgraded if `force-vm-hardware-version` is specified. Powered on after provisioning.
- **Resource pool placement**: VM placed in resource pool specified by availability zone constraint. Must match compute resource hosting the datastore.

**Networking**

- **Network devices**: Primary network interface (eth0) on `primary-network` (default: "VM Network") with DHCP. Optional external network interface (eth1) if `external-network` configured.

**Storage**

- **Root disk**: VMDK from template, extended post-clone if constraint specifies larger size. Datastore selected from compute resource's accessible datastores.

(vsphere-controller-template-management)=
### Template management

Templates are created from OVA imports with image integrity verification and stored for reuse. Reusing templates speeds up subsequent bootstrap and machine creation.

```{ibnote}
See more: {ref}`vsphere-appendix-using-templates`
```

(vsphere-model)=
## Models

```{ibnote}
See also: {ref}`model`, {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

(vsphere-model-configuration-keys)=
### Configuration keys

VMware vSphere supports the following {ref}`cloud-specific model configuration keys <model-config-cloud-specific-key>`:

**Compute**

(vsphere-model-force-vm-hardware-version)=
- **`force-vm-hardware-version`**: The HW compatibility version to use when cloning a VM template to create a VM. The version must be supported by the remote compute resource, and greater than or equal to the template's version. Type: `int`. Default: `0`.

**Networking**

(vsphere-model-primary-network)=
- **`primary-network`**: The primary network that VMs will be connected to. If this is not specified, Juju will look for a network named "VM Network". Type: `string`. Default: none.

(vsphere-model-external-network)=
- **`external-network`**: An external network that VMs will be connected to. The resulting IP address for a VM will be used as its public address. Type: `string`. Default: `""`.

**Storage**

(vsphere-model-datastore)=
- **`datastore`**: The datastore in which to create VMs. If this is not specified, the process will abort unless there is only one datastore available. Type: `string`. Default: none.

(vsphere-model-disk-provisioning-type)=
- **`disk-provisioning-type`**: Specify how the disk should be provisioned when cloning the VM template. Allowed values: `thickEagerZero` (default), `thick`, `thin`. Type: `string`. Default: `"thick"`.

(vsphere-model-enable-disk-uuid)=
- **`enable-disk-uuid`**: Expose consistent disk UUIDs to the VM, equivalent to `disk.EnableUUID`. Enables consistent `/dev/disk/by-id/` paths in guest OS. Type: `bool`. Default: `true`.

(vsphere-machine)=
## Machines

```{ibnote}
See also: {ref}`machine`, {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

(vsphere-machine-constraints)=
### Constraints

VMware vSphere supports the following {ref}`constraints <constraint>`:

**Compute**

- {ref}`constraint-arch`. Valid values: `amd64`.
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-type`
- {ref}`constraint-mem`

**Networking**

- {ref}`constraint-zones`. Specifies resource pools within a host or cluster. Examples: `zones=myhost`, `zones=myfolder/myhost`, `zones=mycluster/mypool`, `zones=mycluster/myparent/mypool`.

**Storage**

- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`. Specifies the datastore for the root disk.

(vsphere-machine-placement-directives)=
### Placement directives

VMware vSphere supports the following {ref}`placement directives <placement-directive>`:

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`: Valid values: `<cluster|host>`.

```{caution}
If your topology has a cluster without a host, Juju will see this as an availability zone and may fail silently. To solve this, either ensure the host is within the cluster, or use a placement directive: `juju bootstrap vsphere/<datacenter> <controllername> --to zone=<cluster|host>`.
```

(vsphere-machine-resources-created-per-machine)=
### Resources created per machine

Applies to all machines, including controller machines. Controller-specific defaults are documented in {ref}`vsphere-controller-resources-created-at-bootstrap`.

**Compute**

- **VM**: Created by cloning from a template. Stored in the controller/model folder hierarchy.
- **Hardware resources**: Memory, CPU cores, CPU power from constraints. Hardware version optionally upgraded via `force-vm-hardware-version` model config.
- **Resource pool placement**: VM placed in resource pool specified by availability zone constraint.
- **Tags & metadata**: Juju writes controller/model metadata to VM extra config to support inventory and cleanup operations.
- **Additional packages**: Cloud-init installs `open-vm-tools` and `iptables-persistent`.

**Networking**

- **Network devices**: Primary interface (eth0) on `primary-network` with DHCP, MAC generated. Optional external interface (eth1) on `external-network` with DHCP, MAC generated. Cloud-init network config added for both interfaces.

**Storage**

- **Root disk**: VMDK from template, extended post-clone if constraint specifies larger size. Provisioning type: `thin`, `thick`, or `thickEagerZero` via `disk-provisioning-type` config. Datastore selected from compute resource's accessible datastores (must be explicit if multiple available).

(vsphere-machine-networking-behavior)=
### Networking behavior

- **Network selection**: Primary network from `primary-network` model config (default: "VM Network"). Optional external network from `external-network` config. Port groups referenced by network name string.
- **IP assignment**: DHCP from guest OS. No static IP support in provider. Cloud-init configures interfaces with DHCP.
- **Public/private addressing**: Primary network provides private/internal addressing. External network (if configured) provides public address (used as public address by Juju).
- **Port groups/VLANs**: No explicit VLAN configuration. Relies on vSphere port group mapping.

(vsphere-machine-storage-behavior)=
### Storage behavior

- **VMDK only**: All storage operations use VMDK provisioning from templates. Only root disk is supported -- no secondary volumes, snapshots, or persistent volume creation.
- **Datastore selection**: Must be specified via `datastore` model config if multiple datastores are available; otherwise bootstrap aborts.
- **Disk provisioning type**: Configurable via `disk-provisioning-type` model config (`thickEagerZero`, `thick`, or `thin`).

(vsphere-storage)=
## Storage

```{ibnote}
See also: {ref}`storage`, {ref}`Juju | Manage storage <manage-storage>`
```

VMware vSphere has no cloud-specific storage providers. All storage operations use VMDK provisioning from templates — see {ref}`vsphere-machine-storage-behavior`.

(vsphere-appendix-using-templates)=
## Appendix: Using templates

To speed up bootstrap and deploy, you can use VM templates already created in your vSphere. Templates can be created by hand on your vSphere, or created from an existing VM.

Examples assume that the templates are in directory `$DATA_STORE/templates`.

**Via simplestreams:**

```bash
mkdir -p $HOME/simplestreams
juju-metadata generate-image -d $HOME/simplestreams/ -i "templates/juju-focal-template" --base ubuntu@22.04 -r $DATA_STORE -u $CLOUD_ENDPOINT
juju-metadata generate-image -d $HOME/simplestreams/ -i "templates/juju-noble-template" --base ubuntu@24.04 -r $DATA_STORE -u $CLOUD_ENDPOINT
juju bootstrap --metadata-source $HOME/image-streams vsphere
```

**Bootstrap with specific template:**

```bash
juju bootstrap vsphere --bootstrap-image="templates/focal-test-template" --bootstrap-base ubuntu@22.04 --bootstrap-constraints "arch=amd64"
```

**Using add-image:**

```bash
juju metadata add-image templates/bionic-test-template --base ubuntu@22.04
```

```{ibnote}
See more: [Discourse | Add custom machine images with the juju metadata command](https://discourse.charmhub.io/t/new-feature-in-juju-2-8-add-custom-machine-images-with-the-juju-metadata-command/3171)
```
