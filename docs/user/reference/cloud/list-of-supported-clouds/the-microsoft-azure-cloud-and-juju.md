(cloud-azure)=
# The Microsoft Azure cloud and Juju


This document describes details specific to using your existing Microsoft Azure cloud with Juju. 

> See more: [Microsoft Azure](https://azure.microsoft.com/en-us) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).



## Requirements 

**If you're in a locked-down environment:** <br> Permissions: <p>- `Microsoft.Compute/skus (read)` <p> - `Microsoft.Resources/subscriptions/resourceGroups (read, write, delete)` <p> - `Microsoft.Resources/deployments/ (write/read/delete/cancel/validate)` <p> - `Microsoft.Network/networkSecurityGroups (write, read, delete, other - join)` <p> - `Microsoft.Network/virtualNetworks/ (write, read, delete)` <p> - `Microsoft.Compute/virtualMachineScaleSets/ (write, read, delete, other - start action, other - deallocate action, other - restart action, other powerOff action)` <p> - `Microsoft.Network/virtualNetworks/subnets/ (read, write, delete, other - join)` <p> - `Microsoft.Compute/availabilitySets (write, read, delete)` <p> - `Microsoft.Network/publicIPAddresses (write, read, delete, other - join - optional for public services)` <p> - `Microsoft.Network/networkInterfaces (write, read, delete, other - join)` <p> - `Microsoft.Compute/virtualMachines (write, read, delete, other - start, power off, restart, deallocate)` <p> - `Microsoft.Compute/disks (write, read, delete)`


## Notes on `juju add-cloud`

Type in Juju: `azure`.

Name in Juju: `azure`.


## Notes on `juju add-credential`
```{note}

Credentials for the `azure` cloud have been reported to occasionally stop working over time. If this happens, try `juju update-credential` (passing as an argument the same credential) or `juju add-credential` (passing as an argument a new credential) + `juju default-credential`.

```

```{note}

See Appendix: Example authentication workflows.


```

### Authentication types

#### `managed-identity` (preferred)
> *Requirements:*  
> - Juju 3.6+. 
> - A managed identity. See more: Appendix: How to create a managed identity.
> - The managed identity and the Juju resources must be created on the same subscription. 
> - The `add-credential` steps must be run from either [the Azure Cloud Shell^](https://shell.azure.com/) or a jump host running in Azure in order to allow the cloud metadata endpoint to be reached.

This is the recommended way to authenticate with Azure as this way you are never touching your cloud credentials directly.

> See more: {ref}`azure-appendix-workflow-1`


#### `interactive` = "service-principal-secret-via-browser"

This is the recommended way to authenticate with Azure if you want to use a service principal secret.

When you add the credential in this way and provide the subscription ID, Juju will open up a browser and you’ll be prompted to log in to Azure. 

Note: If you are using the unconfined `juju` snap `/snap/juju/current/bin/juju add-credential azure` and have the `azure` CLI and you are logged in and you want to use the currently logged in user: You may leave the subscription ID empty -- Juju will fill it in for you. 

Caution: If you decide to fill in the optional fields as well: Make sure to set them to unique values (i.e., the `application-name` and `role-definition-name` fields cannot be the same).

Tip: Starting with Juju 3.6, you can also combine this authentication type with a managed identity by bootstrapping with the `instance-role` constraint.

> See more: {ref}`azure-appendix-workflow-2`, {ref}`azure-appendix-workflow-3`


#### `service-principal-secret` (dispreferred)

Starting with Juju 3.6, you can also combine this with a managed identity by bootstrapping with the `instance-role` constraint.

> See more: {ref}`azure-appendix-workflow-2`, {ref}`azure-appendix-workflow-3`


## Notes on `juju bootstrap`

If during `juju add-credential` you chose `interactive` (= "service-principal-secret-via-browser") or `service-principal-secret`: You can still combine this with a managed identity by running `juju bootstrap` with `--constraints instance-role=...`. 

> See more: {ref}`azure-appendix-workflow-1`, Supported constraints


## Cloud-specific model configuration keys


### `load-balancer-sku-name`
Mirrors the LoadBalancerSkuName type in the Azure SDK.

| | |
|-|-|
| type | string |
| default value | "Standard" |
| immutable | false |
| mandatory | true |

### `resource-group-name`
If set, use the specified resource group for all model artefacts instead of creating one based on the model UUID.

| | |
|-|-|
| type | string |
| default value | schema.omit{} |
| immutable | true |
| mandatory | false |

### `network`
If set, use the specified virtual network for all model machines instead of creating one.

| | |
|-|-|
| type | string |
| default value | schema.omit{} |
| immutable | true |
| mandatory | false |
## Supported constraints 

| {ref}`CONSTRAINT <constraint>`         |                                                |
|----------------------------------------|------------------------------------------------|
| conflicting:                           | `{ref}`instance-type]` vs `[arch, cores, mem]` |
| supported?                             |                                                |
| - {ref}`constraint-allocate-public-ip` | &#10003;                                       |
| - {ref}`constraint-arch`               | &#10003; <br> Valid values: `amd64`            |
| - {ref}`constraint-container`          | &#10003;                                       |
| - {ref}`constraint-cores`              | &#10003;                                       |
| - {ref}`constraint-cpu-power`          | &#10005;                                       |
| - {ref}`constraint-image-id`           | &#10005;                                       |
| - {ref}`constraint-instance-role`      | *Starting with Juju 3.6:* &#10003; <p> Valid values: `auto` (Juju creates a managed identity for you) or a [managed identity^](https://www.google.com/url?q=https://learn.microsoft.com/en-us/entra/identity/managed-identities-azure-resources/overview&sa=D&source=docs&ust=1720105912478784&usg=AOvVaw2eioSYvtSn1pn-BWstI6AU) name in one of the following formats:<p> - **If the managed identity is created in a resource group on the same subscription:** <br>`<resource group>/<identity name>` <p> - **If the managed identity is created in a resource group on a different subscription:** <br> `<subscription>/<resource group>/<identity name>` <p> - **If the managed identity is created in a resource group and that resource group is used to host the controller model:** <br>`<identity name>` <br> e.g., `juju bootstrap azure --config resource-group-name=<resource group> --constraints instance-role=<identity name>` <p> Note: If you want your controller to be in the same resource group as the one used for the managed identity, during bootstrap also specify `--config resource-group-name=<theresourcegroup>`.  <p> > See more: Appendix: Supported authentication types: Example workflows.  |
| - {ref}`constraint-instance-type`      | &#10003; <br> Valid values: See cloud provider. |
| - {ref}`constraint-mem`                | &#10003;                                       |
| - {ref}`constraint-root-disk`          | &#10003;                                       |
| - {ref}`constraint-root-disk-source`   | &#10003;  <br> Represents the juju {ref}`storage pool <storage-pool>` for the root disk. By specifying a storage pool, the root disk can be configured to use encryption.                                      |
| - {ref}`constraint-spaces`             | &#10005;                                       |
| - {ref}`constraint-tags`               | &#10005;                                       |
| - {ref}`constraint-virt-type`          | &#10005;                                       |
| - {ref}`constraint-zones`              | &#10003;                                       |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |          |
|--------------------------------------------------|----------|
| {ref}`placement-directive-machine`               | TBA      |
| {ref}`placement-directive-subnet`                | &#10003; |
| {ref}`placement-directive-system-id`             | &#10005; |
| {ref}`placement-directive-zone`                  | TBA      |




## Appendix: Example authentication workflows

(azure-appendix-workflow-1)=
### Worflow 1 -- Managed identity only (recommended)
> *Requirements:*  
> - Juju 3.6+. 
> - A managed identity. See more: Appendix: How to create a managed identity.
> - The managed identity and the Juju resources must be created on the same subscription. 
> - The `add-credential` steps must be run from either [the Azure Cloud Shell^](https://shell.azure.com/) or a jump host running in Azure in order to allow the cloud metadata endpoint to be reached.

1. Create a managed identity. See more: Appendix: How to create a managed identity. 
1. Run `juju add-credential azure`; choose `managed-identity`; supply the requested information (the“managed-identity-path” must be of the form `<resourcegroup>/<identityname>`). 
1. Bootstrap as usual.  

```{tip}

**Did you know?** With this workflow where you provide the managed identity during `add-credential` you avoid the need for either your Juju client or your Juju controller to store your credential secrets. Relatedly, the user running `add-credential` / `bootstrap` doesn't need to have any credential secrets supplied to them. 

```

(azure-appendix-workflow-2)=
### Workflow 2 -- Service principal secret + managed identity 
> *Requirements:*  
> - Juju 3.6+. 
> - A managed identity. See more: Appendix: How to create a managed identity.

1. Create a managed identity.
1. Add a service-principal-secret:
    - `interactive`  = "service-principal-via-browser" (recommended):
        - If you have the `azure` CLI and you are logged in and you want to use the currently logged in user: Run `/snap/juju/current/bin/juju add-credential azure`; choose `interactive`, then leave the subscription ID field empty -- Juju will fill this in for you. 
         - Otherwise: Run `juju add-credential azure`, choose `interactive`, then provide the subscription ID -- Juju will open up a browser and you’ll be prompted to log in to Azure.
    - `service-principal-secret`: Run `juju add-credential azure`, then choose `service-principal-secret` and supply all the requested information.  
1. During bootstrap, provide the managed identity to the controller by using the `instance-role` constraint.

```{tip}

**Did you know?** With this workflow where you provide the managed identity during `bootstrap` you avoid the need for your Juju controller to store your credential secrets. Relatedly, the user running / `bootstrap` doesn't need to have any credential secrets supplied to them. 

```

(azure-appendix-workflow-3)=
### Workflow 3 -- Service principal secret only (dispreferred)

1. Add a service-principal-secret:
    - `interactive`  = "service-principal-via-browser" (recommended):
        - If you have the `azure` CLI and you are logged in and you want to use the currently logged in user: Run `/snap/juju/current/bin/juju add-credential azure`; choose `interactive`, then leave the subscription ID field empty -- Juju will fill this in for you. 
         - Otherwise: Run `juju add-credential azure`, choose `interactive`, then provide the subscription ID -- Juju will open up a browser and you’ll be prompted to log in to Azure.
    - `service-principal-secret`: Run `juju add-credential azure`, then choose `service-principal-secret` and supply all the requested information.  
1. Bootstrap as usual.


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

