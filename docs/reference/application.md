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

All charms have an implicit (not in their `metadata.yaml` / `charmcraft.yaml`) endpoint with name `juju-info`, role `provides`, and interface `juju-info` which can be used to form {ref}`subordinate relations <subordinate-relation>` with subordinate charms that have an explicit endpoint with name `juju-info`, role `requires`, and interface `juju-info` (e.g., [`mysql-router`](https://charmhub.io/mysql-router/integrations#juju-info)).
