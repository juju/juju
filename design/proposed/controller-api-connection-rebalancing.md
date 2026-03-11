# Controller API connection rebalancing

This document captures the problem statement for active rebalancing of agent
API connections across highly available Juju controllers.

## Problem statement

Juju already gets some passive load distribution from API server rate limiting
during agent reconnect storms. When an HA controller is restarted as a whole,
agents reconnect over time rather than all at once, and those reconnections
tend to end up spread relatively evenly across the available controller nodes.

That behaviour does not extend to steady-state operation after a partial
failure. If one controller node restarts or becomes temporarily unavailable,
the agents connected to it reconnect to the remaining controller nodes. When
the original node returns, there is no mechanism to move a share of those
long-lived connections back. The recovered node stays underutilised, the other
nodes stay over-subscribed, and the controller remains imbalanced until another
large-scale reconnect event happens.

For HA controllers serving large numbers of agents, this creates avoidable skew
in API load, memory use, and failure exposure. The system needs an active
rebalancing mechanism that can restore a healthy connection distribution during
normal operation, without requiring a full controller restart.

## Success criteria

For this feature, success should be measured across controller nodes that are
up and eligible to accept agent API connections. The initial definition should
assume all such controllers have equal capacity, so the target state is an
approximately even split of agent API connections.

### Balance metric

Let:

- `N` be the number of up controllers.
- `C` be the total number of agent API connections across those controllers.
- `T = C / N` be the target connection count per controller.
- `c_i` be the observed connection count on controller `i`.

A useful balance metric is the maximum deviation from target, with both an
absolute tolerance and a relative tolerance:

- `deviation_i = abs(c_i - T)`
- `allowed = max(A, R * T)`
- `imbalance_score = max_i(deviation_i) / allowed`

Where:

- `A` is a minimum slack in raw connection count.
- `R` is the relative slack as a fraction of the target.

The controller set is considered balanced when `imbalance_score <= 1`.

This avoids overreacting in small deployments while still requiring meaningful
rebalancing in large ones. As an initial working definition for the spec:

- `A = 25` connections
- `R = 0.10` (10 percent)

Examples:

- With 7 total connections across 2 up controllers, `T = 3.5` and
  `allowed = max(25, 0.35) = 25`. A `5 / 2` split is acceptable.
- With 6000 total connections across 3 up controllers, `T = 2000` and
  `allowed = max(25, 200) = 200`. Any controller outside the range
  `1800-2200` is considered imbalanced.

### Rebalance preconditions

Rebalancing should not begin as soon as a controller becomes healthy. It
should first wait until a sufficient proportion of the tracked rebalance
population is currently connected, so the controller set is being judged
against a representative steady-state load.

This gate should be driven from controller-scoped connection tracking state in
the `controllernode` domain, not from model-scoped agent presence.

The controller-scoped tracking state must retain rows for disconnected agents.
The `controllernode` domain does not own the model's unit, machine, or
application inventories, so it cannot derive a denominator by comparing
currently connected agents with model entity tables. Instead, it must keep a
tracked population of rebalance participants and record whether each one is
currently connected.

Let:

- `K` be the number of tracked rebalance participants in controller-scoped
  connection state.
- `P` be the number of those participants currently marked connected.
- `connected_ratio = P / K`.

If `K = 0`, no rebalancing is required.

The controller should be eligible for rebalancing only when:

- if `K <= F`, then `P = K`
- if `F < K <= S`, then `P >= K - M`
- if `K > S`, then `connected_ratio >= G`

As an initial working definition for the spec:

- `F = 20` agents
- `M = 2` agents
- `S = 100` agents
- `G = 0.98` (98 percent)

Examples:

- With 7 tracked agents, all 7 must be connected before rebalancing starts.
- With 40 tracked agents, at least 38 must be connected before rebalancing
  starts.
- With 6000 tracked agents, at least 5880 must be connected before
  rebalancing starts.

This gate is intended to stop the system from rebalancing while a recovery is
still in progress. In the initial design, the tracked population should cover
the same rebalance participants as the recording implementation, which
currently means unit and machine agents.

### Convergence target

Once a controller is healthy and the rebalance preconditions are satisfied, the
system should return to the allowed balance range within 15 minutes, without
requiring a full controller restart.

### Hysteresis

To avoid flapping and connection churn, the initial specification should use
all of the following rules:

- start rebalancing only when `imbalance_score > 1.0`
- stop rebalancing when `imbalance_score <= 0.8`
- do not trigger a rebalance action unless it would move at least 50
  connections

## Implementation stages

The implementation is expected to proceed in stages. This document currently
specifies the first two stages, but it does not yet define the adjudication,
planning, or enforcement logic for moving connections between controller
nodes.

### Stage 1: Recording connections

#### Scope

The first stage should track the current controller node for agent API
connections that participate in the rebalance problem:

- unit agents
- machine agents

Application agents are out of scope for the first cut. This keeps the recorded
population aligned with the initial rebalance population used by the
controller-scoped connection-state gate.

#### Canonical source and transition

The controller-scoped connection record introduced in this stage should be
designed as the long-term canonical connection ledger for unit and machine
agents.

