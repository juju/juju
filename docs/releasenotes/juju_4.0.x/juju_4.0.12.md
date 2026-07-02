(juju4012)=
# Juju 4.0.12
🗓️ 28 Jun 2026

This is a bug fix release for Juju 4.0, covering changes from `4.0.11` to
`4.0.12`.

## 🎯 Highlights

* **Migration and upgrade paths are more predictable**: fixes cover migration
  precheck authentication, external controller data, missing subnets, storage
  directives, ModelUpgrader compatibility, and upgrade-controller workflows.
* **Secret handling is more consistent under hooks**: secret create, update,
  delete, grant, revoke, and track-latest operations are now handled more
  atomically with hook commits.
* **Kubernetes, LXD, MAAS, and resource lifecycle behavior improves**: fixes
  restore Kubernetes status and constraints, LXD space-aware container NICs,
  MAAS VM provisioning, OCI registry handling, and `network-get` output.
* **Controller and agent stability is tighter**: fixes address Dqlite client
  closure, reboot handling, deployer startup ordering, log forwarding during
  upgrades, transaction retry behavior, and dependency vulnerabilities.

Full list of changes:
https://github.com/juju/juju/compare/v4.0.11...v4.0.12

## 🛠️ Fixes

### 🔒 Security, access, and authentication

This release tightens authentication and credential validation paths, improves
SSH support in the snap, and updates dependencies to resolve known
vulnerabilities.

