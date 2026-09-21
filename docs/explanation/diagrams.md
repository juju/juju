(diagrams)=
# Diagram POC — one model, how each diagram was produced

Everything we've built, from one file (`juju.ggarch`). The captions on
this page are about **production** — how each drawing was made:

- **Auto-layout** (no positions declared): the engine decides —
  layered columns, barycenter rows, corridor budgets, port discipline.
- **Refinement** (declared planes/fans over the synthesized base):
  the author states the arrangement as a sentence; synthesis fills
  the rest (ADR-007).

Caption meaning comes later: diagrams embedded in real docs carry
meaning captions; this page surfaces the catalogue.

## Intro: the problem

```{ggarch}
:file: ../juju.ggarch
:view: Intro: the problem (synthesized)
:no-legend:
:caption: Without Juju, every application is its own island — the operator reaches across cloud boundaries manually. Three applications as instances of the one real archetype; the cloud grouping is drawn as declared regions, not claimed as model facts. Auto-layout — no positions declared.
:alt: A user with direct "operates" arrows to three application instances, grouped by dashed boxes labelled on cloud 1 and on cloud 2.
```

## Intro: Juju enters

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters (synthesized)
:no-legend:
:caption: Juju inserts a single controller between the operator and everything else. Clouds and charmed applications are instances of grounded archetypes (cloud_rec, application_rec); model membership is drawn as declared regions — one model per cloud, as the schema requires (model.cloud_uuid). Auto-layout — no positions declared.
:alt: User and client on the left, controller in the centre, two cloud instances fanned above, Charmhub below, three charmed application instances fanned to the right, grouped by dashed boxes labelled model 1 (on cloud 1) and model 2 (on cloud 2).
```

## Intro: Juju enters (declared)

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju enters (declared)
:no-legend:
:caption: The same view under ADR-007 refinement: the arrangement declared as a sentence — user, client and controller on one horizontal plane; cloud 1 and cloud 2 fanned above the controller from its mid-north face, spacing sized for the two "provisions on" labels; Charmhub below on the same axis; three applications fanned right, gap sized for the three "converges toward" labels. Synthesis fills every geometry the sentence leaves undeclared. Compare with the zero-declaration original above.
:alt: User, client, controller on one horizontal line, two cloud boxes fanned above the controller from its top face, Charmhub below it, three application boxes fanned to the right with converging arrows into the controller's right face. The same arrangement as the authored juju3 preview, produced from declarations.
```

## Intro: Juju unpacked

```{ggarch}
:file: ../juju.ggarch
:view: Intro: Juju unpacked (synthesized)
:no-legend:
:caption: Each unit is an instance of the real structure: a unit pod containing a charm container (unit agent + charm) and a workload container (Pebble + workload). Every wire is real — the agent watches the controller (stream) and calls it (API); the charm drives the workload through Pebble. No scope chips: provenance is the illustration's story, not a model fact. Auto-layout — no positions declared.
:alt: User, client, controller on the left. Three unit pod instances fanned to the right, each containing a unit agent and charm in a charm container, and Pebble and workload in a workload container. Each unit agent has watch and API arrows to the controller.
```

## Data model

```{ggarch}
:file: ../juju.ggarch
:view: Data model (synthesized)
:no-legend:
:caption: The controller's database is the single source of truth for the entire deployment. Every runtime entity — application, unit, machine, charm, relation — has a record here; what you see in "juju status" is mostly these records, plus live agent liveness. Auto-layout — no positions declared.
:alt: Six record nodes: charm above application, application connected to unit, unit connected to machine/pod, relation below application connected to endpoint.
```

## Data model (full spine)

