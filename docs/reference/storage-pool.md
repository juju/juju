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
:caption: A storage pool holds the settings Juju uses to provision storage. Here the application's charm asks for storage. Juju uses the pool named in the application's storage directive, or the model default if the directive names no pool. Juju provisions a storage instance from that pool and attaches it to a unit. The unit's own storage directive supplies omitted settings when you add storage to the unit later.
:alt: The model has two layers. “What you declare” shows the application linked to its storage directive; both the directive and the model default point to the pool. “What Juju provisions” shows the application running as a unit with a storage instance supplied by the pool.
```

In Juju, a **storage pool** holds the attributes a {ref}`model <model>` uses to provision storage with a particular {ref}`storage provider <storage-provider>`. A {ref}`user <user>` {ref}`creates <creating-a-storage-pool>` a pool once. When a {ref}`client <client>` requests {ref}`storage <storage>` for an {ref}`application <application>`, it can give a {ref}`pool name <storage-pool-names>` in the application's {ref}`storage directive <storage-directive>`. If the application's directive names no pool, Juju uses the {ref}`model default <default-storage-pools>` for the requested kind of storage.

A user can {ref}`list <listing-storage-pools>`, {ref}`update <updating-a-storage-pool>` and {ref}`remove <removing-a-storage-pool>` pools in the model. A pool is {ref}`in use <storage-pools-in-use>` while a storage directive or model default refers to it, or while storage provisioned from it exists. A user cannot remove an in-use pool or change its provider.

(storage-pool-names)=
## Storage pool names

A pool name must have 1 to 128 characters, all of them ASCII letters, digits or hyphens. It must start with a letter and end with a letter or digit. A name must also be unique. A model cannot have two pools with the same name.

The controller refuses a name that breaks these rules when a user creates or updates a pool, or lists pools by name.

(default-storage-pools)=
## Default storage pools

A new model already contains pools supplied by its storage providers. For example, a new machine model includes pools named `rootfs` and `tmpfs`. The cloud-specific providers are listed under {ref}`List of supported clouds <list-of-supported-clouds>` > your cloud > Storage.

A pool being present does not make it the one Juju uses when an application's storage directive names no pool. The model selects a default separately for {ref}`block storage <model-config-storage-default-block-source>` and {ref}`filesystem storage <model-config-storage-default-filesystem-source>`. Check the default source for the kind of storage the application requests.

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

Creating a pool adds a set of attributes for a storage provider to the model. A user gives the pool's name and its storage provider, and may add attributes. The available providers depend on the model's {ref}`cloud <cloud>`. On a machine cloud they also include `rootfs`, `tmpfs` and `loop`. The provider defines which attributes it accepts and validates them. For the attributes of a cloud-specific provider, see {ref}`List of supported clouds <list-of-supported-clouds>` > your cloud > Storage.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* `storage pool name is either not set or is not a valid name`. *Trigger:* The name is missing or is not a valid {ref}`pool name <storage-pool-names>`. *What to do:* Choose an unused name such as `workload-storage`.
- *Error message:* `provider type is not a valid value that can be used for a new storage pool`. *Trigger:* The provider name is not a valid provider name. *What to do:* Give the name of a storage provider that the model has.
- *Error message:* `no storage provider "<provider>" exists in the current model`. *Trigger:* The model does not have the storage provider. *What to do:* Give the name of a storage provider that the model has.
- *Error message:* `storage pool for name "<name>" already exists in the model`. *Trigger:* The model already has a pool with that name. *What to do:* Use another name, or update the existing pool.
- *Error message:* `unable to create storage pool "<name>" due to an unknown error`. *Trigger:* The provider rejects an attribute. The message does not say which one. *What to do:* Check the attributes against the provider's documentation; the controller's log records the reason.

(listing-storage-pools)=
## Listing storage pools

```{ibnote}
See also: {ref}`View storage pools with the Juju CLI <view-the-available-storage-pools>`
```

Listing pools returns each matching pool's name, storage provider and attributes. Without a filter, the list contains every pool in the model, including the default pools of its storage providers.

A user may give a filter that names pools, providers, or both. The list includes a pool if its name is any of the names and its provider is any of the providers; a filter that gives only names, or only providers, applies only that condition.

To inspect one pool, select it by its exact name. A valid name that matches no pool contributes no result; it does not exclude pools that match other names in the filter. If nothing matches, the list is empty, not an error.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>`, or with {ref}`controller superuser access <user-access-controller-superuser>`.

**Errors:**

- *Error message:* `pool name "<name>" not valid`. *Trigger:* The user gives a filter with an invalid {ref}`pool name <storage-pool-names>`. *What to do:* List the pools without a name filter and use the intended pool's listed name.
- *Error message:* `storage provider "<provider>" not found`. *Trigger:* The user gives a filter with a provider that the model does not have. *What to do:* Give the name of a storage provider that the model has.

(updating-a-storage-pool)=
## Updating a storage pool

Updating a pool replaces its attributes and can change its provider. A user gives the pool's name and the attributes it should have, and may also give a different storage provider. The update replaces the pool's attributes with the attributes the user gives, so the pool keeps no attribute that the user omits; the controller accepts an update that gives none, as long as the provider accepts an empty set. The pool keeps its storage provider unless the user gives a different one. The provider validates the resulting attributes as it does at creation.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* ends with `storage pool "<name>" not found`. *Trigger:* The model has no pool with that name. *What to do:* List the pools and use the name of one.
- *Error message:* `pool name "<name>" not valid`. *Trigger:* The name is not a valid {ref}`pool name <storage-pool-names>`. *What to do:* List the pools and retry with the intended pool's listed name.
- *Error message:* ends with `storage pool is in use`. *Trigger:* The user changes the provider of a pool that is {ref}`in use <storage-pools-in-use>`. *What to do:* Keep the pool's provider, or create a new pool with the other provider.

(removing-a-storage-pool)=
## Removing a storage pool

Removing a pool deletes its settings from the model, not the storage provisioned from it. Juju refuses removal while any such storage still exists or the pool is otherwise {ref}`in use <storage-pools-in-use>`. A user gives the pool's name.

**Who can do this:** A user with at least {ref}`model write access <user-access-model-write>`.

**Errors:**

- *Error message:* ends with `storage pool "<name>" not found`. *Trigger:* The model has no pool with that name. *What to do:* List the pools and use the name of one.
- *Error message:* ends with `storage pool is in use`. *Trigger:* The pool is still in use. *What to do:* Check for storage instances provisioned from the pool, application and unit storage directives that name it, and model defaults for block or filesystem storage that select it. Keep the pool if its storage is still needed. Retry removal only when no storage instance, directive or default refers to it.