The existing model-scoped agent presence records in the `status` domain should
be treated as a temporary compatibility projection during this stage. They
remain only so existing status, liveness, export, and clean-up paths can
continue to function until those consumers are migrated in a later stage.

This transition is desirable because the controller-scoped record can retain
connection identity and apply conditional disconnect updates, which avoids an
older disconnect clearing a newer replacement session for the same logical
agent.

#### Ownership

Controller placement for agent API connections should be recorded as
controller-scoped state in the `controllernode` domain.

This state belongs there because connection distribution is a controller-wide
concern rather than a model status concern. Keeping it in `controllernode`
allows later adjudication to reason about connection placement directly,
without aggregating placement data out of model-scoped status tables.

#### Recording path

The recording path should reuse the existing API observer lifecycle.

The current observer that records agent presence should be expanded into a
broader connection-recording observer, or replaced by an observer with the same
lifecycle hooks and both responsibilities:

- refresh the temporary model-scoped presence projection in the `status`
  domain
- record the canonical controller-scoped agent placement in the
  `controllernode` domain

The `apiserver` worker should continue to create one observer per API
connection. That observer should capture:

- the connection identity when the WebSocket connection is established
- the authenticated agent identity when login succeeds
- the local controller node identity for the serving controller

The local controller node identity should be supplied as configuration to the
observer from the controller agent or `apiserver` worker context, rather than
looked up indirectly from a model service.

#### Recorded state

The controller-scoped placement record must contain enough information to
identify both the logical agent and the specific API connection instance that
created the record.

At minimum, the recorded state should capture:

- model UUID
- agent kind
- agent identity within the model
- connection state
- controller node identity for the current connected session
- connection identity
- last seen timestamp

The logical agent identity must be model-qualified. Unit names and machine
names are not globally unique across the controller.

The connection identity is required so that disconnect handling can distinguish
between an old connection closing and a newer replacement connection that has
already logged in for the same agent.

The connection state is required because controller-scoped tracking must retain
rows for disconnected agents in order to preserve the tracked population used
by rebalance preconditions.

#### Write semantics

On successful login of an in-scope agent, the observer should:

- refresh the temporary model-scoped presence projection in the `status`
  domain
- upsert the canonical controller-scoped placement record in the
  `controllernode` domain to mark the agent connected on the current
  controller node with the current connection identity

On connection leave, the observer should:

- remove the temporary model-scoped presence projection in the `status`
  domain
- update the canonical controller-scoped placement record to mark the agent
  disconnected only if the stored connection identity still matches the
  leaving connection

This conditional update is required to avoid an older connection marking a
newer session disconnected after the same agent has already reconnected.

#### Failure handling

Recording must not make the login path unavailable.

The observer should treat the status-domain presence write and the
controller-domain placement write as separate operations. Failures should be
logged, but should not fail the API login or leave handling path.

This means the later adjudication logic must tolerate temporary recording
inaccuracy and rely on subsequent reconnects and disconnects to repair missed
updates.

#### Non-goals for this phase

The following remain out of scope for the recording phase:

- calculating imbalance
- deciding which controller should shed or receive connections
- selecting which live connections to terminate
- biasing reconnects towards a target controller
- changing model-facing status logic to derive presence from controller-scoped
  connection state
- deleting the legacy model-scoped presence writes and tables

### Stage 2: Status migration and legacy removal

The second stage should change model-facing status and liveness behaviour to
derive agent presence from the controller-scoped connection ledger introduced
in stage 1, rather than from model-scoped presence tables.

This stage should update existing status-domain consumers that currently depend
on model-scoped presence recording, including status derivation and any other
remaining liveness, export, or clean-up paths that still read or maintain the
legacy model-scoped presence state.

This stage must also update removal jobs so they delete controller-scoped
connection information for entities and models that are being removed. Once
controller-scoped connection state becomes the source of truth for presence and
liveness, it must not retain rows for units, machines, or whole models that no
longer exist.

This stage must also handle the case where a controller crashes. A crashed
controller will not run the normal API connection leave workflow for the agent
connections it was serving, so its controller-scoped connection rows cannot be
left marked connected indefinitely.

The replacement for model-scoped presence should therefore include a
controller-side clean-up worker that monitors peer controller availability and
marks connection records for an unavailable controller as disconnected.

This worker should follow the existing distributed controller-set pattern
rather than using a singular lease-holder. Each healthy controller should run
the worker and monitor its peer controllers, reusing the same style of
peer-observation that is already used for controller presence today. When a
peer controller is observed unavailable, the clean-up operation should
idempotently mark all rebalance-participant connection records currently
assigned to that controller as disconnected.

This work belongs in stage 2 because it is part of making controller-scoped
connection state the source of truth for model-facing presence and liveness.
Once this clean-up path exists and status consumers read from controller-scoped
state, the legacy model-scoped presence path is no longer required for
controller crash handling.

Once those consumers no longer depend on model-scoped presence recording, the
legacy model-scoped presence writes should be removed from the API observer
lifecycle and the model-scoped presence tables should be deleted.