```{ggarch}
:file: ../juju.ggarch
:view: Data model (full spine) (synthesized)
:no-legend:
:caption: The full record spine across both databases — controller DB (user, cloud, credential, model, controller) and model DB (application, charm, unit, machine, relation, endpoint). The provenance walk unit → application → model → cloud is traceable on one drawing; the relation-endpoint indirection is un-flattened. Every node and association here is grounded in the schema DDL (see tools/check-grounding.py). Auto-layout — no positions declared.
:alt: Two dashed boxes: controller database containing user, controller, cloud, credential, model records; model database containing application, charm, unit, machine, relation, endpoint records. Associations run in semantic direction with multiplicity labels.
```

## Worker tree (machine cloud)

```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (machine cloud) (synthesized)
:no-legend:
:caption: The execution chain as a worker tree — the controller drives the machine agent, which hosts the unit agent, which runs the uniter (internal/worker/uniter), which dispatches the charm. See [Worker tree (controller)](#worker-tree-controller) for the controller-side dependency engine. Auto-layout — no positions declared.
:alt: Vertical chain, top to bottom: controller, machine agent, unit agent, uniter, charm, each connected by control arrows labelled drives, hosts, runs hooks via, dispatches.
```

## Worker tree (controller)

```{ggarch}
:file: ../juju.ggarch
:view: Worker tree (controller) (synthesized)
:no-legend:
:caption: The controller's dependency engine, grounded in cmd/jujud-controller/agent/{machine,model}/manifolds.go — every manifold is a worker; arrows run consumer → provider. The centre spine is the capability ladder from agent config through the Dqlite-backed DB accessor, change stream, and domain services up to the API and HTTP servers. Left: the per-model runners (the model worker manager hosts the compute provisioner) and the provider tracker that holds cloud connections. Right: lease manager, primary election, and lease expiry (HA leadership), plus the lease-guarded object store. Every node carries a ground pointer to its manifold source. Auto-layout — no positions declared.
:alt: Five columns of worker boxes. Far left: provider tracker above provider services. Left: compute provisioner above model worker manager, both inside a dashed box labelled model workers (one set per model), undertaker below. Centre spine, top to bottom: agent, DB accessor, change stream, domain services, API server, HTTP server. Right: object store, lease manager below with primary election and lease expiry stacked above. Control arrows connect consumers to providers; the change stream watches the DB accessor.
```

## Cross-model relation (CMR)

```{ggarch}
:file: ../juju.ggarch
:view: Cross-model relation (CMR) (synthesized)
:no-legend:
:caption: Cross-model integration is record-shaped: an application publishes an offer; each consuming integration is an offer connection; the consuming model references the remote controller via an external controller record; and a synthetic remote application participates in a local relation. No unit-to-unit wire exists — the two controllers mediate. All tables grounded (offer, offer_connection, application_remote_offerer, external_controller). Auto-layout — no positions declared.
:alt: Nine record nodes. Top row: application, offer, offer connection. Middle row: relation, endpoint, remote application. Bottom: model and external controller. A dashed box around offer, offer connection, and external controller is labelled cross-model machinery.
```

## HA controller: Dqlite replicaset

```{ggarch}
:file: ../juju.ggarch
:view: HA controller: Dqlite replicaset (synthesized)
:no-legend:
:caption: In a high-availability controller deployment, three controller nodes each run an agent and a Dqlite instance. The Dqlite nodes form a Raft replicaset, and the sync arrows are drawn as the full mesh — every node talks to every other, because that is what Raft replication actually is (the leader replicates to all peers). The dashed box cuts across the container boundaries to show the database layer as a single strongly-consistent unit — the persistence guarantee behind "declare state once; any component can restart and recover". Auto-layout — no positions declared.
:alt: Three controller node instances side by side, each containing a controller agent and a Dqlite database node. A dashed annotation box encloses all three Dqlite nodes, labelled "Raft replicaset (strongly consistent)". Dashed arrows run between every pair of Dqlite nodes — the full mesh of Raft sync.
```

## Sequences

## juju status

