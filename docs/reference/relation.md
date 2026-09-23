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

(the-implicit-juju-info-relation-endpoint)=
##### The implicit `juju-info` relation endpoint

Every application in a model implicitly provides an extra endpoint named `juju-info`, with role `provides`, interface `juju-info`, and global scope. The endpoint is supplied by Juju itself: it does not need to be (and cannot be) declared in the charm's `metadata.yaml` / `charmcraft.yaml`, and it does not appear in `juju info <charm>` or on the charm's Charmhub page. It is supported on every kind of charm, but is only useful for {ref}`machine charms <machine-charm>`, since it exists to allow {ref}`subordinate charms <subordinate-charm>` to attach to a principal that does not otherwise expose a suitable interface.

The `juju-info` endpoint is intended to be consumed by a {ref}`subordinate charm <subordinate-charm>` whose `metadata.yaml` / `charmcraft.yaml` declares an explicit `requires` endpoint with interface `juju-info` (typically with name `juju-info` and `scope: container`). The container scope is what causes the subordinate's unit to be co-located on the same machine as the principal's unit.

The `juju-info` endpoint is the standard mechanism used by general-purpose machine subordinates that need to ride along on every machine of a principal but do not have any application-specific data to exchange. For example, monitoring, logging, or system-administration agents such as [`ntp`](https://charmhub.io/ntp), [`telegraf`](https://charmhub.io/telegraf), [`nrpe`](https://charmhub.io/nrpe), and [`canonical-livepatch`](https://charmhub.io/canonical-livepatch).

The convention is for a subordinate to name its own `requires` endpoint `juju-info`, but any name can be used (for example: `info`, `general-info`); it just needs to use interface `juju-info`.

You integrate against the implicit endpoint the same way as any other endpoint:

```text
juju integrate <subordinate> <principal>
```

If the subordinate also has explicit endpoints whose interfaces match endpoints on the principal, those explicit endpoints take precedence over the implicit `juju-info` one when Juju resolves the relation. To force the implicit endpoint, name it explicitly on either side:

```text
juju integrate <subordinate>:<requires-endpoint> <principal>:juju-info
```

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

## Relation lifecycle

### Relation creation

A relation is created by `juju integrate`. Creating the relation
writes a relation record and wakes both sides: each unit's watcher
fires, and the units run their relation hooks in lockstep --
`relation-created`, then `relation-joined` and `relation-changed` --
exchanging data through the databags as they go.

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:alt: User calls juju integrate A B. Controller writes relation record and fires watchers to both unit agents. Each runs relation-created, relation-joined, relation-changed hooks, writing relation data; each data write wakes the other side's watcher for a further relation-changed.
:caption: Integrating two applications. The controller writes the relation record; the two units' hooks run in lockstep, each data write waking the other side for another relation-changed.
```

## Relation databag

When you create a relation between two applications, this results in the creation of relation databags. Databags are per relation and per application, and can be application-scoped or unit-scoped. Each unit involved in a relation gets a local copy of all the databags for that relation.

### Permissions around relation databags

```{ggarch}
:file: ../juju.ggarch
:view: Databag permissions
:no-legend:
:caption: The databags are records in the host model's database, and the connection between applications runs through their units: each unit's agent reads and writes them over the API (relation-get/relation-set via the hook tools -> the unit agent's uniter facade). Each application has ONE application databag (per relation endpoint) and unit databags (one per unit). The units (not their agents -- leadership is a property of the unit; the unit agent is the machinery doing the API work) read and write the databags, which are records in the host model's database. Red arrows read + write, green arrows read only: each unit reads + writes ONLY the unit databag associated with its own unit; the leader unit also reads + writes its application's application databag; and all units -- leaders included -- read all of the other application's databags. Peer case (app A related to itself): the permissions turn inward -- every unit reads every databag of its own application, the application databag included. Users have read-only visibility via the API (juju show-unit), never write.
:alt: App A's units (appA/leader, appA/1) and app B's units (appB/leader, appB/1) above one row of databags; red arrows reading and writing the own bags (own unit bag; the leader also the application databag), green arrows reading across to the other application's set; below, the peer panel: one application's units with red own/leader arrows and green reads of every bag, the application databag included.
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
