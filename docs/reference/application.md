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

(application-lifecycle)=
## Application lifecycle

(application-deployment)=
### Application deployment

Deploying an application adds its software to a {ref}`model <model>` and arranges for it to run on infrastructure. The intent -- the application name, the {ref}`charm <charm>`, the {ref}`constraints <constraint>` -- goes to the {ref}`controller <controller>` as an RPC call. The controller writes the application and {ref}`unit <unit>` records to the {ref}`database <database>`, then asks the cloud for resources (a virtual machine or a pod). Once the resource is ready the controller starts the {ref}`unit agent <unit-agent>`, which runs the install sequence: `install`, `config-changed`, `start`.

The mechanism and the state it leaves, per cloud type:

:::::{tab-set}

::::{tab-item} Kubernetes

```{ggarch}
:file: ../juju.ggarch
:slides: Data model | Deploy K8s | K8s deployment topology | juju status
:caption: Deploying on Kubernetes: the records, the mechanism that creates them, the topology that results, and the command that verifies it.
:slide-captions: Entity relationship diagram: The seed: the records a deployment consists of -- charm, application, unit, machine/pod, relation, endpoint -- and where the pointers live. | Sequence diagram: The mechanism: the controller writes the application and unit records, schedules the unit pod, and starts the containeragent, which runs the install hooks to unit active. | Topology: The result: the settled topology -- the controller pod and the unit pod, each with their internal agents and containers. | Sequence diagram: The verification: juju status projects exactly those records plus live agent liveness -- what you just deployed is what status reads.
:alt: The data model records (charm, application, unit, machine/pod, relation, endpoint). Then: user invokes juju deploy; controller writes records and schedules the unit pod; containeragent runs the install hooks and reports active. The resulting topology: controller pod and unit pod with their internal agents and containers. Verification: juju status reads those records plus live agent liveness.
```

::::

::::{tab-item} Machines

```{ggarch}
:file: ../juju.ggarch
:slides: Data model | Deploy machine | Machine deployment topology | juju status
:caption: Deploying on a machine cloud: the records, the mechanism that creates them, the topology that results, and the command that verifies it.
:slide-captions: Entity relationship diagram: The seed: the records a deployment consists of -- charm, application, unit, machine/pod, relation, endpoint -- and where the pointers live. | Sequence diagram: The mechanism: the controller writes the application and unit records, asks the cloud to provision a machine, and starts jujud, which runs the install hooks to unit active. | Topology: The result: one jujud per machine -- the controller machine's jujud runs the controller with Dqlite in-process; the unit machine's jujud hosts the unit agent, which runs the charm, which drives the workload directly (no Pebble on machine clouds). | Sequence diagram: The verification: juju status projects exactly those records plus live agent liveness -- what you just deployed is what status reads.
:alt: The data model records (charm, application, unit, machine/pod, relation, endpoint). Then: user invokes juju deploy; controller writes records and provisions a machine; jujud runs the install hooks and reports active. The resulting topology: controller machine and unit machine, one jujud per machine. Verification: juju status reads those records plus live agent liveness.
```

::::

:::::

```{ibnote}
See more: {ref}`manage-applications`
```

(application-endpoint)=
## Application endpoint

In Juju, an application **endpoint** is a struct defined in an {ref}`application <application>`'s {ref}`charm <charm>`'s `metadata.yaml` / (since Charmcraft 2.5) `charmcraft.yaml` consisting of
- a name (charm-specific),
- a role (one of `provides`, `requires` = 'can use', or `peers`), and
- an interface

whose purpose is to help define a {ref}`relation <relation>`.

For example, the MySQL application deployed from the `mysql` charm has an endpoint called `mysql` with role `provides` and interface `mysql` and this can be used to form  a {ref}`regular relation <regular-relation>` relation with WordPress.

```{ibnote}
See more: [GitHub | `mysql-operator` > `metadata.yaml`](https://github.com/canonical/mysql-operator/blob/2bd2bcc65590937dab18d1d9b0fe21a445557bb6/metadata.yaml#L35), [Charmhub | `mysql`](https://charmhub.io/mysql/integrations#mysql)
```

All charms have an implicit (not in their `metadata.yaml` / `charmcraft.yaml`) endpoint with name `juju-info`, interface `juju-info`, and role `provides`. This endpoint can be used to form {ref}`subordinate relations <subordinate-relation>` with subordinate charms that have an explicit endpoint with interface `juju-info` and role `requires`. See {ref}`the-implicit-juju-info-relation-endpoint` for details. Examples: [`ntp`](https://charmhub.io/ntp/integrations#juju-info), [`mysql-router`](https://charmhub.io/mysql-router/integrations#juju-info)
