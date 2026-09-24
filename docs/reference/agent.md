---
myst:
  html_meta:
    description: "Understand Juju agents: controller, machine, unit, and model agents that manage deployments through jujud and containeragent processes. The agent identity, types, operations, and watchers."
---

(agent)=
# Agent

In Juju, an **agent** is a {ref}`jujud` / {ref}`containeragent` process that works to realise the state declared by a Juju end-user with a Juju client (e.g., {ref}`the juju CLI <juju-cli>`) for a Juju entity (e.g., {ref}`controller <controller>`, {ref}`model <model>`, {ref}`machine <machine>`, {ref}`unit <unit>`) via {ref}`workers <worker-cont>`.

On machines, an agent is managed by `systemd`.

(the-agent-record)=
## The agent record

An agent is a process, not a database record: there is no agent table.
The records are the **served entity's** agent satellites -- the
agent's password, its start time, the version it reported as running,
and its presence (the last time it was seen) -- plus the model's
agent-version record, the version the model's agents *should* run and
the latest one known (see
{ref}`the model's agent version <the-model-record>`). What identifies
an agent is its **tag**: the entity it serves -- a machine's number, a
unit's name, an application's name, or `controller-0` for the
controller agent.

(types-of-agents)=
## Types of agents

An agent's kind is the entity it serves, and the kinds are exclusive
by construction -- one process serves one entity. The `jujud` binary
registers exactly two agent commands -- the **model agent** and the
**machine agent** -- and the other kinds are roles those run: the
machine agent starts a unit agent (the Uniter worker) per unit it
hosts, and the model agent runs a model's workers; on Kubernetes,
`containeragent` is the unit agent as a single binary.

```{ggarch}
:file: ../juju.ggarch
:view: Agent taxonomy
:no-legend:
:caption: Taxonomy tree: The four agent types and their channels: every agent makes API calls to the controller; the machine agent hosts unit agents on machine clouds; containeragent is the unit-agent role as a single Kubernetes binary.
:alt: Controller, machine agent, unit agent, and containeragent in a row; each agent makes API calls to the controller; the machine agent hosts the unit agent.
```


(controller-agent)=
### Controller agent

On machine and Kubernetes clouds, a `jujud` process running workers responsible for a {ref}`controller <controller>`. This includes, among others, the `apiserver` worker, which is responsible for running the Juju API server.

```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (controller)
:alt: The controller agent's worker tree: a Dqlite hub at the centre with workers arranged around it — API server, domain services, object store, lease manager, provider services, change stream, provisioner and others, each with the verb that names what it does.
:caption: Topology: Inside the controller agent: its workers, arranged around the embedded Dqlite database each of them reads and writes. The API server fronts the {ref}`juju-cli` and the {ref}`unit-agent`; the domain services own models; the provider tracker mediates every cloud call.
```

(machine-agent)=
### Machine agent

On machine clouds, a `jujud` process running workers responsible for a {ref}`machine <machine>`.

(model-agent)=
### Model agent

On machine and Kubernetes clouds, a `jujud` process running workers responsible for all the {ref}`models <model>` associated with a given controller.

(unit-agent)=
### Unit agent

On machine / Kubernetes clouds, a `jujud` / `containeragent` process responsible for a {ref}`unit <unit>`.

When a Juju user uses the client (e.g., types a command in the CLI), this goes to the controller agent's `apiserver`, which passes it on to the database. The database runs a background process that checks if anything has changed and, if so, emits an event (think "I've seen something that's changed. Do you care about it?"). The event cascades through Juju. The unit agent becomes aware of it by always polling the controller agent as part of a reconciliation loop trying to reconcile the unit agent's local state to the remote state on the controller (i.e., the state in the controller's database).

(the-agent-in-the-data-model)=
## The agent in the data model

The agent's records hang off the entities: the machine and the unit
each carry an agent-version record (the version that agent reported
running) and the machine a presence record (the last time its agent
was seen); the machine's and the controller node's passwords are
agent credentials; and the model database carries the model-level
agent-version singleton -- the target and latest versions, with the
agent stream they come from (released, proposed, testing, devel).

(the-agent-states)=
## Agent states

An agent has no state machine: it runs or it does not. Its liveness
is *presence* -- the controller records the agent's last login -- and
a silent agent reads as `lost` in the status projections, a display
rule computed on read (see {ref}`unit status <unit-status>` and the
{ref}`Status domains <status>` view).

(the-agent-operations)=
## Agent operations

### Starting agents

The machine agent starts with its machine (via `systemd`) and the
model agent with the model's workers; the machine agent starts a unit
agent per unit it hosts. Bootstrap creates the first agent -- the
controller's -- with a one-time nonce (see
{ref}`controller bootstrap <controller-bootstrap>`).

### Agent version targeting and reporting

The model's target agent version (and its stream) is what an upgrade
sets; each agent reports the version it actually runs into its
entity's agent-version record, and the controller's nodes do the same
controller-side (see {ref}`upgrading things <upgrading-things>`).

(the-agent-watchers)=
## Agent watchers

Agents are the *consumers* of nearly every watch surface in the model
-- each domain's watchers exist so an agent can reconcile without
polling (see the per-entity watcher sections, for example
{ref}`machine watchers <machine-watchers>` and
{ref}`application watchers <the-application-watchers>`). What is
watched *about* an agent is its presence: the controller's agent
presence machinery records agent logins, and that is the input the
`lost` display rule reads.

(the-agent-rules-and-errors)=
## Agent rules and errors

- an agent's identity is its tag, and the API authenticates the tag
  kinds separately -- machine, unit, application and controller agents
  each have their own authentication class;
- the controller agent's tag is fixed (`controller-0`); bootstrap
  requires the matching one-time nonce;
- the machine agent's password is unique across machines -- one agent
  cannot impersonate another.

(related-entities-agent)=
## Related entities

- **The controller, models, machines and units** are what agents
  serve -- one process per entity (see {ref}`controller <controller>`,
  {ref}`model <model>`, {ref}`machine <machine>`,
  {ref}`unit <unit>`).
- **`jujud` and `containeragent`** are the agent binaries -- the
  former for machines and controllers, the latter the unit agent's
  Kubernetes form (see {ref}`jujud <jujud>`,
  {ref}`containeragent <containeragent>`).
- **Workers** are what an agent runs; the worker tree is the agent's
  actual content (see {ref}`workers <worker-cont>`).
- **The agent version** is the upgrade machinery's target (see
  {ref}`upgrading things <upgrading-things>`).
- **Status** derives an agent's liveness: presence in, `lost` out
  (see {ref}`unit status <unit-status>`).
