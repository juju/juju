(relation)=
# Relation (integration)

> See also: {ref}`manage-relations`

In Juju, a **relation** (**integration**) is a connection an {ref}`application <application>` supports by virtue of having a particular {ref}`endpoint <application-endpoint>`.


<!--
Most applications rely on other applications to function correctly. For example, typically web apps require a database to connect to. Relations avoid the need for manual intervention when the charm’s environment changes. The charm will be notified of new changes, re-configure and restart the application automatically.

Relations are a Juju abstraction that enables applications to inter-operate. They are a communication channel between charmed operators.

A certain charm knows that it requires, say, a database and, correspondingly, a database charm knows that it is capable of satisfying another charm’s requirements. The act of joining such mutually-dependent charmed operators causes code (*hooks*) to run in each charm in such a way that both charmed operators can effectively talk to one another. When charmed operators have joined logically in this manner they are said to have formed a *relation*.
-->

## Relation taxonomy

![relation-taxonomy](relation-taxonomy.svg)

(peer-relation)=
### Peer relation

A **peer** relation is a relation that an application has to itself (i.e., its units respond to one another) automatically by virtue of having a `peers` endpoint.

Because every relation results in the creation of unit and application databags in Juju's database, peer relations are sometimes used by charm authors as a way to persist charm data. When the application has multiple units, peer relations are also the mechanism behind {ref}`high availability <high-availability>`.

(non-peer-relation)=
### Non-peer relation

A **non-peer** relation is a relation from one application to another, where the applications support the same endpoint interface and have opposite `provides` / `requires` endpoint roles.

<!--
![relations](https://assets.ubuntu.com/v1/4f0eba09-juju-relations.png)
<br> *Example non-peer relation: The WordPress application with actual relations to MySQL and Apache and a potential relation to HAProxy, by virtue of the `wordpress` charm having a [`requires` endpoint that supports the `mysql` interface](https://charmhub.io/wordpress/integrations#db), compatible with `mysql`'s [`provides` endpoint supporting the same interface](https://charmhub.io/mysql/integrations#mysql), and a [`provides` endpoint that supports the `http` interface](https://charmhub.io/wordpress/integrations#website), compatible with `apache2`'s or `haproxy`'s `requires` endpoint supporting the same interface, among others.*
-->

(non-subordinate-relation)=
#### Non-subordinate relation

A **non-subordinate** relation (aka 'regular') is a {ref}`non-peer <non-peer-relation>` relation where the applications are both principal.

##### Non-cross-model relation

A **non-cross-model** relation is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on  the same model.


(cross-model-relation)=
##### Cross-model relation
> See also: {ref}`manage-relations`


A **cross-model** relation (aka 'CMR') is a {ref}`non-subordinate <non-subordinate-relation>` relation where the applications are on different models (+/- different controllers, +/- different clouds).

Cross-model relations enable, for example, scenarios where  your databases are hosted on bare metal, to take advantage of I/O performance, and your applications live within Kubernetes, to take advantage of scalability and application density.

If the network topology is anything other than flat, the Juju controllers will need to be bootstrapped with `--controller-external-ips`, `--controller-external-name`, or both, so that the controllers are able to communicate. Note that these config values can only be set at bootstrap time, and are read-only thereafter.

A cross-model relation has two sides: the offering side (aka "offerer") and the consume side (aka 'saas'). It does not make a difference which side of the relation (provider or requirer) is the offerer and which is the saas - the two are interchangeable. However, the endpoint type does influence on how juju sets up firewall rules: it is assumed that a requirer is the client and the provider is the server, so ports are opened on the provider side.

Note that application names are obfuscated (anonymised) to the offerer side:
- Applications that relate to the saas appear to the offerer as remote + token, e.g. `remote-76cd96ab50f146b284912afd1cc13a0e`.
- For the consumer, the remote app names is the saas name, e.g. `prometheus`.

(subordinate-relation)=
#### Subordinate relation

A **subordinate** relation is a {ref}`non-peer <non-peer-relation>` relation where one application is principal and the other subordinate.

A subordinate charm is by definition a charm deployed on the same machine as the principal charm it is intended to accompany. When you deploy a subordinate charm, it appears in your Juju model as an application with no unit. The subordinate relation helps the subordinate application acquire a unit. The subordinate application then scales automatically when the principal application does, by virtue of this relation.

<!--

CMR addresses the case where one may wish to centralise a service. This allows your models to become more targeted and can reduce the cloud resources they may require.  A common use case is to deploy [prometheus](https://charmhub.io/prometheus2) monitoring in a single, central model, and relate it to various data sources in other models hosting various workloads.

Some services that can benefit from central administration:

- Certificate Authorities, such as the `easyrsa` charm
- secret management, such as Vault
- logging and monitoring
- block storage management
- databases

Another use case would be when you are simply using different cloud types and wish to integrate the management of services across those different clouds.
-->

## Relation identification

A relation is identified by a **relation ID** (assigned automatically by Juju; expressed in monotonically increasing numbers) or a **relation key** (derived from the endpoints, format: `application1:[endpoint] application2:[endpoint]`).

## Permissions around relation databags

<!--The primary means for applications to communicate over a relation is using relation data.-->

![relation databag permissions](relation-databags.svg)

When an application becomes involved in a relation, each one of its units gets a databag in the Juju database, as follows:

- each unit gets a **unit databag**
- each application gets an **application databag**

While the relation is maintained,

- in a non-peer relation, whether regular or subordinate:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the local application databag;
    - all units of an application can read all of the remote application's databags.
- in a peer relation:
    - each unit can read and write to its own databag;
    - leader units can also read and write to the application databag;
    - all units can read all of the application's databags. That is, whether leader or not, every unit can read its own unit databag as well as every other unit's unit databag as well as the application databag.

```{important}

Note that, in peer relations, all permissions related to the remote application are turned inwards and become permissions related to the local application.

```
