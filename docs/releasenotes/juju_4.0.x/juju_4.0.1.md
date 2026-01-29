(juju401)=
# Juju 4.0.1
🗓️ 22 Jan 2026

## 🎯 Highlights

* **Migration/import reliability work continues**: more of the end-to-end plumbing lands (activation, rollback safety, and
  import ordering fixes).
* **Controller destruction is safer and clearer**: better behavior when the controller still hosts models.
* **Vault secrets are easier to integrate**: mount-path support reduces “works only with my Vault layout” friction.
* **More helpful operational visibility**: richer status/cluster visibility and more resilient debug tooling.

## ⚠️ Breaking changes

* **Kubernetes attach-storage feature flag removed**: `--attach-storage` works on Kubernetes models without needing a feature flag.
  [feat: remove k8s attach storage feature flag](https://github.com/juju/juju/pull/20634#top)

## 🚀 Features (key changes)

This is a patch release focused on reliability and usability improvements. There is no new major functionality. 
Key changes include:

* **Model import becomes “transactional” at the end**: importing a model now **activates it only at the final step**,
  treating activation as the commit point across multiple import transactions. This reduces “half-imported model” states.
  [feat: activate model on import step finale](https://github.com/juju/juju/pull/21493#top)

* **Import/migration gains stronger rollback tooling**: adds tooling to inspect database tables **inside a transaction**
  before rollback, making foreign-key failures debuggable (SQLite errors can be extremely vague otherwise).
  [feat: add tooling to view tables within transaction before rollback](https://github.com/juju/juju/pull/21361#top)

* **Controller destruction: hosted-model handling is explicit**: Juju now prompts/handles controller destruction
  workflows more safely when hosted models exist (instead of treating “controller model only” and “hosted models present”
  the same way).
  [feat: destroy controller hosted models](https://github.com/juju/juju/pull/21173#top)

* **Secrets + Vault: mount-path support**: Vault-backed secrets can be configured with an explicit **Vault mount path**,
  so Juju can fit into pre-existing Vault mount layouts cleanly.
  [feat(secrets): add support for vault mount path](https://github.com/juju/juju/pull/21229#top)

* **Operational visibility: cluster details in status**: `juju status` can surface cluster-level details, improving
  “at a glance” understanding of controller health/topology.
  [feat: output cluster details in status](https://github.com/juju/juju/pull/21246#top)

Full ist list of changes: https://github.com/juju/juju/releases/tag/v4.0.1

## 🛠️ Fixes

### 🔐 Secrets and secret backends

Secrets had correctness gaps that operators hit quickly. Delete operations failed if the external secret was already 
gone. Rotate policy updates didn’t always stick. Label + refresh paths could return the wrong value.

Listing also needed to help humans. If output lacks backend name or revision context, operators can’t audit or debug. 
And when Kubernetes tunnelling and Vault mount paths interact, Juju must keep behavior consistent instead of 
“works here, fails there.”

* [fix: handle case where external secret not found on delete](https://github.com/juju/juju/pull/21058#top)
* [fix(secretbackend): support complex config values](https://github.com/juju/juju/pull/21211#top)
* [fix: ensure secret rotate policy can be updated](https://github.com/juju/juju/pull/21220#top)
* [fix: ensure secret-get with label and refresh returns the right value](https://github.com/juju/juju/pull/21228#top)
* [fix: k8s tunneller context and vault mount path compatibility](https://github.com/juju/juju/pull/21253#top)
* [fix(secretbackend): immutability for built-in backends](https://github.com/juju/juju/pull/21235#top)
* [fix(secrets): rotate policy handling](https://github.com/juju/juju/pull/21320#top)
* [fix(secret): support filtering secrets by label](https://github.com/juju/juju/pull/21327#top)
* [fix(secret): allows several units having the same label for owned secret](https://github.com/juju/juju/pull/21373#top)
* [fix(secret): updates a secret while passing the label](https://github.com/juju/juju/pull/21468#top)
* [fix: k8s missing secret backend value for k8s and external vault](https://github.com/juju/juju/pull/21508#top)

### 🧭 Migration/import correctness

Import breaks hard when steps run in the wrong order. CMR models exposed this immediately: relations tried to load 
before remote applications existed, and the database rejected the write.

Legacy data also tripped migrations. Juju 4 drops fan networking, but Juju 3 can still export fan addresses. We need 
import to handle those leftovers instead of blocking migrations on old artifacts.

* [fix: issue prevening model migration to 3.x, bump dependencies](https://github.com/juju/juju/pull/21329#top)
* [fix: handle loopback addresses without subnet UUID in import](https://github.com/juju/juju/pull/21369#top)
* [fix(import): generate and include NetNodeUUID when creating machines](https://github.com/juju/juju/pull/21467#top)
* [fix(operation): finalize migration](https://github.com/juju/juju/pull/21531#top)
* [fix: relations must be imported after CMRs](https://github.com/juju/juju/pull/21566#top)
* [fix: import machine with left over fan addr](https://github.com/juju/juju/pull/21593#top)

### 🧱 Controller, API, and lifecycle safety

Operators hit controller flows that should never feel fragile: login, kill-controller, destroy-controller. 
In 4.0.0, some of these failed in edge cases like empty credentials or live hosted models, and the errors didn’t always 
help. HA made this worse. Connections break; Juju should reconnect cleanly and keep going, not restart workers in a way 
that loses the address it needs to recover.

* [fix: login to controllers](https://github.com/juju/juju/pull/21201#top)
* [fix: api remote caller reconnect on broken](https://github.com/juju/juju/pull/21385#top)
* [fix: show a helpful error message when destroying a controller with live models](https://github.com/juju/juju/pull/21382#top)
* [fix: persist controller node agent version](https://github.com/juju/juju/pull/21297#top)
* [fix: handle empty credential for kill-controller](https://github.com/juju/juju/pull/21514#top)

### 🔎 Status/history/logging

Juju leaked or held idle connections too long and could run out of file descriptors. It also logged only RPC-upgraded 
requests, which left gaps for other endpoints that operators care about.

Status history also needed to stay accurate. CAAS status entries missed timestamps, some “newest entry” behavior 
didn’t behave as expected, and application status history sometimes recorded under the wrong key—which breaks tools like
status-log.

* [fix: timeout idle fds](https://github.com/juju/juju/pull/21081#top)
* [fix: log all API requests not just RPC ones](https://github.com/juju/juju/pull/21102#top)
* [fix(status): return the newest entry in GetStatusHistory](https://github.com/juju/juju/pull/21299#top)
* [fix: application status history record](https://github.com/juju/juju/pull/21301#top)
* [fix: show-status-log for applications](https://github.com/juju/juju/pull/21310#top)
* [fix: set Since field on statuses on caas provisioning](https://github.com/juju/juju/pull/21281#top)

### 🌐 Networking, ports, firewalling, MAAS

Provider integrations can cause damage when they misbehave. On OpenStack, Juju risked deleting the wrong ports because 
it filtered too broadly. On MAAS, Juju treated volumes as MAAS-provisioned even when they weren’t, and tag validation 
didn’t always run depending on input shape.

Kubernetes firewaller logic also had sharp edges. It hid “not found” errors and didn’t shut down cleanly when apps 
disappeared, which led to unstable worker behavior and harder debugging.

* [fix: filter ports by server](https://github.com/juju/juju/pull/20791#top)
* [fix: caasfirewaller implementation issues in 4.0](https://github.com/juju/juju/pull/21178#top)
* [fix: tighten permissions on netplan config files](https://github.com/juju/juju/pull/21380#top)
* [fix: overriding managed-by label](https://github.com/juju/juju/pull/21477#top)
* [fix: MAAS storage provisioning](https://github.com/juju/juju/pull/21495#top)

### 📦 Resources, constraints, Charmhub interactions

Some failures came from “looks fine until deploy fails.” Space constraints broke because prepared statement caching 
collided on struct names, and expensive prepare calls inside transactions hurt both performance and stability.

Resource handling also needed to match real use. Juju should accept legitimate empty resources, record resource IDs 
correctly, and block uploads when the model isn’t in the right importing state. Mount path fixes belong here too: Juju 
must consult the right container mount definitions and mount workloads in the right place.

* [fix: use sqlair Preparer for space constraints](https://github.com/juju/juju/pull/21230#top)
* [fix: consume offer should take the offer name from offer url](https://github.com/juju/juju/pull/21236#top)
* [fix(resources): allows empty resources](https://github.com/juju/juju/pull/21268#top)
* [fix(resources): prevent resource upload when model is not importing](https://github.com/juju/juju/pull/21443#top)
* [fix: relation incompatible bases issue](https://github.com/juju/juju/pull/21419#top)
* [fix: charmhub request not using latest base track on error](https://github.com/juju/juju/pull/21427#top)
* [fix: consult charm container volume mounts for the correct mount path](https://github.com/juju/juju/pull/21486#top)
* [fix(modelconfig): coerce provider-specific config attributes from DB](https://github.com/juju/juju/pull/21469#top)

### 🧰 Build/test/tooling correctness

CI and safety checks should fail loudly and clearly when something breaks—and stay quiet when things are fine. Some 
integration tests failed without useful output, which slowed triage. Some watcher tests flaked under load due to timing 
assumptions.

* [fix: target specific build tags](https://github.com/juju/juju/pull/21252#top)
* [fix: task logs watcher test flakyness](https://github.com/juju/juju/pull/21260#top)
* [fix(charmrevisionner): refresh timer incorrectly reset on model-config change](https://github.com/juju/juju/pull/21316#top)
* [fix: obsolete rev watcher and consumed secrets watcher](https://github.com/juju/juju/pull/21333#top)
* [fix: transaction rollback handling](https://github.com/juju/juju/pull/21370#top)
* [fix: get correct controller uuid for image metadata cli](https://github.com/juju/juju/pull/21376#top)
* [fix: ddlgen script for versions with tags](https://github.com/juju/juju/pull/21416#top)
* [fix(ci): metrics tests didn't check the right logger](https://github.com/juju/juju/pull/21489#top)
* [fix(ci): update ubuntu charm in run_secret_drain](https://github.com/juju/juju/pull/21505#top)
* [fix(operation): improve error handling for undefined charm actions](https://github.com/juju/juju/pull/21498#top)
* [fix: check agent binary version and architecture](https://github.com/juju/juju/pull/21555#top)

### 🔒 Security / hardening

Provisioning needs modern config. Cloud-init’s older apt mirror settings no longer work the way users expect, so Juju 
must use the supported spec. Agent binary fetching needed stricter validation too—version alone isn’t enough.

* [fix: reduce the scope of the resources the mutating webhook matches on](https://github.com/juju/juju/pull/21377#top)
* [fix: use new apt mirror spec with cloud init](https://github.com/juju/juju/pull/21509#top)

### 📚 Documentation updates
**Secret backend package documentation**: introduces internal documentation for `secretbackend`, making backend behavior
  and extension points easier to reason about.
  [docs(secretbackend): introduce package documentation](https://github.com/juju/juju/pull/21237#top)

## 📘 Summary

Overall, `4.0.1` focuses on making the Juju `4.0` more dependable for real migrations, safer controller lifecycle 
operations, and smoother day-2 debugging and secrets usage.
