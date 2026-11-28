---
myst:
  html_meta:
    description: "Juju application reference: understand application structure, units, configuration, resources, relations, and lifecycle management."
---

(application)=
# Application

```{ibnote}
See also: {ref}`manage-applications`
```

In Juju, an **application** is a running abstraction of a {ref}`charm <charm>` in the Juju {ref}`model <model>`. It is whatever software is defined by the charm. This could correspond to a traditional software package but it could also be less or more.

An application is always hosted within a {ref}`model <model>` and consists of one or more {ref}`units <unit>`.

An application can have {ref}`resources <charm-resource>`, a {ref}`configuration <application-configuration>`, the ability to form {ref}`relations <relation>`, and {ref}`actions <action>`.

(application-endpoint)=
## Application endpoint

In Juju, an application **endpoint** is a struct defined in an {ref}`application <application>`'s {ref}`charm <charm>`'s `metadata.yaml` / (since Charmcraft 2.5) `charmcraft.yaml` consisting of
- a name (charm-specific),
- a role (one of `provides`, `requires` = 'can use', or `peers`), and
- an interface

whose purpose is to help define a {ref}`relation <relation>`.

For example, the MySQL application deployed from the `mysql` charm has an endpoint called `mysql` with role `provides` and interface `mysql` and this can be used to form  a {ref}`non-subordinate relation <non-subordinate-relation>` relation with WordPress.

```{ibnote}
See more: [GitHub | `mysql-operator` > `metadata.yaml`](https://github.com/canonical/mysql-operator/blob/2bd2bcc65590937dab18d1d9b0fe21a445557bb6/metadata.yaml#L35), [Charmhub | `mysql`](https://charmhub.io/mysql/integrations#mysql)
```

All charms have an implicit (not in their `metadata.yaml` / `charmcraft.yaml`) endpoint with name `juju-info`, interface `juju-info`, and role `provides`. This endpoint can be used to form {ref}`subordinate relations <subordinate-relation>` with subordinate charms that have an explicit endpoint with interface `juju-info` and role `requires`. See {ref}`the-implicit-juju-info-relation-endpoint` for details. Examples: [`ntp`](https://charmhub.io/ntp/integrations#juju-info), [`mysql-router`](https://charmhub.io/mysql-router/integrations#juju-info)

(reading-an-applications-storage-requests)=
## Reading an application's storage requests

The Application API's `GetApplicationStorage` method (v22) returns each application's {ref}`storage requests <storage-requests>` keyed by the charm's storage names. Each entry gives the count per unit, size in MiB and source. Each requested application has its own result. The separate `UpdateApplicationStorage` method returns a not-implemented error for each submitted change.

**Who can do this:** A user with at least {ref}`model read access <user-access-model-read>` can read the requests.

**Errors:** Reading an application's storage requests can return this error:

- *Error message:* `application "<name>" not found`. *Trigger:* The selected model has no application with that name. *What to do:* Check the application name in that model.
