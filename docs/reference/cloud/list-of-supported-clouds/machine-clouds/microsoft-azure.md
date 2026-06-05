# Microsoft Azure

In Juju, [Microsoft Azure](https://azure.microsoft.com/en-us) is a {ref}`machine cloud <machine-cloud>`. It behaves like all {ref}`machine clouds <machine-clouds>`, except for a few points of variation related to the cloud, credentials, controllers, models, machines, and storage, described below.

(azure-cloud)=
## The cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

(azure-cloud-definition)=
### Definition

Type in Juju: `azure`

Name in Juju: `azure`

(azure-cloud-requirements)=
### Requirements

**Required Azure API permissions:**

- `Microsoft.Compute/skus` (read)
- `Microsoft.Resources/subscriptions/resourceGroups` (read, write, delete)
- `Microsoft.Resources/deployments/*` (write, read, delete, cancel, validate)
- `Microsoft.Network/networkSecurityGroups` (write, read, delete, join)
- `Microsoft.Network/virtualNetworks/*` (write, read, delete)
- `Microsoft.Compute/virtualMachineScaleSets/*` (write, read, delete, start, deallocate, restart, powerOff)
- `Microsoft.Network/virtualNetworks/subnets/*` (read, write, delete, join)
- `Microsoft.Compute/availabilitySets` (write, read, delete)
- `Microsoft.Network/publicIPAddresses` (write, read, delete, join) -- optional for public-facing services
- `Microsoft.Network/networkInterfaces` (write, read, delete, join)
- `Microsoft.Compute/virtualMachines` (write, read, delete, start, powerOff, restart, deallocate)
- `Microsoft.Compute/disks` (write, read, delete)

(azure-cloud-other)=
### Other

#### Concepts

The following table shows how Azure's native abstractions map to Juju concepts:

| Azure | Juju |
| - | - |
| [Resource Group](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/overview#resource-groups) | {ref}`model <model>` (roughly) |
| [Virtual Machine](https://learn.microsoft.com/en-us/azure/virtual-machines/) | {ref}`machine <machine>` |
| Process or container within a VM | {ref}`unit <unit>` |
| Collection of VMs running the same workload | {ref}`application <application>` |
| [Managed Disk](https://learn.microsoft.com/en-us/azure/virtual-machines/managed-disks-overview) | {ref}`storage <storage>` |
| [Subnet](https://learn.microsoft.com/en-us/azure/virtual-network/virtual-network-vnet-plan-design-arm) | Network space (roughly) |

(azure-credential)=
## Credentials

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`, {ref}`Terraform Provider for Juju | Manage credentials <tfjuju:manage-credentials>`
```

(azure-credential-authentication-types)=
### Authentication types

Microsoft Azure supports the following authentication types:

(azure-credential-managed-identity)=
#### `managed-identity`

**Requirements:**
- Juju 3.6+
- Managed identity created in Azure
- Same subscription for managed identity and Juju resources
- Credential addition must occur from Azure Cloud Shell or Azure-hosted jump host (for cloud metadata endpoint access)

**Behavior:** Controller uses managed identity for Azure API operations without storing credential secrets.

```{ibnote}
See more: {ref}`azure-appendix-create-a-managed-identity`, {ref}`azure-appendix-workflow-1`
```

(azure-credential-interactive)=
#### `interactive`

Browser-based OAuth flow. If using unconfined `juju` snap with Azure CLI logged in, subscription ID can be auto-filled.

**Note:** Optional fields `application-name` and `role-definition-name` must have unique values if specified.

**Version note:** Starting with Juju 3.6, can be combined with managed identity via `instance-role` constraint during bootstrap.

```{ibnote}
See more: {ref}`azure-appendix-workflow-2`, {ref}`azure-appendix-workflow-3`
```

(azure-credential-service-principal-secret)=
#### `service-principal-secret`

Requires application ID, subscription ID, and client secret.

**Version note:** Starting with Juju 3.6, can be combined with managed identity via `instance-role` constraint during bootstrap.

```{ibnote}
See more: {ref}`azure-appendix-workflow-2`, {ref}`azure-appendix-workflow-3`
```

(azure-credential-known-issues)=
### Known issues

Credentials occasionally stop working over time. Refresh using credential update or re-add credential.

(azure-controller)=
## Controllers

```{ibnote}
See also: {ref}`Juju | Manage controllers <manage-controllers>`, {ref}`Terraform Provider for Juju | Manage controllers <tfjuju:manage-controllers>`
```

(azure-controller-bootstrap-behavior)=
### Bootstrap behavior

Creates controller and initial model on Azure.

(azure-controller-resources-created-at-bootstrap)=
### Resources created at bootstrap

- **Resource group**: Contains all resources for the model. Auto-generated name or user-specified via `resource-group-name` config.
- **Virtual network**: Named "juju-internal-network" with 192.168.0.0/16 address space. User-configurable via `network` config.
- **Subnets**:
  - Controller subnet (192.168.16.0/20) for controller machines
  - Internal subnet (192.168.0.0/20) for application machines
- **Network security group**: Named "juju-internal-nsg". Rules: SSH (port 22) to all machines, Juju API (port 17070) to controller subnet.
- **Controller virtual machine**: Ubuntu LTS. Size configurable via `instance-type` constraint.

(azure-controller-other)=
### Other

#### Instance role integration

Service principal authentication types can be combined with managed identity via `instance-role` constraint. Allows controller to use managed identity for Azure API operations without storing credential secrets in the controller.

```{ibnote}
See more: {ref}`azure-machine-supported-constraints`
```

(azure-model)=
## Models

```{ibnote}
See also: {ref}`Juju | Manage models <manage-models>`, {ref}`Terraform Provider for Juju | Manage models <tfjuju:manage-models>`
```

When configuring a model on Microsoft Azure, Juju recognizes the following cloud-specific keys.

(azure-model-cloud-specific-configuration-keys)=
(azure-model-configuration-keys)=
### Configuration keys

Microsoft Azure supports the following cloud-specific model configuration keys:

(azure-model-load-balancer-sku-name)=
#### `load-balancer-sku-name`

Mirrors the LoadBalancerSkuName type in the Azure SDK.

- **Type**: `string`
- **Default value**: `"Standard"`
- **Immutable**: `false`
- **Mandatory**: `true`

(azure-model-resource-group-name)=
#### `resource-group-name`

If set, use the specified resource group for all model resources instead of creating one based on the model UUID.

- **Type**: `string`
- **Default value**: none
- **Immutable**: `true`
- **Mandatory**: `false`

(azure-model-network)=
#### `network`

If set, use the specified virtual network for all model machines instead of creating one.

- **Type**: `string`
- **Default value**: none
- **Immutable**: `true`
- **Mandatory**: `false`

(azure-machine)=
## Machines

```{ibnote}
See also: {ref}`Juju | Manage machines <manage-machines>`
```


(azure-machine-supported-constraints)=
(azure-machine-constraints)=
### Constraints

Microsoft Azure supports the following constraints:

```{note}
The constraints `instance-type` and `[arch, cores, mem]` are mutually exclusive.
```

- {ref}`constraint-allocate-public-ip`: Controls public IP address creation.
- {ref}`constraint-arch`: Valid values: `amd64`.
- {ref}`constraint-container`
- {ref}`constraint-cores`
- {ref}`constraint-instance-role`: Juju 3.6+. Valid values: `auto` or managed identity name in format `<resource-group>/<identity-name>` or `<subscription>/<resource-group>/<identity-name>`.
- {ref}`constraint-instance-type`: See Azure VM sizes documentation.
- {ref}`constraint-mem`
- {ref}`constraint-root-disk`: Minimum 30 GiB.
- {ref}`constraint-root-disk-source`: Specifies {ref}`storage pool <storage-pool>` for root disk. Enables encryption configuration.
- {ref}`constraint-zones`

(azure-machine-supported-placement-directives)=
(azure-machine-placement-directives)=
### Placement directives

Microsoft Azure supports the following placement directives:

- {ref}`placement-directive-subnet`

(azure-machine-resources-created-per-machine)=
### Resources created per machine

Each machine (controller or application) receives:

- **Virtual machine**: Type configurable via `instance-type` constraint.
- **OS disk**: 30 GiB minimum, StandardSSD_LRS type by default. Size and type configurable via `root-disk` and `root-disk-source` constraints.
- **Network interface**: Connected to appropriate subnet (controller or internal) with dynamically-allocated private IP address.
- **Public IP address**: Static IPv4 address created by default. Disable via `allocate-public-ip` constraint.
- **Additional storage**: Created when requested via storage specifications.

**Resource tags:** All resources tagged with `juju-model` (model UUID), `juju-controller` (controller UUID), `juju-machine-name` (machine identifier).

(azure-machine-networking-behavior)=
### Networking behavior

- **IP addressing**: Private IPs allocated dynamically via DHCP. Public IPs use static allocation.
- **Subnet placement**: Controller machines → 192.168.16.0/20; application machines → 192.168.0.0/20
- **NSG rules**: SSH (port 22) accessible on all machines. Juju API (port 17070) accessible on controller subnet only.

(azure-storage)=
(azure-storage)=
## Storage

```{ibnote}
See also: {ref}`Juju | Manage storage <manage-storage>`
```

In addition to {ref}`generic storage providers <storage-provider>`, Microsoft Azure provides the following cloud-specific storage providers:

(storage-provider-azure)=
### `azure`

**Type:** Azure Managed Disks

**Configuration options:**

- `account-type`: Disk type
  - `Standard_LRS`: Standard HDD (associated with pool `azure`)
  - `Premium_LRS`: Premium SSD (associated with pool `azure-premium`)

```{ibnote}
See more: [Azure Managed Disks Overview](https://docs.microsoft.com/en-us/azure/virtual-machines/windows/managed-disks-overview)
```

(azure-appendix-example-authentication-workflows)=
## Appendix: Example authentication workflows

(azure-appendix-workflow-1)=
### Workflow 1 -- Managed identity only (recommended)
> *Requirements:*
> - Juju 3.6+.
> - A managed identity. See more: {ref}`azure-appendix-create-a-managed-identity`
> - The managed identity and the Juju resources must be created on the same subscription.
> - The `add-credential` steps must be run from either [the Azure Cloud Shell](https://shell.azure.com/) or a jump host running in Azure in order to allow the cloud metadata endpoint to be reached.

1. Run `juju add-credential azure`; choose `managed-identity`; supply the requested information (the `managed-identity-path` must be of the form `<resourcegroup>/<identityname>`).
2. Bootstrap as usual.

```{tip}
With this workflow where you provide the managed identity during `add-credential` you avoid the need for either your Juju client or your Juju controller to store your credential secrets. Relatedly, the user running `add-credential` / `bootstrap` doesn't need to have any credential secrets supplied to them.
```

(azure-appendix-workflow-2)=
### Workflow 2 -- Service principal secret + managed identity
> *Requirements:*
> - Juju 3.6+.
> - A managed identity. See more: {ref}`azure-appendix-create-a-managed-identity`

1. Add a service-principal-secret:
    - `interactive`  = "service-principal-via-browser" (recommended):
        - If you have the `azure` CLI and you are logged in and you want to use the currently logged in user: Run `/snap/juju/current/bin/juju add-credential azure`; choose `interactive`, then leave the subscription ID field empty -- Juju will fill this in for you.
         - Otherwise: Run `juju add-credential azure`, choose `interactive`, then provide the subscription ID -- Juju will open up a browser and you'll be prompted to log in to Azure.
    - `service-principal-secret`: Run `juju add-credential azure`, then choose `service-principal-secret` and supply all the requested information.
2. During bootstrap, provide the managed identity to the controller by using the `instance-role` constraint.

```{tip}
With this workflow where you provide the managed identity during `bootstrap` you avoid the need for your Juju controller to store your credential secrets. Relatedly, the user running / `bootstrap` doesn't need to have any credential secrets supplied to them.
```

(azure-appendix-workflow-3)=
### Workflow 3 -- Service principal secret only (dispreferred)

1. Add a service-principal-secret:
    - `interactive`  = "service-principal-via-browser" (recommended):
        - If you have the `azure` CLI and you are logged in and you want to use the currently logged in user: Run `/snap/juju/current/bin/juju add-credential azure`; choose `interactive`, then leave the subscription ID field empty -- Juju will fill this in for you.
         - Otherwise: Run `juju add-credential azure`, choose `interactive`, then provide the subscription ID -- Juju will open up a browser and you'll be prompted to log in to Azure.
    - `service-principal-secret`: Run `juju add-credential azure`, then choose `service-principal-secret` and supply all the requested information.
2. Bootstrap as usual.

(azure-appendix-create-a-managed-identity)=
## Appendix: How to create a managed identity

```{caution}

This is just an example. For more information please see the upstream cloud documentation. See more: [Microsoft Azure | Managed identities](https://learn.microsoft.com/en-us/entra/identity/managed-identities-azure-resources/overview).

```

To create a managed identity for Juju to use, you will need to use the Azure CLI and be logged in to your account. This is a set up step that can be done ahead of time by an administrator.

The 4 values below need to be filled in according to your requirements.

```text
$ export group=someresourcegroup
$ export location=someregion
$ export role=myrolename
$ export identityname=myidentity
$ export subscription=mysubscription_id
```

The role definition and role assignment can be scoped to either the subscription or a particular resource group. If scoped to a resource group, this group needs to be provided to Juju when bootstrapping so that the controller resources are also created in that group.

For a subscription scoped managed identity:

```text
$ az group create --name "${group}" --location "${location}"
$ az identity create --resource-group "${group}" --name "${identityname}"
$ mid=$(az identity show --resource-group "${group}" --name "${identityname}" --query principalId --output tsv)
$ az role definition create --role-definition "{
  	\"Name\": \"${role}\",
  	\"Description\": \"Role definition for a Juju controller\",
  	\"Actions\": [
            	\"Microsoft.Compute/*\",
            	\"Microsoft.KeyVault/*\",
            	\"Microsoft.Network/*\",
            	\"Microsoft.Resources/*\",
            	\"Microsoft.Storage/*\",
            	\"Microsoft.ManagedIdentity/userAssignedIdentities/*\"
  	],
  	\"AssignableScopes\": [
        	\"/subscriptions/${subscription}\"
  	]
  }"
$ az role assignment create --assignee-object-id "${mid}" --assignee-principal-type "ServicePrincipal" --role "${role}" --scope "/subscriptions/${subscription}"
```

A resource scoped managed identity is similar except:
-  the role definition assignable scopes becomes
```
      \"AssignableScopes\": [
            \"/subscriptions/${subscription}/resourcegroups/${group}\"
      ]
```
- the role assignment scope becomes

`--scope "/subscriptions/${subscription}/resourcegroups/${group}"`
