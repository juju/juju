---
myst:
  html_meta:
    description: "Juju storage pool reference: what a storage pool is, its names, default pools and use, and what the controller accepts and refuses when pools are created, listed, updated and removed."
---

(storage-pool)=
# Storage pool

```{ibnote}
See also: {ref}`manage-storage-pools`
```

```{ggarch}
:file: ../juju.ggarch
:view: Storage pool in a deployment
:caption: A storage pool holds the settings Juju uses to provision storage. In this example, the application's charm asks for storage: the application's storage directive refers to the pool, or the model default does when the directive gives no pool, and Juju then provisions a storage instance from the pool and attaches it to a unit. A unit also has its own storage directive, which fills in what you leave out when you add storage to that unit later.
:alt: An example configuration, drawn as a box labelled Model with two layers. The top layer, What you declare, holds Application, Storage directive, Storage pool and Model default. The application has a storage directive, which refers to the storage pool, and the model default also refers to the pool. The lower layer, What Juju provisions, holds Unit and Storage instance. The application runs as a unit, the storage pool supplies the storage instance, and the unit has the storage instance attached.
```

In Juju, a **storage pool** is a set of attributes for a {ref}`storage provider <storage-provider>`, kept in one {ref}`model <model>`. A user {ref}`creates <creating-a-storage-pool>` a pool once. When a client asks Juju to provide an application with storage, it gives a {ref}`pool name <storage-pool-names>` in a {ref}`storage directive <storage-directive>`, and Juju provisions the storage with that pool's provider and attributes. A pool is considered {ref}`in use <storage-pools-in-use>` while a storage directive or a model default refers to it, or while storage provisioned from it exists; a user cannot remove such a pool or change its provider. A user can also {ref}`list <listing-storage-pools>`, {ref}`update <updating-a-storage-pool>` and {ref}`remove <removing-a-storage-pool>` pools.

(storage-pool-names)=
## Storage pool names

A pool name must have 1 to 128 characters, all of them ASCII letters, digits or hyphens. It must start with a letter and end with a letter or digit. A name must also be unique. A model cannot have two pools with the same name.

The controller refuses a name that breaks these rules when a user creates or updates a pool, or lists pools by name.

(default-storage-pools)=
## Default storage pools

A new model already contains the default pools of its storage providers, such as `rootfs` and `tmpfs`. For the cloud-specific providers, see {ref}`List of supported clouds <list-of-supported-clouds>` > your cloud > Storage.

(storage-pools-in-use)=
## Storage pools in use

A pool is in use while any of these refers to it:

- A storage instance in the model.
- A storage directive of an application in the model.
- A storage directive of a unit in the model.
- The model's default pool for block storage or for filesystem storage, whether set with the {ref}`storage-default-block-source <model-config-storage-default-block-source>` or {ref}`storage-default-filesystem-source <model-config-storage-default-filesystem-source>` model configuration key, or recorded by Juju as the default for the model.

A user cannot remove an in-use pool or change its provider, but can still replace its attributes.

(creating-a-storage-pool)=
## Creating a storage pool

Creating a pool adds a set of attributes for a storage provider to the model. A user gives the pool's name and its storage provider, and may add attributes. A model has the storage providers of its {ref}`cloud <cloud>` (to Juju, any entity with an API that provides compute, networking and, optionally, storage resources), and Juju manages their storage from outside the machine. On a machine cloud it also has `rootfs`, `tmpfs` and `loop`, whose storage Juju manages from within the machine. The provider defines which attributes it accepts and validates them. For the attributes of a cloud-specific provider, see {ref}`List of supported clouds <list-of-supported-clouds>` > your cloud > Storage.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* `storage pool name is either not set or is not a valid name`. *Trigger:* The name is not a valid {ref}`pool name <storage-pool-names>`. *What to do:* Use a name that follows the rules.
- *Error message:* `provider type is not a valid value that can be used for a new storage pool`. *Trigger:* The provider name is not a valid provider name. *What to do:* Give the name of a storage provider that the model has.
- *Error message:* `no storage provider "<provider>" exists in the current model`. *Trigger:* The model does not have the storage provider. *What to do:* Give the name of a storage provider that the model has.
- *Error message:* `storage pool for name "<name>" already exists in the model`. *Trigger:* The model already has a pool with that name. *What to do:* Use another name, or update the existing pool.
- *Error message:* `unable to create storage pool "<name>" due to an unknown error`. *Trigger:* The provider rejects an attribute. The message does not say which one. *What to do:* Check the attributes against the provider's documentation; the controller's log records the reason.

(listing-storage-pools)=
## Listing storage pools

Listing pools shows the pools of a model. Without a filter, the list contains every pool in the model, including the default pools of its storage providers.

A user may give a filter that names pools, providers, or both. The list includes a pool if its name is any of the names and its provider is any of the providers; a filter that gives only names, or only providers, applies only that condition.

A valid name that matches no pool is not an error. The list is empty.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>`, or with {ref}`controller superuser access <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `pool name "<name>" not valid`. *Trigger:* The user gives a filter with an invalid {ref}`pool name <storage-pool-names>`. *What to do:* Use a name that follows the rules.
- *Error message:* `storage provider "<provider>" not found`. *Trigger:* The user gives a filter with a provider that the model does not have. *What to do:* Give the name of a storage provider that the model has.

(updating-a-storage-pool)=
## Updating a storage pool

Updating a pool replaces its attributes and can change its provider. A user gives the pool's name and the attributes it should have, and may also give a different storage provider. The update replaces the pool's attributes with the attributes the user gives, so the pool keeps no attribute that the user omits; the controller accepts an update that gives none, as long as the provider accepts an empty set. The pool keeps its storage provider unless the user gives a different one. The provider validates the resulting attributes as it does at creation.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* ends with `storage pool "<name>" not found`. *Trigger:* The model has no pool with that name. *What to do:* List the pools and use the name of one.
- *Error message:* `pool name "<name>" not valid`. *Trigger:* The name is not a valid {ref}`pool name <storage-pool-names>`. *What to do:* Use a name that follows the rules.
- *Error message:* ends with `storage pool is in use`. *Trigger:* The user changes the provider of a pool that is {ref}`in use <storage-pools-in-use>`. *What to do:* Keep the pool's provider, or create a new pool with the other provider.

(removing-a-storage-pool)=
## Removing a storage pool

Removing a pool deletes it from the model together with its attributes. A user gives the pool's name.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* ends with `storage pool "<name>" not found`. *Trigger:* The model has no pool with that name. *What to do:* List the pools and use the name of one.
- *Error message:* ends with `storage pool is in use`. *Trigger:* The pool is {ref}`in use <storage-pools-in-use>`. *What to do:* Remove or change whatever refers to the pool, then remove the pool.
