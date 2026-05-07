---
myst:
  html_meta:
    description: "Understand Juju agents: controller, machine, unit, and model agents that manage deployments through jujud and containeragent processes."
---

(agent)=
# Agent

In Juju, an **agent** is a {ref}`jujud` / {ref}`containeragent` process that works to realise the state declared by a Juju end-user with a Juju client (e.g., {ref}`the juju CLI <juju-cli>`) for a Juju entity (e.g., {ref}`controller <controller>`, {ref}`model <model>`, {ref}`machine <machine>`, {ref}`unit <unit>`) via {ref}`workers <worker>`.

On machines, an agent is managed by `systemd`.

## Types of agents

(controller-agent)=
### Controller agent

On machine and Kubernetes clouds, a `jujud` process running workers responsible for a {ref}`controller <controller>`. This includes, among others, the `apiserver` worker, which is responsible for running the Juju API server.

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

