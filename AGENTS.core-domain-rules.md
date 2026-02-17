# Juju Core Domain Rules for Coding Agents

## Core Priorities

- Correctness under concurrency
- Eventual consistency where required

## Glossary

- **Controller**: Management plane. Runs API server, database, workers. Manages multiple models.
- **Model**: Deployment environment containing applications, machines, relations. Two types:
    - **IAAS**: Provisions machines (LXD, AWS, GCE, Azure, MAAS).
    - **CAAS**: Targets existing Kubernetes clusters.
- **Application**: A deployed charm with configuration and scale.
- **Unit**: A running instance of an application (e.g. mysql/0, mysql/1).
- **Machine**: A compute resource (IAAS only). Can nest containers (e.g., 0/lxd/1).
- **Charm**: An operator package that defines how to deploy and manage an application.
- **Relation** (aka Integration): A typed connection between two applications that exchanges data.
- **Cloud**: A substrate provider (AWS, GCE, LXD, MicroK8s, etc.).
- **Credential**: Authentication material for a cloud.

## Life

- Life is a first-class value for lifecycle-managed model entities.
- Baseline lifecycle-managed entities include machines, units, applications, and relations.
- Juju lifecycle is represented by canonical types `core/life.Value` and
  `domain/life.Life`, with values `Alive`, `Dying`, and `Dead`.
- Lifecycle progression is monotonic (`Alive -> Dying -> Dead`) and does not transition backwards.
- `Alive` represents normal participation in model workflows.
- Transitioning to `Dying` starts departure work (for example relation departure,
  storage clean-up, and cloud instance clean-up).
- `Alive -> Dying` transitions are idempotent, so repeated removal requests for
  already non-`Alive` entities are generally no-ops unless an API contract says otherwise.
- `Dead` is reached after required teardown and dependent clean-up completes;
  blocked clean-up surfaces as explicit blocking errors.
- Deletion is tied to `Dead`, with an exception where some entities become
  `Dead` and are removed in one operation, so `Dead` might not be externally observed.
- Forced removal is an exception path that may bypass parts of the normal
  `Dying` workflow when explicitly required by the API contract.
- Lifecycle cascades are transactional where possible so related entities do not split into inconsistent states.
- Life lookup mappings remain consistent across code and schema (`alive`, `dying`, `dead`).

## Watchers and Notifications

- `NotifyWatcher` is used for single-subject change streams with `struct{}` notifications.
- `StringsWatcher` is used for collection change streams with `[]string` changed identifiers.
- Notifications communicate that change happened rather than carrying full entity state.
- Collection watchers report initial collection members and then deltas.
- Collection notifications carry only changed identifiers relevant to the watcher concern.
- Coalesced notifications are expected: multiple changes between reads may
  collapse to one notification or one changed identifier.
- Consumer workflows re-query current state for changed entities before acting.
- Mapper/filter logic can drop events; empty identifier notifications are valid and are not errors.

## Relations

- A relation connects two application endpoints with compatible interface types.
- Endpoint roles: `provides`, `requires`, `peer`.
- Peer relations connect units of the same application (e.g., for clustering).
- Departure hooks (relation-departed, relation-broken) fire during Dying.
- Cross-model relations (CMR) use offers and consumers across model boundaries.
  The relation data exchange is the same; the transport is different.

## Secrets

- Secrets have owners (unit or application) and access grants via relations.
- Secret lifecycle: create -> grant -> get -> rotate -> expire -> revoke -> remove.
- Secrets are backend-agnostic. Internal backend (controller DB) is default;
  external backends (e.g., Vault) are configurable per model.
- Secret content is versioned by revision. Consumers track their current revision
  and must explicitly refresh to see newer revisions.
