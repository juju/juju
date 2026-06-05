---
myst:
  html_meta:
    description: "Configure VMware vSphere cloud with Juju, including ESXi requirements, vSAN support, and authentication for vSphere deployments."
---

(cloud-vsphere)=
# VMware vSphere

In Juju, [VMware vSphere](https://www.vmware.com/products/vsphere.html) is a {ref}`machine cloud <machine-cloud>`. It behaves like all {ref}`machine clouds <machine-clouds>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(vsphere-cloud)=
## The cloud

The VMware vSphere cloud in Juju.

(vsphere-cloud-definition)=
### Definition

Type in Juju: `vsphere`

Name in Juju: User-defined

(vsphere-cloud-requirements)=
### Requirements

In order to add a vSphere cloud you will need an existing vSphere installation which supports, or has access to, the following:

- VMware Hardware Version 8 or greater
- ESXi 5.0 or greater
- Internet access
- DNS and DHCP

Juju supports both high-availability vSAN deployments and standard deployments.

(vsphere-credential)=
## Credentials

Credentials for the VMware vSphere cloud.

(vsphere-credential-authentication-types)=
### Authentication types

(vsphere-credential-userpass)=
####  `userpass`

Attributes:

- `user`: The username to authenticate with (required).
- `password`: The password to authenticate with (required).
- `vmfolder`: The folder to add VMs from the model (optional).

(vsphere-controller)=
## Controllers

Controllers bootstrapped on the VMware vSphere cloud.

(vsphere-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates a controller VM on vSphere via template cloning. Uses imperative vSphere API calls (govmomi) with synchronous task waiting -- no declarative templates.

Bootstrap downloads a cloud image to the client, uploads it to the ESX host, and creates a template. This process can be slow depending on network connection. Using pre-created templates speeds up bootstrap and machine deployment.

```{tip}
Bootstrap with cloud-specific model-configuration keys `datastore` and `primary-network` to avoid ambiguity.
```

(vsphere-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **VM folder hierarchy**: Creates folder `Juju Controller (<controller-uuid>)` with nested structure `<vm-folder>/Juju Controller (UUID)/Model "name" (UUID)/`. Folders enable cleanup by controller/model.
- **Template cache**: Creates `Juju Controller (<uuid>)/templates/<os>_<track>/` folder. Templates named `juju-template-<sha256>` with architecture tag in extra config. Each base image (ubuntu_20.04, ubuntu_22.04, etc.) gets own folder.
- **Controller VM**: Created by cloning from template VM via `VirtualMachine.Clone()`. Disk extended if needed via `ReconfigureVirtualMachineSpec`. Hardware upgraded if `force-vm-hardware-version` specified. Powered on via `PowerOn()` task.
- **Network devices**: Primary network interface (eth0) on `primary-network` (default: "VM Network") with DHCP. Optional external network interface (eth1) if `external-network` configured.
- **Root disk**: VMDK from template, extended post-clone if constraint specifies larger size. Datastore selected from compute resource's accessible datastores.
- **Resource pool placement**: VM placed in resource pool specified by availability zone constraint. Must match compute resource hosting the datastore.

(vsphere-controller-other)=
### Other

(vsphere-controller-template-management)=
#### Template management

Templates are created via OVA import with SHA-256 verification during VMDK streaming. Marked as `IsTemplate()` in vSphere. Locked download via mutex prevents concurrent imports of same base.

```{ibnote}
See more: {ref}`vsphere-appendix-using-templates`
```

(vsphere-model)=
## Models

Models connected to the VMware vSphere cloud.

(vsphere-model-cloud-specific-configuration-keys)=
(vsphere-model-configuration-keys)=
### Configuration keys

(vsphere-model-datastore)=
#### `datastore`

The datastore in which to create VMs. If this is not specified, the process will abort unless there is only one datastore available.

- **Type**: `string`
- **Default value**: (omitted)
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-model-primary-network)=
#### `primary-network`

The primary network that VMs will be connected to. If this is not specified, Juju will look for a network named "VM Network".

- **Type**: `string`
- **Default value**: (omitted)
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-model-external-network)=
#### `external-network`

An external network that VMs will be connected to. The resulting IP address for a VM will be used as its public address.

- **Type**: `string`
- **Default value**: `""`
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-model-force-vm-hardware-version)=
#### `force-vm-hardware-version`

