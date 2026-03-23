---
myst:
  html_meta:
    description: "Juju relation (integration) reference: application connections through endpoints. These include regular relations, peer relations, subordinate relations, and cross-model relations, and involve relation databags."
---

(relation)=
# Relation (integration)

```{ibnote}
See also: {ref}`manage-relations`
```

In Juju, a **relation** (**integration**) is a connection an {ref}`application <application>` supports by virtue of having a particular {ref}`endpoint <application-endpoint>`.

## Relation taxonomy

```{figure} relation-taxonomy.svg
  :figclass: only-light
  :align: center
  :alt: Juju relation taxonomy
  _A relation is between two applications. When the applications are such that one is principal and one is subordinate, the result is a subordinate relation. When the applications are on two separate models, the result is a cross-model relation. When the applications are identical -- that is, we are speaking of the relation an application has to itself -- the result is a peer relation._
```
```{figure} relation-taxonomy.dark.svg
  :figclass: only-dark
  :align: center
  :alt: Juju relation taxonomy
  _A relation is between two applications. When the applications are such that one is principal and one is subordinate, the result is a subordinate relation. When the applications are on two separate models, the result is a cross-model relation. When the applications are identical -- that is, we are speaking of the relation an application has to itself -- the result is a peer relation._
```

(peer-relation)=
### Peer relation

A **peer** relation is a relation that an application has to itself (i.e., its units respond to one another) automatically by virtue of having a `peers` endpoint.

Because every relation results in the creation of unit and application databags in Juju's database, peer relations are sometimes used by charm authors as a way to persist charm data. When the application has multiple units, peer relations are also the mechanism behind {ref}`high availability <high-availability>`.

(non-peer-relation)=
### Non-peer relation

A **non-peer** relation is a relation from one application to another, where the applications support the same endpoint interface and have opposite `provides` / `requires` endpoint roles.

(subordinate-relation)=
#### Subordinate relation

A **subordinate** relation is a {ref}`non-peer <non-peer-relation>` relation where one application is principal and the other subordinate.

A subordinate charm is by definition a charm deployed on the same machine as the principal charm it is intended to accompany. When you deploy a subordinate charm, it appears in your Juju model as an application with no unit. The subordinate relation helps the subordinate application acquire a unit. The subordinate application then scales automatically when the principal application does, by virtue of this relation.

(non-subordinate-relation)=
#### Non-subordinate relation

A **non-subordinate** relation (aka 'regular') is a {ref}`non-peer <non-peer-relation>` relation where the applications are both principal.

(cross-model-relation)=
##### Cross-model relation

```{ibnote}
See also: {ref}`manage-relations`
```

A **cross-model** relation (aka 'CMR') is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on different models (+/- different controllers, +/- different clouds).

Cross-model relations enable, for example, scenarios where  your databases are hosted on bare metal, to take advantage of I/O performance, and your applications live within Kubernetes, to take advantage of scalability and application density.

If the network topology is anything other than flat, the Juju controllers will need to be bootstrapped with `--controller-external-ips`, `--controller-external-name`, or both, so that the controllers are able to communicate. Note that these config values can only be set at bootstrap time, and are read-only thereafter.

A cross-model relation has two sides: the offering side (aka "offerer") and the consume side (aka 'saas'). It does not make a difference which side of the relation (provider or requirer) is the offerer and which is the saas - the two are interchangeable. However, the endpoint type does influence on how juju sets up firewall rules: it is assumed that a requirer is the client and the provider is the server, so ports are opened on the provider side.

Note that application names are obfuscated (anonymised) to the offerer side:
- Applications that relate to the saas appear to the offerer as remote + token, e.g. `remote-76cd96ab50f146b284912afd1cc13a0e`.
- For the consumer, the remote app names is the saas name, e.g. `prometheus`.
##### Non-cross-model relation

A **non-cross-model** relation is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on  the same model.

## Relation identification

A relation is identified by a **relation ID** (assigned automatically by Juju; expressed in monotonically increasing numbers) or a **relation key** (derived from the endpoints, format: `application1:[endpoint] application2:[endpoint]`).

## Relation databag

When you create a relation between two applications, this results in the creation of relation databags. Databags are per relation and per application, and can be application-scoped or unit-scoped. Each unit involved in a relation gets a local copy of all the databags for that relation.

### Permissions around relation databags

```{figure} relation-databags.svg
  :figclass: only-light
  :align: center
  :alt: Juju relation databags -- permissions
```
```{figure} relation-databags.dark.svg
  :figclass: only-dark
  :align: center
  :alt: Juju relation databags -- permissions
  _Given a unit involved in a relation, the unit's access to a relation databag depends on whether the relation is peer or not, whether the unit is leader or not, and whether the databag belongs to the unit's application or not._
```

While the relation is maintained,

- in a non-peer relation, whether regular or subordinate:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the local application databag;
    - all units of an application can read all of the remote application's databags.
- in a peer relation:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the application databag;
    - all units can read all of the application's databags. That is, whether leader or not, every unit can read its own unit databag as well as every other unit's unit databag as well as the application databag.

Note that, in peer relations, all permissions related to the remote application are turned inwards and become permissions related to the local application.
