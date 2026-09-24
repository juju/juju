---
myst:
  html_meta:
    description: "Juju credentials reference: authentication material for cloud access, credential types, storage, and model associations."
---

(credential)=
# Credential

```{ibnote}
See also: {ref}`manage-credentials`
```

In Juju, a **credential** represents a collection of authentication material (like username & password, or client id & secret key) that is specific to a Juju {ref}`user <user>` and a {ref}`cloud <cloud>` and allows that user to interact with that cloud.

```{important}
In Juju a 'credential' always refers to authentication material used to access a *cloud*.
```

Clouds can have one or more sets of credentials associated with them.

When you create a  {ref}`model <model>` in Juju it must always be associated with a cloud/credential pair -- the model needs that to create resources on the underlying cloud.

## Credential definition

```{ggarch}
:file: ../juju.ggarch
:view: Credential chain
:no-legend:
:caption: Topology: The credential chain lives in the controller DB: a user owns 0..N cloud credentials (cloud/owner/name is the natural key; 15 auth types); a cloud defines 0..N credentials; a model uses 0..1 credential and belongs to one cloud. The model DB carries only a read-only denormalised copy (credential owner/name as text). Access grants are a separate permission table (there is no credential object type; credential access is ownership plus cloud-level add-model/admin).
:alt: User record, cloud record, credential record, and model record with FK arrows.
```

The structure of a credential and its supported authentication types depend on the cloud. See the relevant {ref}`cloud reference page <list-of-supported-clouds>` for details.

## Client vs. controller credential

Juju credentials can be created for either the Juju client or the Juju controller or both -- where a **client credential** (previously known as a 'local credential') denotes a credential that the client is aware of and a **controller credential** (previously known as a 'remote credential') denotes a credential that a controller is aware of. When you bootstrap a controller and use a client credential, this credential gets automatically uploaded to the controller, so it becomes a controller credential also.

```{important}
The set of client credentials and controller credentials can end up being the same. However, they don't have to.
```