The HW compatibility version to use when cloning a VM template to create a VM. The version must be supported by the remote compute resource, and greater than or equal to the template's version.

- **Type**: `int`
- **Default value**: `0`
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-model-enable-disk-uuid)=
#### `enable-disk-uuid`

Expose consistent disk UUIDs to the VM, equivalent to `disk.EnableUUID`. Enables consistent `/dev/disk/by-id/` paths in guest OS.

- **Type**: `bool`
- **Default value**: `true`
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-model-disk-provisioning-type)=
#### `disk-provisioning-type`

Specify how the disk should be provisioned when cloning the VM template. Allowed values: `thickEagerZero` (default), `thick`, `thin`.

- **Type**: `string`
- **Default value**: `"thick"`
- **Immutable**: `false`
- **Mandatory**: `false`

(vsphere-machine)=
## Machines

Machines provisioned on the VMware vSphere cloud.

(vsphere-machine-supported-constraints)=
(vsphere-machine-constraints)=
### Constraints

- {ref}`constraint-arch`: Valid values: `amd64`.
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-cpu-power`
- {ref}`constraint-instance-type`
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`
- {ref}`constraint-root-disk-source`: Specifies the datastore for the root disk.
- {ref}`constraint-zones`: Specifies resource pools within a host or cluster. Examples: `zones=myhost`, `zones=myfolder/myhost`, `zones=mycluster/mypool`, `zones=mycluster/myparent/mypool`.

(vsphere-machine-supported-placement-directives)=
(vsphere-machine-placement-directives)=
### Placement directives

- {ref}`placement-directive-machine`
- {ref}`placement-directive-zone`: Valid values: `<cluster|host>`.

```{caution}
If your topology has a cluster without a host, Juju will see this as an availability zone and may fail silently. To solve this, either ensure the host is within the cluster, or use a placement directive: `juju bootstrap vsphere/<datacenter> <controllername> --to zone=<cluster|host>`.
```

(vsphere-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **VM**: Created by cloning from template via `VirtualMachine.Clone()`. VM stored in `<vm-folder>/Juju Controller (<uuid>)/Model "name" (<uuid>)/<machine-name>`. Machine name derived from `namespace.Hostname(MachineId)`.
- **Hardware resources**: Memory, CPU cores, CPU power from constraints. Hardware version optionally upgraded via `force-vm-hardware-version` model config.
- **Root disk**: VMDK from template, extended post-clone if constraint specifies larger size. Provisioning type: `thin`, `thick`, or `thickEagerZero` via `disk-provisioning-type` config. Datastore selected from compute resource's accessible datastores (must be explicit if multiple available).
- **Network devices**: Primary interface (eth0) on `primary-network` with DHCP, MAC generated. Optional external interface (eth1) on `external-network` with DHCP, MAC generated. Cloud-init network config added for both interfaces.
- **Resource pool placement**: VM placed in resource pool specified by availability zone constraint.
- **Tags & metadata**: Stored as `map[string]string` in VM extra config. Example keys: `JujuController: "true"`. Architecture tag added to templates: `arch: amd64` or `arm64`.
- **Additional packages**: Cloud-init installs `open-vm-tools` and `iptables-persistent`.

(vsphere-machine-networking-behavior)=
### Networking behavior

- **Network selection**: Primary network from `primary-network` model config (default: "VM Network"). Optional external network from `external-network` config. Port groups referenced by network name string.
- **IP assignment**: DHCP from guest OS. No static IP support in provider. Cloud-init configures interfaces with DHCP.
- **Public/private addressing**: Primary network provides private/internal addressing. External network (if configured) provides public address (used as public address by Juju).
- **Port groups/VLANs**: No explicit VLAN configuration. Relies on vSphere port group mapping.

(vsphere-storage)=
(vsphere-storage)=
## Storage

Storage provisioned on the VMware vSphere cloud.

### Storage providers

No custom storage providers. All storage operations use VMDK provisioning from templates. Only root disk supported -- no secondary volumes, snapshots, or persistent volume creation.

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
