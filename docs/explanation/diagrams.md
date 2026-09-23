(diagrams)=
# Diagram catalogue — one model, every diagram, grouped by target doc

Everything we've built, from one file (`juju.ggarch`), grouped by the
doc each diagram is going to be inserted into: an h2 per doc kind
(tutorial, reference, explanation), an h3 per doc page, and under it
the diagrams with **the section of that page where each goes** (the
staging convention: a diagram belongs to the doc section that owns its
topic — e.g. the bootstrap sequences under
`reference/controller.md` → *Controller bootstrap*). An entity
reference page may carry several layers — structure, mechanism,
lifecycle: `reference/unit.md`'s *Unit removal* section is the unit's
lifecycle layer, not part of its structure story. Lifecycle sections
are named for the event, entity-scoped (*Unit removal*, *Relation
creation*) — activity titles (*Integrating applications*) stay in the
how-tos; when a page carries ≥2 lifecycle layers they group under an
`<Entity> lifecycle` umbrella with event-named subsections (the
`secret.md` precedent: *Secret lifecycle* → *Charm-secret lifecycle* /
*User-secret lifecycle*). The umbrella is content-triggered — never an
empty scaffold. Diagrams with no
home yet sit in **Other** at the bottom.

Every entry shows as a **side-by-side pair**: the **synthesized**
variant (zero declared positions — the engine decides: typed hub
planes, two-sided fans, port discipline) on the left, the **declared
arrangement** (the author's `positions` sentence, ADR-007 refinement)
on the right — except sequence/state views, which render single.
Caption meaning comes later: diagrams embedded in real docs carry
meaning captions; this page is the catalogue. Click any diagram to
expand it.

The per-doc readout (the point of this grouping): the tutorial carries
the 3-step progressive reveal; the explanation pages carry the
architecture narrative (Intros), the living architecture page
(overview + topology + control flow + data model + the five
mechanism sequences), and the principles drafts carry the topology
demos; the reference pages carry 23 grounded views across 23 of 48
concept pages — one mechanism, data model, or process per page.

## Tutorial

### tutorial/index.md

#### Tutorial: setup

**Insert at:** § Set up Juju.

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: setup
:no-legend:
:caption: What Juju consists of before anything runs: a client and a controller, with access to a cloud (compute, networking, storage) and to Charmhub. The arrows name what each connection carries.
:alt: A client talks to the controller; the controller talks to clouds above it and to Charmhub below it.
```

#### Tutorial: auth

**Insert at:** § Handle authentication and authorization.

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: auth
:no-legend:
:caption: The reveal adds the user; everything else stays. Everything a user does in Juju is commands sent through the client to the controller, which authenticates and authorizes them.
:alt: The user sends commands to the client, the client calls the Juju API on the controller, and the controller still talks to the clouds above it and to Charmhub below it.
```

#### Tutorial: provision & deploy

**Insert at:** § Provision infrastructure and operate applications.

```{ggarch}
:file: ../juju.ggarch
:view: Tutorial: provision & deploy
:no-legend:
:caption: The reveal adds the charmed applications: the controller provisions infrastructure on the cloud and fetches charms from Charmhub; the applications record their state back to the controller.
:alt: The user sends commands through the client to the controller; the controller provisions on clouds and fetches charms from Charmhub; the charmed applications record their state on the controller.
```

## Reference

### reference/action.md

#### Action run flow (sequence)

**Insert at:** § Action execution.

```{ggarch}
:file: ../juju.ggarch
:sequence: Action run flow
:no-legend:
:caption: juju run enqueues an operation; the controller records per-unit tasks (pending) and the unit agent's watcher resolves them; the task runs via the charm's dispatch script (action-get/set/fail/log during execution), and finishing stores results in the object store. juju cancel-task moves a running task to aborting; the process is killed and the task reports aborted.
:alt: User calls juju run; client enqueues the operation on the controller; controller records operation and per-unit tasks pending; controller notifies unit agent; agent resolves and starts the task (running); agent runs the charm action with jujuc action commands; on cancel the agent aborts; otherwise results stream back and the task completes.
```

### reference/agent.md

#### Agent taxonomy (who runs what)

**Insert at:** § Types of agents.

```{ggarch}
:file: ../juju.ggarch
:view: Agent taxonomy
:no-legend:
:caption: The four agent types and their channels: every agent makes API calls to the controller; the machine agent hosts unit agents on machine clouds; containeragent is the unit-agent role as a single Kubernetes binary.
:alt: Controller, machine agent, unit agent, and containeragent in a row; arrows: machine agent hosts unit agent; each agent makes API calls to the controller.
```

### reference/bundle.md

#### Bundle deploy (sequence)

**Insert at:** page top.

```{ggarch}
:file: ../juju.ggarch
:sequence: Bundle deploy
:no-legend:
:caption: juju deploy <bundle> --overlay reads the bundle as a YAML multidoc (first document = base, the rest = overlays: relations append, machines overwrite, an empty overlay application REMOVES the base app), snapshots the model status, builds the change graph and topologically sorts it, then applies each change in order (addCharm, deploy, addMachines, addRelation, addUnit, expose, setOptions, create/consume offers). Any error aborts the whole apply.
:alt: User calls juju deploy; client merges overlay into base; controller returns model status snapshot; client builds the change graph and applies changes in order.
```

### reference/charm.md

#### Charm origins

**Insert at:** § Charm taxonomy.

```{ggarch}
:file: ../juju.ggarch
:view: Charm origins
:no-legend:
:caption: There is no charm-revision table: each charm REVISION is its own charm row (unique on source + reference name + revision); the application's charm_uuid is a mutable pointer refreshed on update; channels (track/risk/branch) are per-application, not per-charm; download provenance and the immutable charmhub hash hang off the charm row 1:1; every deployed unit pins its own charm revision.
:alt: Application and unit records point at the charm record; charm metadata and download info hang off charm; application channel and platform records point at application.
```

### reference/configuration.md

#### Configuration levels (where each lives)

**Insert at:** § Controller configuration.

```{ggarch}
:file: ../juju.ggarch
:view: Configuration levels
:no-legend:
:caption: Three config levels, three homes: controller config lives in the controller DB (set by juju controller-config; bootstrap seeds it); model config lives in the model DB (set by juju model-config; defaults funnel from Juju -> provider -> cloud -> region); application config lives in per-application rows in the model DB (set by juju config -- there is NO config-set hook command; charms read via config-get). The application-level trust key is intercepted into its own boolean column and gates the uniter's cloud-credential access.
:alt: User and controller above the three config records; charm beside application config (it reads, it cannot write).
```

### reference/controller.md

#### Worker tree (controller)

**Insert at:** page top.

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

#### Bootstrap K8s

**Insert at:** § Controller bootstrap (staged).

```{ggarch}
:file: ../juju.ggarch
:sequence: Bootstrap K8s
:no-legend:
:caption: Bootstrapping on Kubernetes is mostly client-side orchestration: the CLI authenticates, schedules the controller pod, and then waits. Once jujud declares the API ready, the controller is fully autonomous.
:alt: User calls juju bootstrap. Client authenticates with K8s and creates the controller pod namespace. Controller pod self-starts jujud, the API server, and the database. Controller pod signals API ready to Client. Client reports success to User.
```

#### Deploy K8s

**Insert at:** § Controller deploy (staged).

```{ggarch}
:file: ../juju.ggarch
:sequence: Deploy K8s
:no-legend:
:caption: Deploying to Kubernetes is a two-phase handoff: the controller writes intent into the database and schedules the pod, then the unit agent (containeragent) takes over and drives the charm lifecycle independently.
:alt: User calls juju deploy. Client sends Deploy RPC to Controller. Controller writes records and schedules pod on Kubernetes. K8s returns pod running. Controller starts containeragent. containeragent runs install, config-changed, start hooks and returns unit active. Controller signals deploy complete back to Client and User.
```

### reference/credential.md

#### Credential chain

**Insert at:** § Credential definition.

```{ggarch}
:file: ../juju.ggarch
:view: Credential chain
:no-legend:
:caption: The credential chain lives in the controller DB: a user owns 0..N cloud credentials (cloud/owner/name is the natural key; 15 auth types); a cloud defines 0..N credentials; a model uses 0..1 credential and belongs to one cloud. The model DB carries only a read-only denormalised copy (credential owner/name as text). Access grants are a separate permission table (object types cloud/controller/model/offer — there is no credential object type; credential access is ownership plus cloud-level add-model/admin).
:alt: User record, cloud record, credential record, and model record with FK arrows: user owns credentials, cloud defines credentials, model uses one credential and is deployed on one cloud.
```

### reference/database.md

#### Data model (full spine)

**Insert at:** § Model databases.

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

#### HA controller: Dqlite replicaset

**Insert at:** § Database implementation. also: reference/high-availability.md, howto/manage-the-databases.md.

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

### reference/hook.md

#### Uniter operation (state machine)

**Insert at:** § Hook execution.

```{ggarch}
:file: ../juju.ggarch
:view: Uniter operation
:no-legend:
:caption: The uniter's three-phase operation executor — idle → preparing → executing → committing — with the error path (hook fails) and the retry loop. The state labels are the verbatim strings from internal/worker/uniter/operation/executor.go; guards: ErrHookFailed, ErrNeedsReboot.
:alt: State machine: idle to preparing on hook queued, preparing to executing, executing to committing on hook exits 0, executing to error on hook fails, error to idle on retry, committing to idle on write complete.
```

### reference/juju-web-cli.md

#### Web CLI (sequence)

**Insert at:** § Features.

```{ggarch}
:file: ../juju.ggarch
:sequence: Web CLI
:no-legend:
:caption: The dashboard serves the /commands websocket; each submitted command passes the 90-command whitelist (plugins doubly excluded, upgrade-controller unregistered), then runs as an embedded juju CLI IN the controller process, dialing its own API with the submitted credentials; stdout/stderr stream back as CLICommandStatus lines.
:alt: The dashboard charm serves the websocket, filters through the whitelist, runs the embedded CLI in-process, and streams response lines.
```

### reference/jujud.md

#### Worker tree (machine cloud)

**Insert at:** page top.

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

### reference/log.md

#### Log flow (sequence)

**Insert at:** § Juju agent logs - machines.

```{ggarch}
:file: ../juju.ggarch
:sequence: Log flow
:no-legend:
:caption: Agents buffer their log records in memory and ship them to the controller's /logsink websocket endpoint; the controller batches them as JSON lines into logsink.log, which juju debug-log tails through the API. On Kubernetes, agent logs also go to the container's stdout.
:alt: Unit agent and machine agent buffer records and ship them to the controller; the controller batches them into logsink.log; the user tails via juju debug-log.
```

### reference/machine.md

#### Machine designations (two provisioning paths)

**Insert at:** § Machines and system (LXD) containers.

```{ggarch}
:file: ../juju.ggarch
:view: Machine designations
:no-legend:
:caption: What a machine designation names, grounded in domain/machine: machine 0 and its LXD container are rows in the SAME machine table (the container linked by a machine-parent record; one nesting level only), so the designation is the containment path. The provisioning split: the controller (its compute provisioner) starts base machines (StartInstance); the host machine's agent provisions its own containers through the LXD broker (containerprovisioner on the machine agent) and watches them via the API (WatchContainers). Containers are machines: each runs its own machine agent, which hosts the unit agent. Placement scope '#' = existing, 'lxd:' = new; --to is machine-cloud only.
:alt: The controller provisions machine 0; machine 0's agent provisions the LXD container via the LXD broker and watches its containers through the controller API; the container's own machine agent hosts the unit agent.
```

### reference/metadata.md

#### Simplestreams lookup (sequence)

**Insert at:** § Basic workflow.

```{ggarch}
:file: ../juju.ggarch
:sequence: Simplestreams lookup
:no-legend:
:caption: The metadata search path (reference/metadata.md): the client walks the four locations in priority order — the controller database (a running model), the user-supplied URL (agent-metadata-url / image-metadata-url), provider-specific locations (the keystone product-streams endpoints on Openstack), and streams.canonical.com — trying each location signed (.sjson) first, then unsigned, and using the first location that answers. Signed metadata is verified with the public keys Juju ships with.
:alt: User calls juju bootstrap or juju deploy; the client tries the controller database, then the user-supplied metadata URL, then the provider locations, then streams.canonical.com, each attempted signed first then unsigned; the client verifies signatures with the shipped public keys and uses the first hit.
```

### reference/model.md

#### Model removal

**Insert at:** § Model removal. also: explanation/architecture.md § Remove.

```{ggarch}
:file: ../juju.ggarch
:sequence: Model removal
:no-legend:
:caption: Model destruction is coordinated by the Undertaker, a worker that runs inside the controller agent. The controller never deletes its own database — the Undertaker does, as the final act after all cloud resources have been released.
:alt: User calls juju destroy-model. Controller marks model Dying and fires watcher to Undertaker. Undertaker destroys all applications. Controller releases all machines and marks model Dead. Undertaker deletes model records and Dqlite database.
```

### reference/offer.md

#### Cross-model relation (CMR)

**Insert at:** page top.

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

### reference/relation.md

#### Integrate

**Insert at:** § Relation creation (staged). also: explanation/architecture.md § Integrate.

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:caption: Relation data never flows between units directly. The controller holds all relation bags and notifies each unit agent when the bag it reads changes. Units write to the controller; the controller fans out the change.
:alt: User calls juju integrate. Client sends Integrate RPC to Controller. Controller writes relation record and fires watchers to both unit agents. Each agent runs relation-created, relation-joined, and relation-changed hooks and writes its relation data to the controller. The controller notifies the other agent after each write.
```

#### Databag permissions

**Insert at:** § Permissions around relation databags.

```{ggarch}
:file: ../juju.ggarch
:view: Databag permissions
:no-legend:
:caption: The databags are records in the host model's database (on the controller), and the connection between applications runs through their units: each unit's AGENT reads and writes them over the API (relation-get/relation-set via the hook tools -> the unit agent's uniter facade). Each application has ONE application databag (per relation endpoint) and unit databags (one per unit). LEAN form (reviewer round 20): units, not unit agents (leadership is a property of the unit); no model-database box -- the caption says the bags are records in the host model's database. Red = reads + writes, green = reads; each unit reads + writes ONLY its own unit databag, the leader unit also its application's application databag; and all units -- leaders included -- read ALL of the other application's databags. The permission story is the same whether the applications share a model or not. Peer panel (app A related to itself): the permissions turn inward -- every unit reads every databag of its own application, the application databag included. Users have read-only visibility via the API (juju show-unit), never write.
:alt: App A's units (appA/leader, appA/1) and app B's units (appB/leader, appB/1) above one row of databags; red arrows reading and writing within each set (own bags; the leader also the application databag), green arrows reading across to the other application's set; below, the peer panel with one set and inward green reads of every bag, the application databag included.
```

### reference/script.md

#### Operation hierarchy

**Insert at:** § Script task.

```{ggarch}
:file: ../juju.ggarch
:view: Operation hierarchy
:no-legend:
:caption: The entity hierarchy behind the Action run flow: an operation groups 1..N tasks (one per receiver); the parallel and execution-group flags live on the operation, shared by all tasks; an operation_action row exists 1:1 only when the operation is an action (its absence = an exec, modelled as the predefined 'juju-exec' action); each task reports 0..1 status and runs on a unit or machine; results go to the object store.
:alt: Operation record to task record to unit task to unit; operation action record above operation; task status below task.
```

### reference/secret.md

#### Secret lifecycle (state machine)

**Insert at:** § Secret lifecycle.

```{ggarch}
:file: ../juju.ggarch
:view: Secret lifecycle
:no-legend:
:caption: The life of a secret, grounded in domain/secret: reserved (URI minted) -> active (latest revision, content in a backend) -> granted (view | manage roles) -> superseded; a rotate policy fires secret-rotate (leader), expiry fires secret-expired; a revision no consumer tracks becomes obsolete (pending delete) and the owner charm retires it via secret-remove (or user secrets auto-prune). Consumers see secret-changed.
:alt: State machine: reserved to active on create, active self-loops for grant/revoke and new-revision publication, active to rotate-due on the rotate policy and back via secret-rotate, active to expiry-due and on to removed via secret-expired then secret-remove, active to obsolete when superseded, obsolete to removed on prune.
```

### reference/space.md

#### Network spaces

**Insert at:** § Spaces as constraints and bindings. also: reference/subnet.md.

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:no-legend:
:caption: A space groups subnets; a subnet belongs to 0..1 space (the alpha space exists by default); an application's default binding points at one space, and each charm-relation endpoint can bind 0..1 space of its own.
:alt: Application record to space record to subnet record; arrows: subnet belongs to 0..1 space; application default binding (one).
```

### reference/status.md

#### Status domains (who sets what)

**Insert at:** § Types of status.

```{ggarch}
:file: ../juju.ggarch
:view: Status domains
:no-legend:
:caption: Who sets each status domain: the unit agent sets its own status (the controller derives allocating and lost); the charm sets the workload status via status-set; the leader unit sets the application status via status-set --application and the relation lifecycle (joining, joined, broken), else Juju computes the application status from the unit statuses; the machine agent sets the machine status; the controller suspends/resumes cross-model relations (suspending, suspended, resume to joining). Transitions are free-form enumerations except relation and storage (enforced machines).
:alt: Actor nodes pointing at the status domains they set: charm to workload status, unit agent to unit agent status, leader unit to application and relation status, machine agent to machine status, controller to relation status (suspends and resumes cross-model relations).
```

### reference/storage.md

#### Storage model

**Insert at:** § Storage directive.

```{ggarch}
:file: ../juju.ggarch
:view: Storage model
:no-legend:
:caption: The storage walk, grounded in 0011-storage.sql: the charm defines storage names (kind block|filesystem, count, size); a directive pins one pool (user- or provider-default origin) per application; an instance carries the charm name, kind and requested size and is backed by exactly one volume or filesystem; attachments bind instances to units; volumes bind to net nodes (the machine or unit network identity). Provision scope: model = machine-independent, machine = dies with the machine.
:alt: Record chain: charm storage to directive to pool to instance; volume to the right of instance, filesystem below, attachment below charm storage, net node above volume. Arrows carry multiplicities.
```

### reference/unit.md

#### Unit removal

**Insert at:** § Unit removal. also: explanation/architecture.md § Remove.

```{ggarch}
:file: ../juju.ggarch
:sequence: Unit removal
:no-legend:
:caption: Removal is a cooperative shutdown: the controller marks the unit Dying, the unit agent runs its teardown hooks in order, then marks itself Dead. Only after that does the controller release the underlying machine.
:alt: User calls juju remove-unit. Controller marks unit Dying and fires watcher to unit agent. Unit agent runs stop, teardown, and remove hooks, then marks unit Dead. Controller releases machine and deletes unit records.
```

### reference/upgrading-things.md

#### Upgrade paths (sequence)

**Insert at:** page top.

```{ggarch}
:file: ../juju.ggarch
:sequence: Upgrade paths
:no-legend:
:caption: The upgrade order is client first (refresh the juju snap), then the controller and the model. Patch-version deltas (and minor-version deltas before Juju 3.0) upgrade in place: juju upgrade-controller then juju upgrade-model. Major-version deltas and minor-version deltas after 3.0 bootstrap a new controller and migrate the models to it before juju upgrade-model -- model migration is the safer path for risky upgrades, and the upgrade path may be staged (e.g. 2.2 -> 2.9 -> 3.0). Application (charm) upgrades are independent.
:alt: User asks to upgrade; the client refreshes the juju snap; then either upgrade-controller and upgrade-model in place (patch or pre-3.0 minor deltas) or bootstrap a new controller, migrate the models, and upgrade-model (major or post-3.0 minor deltas).
```

## Explanation

### explanation/architecture.md

#### Juju overview

**Insert at:** § Insight.

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

#### K8s deployment topology

**Insert at:** § Topology. also embedded: reference/containeragent.md, reference/jujuc.md, reference/pebble.md; and as the result slide of the deploy slideshow (§ Deploy, Kubernetes tab).

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

#### Data model

**Insert at:** § Data model. also embedded as the seed slide of both deploy slideshows (§ Deploy, Kubernetes and Machine tabs).

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

#### Machine deployment topology

**Insert at:** § Deploy (Machine tab) — the deploy slideshow's result slide. The machine-cloud mirror of the K8s deployment topology.

`````{grid} 2
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Machine deployment topology (synthesized)
:no-legend:
:caption: Auto-layout — no positions declared. The typed hub planes found the unit-machine chain as a horizontal row on their own.
:alt: Controller machine and unit machine side by side; cloud above the controller machine, Charmhub below; the unit machine's machine agent phones home to the controller agent; inside the unit machine a chain machine agent, unit agent, charm, workload.
```
````
````{grid-item}
```{ggarch}
:file: ../juju.ggarch
:view: Machine deployment topology
:no-legend:
:caption: Declared arrangement — one jujud per machine: the controller machine's jujud runs the controller with Dqlite in-process; the unit machine's jujud hosts the unit agent, which runs the charm, which drives the workload directly (no Pebble on machine clouds — Pebble is injected only into K8s workload containers). Compare with the synthesized variant.
:alt: Controller machine and unit machine side by side; cloud above the controller machine, Charmhub below; the unit machine's machine agent phones home to the controller agent; inside the unit machine a vertical chain machine agent, unit agent, charm, workload.
```
````
`````

#### Bootstrap K8s result

**Insert at:** § Bootstrap (Kubernetes tab) — the bootstrap slideshow's result slide.

```{ggarch}
:file: ../juju.ggarch
:view: Bootstrap K8s result
:no-legend:
:caption: The state after `juju bootstrap` on Kubernetes: one controller, one model, no applications — the controller pod running jujud, the API server and Dqlite in-process.
:alt: Kubernetes cloud above, controller pod below with the controller agent inside.
```

#### Bootstrap machine result

**Insert at:** § Bootstrap (Machine tab) — the bootstrap slideshow's result slide.

```{ggarch}
:file: ../juju.ggarch
:view: Bootstrap machine result
:no-legend:
:caption: The state after `juju bootstrap` on a machine cloud: one controller, one model, no applications — the controller machine running jujud, the API server and Dqlite in-process.
:alt: Cloud above, controller machine below with the controller agent inside.
```

#### juju status

**Insert at:** § Deploy (both tabs) — the closing verification slide of both deploy slideshows.

```{ggarch}
:file: ../juju.ggarch
:sequence: juju status
:no-legend:
:caption: What "juju status" actually is: the controller reads the status records, derives live agent liveness from connection state, and projects both back. Status is records plus liveness — an overlap, not an identity.
:alt: User calls juju status. Client sends a Status API call to the controller. The controller reads status records and derives agent liveness, then returns the projected status. Client shows the status output to the user.
```

#### Hook execution

**Insert at:** § Control flow.

```{ggarch}
:file: ../juju.ggarch
:sequence: Hook execution
:no-legend:
:caption: Every hook runs the same cycle: the controller notifies, the agent snapshots remote state, resolves the next hook, and dispatches. Hook commands are served locally by the agent acting as a proxy — the charm never calls the controller directly.
:alt: API server fires watcher to unit agent. Unit agent snapshots state and resolves hook. Loop: charm calls hook command, unit agent proxies it to API server. On success: flush writes. On failure: discard writes, set unit error.
```

### explanation/juju-architecture.md

#### Intro: the problem

**Insert at:** top (the architecture narrative).

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

#### Intro: Juju enters

**Insert at:** top (the architecture narrative).

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

#### Intro: Juju unpacked

**Insert at:** top (the architecture narrative).

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

## Other

Diagrams with no confirmed home yet.

### User authentication (the verification commands)

*home TBD (candidates: reference/user.md, howto/manage-users) — pulled from the tutorial at reviewer direction.*

```{ggarch}
:file: ../juju.ggarch
:sequence: User authentication
:no-legend:
:caption: What the tutorial's verification commands actually do, grounded in cmd/juju: bootstrap created the admin user (agentbootstrap, superuser access) and cached the account in the client store (environs/bootstrap/prepare.go); juju whoami answers from that cache without an API call; juju show-user admin calls UserManager.UserInfo on the controller and reports access: superuser. Home TBD — removed from the tutorial at reviewer direction (the tutorial only needs to signal user management); pending a reference home.
:alt: User runs juju whoami; the client reads the admin account cached locally at bootstrap. User runs juju show-user admin; the client calls UserManager.UserInfo on the controller; the controller returns the user info with superuser access.
```

## Pending round-2 views

Pages still on hand-drawn visuals: `reference/relation.md` (the
relation taxonomy excalidraw — the databags excalidraw is superseded
by the Databag permissions view), `reference/hook.md` (the
hook-charm-lifecycle PNG — the Uniter operation machine covers its
execution story). The remaining reference pages without a view are
the coverage-round-2 opportunity map.
