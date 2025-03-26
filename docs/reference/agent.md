(agent)=
# Agent


In Juju, an **agent** is a {ref}`jujud` / {ref}`containeragent` process that works to realise the state declared by a Juju end-user with a Juju client (e.g., {ref}`the Juju CLI <juju-cli>`) for a Juju entity (e.g., {ref}`controller <controller>`, {ref}`model <model>`, {ref}`machine <machine>`, {ref}`unit <unit>`) via {ref}`workers <worker>`. 

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

<!--
The unit agent has  a local state and a remote state (or controller state, e.g., state in the controller's database) and is always running a reconciliation loop to make the local state catch up to the remote state. When
-->
<!--

Agents run trees of [workers](https://juju.is/docs/dev/worker). These may have different areas of responsibility, at the level of a {ref}`machine <machine>`, a {ref}`unit <unit>`, or a {ref}`controller <controller>`. As such, agents are sometimes also classified in those terms, that is, into *machine* agents, *unit agents*, and *controller* agents.

While machine agents are in charge of machine deployments and container agents are in charge of Kubernetes deployments, the trees of workers they run are analogous, and some of the workers they use are also the same.
-->
<!--
A machine agent has 4 types of worker trees / responsibilities;

1. machine (always)
2. controller (if it’s a controller agent, that is, if the machine agent has controller responsibility, represented by a tree of workers)
3. model (if it’s a controller agent, that is, if the machine agent has controller responsibility, represented by a tree of workers)
4. unit (if there are units deployed)
-->

<!--
For a Kubernetes deployment, container agent is a dependency engine that is placed in a sidecar container to operate a unit in Kubernetes. it has no parents.

For a non-Kubernetes deployment, unit agent dependency engine sits under a machine agent
-->





<!--FROM A PREVIOUS DRAFT:
There are three types of agents in Juju: controller agents, unit agents and machine agents. An agent performs different roles depending on its type:

- A controller agent is responsible for running a Juju controller node.
- A unit agent is responsible for managing the lifecycle of an application’s unit running on the machine or the k8s pod.
- A machine agent manages its respective unit agents. In particular, it is the machine agent that creates unit agents for deployed application units. A machine agent also manages any containers that may be requested on that machine and also the resources on the machine, for example, storage for application units.
-->




<!--CONTENT FROM https://discourse.charmhub.io/t/juju-logs/1184#heading--juju-agents . REMOVED IT FROM THERE AS IT DIDN'T BELONG---THE INFO DESCRIBES THE NOTION OF AGENT, THERE IS NOTHING IN IT SPECIFIC TO LOGGING.
## Directory

There is one agent for every Juju machine and unit. For instance, for a machine with an id of '2', we see evidence of such agents:

``` bash
juju ssh 2 ls -lh /var/lib/juju/agents
```

This example has the following output:

``` bash
drwxr-xr-x 2 root root 4.0K Apr 28 00:42 machine-2
drwxr-xr-x 4 root root 4.0K Apr 28 00:42 unit-nfs2-0
```

So there are two agents running on this machine. One for the machine itself and one for a service unit.

The contents of one of these directories

``` bash
juju ssh 2 ls -lh /var/lib/juju/agents/machine-2
```

reveals the agent's configuration file:

``` bash
-rw------- 1 root root 2.1K Apr 28 00:42 agent.conf
```

-->
