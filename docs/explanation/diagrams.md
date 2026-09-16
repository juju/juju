(diagrams)=
# juju.ggarch — diagram preview

All views produced by `juju.ggarch`.

## Diagrams

### Intro: the problem

```{ggarch}
:file: ../juju.ggarch
:view: Intro: the problem
:no-legend:
:caption: Without Juju, every application is its own island — the operator must reach across cloud and topology boundaries manually.
:alt: A user with direct "operates" arrows to three separate bare applications spread across two clouds.
```

### Intro: Juju enters

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters
:no-legend:
:caption: Juju inserts a single controller between the operator and everything else. The operator stops touching apps directly; the controller becomes the single point of intent.
:alt: Client and User on the left, Controller in the centre, two clouds fanned above, Charmhub below, and three charmed apps with provenance chips fanned to the right.
```

### Intro: Juju unpacked

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju unpacked
:no-legend:
:caption: Each unit runs its own agent that watches the controller for changes, then drives its local charm and workload into the desired state. No agent ever talks to another directly.
:alt: Controller on the left. Three unit containers to the right, each enclosing a unit agent, charm, and workload. Every unit agent has a watch arrow back to the controller.
```

### Juju overview

```{ggarch}
:file: ../juju.ggarch
:view: Juju overview
:no-legend:
:caption: The intent/execution split: commands flow left-to-right from operator to controller; agents pull their mandate from the controller and push it into the real world. The controller knows the clouds and the charm store; agents know only the controller.
:alt: Horizontal chain: User, Client, Controller, Agents, Charmed applications. Clouds above the Controller. Charmhub below.
```

### K8s deployment topology

```{ggarch}
:file: ../juju.ggarch
:view: K8s deployment topology
:no-legend:
:caption: On Kubernetes every Juju component is a container. The controller agent (jujud) manages the cluster; charm code drives Pebble, which owns the workload process. The unit agent is the only path between charm and controller.
:alt: Controller pod on the left containing jujud. Unit pod on the right with charm container (unit agent and charm) and workload container (Pebble and workload). Kubernetes cloud above, Charmhub below.
```

### Data model

```{ggarch}
:file: ../juju.ggarch
:view: Data model
:no-legend:
:caption: The controller's database is the single source of truth for the entire deployment. Every runtime entity — application, unit, machine, charm, relation — has a record here; what you see in "juju status" is this model projected back at you.
:alt: Five record nodes: charm at top connected to application, application connected to unit, unit connected to machine/pod, relation connected to application.
```

## Sequences

### Hook execution

```{ggarch}
:file: ../juju.ggarch
:sequence: Hook execution
:no-legend:
:caption: Every hook runs the same cycle: the controller notifies, the agent snapshots remote state, resolves the next hook, and dispatches. Hook commands are served locally by the agent acting as a proxy — the charm never calls the controller directly.
:alt: API server fires watcher to unit agent. Unit agent snapshots state and resolves hook. Loop: charm calls hook command, unit agent proxies it to API server. On success: flush writes. On failure: discard writes, set unit error.
```

### Bootstrap K8s

```{ggarch}
:file: ../juju.ggarch
:sequence: Bootstrap K8s
:no-legend:
:caption: Bootstrapping on Kubernetes is mostly client-side orchestration: the CLI authenticates, schedules the controller pod, and then waits. Once jujud declares the API ready, the controller is fully autonomous.
:alt: User calls juju bootstrap. Client authenticates with K8s and creates the controller pod namespace. Controller pod self-starts jujud, the API server, and the database. Controller pod signals API ready to Client. Client reports success to User.
```

### Bootstrap machine

```{ggarch}
:file: ../juju.ggarch
:sequence: Bootstrap machine
:no-legend:
:caption: On a machine cloud the client reaches into the provisioned VM to seed the configuration, then hands off. The controller machine bootstraps itself from there — the client is not involved once jujud starts.
:alt: User calls juju bootstrap. Client authenticates with Cloud and provisions a VM. Client installs jujud and seeds config on the controller machine. Controller machine self-starts the controller agent, API server, and database, then signals API ready. Client reports success to User.
```

### Deploy K8s

```{ggarch}
:file: ../juju.ggarch
:sequence: Deploy K8s
:no-legend:
:caption: Deploying to Kubernetes is a two-phase handoff: the controller writes intent into the database and schedules the pod, then the unit agent (containeragent) takes over and drives the charm lifecycle independently.
:alt: User calls juju deploy. Client sends Deploy RPC to Controller. Controller writes records and schedules pod on Kubernetes. K8s returns pod running. Controller starts containeragent. containeragent runs install, config-changed, start hooks and returns unit active. Controller signals deploy complete back to Client and User.
```

### Deploy machine

```{ggarch}
:file: ../juju.ggarch
:sequence: Deploy machine
:no-legend:
:caption: On a machine cloud the controller provisions the VM and starts jujud on it. From that point the unit agent drives its own lifecycle; the controller only watches.
:alt: User calls juju deploy. Client sends Deploy RPC to Controller. Controller writes records and provisions machine via Cloud. Controller starts jujud unit agent. jujud runs install, config-changed, start hooks and reports unit active. Controller signals deploy complete back to Client and User.
```

### Integrate

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:caption: Relation data never flows between units directly. The controller holds all relation bags and notifies each unit agent when the bag it reads changes. Units write to the controller; the controller fans out the change.
:alt: User calls juju integrate. Client sends Integrate RPC to Controller. Controller writes relation record and fires watchers to both unit agents. Each agent runs relation-created, relation-joined, and relation-changed hooks and writes its relation data to the controller. The controller notifies the other agent after each write.
```

### Unit removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Unit removal
:no-legend:
:caption: Removal is a cooperative shutdown: the controller marks the unit Dying, the unit agent runs its teardown hooks in order, then marks itself Dead. Only after that does the controller release the underlying machine.
:alt: User calls juju remove-unit. Controller marks unit Dying and fires watcher to unit agent. Unit agent runs stop, teardown, and remove hooks, then marks unit Dead. Controller releases machine and deletes unit records.
```

### Model removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Model removal
:no-legend:
:caption: Model destruction is coordinated by the Undertaker, a worker that runs inside the controller agent. The controller never deletes its own database — the Undertaker does, as the final act after all cloud resources have been released.
:alt: User calls juju destroy-model. Controller marks model Dying and fires watcher to Undertaker. Undertaker destroys all applications. Controller releases all machines and marks model Dead. Undertaker deletes model records and Dqlite database.
```