```{ggarch}
:file: ../juju.ggarch
:sequence: juju status
:no-legend:
:caption: What "juju status" actually is: the controller reads the status records, derives live agent liveness from connection state, and projects both back. Status is records plus liveness — an overlap, not an identity.
:alt: User calls juju status. Client sends a Status API call to the controller. The controller reads status records and derives agent liveness, then returns the projected status. Client shows the status output to the user.
```

## Uniter operation (state machine)

```{ggarch}
:file: ../juju.ggarch
:view: Uniter operation
:no-legend:
:caption: The uniter's three-phase operation executor — idle → preparing → executing → committing — with the error path (hook fails) and the retry loop. The state labels are the verbatim strings from internal/worker/uniter/operation/executor.go; guards: ErrHookFailed, ErrNeedsReboot.
:alt: State machine: idle to preparing on hook queued, preparing to executing, executing to committing on hook exits 0, executing to error on hook fails, error to idle on retry, committing to idle on write complete.
```

## Hook execution

```{ggarch}
:file: ../juju.ggarch
:sequence: Hook execution
:no-legend:
:caption: Every hook runs the same cycle: the controller notifies, the agent snapshots remote state, resolves the next hook, and dispatches. Hook commands are served locally by the agent acting as a proxy — the charm never calls the controller directly.
:alt: API server fires watcher to unit agent. Unit agent snapshots state and resolves hook. Loop: charm calls hook command, unit agent proxies it to API server. On success: flush writes. On failure: discard writes, set unit error.
```

## Bootstrap K8s

```{ggarch}
:file: ../juju.ggarch
:sequence: Bootstrap K8s
:no-legend:
:caption: Bootstrapping on Kubernetes is mostly client-side orchestration: the CLI authenticates, schedules the controller pod, and then waits. Once jujud declares the API ready, the controller is fully autonomous.
:alt: User calls juju bootstrap. Client authenticates with K8s and creates the controller pod namespace. Controller pod self-starts jujud, the API server, and the database. Controller pod signals API ready to Client. Client reports success to User.
```

## Deploy K8s

```{ggarch}
:file: ../juju.ggarch
:sequence: Deploy K8s
:no-legend:
:caption: Deploying to Kubernetes is a two-phase handoff: the controller writes intent into the database and schedules the pod, then the unit agent (containeragent) takes over and drives the charm lifecycle independently.
:alt: User calls juju deploy. Client sends Deploy RPC to Controller. Controller writes records and schedules pod on Kubernetes. K8s returns pod running. Controller starts containeragent. containeragent runs install, config-changed, start hooks and returns unit active. Controller signals deploy complete back to Client and User.
```

## Integrate

```{ggarch}
:file: ../juju.ggarch
:sequence: Integrate
:no-legend:
:caption: Relation data never flows between units directly. The controller holds all relation bags and notifies each unit agent when the bag it reads changes. Units write to the controller; the controller fans out the change.
:alt: User calls juju integrate. Client sends Integrate RPC to Controller. Controller writes relation record and fires watchers to both unit agents. Each agent runs relation-created, relation-joined, and relation-changed hooks and writes its relation data to the controller. The controller notifies the other agent after each write.
```

## Unit removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Unit removal
:no-legend:
:caption: Removal is a cooperative shutdown: the controller marks the unit Dying, the unit agent runs its teardown hooks in order, then marks itself Dead. Only after that does the controller release the underlying machine.
:alt: User calls juju remove-unit. Controller marks unit Dying and fires watcher to unit agent. Unit agent runs stop, teardown, and remove hooks, then marks unit Dead. Controller releases machine and deletes unit records.
```

## Model removal

```{ggarch}
:file: ../juju.ggarch
:sequence: Model removal
:no-legend:
:caption: Model destruction is coordinated by the Undertaker, a worker that runs inside the controller agent. The controller never deletes its own database — the Undertaker does, as the final act after all cloud resources have been released.
:alt: User calls juju destroy-model. Controller marks model Dying and fires watcher to Undertaker. Undertaker destroys all applications. Controller releases all machines and marks model Dead. Undertaker deletes model records and Dqlite database.
```