* [chore: update deps for vuln GO-2026-5224](https://github.com/juju/juju/pull/22739#top)
* [fix(deps): bump Go 1.26.4 to resolve govulncheck failures](https://github.com/juju/juju/pull/22561#top)
* [fix(deps): eliminate github.com/juju/utils/v3 indirect dependency (GO-2025-3798)](https://github.com/juju/juju/pull/22582#top)
* [fix(credential): validate auth type against cloud on CheckCredentialModels](https://github.com/juju/juju/pull/22512#top)
* [fix: add model tag to JWT AuthInfo](https://github.com/juju/juju/pull/22598#top)
* [fix: token transport for bearer token auth had a typo](https://github.com/juju/juju/pull/22569#top)
* [fix(apiclient proxy): fix apiclient http transport proxy](https://github.com/juju/juju/pull/22507#top)
* [fix(snap): add FIDO/U2F security key support for juju ssh](https://github.com/juju/juju/pull/22676#top)

### 🔁 Migration, upgrade, and CMR correctness

Migration and upgrade fixes make the `4.0` transition path safer. Juju now
handles target authentication, unsupported same-version migrations, missing
network data, external controller addresses, storage directives, and upgrade
facade compatibility more explicitly.

* [feat(migration): exchange admin password for macaroon during prechecks](https://github.com/juju/juju/pull/22675#top)
* [feat: prevent 4.0 -> 4.0 migrations](https://github.com/juju/juju/pull/22659#top)
* [feat: migration export/import ddl](https://github.com/juju/juju/pull/22391#top)
* [feat(migration): add serialized model v2 export contract](https://github.com/juju/juju/pull/22601#top)
* [feat(modelmigration): add controller export helpers](https://github.com/juju/juju/pull/22614#top)
* [feat(modelmigration): wire SourceControllerInfo through domain service](https://github.com/juju/juju/pull/22653#top)
* [refactor: wire the migration master worker with the modelmigration domain](https://github.com/juju/juju/pull/22636#top)
* [feat(modelmigration): implement source-side domain service and state](https://github.com/juju/juju/pull/22555#top)
* [feat(modelmigration): implement model migration domain watchers](https://github.com/juju/juju/pull/22559#top)
* [feat: re-enable migrate check](https://github.com/juju/juju/pull/22581#top)
* [fix: 3.6 → 4.0 migration fails due to missing subnet (JUJU-9647)](https://github.com/juju/juju/pull/22605#top)
* [fix(migration): import application storage directives](https://github.com/juju/juju/pull/22615#top)
* [fix: ensure external controller addresses are always migrated](https://github.com/juju/juju/pull/22518#top)
* [feat: compatibility shim for ModelUpgrader facade](https://github.com/juju/juju/pull/22488#top)
* [fix: upgrade-controller with --build-agent](https://github.com/juju/juju/pull/22689#top)
* [fix: when upgrading, allow for the controller app to be missing](https://github.com/juju/juju/pull/22622#top)
* [feat: consistently show offer connections](https://github.com/juju/juju/pull/22541#top)
* [fix(cmr): offerer relations watcher initial query](https://github.com/juju/juju/pull/22566#top)
* [fix(relation): return remote-app units in ApplicationRelationsInfo for non-peer relations](https://github.com/juju/juju/pull/22720#top)

### 🔐 Secrets and transaction consistency

Secret operations triggered by hooks are now better aligned with the
`CommitHookChanges` transaction. The fixes reduce partial-update states for
secret deletion, grant, revoke, tracking, update, and backend reference
handling.

* [feat(uniter): move secret deletions into CommitHookChanges transaction](https://github.com/juju/juju/pull/22465#top)
* [JUJU-9033: Move secret grant/revoke into CommitHookChanges txn](https://github.com/juju/juju/pull/22544#top)
* [feat(uniter): move track-latest-secret into CommitHookChanges transaction](https://github.com/juju/juju/pull/22580#top)
* [feat: move secret updates into CommitHookChanges transaction](https://github.com/juju/juju/pull/22651#top)
* [fix: correct JOIN condition in secret content deletion queries](https://github.com/juju/juju/pull/22656#top)
* [fix(unitstate): prevent panic when charm state is empty at commit time](https://github.com/juju/juju/pull/22531#top)
* [feat: limit size of charm config and relation settings](https://github.com/juju/juju/pull/22664#top)
* [fix: transaction retry issues](https://github.com/juju/juju/pull/22562#top)

### 🗃️ Kubernetes, LXD, MAAS, networking, and resources

Provider and resource fixes improve day-2 behavior for Kubernetes applications,
LXD containers, MAAS-provisioned VMs, OCI-backed charm resources, and hook-tool
network output.

* [fix(status): restore k8s application address in status](https://github.com/juju/juju/pull/22459#top)
* [fix: change how k8s unit churn is detected to avoid incorrect status](https://github.com/juju/juju/pull/22498#top)
* [fix: properly handle legacy k8s modeloperator labels](https://github.com/juju/juju/pull/22510#top)
* [fix: preserve app constraints without model defaults](https://github.com/juju/juju/pull/22703#top)
* [fix: network-get returning empty placeholder device](https://github.com/juju/juju/pull/22726#top)
* [fix(lxd): resolve container NIC bridges from ProviderNetworkId so space subnets are attached again](https://github.com/juju/juju/pull/22714#top)
* [perf(add-model): add-model in lxd](https://github.com/juju/juju/pull/22657#top)
* [feat(maas): support explicit VM provisioning via virtual-machine virt-type](https://github.com/juju/juju/pull/22285#top)
* [fix: do not drop root disk constraints when custom networking](https://github.com/juju/juju/pull/22473#top)
* [fix: handle slow upload of k8s oci image metadata](https://github.com/juju/juju/pull/22553#top)
* [fix: include trailing / in oci repo paths when needed](https://github.com/juju/juju/pull/22554#top)

### 🧱 Controller, API, worker, and observability stability

Controller and agent paths handle more edge cases without leaking clients,
restarting unnecessarily, blocking upgrades, or losing operational visibility.
The fixes also improve metrics, status history, schema performance, and
workflow reliability.

* [fix: ensure Dqlite clients are closed](https://github.com/juju/juju/pull/22672#top)
* [fix(deployer): gate deployer on controlleragentconfig socket readiness](https://github.com/juju/juju/pull/22515#top)
* [fix: use errors.Is to detect reboot/shutdown sentinels in machine agent](https://github.com/juju/juju/pull/22708#top)
* [fix: guard against cloud init overwriting agent.conf on reboot](https://github.com/juju/juju/pull/22632#top)
* [feat: handle logsink 503 failure](https://github.com/juju/juju/pull/22590#top)
* [feat: include workload version in history logging](https://github.com/juju/juju/pull/22648#top)
* [feat: httpclient metrics](https://github.com/juju/juju/pull/22535#top)
* [fix: db repl](https://github.com/juju/juju/pull/22635#top)
* [fix: wait for workerpool to become idle](https://github.com/juju/juju/pull/22494#top)
* [fix(externalcontrollerupdater): close watcher client on shutdown race](https://github.com/juju/juju/pull/22607#top)
* [feat: add missing indexes and PKs](https://github.com/juju/juju/pull/22502#top)
* [fix(unmanaged): implement Schema() to satisfy ModelConfigProvider interface](https://github.com/juju/juju/pull/22514#top)

## 📘 Summary

`4.0.12` is a broad reliability patch release. It strengthens migration and
upgrade behavior, makes hook-driven secret changes more atomic, improves
Kubernetes, LXD, MAAS, networking, and resource workflows, and hardens
controller, worker, authentication, and dependency paths found after `4.0.11`.
