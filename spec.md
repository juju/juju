# Controller API addresses: patch series

Updated 2026-10-01 from the local source and projection branches. The original
plan used Juju `4.1` at `c9f6752833858c81acc5dd40bd3832e08374c213`
(`4.1-beta3`) and the [agreed specification](https://github.com/juju/juju-agent-specs/blob/2121f49d2a3a2f53fa9534832236017c55e21b05/2610/JUJU-8694/controller-api-addresses-4.1-plan.md).
The implementation now proceeds as separate source, projection and consumer
changes against newer `4.1` heads. Patch numbers below retain their existing
review references; they are not landing order.

The primary-controller `apiaddresssetter` owns publication for all audiences.
It will read network sources from the controller model database, combine them
with membership/configuration from the controller database, and publish the
selected addresses through controller-domain services. Cross-database source
coordination belongs in this worker. Controller-domain services and the
published projections are backed by the controller database, which is distinct
from the controller model database.

Three controller-database tables hold the projections:

| Table | Audience | Controller identity |
| --- | --- | --- |
| `controller_agent_address` | Ordinary agent discovery and reconnect | Nullable for shared endpoints |
| `controller_client_address` | Client discovery and external access | Present only for endpoints that reach the named node |
| `controller_peer_address` | Controller-to-controller connections, including object-store peers | Required |

There is no stored audience boolean or separate external-target table. External
operations use client addresses. Operations that must reach a named controller
require a client row with that identity and a supported transport; a shared
Kubernetes Service does not establish node-specific routing. Peer consumers
use peer addresses. No controller-model network service is passed through
`objectstoreservices` for this purpose.

## Implementation status and landing order

The local `4.1` branch includes patch 1's [PR #23364](https://github.com/juju/juju/pull/23364)
at `2a07b4e4ea` and patch 2's [PR #23416](https://github.com/juju/juju/pull/23416)
at `03708aaf00`. These are local Git observations, not a fresh remote PR review.

| Patch | Work | Status / dependency |
| --- | --- | --- |
| 1 | Preserve Kubernetes Service IP and DNS sources | Included in local `4.1` |
| 2 | Reconcile the controller API Service lifecycle | Included in local `4.1` |
| 3 | Add controller network queries and watcher | Committed through `4dff9a3222` on `4.1-controller-addr-targets`; publisher integration remains in 9 |
| 8a | Add the three projection tables, triggers and export support | Committed as `211262fdb6` on the separate projection branch |
| 8b | Make the existing setter write agent/client projections | Committed as `bc2938254d`; keeps the existing source watcher |
| 8c | Move current consumers to audience-specific reads/watches | Committed through `1e8c658920` on `4.1-new-controller-address-projections`; follow-ups are recorded in section 8 |
| 9 | Integrate the new source services and publish all three audiences | Next publisher change after 1–3 and 8 land |
| 4 | Switch API remote callers to the peer projection | After 9 supplies peer addresses |
| 5 | Complete diagnostic targeting and Kubernetes HA restrictions | Client projection from 8, final publication policy from 9 |
| 6 | Complete reverse-SSH targeting | Client projection from 8, final publication policy from 9 |
| 7 | Reconcile certificate identities from typed sources | Sources from 1–3; interim client-projection watcher is already in 8c |

The current checkout is `4.1-controller-addr-targets` at `4dff9a3222`.
Projection work is committed on its separate branch and is not part of this
checkout. The untracked `internal/worker/controllernetwork/` directory is an
earlier source-composition prototype, not a recruited worker or a completed
publisher integration. The earlier object-store network accessor/factory
extension has been backed out.

The sequence is intentionally incremental: add tables, change writes, change
readers, then integrate the new sources and finally switch the remote caller.
Intermediate writer/reader continuity and dual-writing the legacy table are
not requirements. There is no longer a single coordinated activation patch
for all readers. Reconcile the two branches when their prerequisites land.

## 1. Preserve Kubernetes Service IP and DNS address sources

Use the existing `k8s_service` → `net_node` relationships, IP tables, and
`fqdn_address` / `net_node_fqdn_address`. Keep source facts separate from API
publication policy.

- Persist Service IPs and hostnames with their original scopes. Generate new
  UUIDs in services and pass them to state; do not send hostnames through subnet
  lookup or IP parsing.
- Make replacement, scope changes, deletion and recreation reconcile both
  address forms. Unlink obsolete FQDN associations and delete orphan records
  without deleting a hostname still owned by another net node.
- Preserve scope per source by making FQDN identity the pair of hostname and
  scope. Net nodes share rows when both agree; scope changes must not mutate
  another owner's address.
- Apply the same representation to cloud-Service migration imports.
- Establish change notifications for all source tables/links introduced into
  the read contract; the composed controller watchers follow in patches 2–3.

Primary areas: [application service/state](domain/application/),
[network import](domain/network/modelmigration/import_cloudservice.go),
[network state](domain/network/state/cloudservice_import.go), and
[model schema/triggers](domain/schema/model/).

Acceptance: IPv4, IPv6, public LB hostname, unchanged text with changed scope,
IP↔hostname replacement, shared hostname ownership, delete/recreate, and legacy
import. Existing IP-only behaviour continues to work.

## 2. Reconcile the controller API Service through its full lifecycle

- Add/use a controller API Service lookup for the normal `controller-service`.
  Watch that exact Service identity, including creation, ingress changes and
  deletion. Preserve the separate StatefulSet/pod selectors.
- Bootstrap persists the actual normal Service source data. Stop representing
  `controller-0` or a headless per-pod address as that Service.
- Carry ordinary API bootstrap addresses separately from the per-node identity
  used for Dqlite. Preserve the latter throughout this series.
- Where normal Service DNS is supplied, construct it from supported provider
  namespace, Service and cluster-domain information. Do not invent an alias or
  silently substitute headless pod DNS.
- Repair Service source data and bootstrap acquisition while retaining the
  existing publisher. Projection persistence changes separately in patch 8;
  the source/policy integration follows in patch 9.

Primary areas: [Kubernetes provider](internal/provider/kubernetes/k8s.go),
[application watcher](internal/provider/kubernetes/application/application.go),
[Kubernetes bootstrap](internal/bootstrap/deployer_k8s.go), and
[bootstrap address finder](internal/worker/bootstrap/addressfinder.go).

Acceptance: changing only LB ingress, with no pod event, triggers persistence;
Service replacement/deletion is observed; bootstrap accepts LB IP and hostname;
Dqlite still receives the intended stable per-node identity. Use the Kubernetes
fake clientset and reactors for provider interaction tests.

Implementation: controller address reads and the Service watcher select
`controller-service` by name, independently of workload and replica selectors.
Bootstrap records that Service's UID and scoped addresses; the controller pod
ID and stable Dqlite FQDN remain separate. Runtime reconciliation retains the
ClusterIP alongside external controller Service addresses. A confirmed missing
Service clears its stored IPs and FQDN associations, retaining the Service/net
node for recreation. Provider failures and missing workload/status resources do
not clear the snapshot. Bootstrap no longer substitutes loopback for missing
Service addresses.

Validation: full `-race` package tests passed for
`internal/provider/kubernetes`, its `application` and `utils` packages,
`internal/bootstrap`, `internal/worker/bootstrap`,
`internal/worker/caasapplicationprovisioner`, and
`domain/application/{service,state}`. Bounded stress passed 8 runs of the
controller Service suite and 20 runs of the application provisioner suite,
with no failures. CLI, agent and agent-bootstrap dependent packages compiled
with `-race`. Generated mocks, `gci`, and whitespace checks are current.
`make pre-check` skipped static analysis by default; live Kubernetes validation
of patch 2 has not run.

## 3. Add controller network queries and watches for the publisher

Implemented on `4.1-controller-addr-targets` through `4dff9a3222`:

- `GetControllerUnitNetwork` returns `ControllerAPIAddresses` directly.
  `ControllerUnitNetwork` was removed; model type comes from a separate query.
- The existing network `Service` exposes `GetControllerPeerAddresses` and
  `GetControllerTargetAddresses`. The public getters use address-specific
  names for their data. `WatchableService` exposes `WatchControllerNetwork`.
- State queries read the controller unit's IP/DNS sources and validate its
  controller-application association. Alive and Dying targets remain eligible;
  Dead, missing and unrelated targets are rejected.
- Peer selection applies IAAS management-space selection with fallback when
  that space has no eligible addresses. Kubernetes peer selection preserves
  pod IP/DNS identities and their original scopes.
- External node-target selection is independent of management-space policy.
  Kubernetes returns `NotSupported` for this particular operation. This does
  not prevent ordinary external client access through a Service. The target
  source query does not imply a fourth projection table.
- Model watcher namespaces cover unit identity/lifecycle, address/FQDN links,
  devices, subnets and spaces, including network-node reassociation. Additional
  changelog triggers cover mutable device and space inputs.

Publisher integration is deferred to patch 9. It must combine this model
watcher with controller membership/configuration and normal Service sources,
waiting for every required initial event before querying. Resolve the actual
controller-model UUID when constructing its network service. A UUID must never
be compared with `ControllerModelName`. The restricted controller services
already use the controller database; their existing namespace selection does
not require a controller-model network extension.

Reuse or move useful code from the untracked `controllernetwork` prototype into
the publisher's source coordination as appropriate. Do not recruit a separate
publisher or construct that source in `apiremotecaller` or object-store workers.

Primary areas: [network service](domain/network/service/controller.go),
[network state](domain/network/state/controller.go),
[network watcher tests](domain/network/controller_watcher_test.go), and
[address publisher](internal/worker/apiaddresssetter/).

Acceptance for integration: correct controller mapping on IAAS/CAAS,
management-space cases, association/lifecycle validation, network reassociation,
empty initial events, changes during subscription/initial reads, deterministic
shutdown, and access to the actual controller model database.

Earlier source work passed race tests and bounded source/watcher stress tests.
The historical prototype/factory tests do not validate the revised publisher
wiring; repeat the relevant checks when integrating patch 9. Full lint and live
controller validation remain outstanding.

## 4. Resolve API remote callers from the published peer projection

The transitional reader in patch 8c uses agent addresses and
`WatchControllerAgentAddresses`. The peer switch is still pending.

- After patch 9 publishes peer addresses, make `apiremotecaller` read/watch
  `controller_peer_address` through controller-node services, replacing its
  agent-address accessor and watcher. It does not query the controller model
  or construct `controllernetwork.Source`.
- Refresh after published peer-address changes and maintain connections keyed
  by controller identity. Close removed/replaced connections deterministically.
  An authoritative empty peer set retires stale connections; a failed read does
  not masquerade as that result.
- Preserve object-location hints and object retrieval through managed
  connections. No source query is added to every blob request.
- Preserve the controller CA and fixed `juju-apiserver` TLS verification name
  for API and blob HTTP connections when dial addresses change.
- Check other users of the remote caller, including controller presence, as
  part of the same change.

Primary areas: [API remote caller](internal/worker/apiremotecaller/),
[object store](internal/worker/objectstore/),
[controller presence](internal/worker/controllerpresence/), and factory wiring.

Acceptance: retrieve a blob present only on one peer; replace that peer's pod
and reconnect to the correct controller; remove a node and close its connection;
preserve presence behaviour. Test routing via IP/DNS using a trusted certificate
with `juju-apiserver` and no routing-address SAN. Reject a wrong CA or missing
verification identity. Exercise worker startup and retry ordering under stress.

## 5. Read controller diagnostic targets from the published projection

`ControllerDetails` already reaches the client projection through its existing
controller-node getter after patch 8c. The remaining work is targeting policy
and capability handling.

- Use `controller_client_address` for IAAS controller-ID/address mappings.
  Exclude shared rows from named-controller results. Preserve client eligibility
  with and without a management space; agent eligibility is not an exclusion.
- Define the Kubernetes HA restriction using controller-hosting substrate and
  membership. Do not infer it from the workload model, number of addresses or
  currently reachable nodes.
- Enforce the restriction after authentication and before data delivery in
  both the debug-log websocket handler and `Client.StatusHistory`.
- Update client/command handling and `ControllerDetails` consistently. Use a
  distinguishable result so the existing generic `NotSupported` fallback cannot
  silently produce single-node output. Server checks must protect older clients.
- Preserve single-controller Kubernetes and IAAS retrieval, log ingestion, and
  status-history recording. Single-controller Kubernetes can use the connected
  controller; do not manufacture per-node Service mappings.
- Terminate an existing debug-log stream when expansion makes the controller
  Kubernetes HA. Own topology monitoring through the established worker and
  cancellation lifecycle, without starting unmanaged handler goroutines.

Primary areas: [HighAvailability facade](apiserver/facades/client/highavailability/),
[debug-log handler](apiserver/debuglog.go),
[StatusHistory handler](apiserver/facades/client/client/status.go), relevant API
clients/commands, and a narrow domain topology/capability query.

Acceptance: both commands, direct requests, older-client fallback, a Kubernetes
controller hosting an IAAS model, unavailable HA members, 1→3 transition during
a stream, and single-node/IAAS success. Verify auth precedes the restriction and
that recording continues.

## 6. Read reverse-SSH targets from the published projection

The current tunneler still uses the named-controller agent-address getter.

- Use controller-associated client addresses for the controller holding the
  waiting SSH session when that operation requires external reachability.
  Follow patch 9's client publication policy and preserve the actual SSH
  transport/port contract; API endpoints alone do not define an SSH listener.
- Preserve IAAS target identity and reachability policy.
- Return an explicit unsupported result for Kubernetes cases without a
  node-specific transport. Neither a shared Service nor an off-cluster-invisible
  pod IP satisfies this contract.
- Remove the obsolete general-discovery dependency from this worker and its
  interfaces. Designing an external Kubernetes node-specific transport remains
  separate work.

Primary area: [SSH tunneler](internal/worker/sshtunneler/), its service wiring and
the corresponding API/CLI error propagation.

Acceptance: IAAS session reaches the selected controller; changed/departed
targets are handled; unsupported Kubernetes routing gives an actionable error
without advertising a misleading destination.

## 7. Reconcile certificate identities from typed network sources

Patch 8c moved the existing cloud-local reads to `controller_client_address`
and added `WatchControllerClientAddresses`. Initial certificate population now
runs after the watcher's initial event. The purpose-specific SAN source work
below remains separate; the IP-only discovery accessor is still in use.

- Define the IP and DNS identities actually needed by consumers that verify a
  dialled address. Read those facts through a purpose-specific network query.
- Replace the cloud-local discovery accessor and its IP-only parsing in the
  certificate path. Do not add every peer routing address to certificates by
  default; patch 4 preserves the existing fixed peer TLS identity.
- Watch the source inputs to this query, including relevant Service and unit
  changes, and cross the readiness barrier before the first certificate query.
- Preserve existing certificate ownership and update behaviour while removing
  the dependency on ordinary API discovery.

Primary areas: [certificate updater](internal/worker/certupdater/), network
service/state and associated worker wiring.

Acceptance: required IPv4/IPv6/DNS SANs, address replacement, source failures,
readiness races, certificate regeneration and verification. Distinguish these
tests from the fixed-name peer TLS tests in patch 4.

## 8. Add projections and migrate existing publication/consumers

This work was split into three independently reviewable steps on
`4.1-new-controller-address-projections`, based on `5371cf31f3`.

### 8a. Projection schema -- implemented

The three tables defined above each store `uuid`, `controller_id`, `address`
(host or IP with API port), `scope` and `priority`. Lower priority values are
preferred within a controller or shared endpoint group. Agent/client identity
is nullable; peer identity is required. Unique indexes cover per-controller
addresses and shared addresses separately.

Generated changelog triggers, schema tests and controller export types/readers
are included. Agent/client notifications are keyed by address UUID; peer
notifications are keyed by controller ID. Metadata changes notify, while
unchanged updates do not. `controller_api_address` remains in the schema and
export representation pending cleanup.

### 8b. Existing setter writes the new tables -- implemented

- `apiaddresssetter` retains its existing source watcher and calls the existing
  controller-node publication service. It does not yet use patch 3's source
  queries/watcher.
- All existing selected addresses go to the client table. Addresses formerly
  marked `is_agent` also go to the agent table, preserving management-space
  selection and the all-filtered fallback. `IsAgent` is transient publication
  input, not a stored audience flag.
- The two projections reconcile atomically for the supplied controllers.
  Existing addresses retain their UUIDs; scope/priority changes update in
  place; no-op publication does not notify. The write checks controller
  membership/lifecycle in the transaction.
- The legacy table is no longer written. The peer table is not yet populated.
  Shared rows are supported by the schema, but the existing setter still
  publishes controller-associated rows. An empty input map remains a no-op;
  this is not yet the full-snapshot reconciliation needed in patch 9.
- Controller removal now clears agent/client rows as well as legacy rows.
  Peer-row cleanup must be included before peer publication begins.

### 8c. Existing consumers read/watch the new tables -- implemented

- Agent and client state readers select their respective tables. They retain
  UUID/scope/priority, use stable ordering within groups and include nullable
  shared endpoints in ordinary discovery. Named-controller getters exclude
  shared endpoints.
- `WatchControllerAgentAddresses` and `WatchControllerClientAddresses` replace
  the legacy address watcher. Each audience observes only its own table.
- `apiaddressupdater` follows the agent projection through the common
  `APIAddresser` facade. Its RPC shape remains unchanged. Agent facades,
  proxy/no-proxy and CaaS worker adapters use the agent watcher.
- Login and other existing audience-specific getters now read projections.
  Cross-controller registration watches the client table. Migration source
  information reads the client table directly, and its obsolete agent flag
  was removed.
- Certificate maintenance uses the client projection and waits for the initial
  event before reading. Typed SAN selection remains patch 7.
- `apiremotecaller` uses the agent table and watcher for now. Its identity-aware
  peer contract remains patch 4, after the producer in patch 9.

Remaining reader/policy work must not be mistaken for completed migration:

- The direct `domain/controller` query used by `GetConsumeDetails` was moved
  from legacy agent rows to `controller_agent_address`. It still needs to use
  client addresses for external offer consumption. Audit other externally
  returned connection details for the same audience mismatch and test with
  deliberately different agent/client endpoints.
- Service getters still apply their existing scope/IPv4 preference and
  machine-local filtering after reading the projection. Stored priority is
  used within that ordering; publication is not yet the sole policy owner.
  Move final eligibility/ordering to publication with patch 9 and keep readers
  as adapters. In particular, preserve peer DNS and Kubernetes pod scopes.
- Existing agent scope/no-proxy helpers still have IP-only assumptions for
  hostname endpoints. Cover DNS and IPv6 explicitly when moving to canonical
  publication policy; the table migration has not fixed those assumptions.
- Diagnostic and reverse-SSH capability/routing changes remain patches 5–6.
  Legacy schema/export/removal references still need a deliberate cleanup.

Primary areas: [controller-node domain](domain/controllernode/),
[controller schema](domain/schema/controller/sql/0010-controller-node.sql),
[trigger registration](domain/schema/controller.go),
[controller export](domain/export/),
[common addresses](apiserver/common/addresses.go),
[controller info](domain/controller/state/state.go), and
[migration source info](domain/modelmigration/state/controller/state.go).

Validation performed during the schema/writer/consumer work: focused state,
service, facade and affected-worker race tests passed; reader tests distinguish
agent/client/peer/legacy rows and shared versus named endpoints. Watcher tests
cover initial events, setter publication, metadata changes, deletion, no-op
updates and audience isolation. The latest consumer validation passed four
controller-node suite stress runs and twenty runs each for `apiaddressupdater`,
`apiremotecaller`, `certupdater`, `proxyupdater`, `caasmodeloperator` and
`caasapplicationprovisioner`. Agent/API server and object-store/bootstrap
consumers compiled with `-race`. These results cover the implemented changes,
not their future combination with patch 3 or live Kubernetes behaviour.
`STATIC_ANALYSIS=1 make pre-check` passed the preceding checks and stopped at
missing `golangci-lint`; optional licence/shell-format tools were also absent.

## 9. Integrate source services and publish all three projections

This is now the next publisher change after the source and projection work
lands. Ordinary readers are already on the new tables. Integrate the existing
`apiaddresssetter` with the new source/watcher and publication services, then
switch the remote caller separately in patch 4.

- Recruit source coordination under `apiaddresssetter`, retaining its
  primary-controller ownership. Combine controller membership/configuration,
  `WatchControllerNetwork`, normal Service/address association changes and
  relevant policy inputs. Wait for every required initial event before reads.
  Resolve the actual controller-model UUID for model services; projection
  consumers continue to require only controller-database services.
- Build explicit client, agent and peer address sets through domain services,
  replacing the transient `IsAgent` split. Write the related projections
  atomically. Reads preserve the selected policy/order and adapt only to their
  wire or transport shape.
- Kubernetes clients get the preferred external Service tier with private
  Service fallback as appropriate. Agents get internal Service endpoints with
  external fallback. Ordinary discovery excludes pod/headless endpoints.
  Publish shared Service addresses with NULL controller identity.
- Peer publication uses eligible controller-unit addresses, preserving pod
  IP/DNS identities on Kubernetes and management-space policy on IAAS. Each
  peer row identifies the node it actually reaches. No shared Service is
  published as a peer.
- IAAS clients preserve node association and ordinary external eligibility;
  management-space agent restrictions do not exclude client addresses.
  External node-specific capability remains explicit for Kubernetes without
  blocking publication of valid shared client or agent endpoints. It does not
  require a separate target-address table.
- Reconcile full published snapshots, including removed controllers and
  obsolete shared endpoints. Compare scope, identity, port and priority as
  well as address text. Successful empty snapshots clear stale rows; failed
  source reads retain the last successful publication. Define cold-start
  handling for empty projections and retry publication failures even without
  another source event.
- Read coherent snapshots within each source database where possible and
  converge across databases. Prevent an in-flight snapshot from restoring
  removed controllers. Include peer-row removal and deterministic shutdown,
  cancellation and primary handover.
- Reuse publication policy for bootstrap seeding. Establish valid sources and
  initial publication for supported restore/upgrade paths. Consumers can
  subscribe before publication; object-store and remote-caller startup must
  not depend on a running setter, because the setter's domain-service
  dependencies are downstream of those workers.
- Finish section 8's reader policy/audience audit, including offer consumption,
  hostname handling and canonical ordering. Keep the diagnostic, reverse-SSH
  and SAN policy changes independently reviewable in patches 5–7.

Primary areas: [publisher](internal/worker/apiaddresssetter/),
[controller-node domain](domain/controllernode/),
[machine manifolds](cmd/jujuagentd/agent/machine/manifolds.go), bootstrap,
[controller removal](domain/removal/state/controller/controller_node.go), and
consumer adapters requiring final policy changes.

Acceptance: public Service and internal agent/pod endpoints deliberately differ
in fixtures; migration and offer consumption advertise client endpoints.
Verify off-cluster agent fallback, controller/0 removal without losing a shared
Service, late LB assignment, metadata changes, deletion, publication failures,
restart, handover and startup races. Confirm that peer rows identify actual
controllers before enabling patch 4. Routing readers do not query the controller
model or repeat publication selection policy.

After the producer and consumers have migrated, remove obsolete legacy getters,
writer/watcher paths, table definitions, removal cleanup and generated exports
where the supported schema/export transition permits it. Keep only explicitly
required historical compatibility code. No routing consumer may fall back to
`controller_api_address`.

## Release and validation boundaries

Projection DDL, triggers and export support were added in patch 8a as 4.1
schema work. Verify the supported deployment transition against the landing
revision's schema rules; additive SQL alone does not establish an in-place or
rolling-upgrade contract. Patch 9 must rebuild valid sources and projections
through supported bootstrap/restore/upgrade paths. Do not reinterpret old
pod-derived discovery rows as client Service endpoints. Intermediate patch
continuity is not required, but the assembled release must initialize and
recover the projections correctly.

Each implementation patch uses `tc` tests and the required dqlite test setup.
Run changed concurrent packages with `-race`, and compile/race/stress changed
workers with a bounded timeout. Exercise readiness and event orderings, not just
data races. Regenerate mocks/schema/triggers/export artifacts where required,
run `gci` on touched non-generated Go files, and run `make pre-check` before
submission.

The remaining publisher and consumer patches supply integration coverage for:

- Kubernetes HA with an actually off-cluster client and agent, separately
  exercising LB IP and hostname endpoints.
- Blob retrieval from a particular peer, pod replacement, Service replacement,
  controller restart/primary handover and scale transitions.
- IAAS management-space behaviour and reverse SSH.
- Migration source information and CMR offer/consume continuity.
- Both diagnostic restrictions and continued log/status recording.
- Independent Dqlite membership and recovery checks.

Use the existing controller, model/migration, CMR and SSH suites where they fit;
add focused fixtures for the off-cluster/hostname cases. Report unit, stress and
live integration results separately. The original plan was based on static
source inspection; implementation progress and validation are recorded above.

Charm/listener/firewall exposure changes and a new external per-node Kubernetes
transport remain separate workstreams, as in the specification. Runtime API-port
changes also require listener/Service coordination; this series must not claim
that publishing a different port implements that feature.

## Follow-up after the series: Service FQDNs in unit address watching

After the series, revisit the [review comment on PR #23364](https://github.com/juju/juju/pull/23364#discussion_r4092774107)
suggesting that `getNetNodeSpaceAddresses` include `fqdn_address` data for
`GetAddressesHash` and `WatchUnitAddressesHash`. The suggestion has merit, but
the intended behaviour needs checking before extending the query:

- Compare with 3.6 unit and cloud-Service address watching, including the
  resulting `network-get` and relation-network behaviour for hostname-only and
  mixed IP/DNS Service addresses.
- Work through network spaces and endpoint bindings: determine how scoped
  hostnames participate in address selection and hashing when there is no
  associated subnet/space. Make any fallback explicit.
- Trace both the hash inputs and watcher subscriptions. Including FQDNs in the
  query must be accompanied by notifications for relevant `fqdn_address` and
  `net_node_fqdn_address` changes, with the correct Service/net-node filtering.
- Once the behaviour is agreed, add hash and watcher tests for hostname-only
  additions, replacements, scope changes and removals, mixed IP/DNS addresses,
  space/binding changes, and unchanged results that should not notify.