---
myst:
  html_meta:
    description: "Juju agent reference: what an agent is, the machine, controller, unit, and model agents, the workers an agent runs, the engine report, and how an agent logs in, comes, and goes."
---

(agent)=
# Agent

```{ibnote}
See also: {ref}`Juju | Bootstrap a controller <bootstrap-a-controller>`, {ref}`Juju | Manage machines <manage-machines>`
```

An **agent** is a `jujuagentd` or `containeragent` process that works to realize the state that a {ref}`user <user>` declares with a {ref}`client <client>`: it logs in to the {ref}`controller <controller>` API, follows what changes, and runs the {ref}`workers <agent-workers>` that make the {ref}`cloud <cloud>` match the declared state. An agent acts for one entity (a {ref}`machine <machine>`, a {ref}`unit <unit>`, a {ref}`model <model>`, or a controller) and lives where that entity lives. An agent starts when {ref}`bootstrap <bootstrap-a-controller>` or provisioning creates its entity, and stops when the entity is removed.

(agent-kinds)=
## Agent kinds

An agent takes its name from the entity it acts for. The kinds of agent are:

(machine-agent)=
### Machine agent

A **machine agent** is the `jujuagentd` process of a {ref}`machine <machine>`. It runs the workers that keep the machine itself, such as `machiner`, which sets the machine's status and follows its life, and `host-key-reporter`, which publishes the machine's SSH host keys. The `deployer` worker of the machine agent starts one engine for each {ref}`unit <unit>` that runs on the machine. A machine agent runs only on machine clouds. When a machine is provisioned, cloud-init starts the agent as a systemd service named `jujuagentd-machine-<id>`. When the machine is dying or dead, the `machiner` worker ends the agent. The `jujuagentd` binary also provides `safe-mode` and `db-repl`, two variants of the machine agent with their own, smaller sets of workers.

(controller-agent)=
### Controller agent

A **controller agent** is the agent of a controller node (a machine or a pod that runs the {ref}`controller <controller>`). It runs the workers that keep the controller, which the {ref}`controller <controller>` page lists. It also starts one engine for each {ref}`model <model>` and one engine for each unit that runs on the node. On a machine cloud it is the machine agent of a controller machine. On a Kubernetes cloud it runs in the controller pod and logs in with the tag `controller-agent-<id>`.

{ref}`Bootstrapping a controller <controller-bootstrapping>` creates the first controller agent: cloud-init runs `jujuagentd bootstrap-state`, which creates the controller's records and its embedded {ref}`database <database>`, and then starts the agent. A controller with {ref}`high availability <high-availability>` runs one controller agent on each of its nodes. Removing a node stops its agent.

(unit-agent)=
### Unit agent

A **unit agent** is the agent that runs the {ref}`charm <charm>` of a {ref}`unit <unit>`. On a machine cloud it runs inside the machine agent's process, in the engine that the machine agent's `deployer` worker starts for it. The engine logs in with the unit's own password and serves the unit's own introspection socket. On a Kubernetes cloud it is the `containeragent` process in the unit's pod: the pod first runs `containeragent init`, which registers the pod with the controller, and then runs `containeragent unit` as a pebble service.

The unit agent's `uniter` worker reconciles the unit: it waits for a change in the unit's state on the controller, compares the change with the unit's local state, and runs the operation that brings the two together, which runs a {ref}`hook <hook>` of the charm. When a unit is removed, the engine that ran it stops.

(model-agent)=
### Model agent

A **model agent** is the engine that a controller agent runs for one {ref}`model <model>`: the workers that act on the model's records, which the {ref}`controller <controller>` page lists. The `model-worker-manager` worker of the controller agent starts one engine for each model that is active, and stops the engine when the model dies. On a Kubernetes cloud each model also has a **model operator**: a `jujuagentd` process in a pod in the model's Kubernetes namespace, which the controller provisions while the model is active. The model operator logs in with the model's own tag and acts as the model's representative inside the cluster.

(agent-workers)=
## Agent workers

A **worker** is a long-running process inside an agent that does one job, such as serving the API or starting machines. An agent starts its workers with a **dependency engine**, which starts, stops, and restarts each worker as the workers it needs come and go. A worker is declared as a **manifold**: the names of the workers it needs, the code that starts it, and what it offers to the workers that need it. The engine starts a worker only when every worker it needs is running. It stops the worker when one of them stops. A controller agent runs an engine for each model and for each unit beside its own engine. A machine agent runs an engine for each unit on the machine.

The state of an agent's workers is visible in the **engine report**. The report lists every worker of an agent by name, with its state (`starting`, `started`, `stopping`, or `stopped`), the names of the workers it needs, how many times the worker has been started, and the time it last started. An agent serves the report at the route `/depengine` of its introspection socket, a Unix socket named `introspection.socket` in the agent's directory. On a machine the agent also installs shell functions into `/etc/profile.d/juju-introspection.sh`, so that every shell on the machine has them: `juju_engine_report` prints the report of an agent on the machine. The `juju-introspect` command reads any route of any agent's socket. With `--listen`, it serves one socket over HTTP.

(agent-credentials)=
## Agent credentials

The credentials of an agent are its tag (the typed name of its entity) and the password that its configuration file holds. Bootstrap and provisioning write the password into the file. On its first login, the agent replaces the password with one it generates, and the old password stays as a fallback. Every call the agent makes is made as its entity, so a call that concerns another entity fails. No {ref}`access level <user-access-levels>` and no change block applies to an agent's calls: the controller checks a caller's access level only when the caller is a user. A change block (an operator setting that makes the controller refuse the calls that change a model) stops only the calls that a client makes.