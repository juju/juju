(diagrams)=
# Diagram POC — one model, every diagram, auto-layout vs declared

Everything we've built, from one file (`juju.ggarch`). Every diagram
shows as a **side-by-side pair**: the **synthesized** variant (zero
declared positions — the engine decides: typed hub planes, two-sided
fans, port discipline) on the left, the **declared arrangement** (the
author's `positions` sentence, ADR-007 refinement) on the right.
Caption meaning comes later: diagrams embedded in real docs carry
meaning captions; this page is the comparison catalogue. Click any
diagram to expand it.

## Intro: the problem

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: the problem (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The hub planes (user left, apps fanned from the controller) are the engine's typed-plane synthesis, not a declaration.
:alt: A user with direct "operates" arrows to three application instances, grouped by dashed boxes labelled on cloud 1 and on cloud 2.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: the problem
:no-legend:
:caption: Declared arrangement — the author's positions block. Compare with the synthesized variant: same story, engine-chosen geometry on the left.
:alt: A user with direct "operates" arrows to three application instances, grouped by dashed boxes labelled on cloud 1 and on cloud 2.
```
````
`````

## Intro: Juju enters

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The typed hub planes found the declared structure on their own: spine on one row, clouds side by side in the band above, Charmhub below on the axis, apps centred on the hub's row to the east. The app fan anchors at member centres; app2's arrow is align-middle'ed with the controller.
:alt: User and client on the left, controller in the centre, two cloud instances side by side above, Charmhub below, three charmed application instances in a column to the right with arrows converging into the controller's east face.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters (declared)
:no-legend:
:caption: Declared arrangement — user, client and controller on one horizontal plane; the cloud fan from the controller's mid-north, symmetric about the face midpoint; Charmhub below on the same axis; the app fan right, gap sized for the three "converges toward" labels. Synthesis fills the rest (ADR-007).
:alt: User, client, controller on one horizontal line, two cloud boxes fanned above the controller from its top face, Charmhub below it, three application boxes fanned to the right with converging arrows into the controller's right face.
```
````
`````

The authored original (both of the above derive from it):

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters
:no-legend:
:caption: The authored view — the arrangement as first drawn, with the model-membership boxes and record chips off (the 2026-09-20 reviewer call pending a placement design). The synthesized and declared variants above share this select exactly.
:alt: User, client, controller on one horizontal line, two cloud boxes fanned above, Charmhub below, three application boxes fanned right.
```

## Intro: Juju unpacked

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju unpacked (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The unit containers sit east of the controller as the feeder plane; the spine stays one row.
:alt: User, client, controller on the left. Three unit pod instances fanned to the right, each containing a unit agent and charm in a charm container, and Pebble and workload in a workload container. Each unit agent has watch and API arrows to the controller.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju unpacked
:no-legend:
:caption: Declared arrangement — the author's positions. Compare with the synthesized variant: same story, engine-chosen geometry on the left.
:alt: User, client, controller on the left. Three unit pod instances fanned to the right, each containing a unit agent and charm in a charm container, and Pebble and workload in a workload container.
```
````
`````

## Juju overview

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Juju overview (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The horizontal intent/execution chain from the engine's layering.
:alt: Horizontal chain: user, client, controller, agent, applications and clouds, charmhub.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Juju overview
:no-legend:
:caption: Declared arrangement — the full bootstrap-and-run overview as first drawn. Compare with the synthesized variant.
:alt: Horizontal chain: user, client, controller, agent, applications and clouds, charmhub.
```
````
`````

## K8s deployment topology

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: K8s deployment topology (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The controller pod and unit pod as containers; charmhub below.
:alt: Controller pod and unit pod side by side, each with their internal agents and containers, Charmhub below, API arrows between the pods.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: K8s deployment topology
:no-legend:
:caption: Declared arrangement — the re-authored K8s topology (the planned closing gate for the doc-track frictions). Compare with the synthesized variant.
:alt: Controller pod and unit pod side by side, each with their internal agents and containers, Charmhub below, API arrows between the pods.
```
````
`````

## Data model

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Data model (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The child-record stacking rule (charm under its application) is the typed hub planes' lone-sink spoke, not a declaration.
:alt: Six record nodes: charm above application, application connected to unit, unit connected to machine/pod, relation below application connected to endpoint.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Data model
:no-legend:
:caption: Declared arrangement — the FK portrait as first drawn. Compare with the synthesized variant.
:alt: Six record nodes: charm above application, application connected to unit, unit connected to machine/pod, relation below application connected to endpoint.
```
````
`````

## Data model (full spine)

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Data model (full spine) (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. Dense typed web: where the plane heuristics contradict, the view falls back to the plain depth layout — the honest fallback.
:alt: Two dashed boxes: controller database containing user, controller, cloud, credential, model records; model database containing application, charm, unit, machine, relation, endpoint records. Associations run in semantic direction with multiplicity labels.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Data model (full spine)
:no-legend:
:caption: Declared arrangement — the full record spine across both databases as first drawn, grounded in the schema DDL. Compare with the synthesized variant.
:alt: Two dashed boxes: controller database containing user, controller, cloud, credential, model records; model database containing application, charm, unit, machine, relation, endpoint records.
```
````
`````

## Worker tree (machine cloud)

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (machine cloud) (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared.
:alt: Vertical chain, top to bottom: controller, machine agent, unit agent, uniter, charm, each connected by control arrows labelled drives, hosts, runs hooks via, dispatches.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (machine cloud)
:no-legend:
:caption: Declared arrangement — the execution chain as first drawn. See [Worker tree (controller)](#worker-tree-controller) for the controller-side dependency engine. Compare with the synthesized variant.
:alt: Vertical chain, top to bottom: controller, machine agent, unit agent, uniter, charm, each connected by control arrows labelled drives, hosts, runs hooks via, dispatches.
```
````
`````

## Worker tree (controller)

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (controller) (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The typed hub planes restructure the dependency engine: spoke feeders fan east of their hub with RL arrows.
:alt: Five columns of worker boxes. Far left: provider tracker above provider services. Left: compute provisioner above model worker manager, both inside a dashed box labelled model workers (one set per model), undertaker below. Centre spine, top to bottom: agent, DB accessor, change stream, domain services, API server, HTTP server. Right: object store, lease manager below with primary election and lease expiry stacked above. Control arrows connect consumers to providers; the change stream watches the DB accessor.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (controller)
:no-legend:
:caption: Declared arrangement — the controller's dependency engine, grounded in cmd/jujud-controller/agent/{machine,model}/manifolds.go. Every node carries a ground pointer to its manifold source. Compare with the synthesized variant.
:alt: Five columns of worker boxes. Far left: provider tracker above provider services. Left: compute provisioner above model worker manager, both inside a dashed box labelled model workers (one set per model), undertaker below. Centre spine, top to bottom: agent, DB accessor, change stream, domain services, API server, HTTP server. Right: object store, lease manager below with primary election and lease expiry stacked above. Control arrows connect consumers to providers; the change stream watches the DB accessor.
```
````
`````

## Cross-model relation (CMR)

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR) (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared.
:alt: Nine record nodes. Top row: application, offer, offer connection. Middle row: relation, endpoint, remote application. Bottom: model and external controller. A dashed box around offer, offer connection, and external controller is labelled cross-model machinery.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR)
:no-legend:
:caption: Declared arrangement — the cross-model machinery as first drawn, all tables grounded (offer, offer_connection, application_remote_offerer, external_controller). Compare with the synthesized variant.
:alt: Nine record nodes. Top row: application, offer, offer connection. Middle row: relation, endpoint, remote application. Bottom: model and external controller. A dashed box around offer, offer connection, and external controller is labelled cross-model machinery.
```
````
`````

## HA controller: Dqlite replicaset

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared.
:alt: Three controller node instances side by side, each containing a controller agent and a Dqlite database node. A dashed annotation box encloses all three Dqlite nodes, labelled "Raft replicaset (strongly consistent)". Dashed arrows run between every pair of Dqlite nodes — the full mesh of Raft sync.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset
:no-legend:
:caption: Declared arrangement — the three controller nodes with the cross-container Raft box (the per-side padding and inside-bottom label placement demo). Compare with the synthesized variant.
:alt: Three controller node instances side by side, each containing a controller agent and a Dqlite database node. A dashed annotation box encloses all three Dqlite nodes, labelled "Raft replicaset (strongly consistent)".
```
````
`````

## Sequences

### juju status

```{ggarch}
:file: ../juju.ggarch
:sequence: juju status
:no-legend:
:caption: What "juju status" actually is: the controller reads the status records, derives live agent liveness from connection state, and projects both back. Status is records plus liveness — an overlap, not an identity.
:alt: User calls juju status. Client sends a Status API call to the controller. The controller reads status records and derives agent liveness, then returns the projected status. Client shows the status output to the user.
```

### Uniter operation (state machine)

```{ggarch}
:file: ../juju.ggarch
:view: Uniter operation
:no-legend:
:caption: The uniter's three-phase operation executor — idle → preparing → executing → committing — with the error path (hook fails) and the retry loop. The state labels are the verbatim strings from internal/worker/uniter/operation/executor.go; guards: ErrHookFailed, ErrNeedsReboot.
:alt: State machine: idle to preparing on hook queued, preparing to executing, executing to committing on hook exits 0, executing to error on hook fails, error to idle on retry, committing to idle on write complete.
```

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

### Deploy K8s

```{ggarch}
:file: ../juju.ggarch
:sequence: Deploy K8s
:no-legend:
:caption: Deploying to Kubernetes is a two-phase handoff: the controller writes intent into the database and schedules the pod, then the unit agent (containeragent) takes over and drives the charm lifecycle independently.
:alt: User calls juju deploy. Client sends Deploy RPC to Controller. Controller writes records and schedules pod on Kubernetes. K8s returns pod running. Controller starts containeragent. containeragent runs install, config-changed, start hooks and returns unit active. Controller signals deploy complete back to Client and User.
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

## Tutorial: the progressive reveal

The tutorial's figures reveal one element per section, the way the
page teaches: the machinery first, then the user, then the
applications. Each step is a scoped view over the same model.

### Tutorial: setup

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: setup
:no-legend:
:caption: What Juju consists of before anything runs: a client and a controller, with access to a cloud (compute, networking, storage) and to Charmhub. The arrows name what each connection carries.
:alt: A client talks to the controller; the controller talks to clouds above it and to Charmhub below it.
```

### Tutorial: auth

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: auth
:no-legend:
:caption: The reveal adds the user; everything else stays. Everything a user does in Juju is commands sent through the client to the controller, which authenticates and authorizes them.
:alt: The user sends commands to the client, the client calls the Juju API on the controller, and the controller still talks to the clouds above it and to Charmhub below it.
```

### Tutorial: provision & deploy

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: provision & deploy
:no-legend:
:caption: The reveal adds the charmed applications: the controller provisions infrastructure on the cloud and fetches charms from Charmhub; the applications record their state back to the controller.
:alt: The user sends commands through the client to the controller; the controller provisions on clouds and fetches charms from Charmhub; the charmed applications record their state on the controller.
```

## Reference: mechanisms (round 3 — grounded new views)

### Secret lifecycle (state machine)

```{ggarch}
:file: ../juju.ggarch
:view: Secret lifecycle
:no-legend:
:caption: The life of a secret, grounded in domain/secret: reserved (URI minted) -> active (latest revision, content in a backend) -> granted (view | manage roles) -> superseded; a rotate policy fires secret-rotate (leader), expiry fires secret-expired; a revision no consumer tracks becomes obsolete (pending delete) and the owner charm retires it via secret-remove (or user secrets auto-prune). Consumers see secret-changed.
:alt: State machine: reserved to active on create, active self-loops for grant/revoke and new-revision publication, active to rotate-due on the rotate policy and back via secret-rotate, active to expiry-due and on to removed via secret-expired then secret-remove, active to obsolete when superseded, obsolete to removed on prune.
```

### Action run flow (sequence)

```{ggarch}
:file: ../juju.ggarch
:sequence: Action run flow
:no-legend:
:caption: juju run enqueues an operation; the controller records per-unit tasks (pending) and the unit agent's watcher resolves them; the task runs via the charm's dispatch script (action-get/set/fail/log during execution), and finishing stores results in the object store. juju cancel-task moves a running task to aborting; the process is killed and the task reports aborted.
:alt: User calls juju run; client enqueues the operation on the controller; controller records operation and per-unit tasks pending; controller notifies unit agent; agent resolves and starts the task (running); agent runs the charm action with jujuc action commands; on cancel the agent aborts; otherwise results stream back and the task completes.
```

### Status domains (who sets what)

```{ggarch}
:file: ../juju.ggarch
:view: Status domains
:no-legend:
:caption: Who sets each status domain: the unit agent sets its own status (the controller derives allocating and lost); the charm sets the workload status via status-set; the leader unit sets the application status via status-set --application, else Juju computes it from the unit statuses; the machine agent sets the machine status. Transitions are free-form enumerations except relation and storage (enforced machines).
:alt: Actor nodes pointing at the status domains they set: charm to workload status, unit agent to unit agent status, machine agent to machine status, controller to relation status.
```

### Agent taxonomy (who runs what)

```{ggarch}
:file: ../juju.ggarch
:view: Agent taxonomy
:no-legend:
:caption: The four agent types and their channels: every agent makes API calls to the controller; the machine agent hosts unit agents on machine clouds; containeragent is the unit-agent role as a single Kubernetes binary.
:alt: Controller, machine agent, unit agent, and containeragent in a row; arrows: machine agent hosts unit agent; each agent makes API calls to the controller.
```

### Log flow (sequence)

```{ggarch}
:file: ../juju.ggarch
:sequence: Log flow
:no-legend:
:caption: Agents buffer their log records in memory and ship them to the controller's /logsink websocket endpoint; the controller batches them as JSON lines into logsink.log, which juju debug-log tails through the API. On Kubernetes, agent logs also go to the container's stdout.
:alt: Unit agent and machine agent buffer records and ship them to the controller; the controller batches them into logsink.log; the user tails via juju debug-log.
```



## Reference: data models (round 4 — grounded data-model views)

### Storage model

```{ggarch}
:file: ../juju.ggarch
:view: Storage model
:no-legend:
:caption: The storage walk, grounded in 0011-storage.sql: the charm defines storage names (kind block|filesystem, count, size); a directive pins one pool (user- or provider-default origin) per application; an instance carries the charm name, kind and requested size and is backed by exactly one volume or filesystem; attachments bind instances to units; volumes bind to net nodes (the machine or unit network identity). Provision scope: model = machine-independent, machine = dies with the machine.
:alt: Record chain: charm storage to directive to pool to instance; volume to the right of instance, filesystem below, attachment below charm storage, net node above volume. Arrows carry multiplicities.
```

### Network spaces

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:no-legend:
:caption: A space groups subnets; a subnet belongs to 0..1 space (the alpha space exists by default); an application's default binding points at one space, and each charm-relation endpoint can bind 0..1 space of its own.
:alt: Application record to space record to subnet record; arrows: subnet belongs to 0..1 space; application default binding (one).
```

### Databag permissions

```{ggarch}
:file: ../juju.ggarch
:view: Databag permissions
:no-legend:
:caption: The relation databags, grounded in domain/relation: the unit databag stores the writing unit, so a unit writes only its own bag and reads every unit bag in the relation (peers and remotes); the application databag is keyed by the relation ENDPOINT and its writes are leader-gated. Remote applications read the local app bag's counterpart on their own side; users never touch databags directly.
:alt: Own unit writes its unit databag; peer and remote units read all unit databags; the leader unit reads and writes the application databag.
```

## Where each view is embedded (round 1 of the docs push)

Every diagram above is duplicated here; the list below maps the views
to the doc pages that now embed them, so the effects of ggarch
updates are inspectable from this one page.

| View / sequence | Embedded in |
|---|---|
| Intro: the problem | explanation/juju-architecture.md |
| Intro: Juju enters | explanation/juju-architecture.md |
| Intro: Juju unpacked | explanation/juju-architecture.md |
| Juju overview | explanation/architecture.md |
| K8s deployment topology | explanation/architecture.md; **reference/containeragent.md**, **reference/jujuc.md**, **reference/pebble.md** |
| Data model | explanation/architecture.md |
| Data model (full spine) | **reference/database.md** |
| Worker tree (machine cloud) | **reference/jujud.md** |
| Worker tree (controller) | **reference/controller.md** |
| Cross-model relation (CMR) | **reference/offer.md** |
| HA controller: Dqlite replicaset | **reference/database.md**, **reference/high-availability.md**, **howto/manage-the-databases.md** |
| Uniter operation (state machine) | **reference/hook.md** |
| Hook execution | explanation/architecture.md |
| Bootstrap K8s / Bootstrap machine / Deploy K8s / Deploy machine / Integrate | explanation/architecture.md |
| Unit removal | explanation/architecture.md; **reference/removing-things.md** |
| Model removal | explanation/architecture.md; **reference/removing-things.md** |
| juju status | this catalogue |

New tutorial views (all three also embedded in **tutorial/index.md**,
replacing the excalidraw pairs -- the progressive reveal: setup, then
+user, then +applications):
| View | Embedded in |
|---|---|
| Tutorial: setup | tutorial/index.md (replaces tutorial-setup excalidraw) |
| Tutorial: auth | tutorial/index.md (replaces tutorial-handle-auth excalidraw) |
| Tutorial: provision & deploy | tutorial/index.md (replaces tutorial-provision-deploy excalidraw) |

Pages still on hand-drawn visuals, pending round-2 views:
reference/relation.md (relation taxonomy + databags excalidraws),
reference/hook.md (hook-charm-lifecycle PNG — the
Uniter operation machine above now covers its execution story).
