(juju4011)=
# Juju 4.0.11
🗓️ 2 Jun 2026

This is a cumulative bug fix release for Juju 4.0, covering changes from
`4.0.5` to `4.0.11`.

## 🎯 Highlights

* **Security and access checks are tighter**: fixes cover dependency
  vulnerabilities, JAAS bakery authentication, offer permissions, credential
  access, and Kubernetes-backed user secret creation.
* **Model migration and CMR are more reliable**: Juju now handles remote
  secrets, external users, relation key ordering, local charms, and relation
  rejoin behavior more consistently during import and migration.
* **Storage and Kubernetes lifecycle behavior is safer**: storage removal,
  volume tracking, storage directives, block device provenance, Kubernetes
  redeployment, and container-machine startup all receive targeted fixes.
* **Controller and worker stability improves**: fixes reduce hangs, panics,
  goroutine leaks, watcher stalls, and API caller blocking in common controller
  and agent paths.

Full cumulative list of changes:
https://github.com/juju/juju/compare/v4.0.5...20cb167e0dbadab011fcc246d7882220237e5606

## 🛠️ Fixes

### 🔒 Security, access, and authentication

This release tightens user, credential, offer, and secret access paths. It also
includes dependency updates for security vulnerabilities in Go networking and
streaming libraries.

* [fix: add mutex to gate concurrent macaroon user token access](https://github.com/juju/juju/pull/22194#top)
* [fix(access): filter credential model access by owner only](https://github.com/juju/juju/pull/22236#top)
* [fix: application offers permission check](https://github.com/juju/juju/pull/22349#top)
* [fix: jaas-bakery missing cleanDischargeURL and key](https://github.com/juju/juju/pull/22375#top)
* [fix: resolve security vuln in golang.org/x/crypto/ssh](https://github.com/juju/juju/pull/22480#top)
* [fix: resolve vulns found in golang.org/x/net/html](https://github.com/juju/juju/pull/22490#top)
* [fix: pass new secret ID to K8s backend on user secret create](https://github.com/juju/juju/pull/22493#top)
* [fix(deps): upgrade github.com/moby/spdystream to v0.5.1](https://github.com/juju/juju/pull/22501#top)

### 🔁 Migration, CMR, and relation correctness

Several fixes focus on the `3.6` to `4.0` transition path and CMR data
correctness. Remote secrets, local charms, external users, relation key order,
and relation rejoin behavior are handled more reliably.

* [feat(cmr): add support for remote secrets handling during migration](https://github.com/juju/juju/pull/21805#top)
* [feat(cmr): implement support for importing remote secrets](https://github.com/juju/juju/pull/21895#top)
* [fix: bug with migrating local charms](https://github.com/juju/juju/pull/21933#top)
* [fix(secretbackend): migration for secret in builtin CaaS backend](https://github.com/juju/juju/pull/22045#top)
* [fix(modelmigration): create external users on model import](https://github.com/juju/juju/pull/22117#top)
* [fix(cmr): populate offer connection details in list-offers API response](https://github.com/juju/juju/pull/22136#top)
* [fix(uniter): re-integrating the same endpoints re-fires relation-joined](https://github.com/juju/juju/pull/22290#top)
* [fix: correct the order of relation key during import](https://github.com/juju/juju/pull/22499#top)

### 🗃️ Storage and Kubernetes lifecycle

Storage handling is more robust across migration, refresh, removal, Kubernetes
stateful workloads, and status output. The fixes reduce the chance of stale
storage directives, lost volume context, incorrect provider data, or failed
cleanup blocking model operations.

* [feat: adopting filesystem from storage pool in storage domain service](https://github.com/juju/juju/pull/21725#top)
* [fix: drain storage provisioner worker timers](https://github.com/juju/juju/pull/21918#top)
* [fix: removal of units with storage](https://github.com/juju/juju/pull/22123#top)
* [fix: preserve unit storage directives during charm refresh](https://github.com/juju/juju/pull/21916#top)
* [fix: gracefully handle error when storage backing is not found](https://github.com/juju/juju/pull/22242#top)
* [fix: track block device provenance](https://github.com/juju/juju/pull/22240#top)
* [fix: return error instead of dropping unmatched storage attachments](https://github.com/juju/juju/pull/22223#top)
* [fix(storage): ensure storage pool is included](https://github.com/juju/juju/pull/22346#top)
* [fix: k8s redeployment issue with unit starting ordinal change](https://github.com/juju/juju/pull/22329#top)
* [fix: preserve remove-storage attachment errors](https://github.com/juju/juju/pull/22402#top)
* [fix: uninstall lxd-container-provisioner manifold on container machines](https://github.com/juju/juju/pull/22468#top)

### 🧱 Controller, API, worker, and provisioning stability

Controller and agent workers now handle more edge cases without hanging,
blocking, leaking goroutines, or panicking. The fixes target API callers,
watchers, the async charm downloader, the provisioner, the SSH server, Dqlite
listen addresses, and dbaccessor shutdown behavior.

* [fix: apiremotecaller racy report](https://github.com/juju/juju/pull/21902#top)
* [fix: only report controller set changes in apiaddresssetter](https://github.com/juju/juju/pull/21904#top)
* [fix: avoid blocking the apiremoterelationcaller loop](https://github.com/juju/juju/pull/21907#top)
* [fix: over-eager http client creation in asynccharmdownloader](https://github.com/juju/juju/pull/21908#top)
* [fix: sshserver check watcher closure and port changes](https://github.com/juju/juju/pull/21912#top)
* [fix: dbaccessor merged-channel leak](https://github.com/juju/juju/pull/21922#top)
* [fix: infinite recursive loop in provisioner task](https://github.com/juju/juju/pull/22068#top)
* [feat: add a timeout to downloads for asynccharmdownloader](https://github.com/juju/juju/pull/22082#top)
* [fix: dqlite mutual TLS dbaccessor now uses mutual TLS for dqlite peers](https://github.com/juju/juju/pull/21928#top)
* [fix: misc asynccharmdownloader issues](https://github.com/juju/juju/pull/22116#top)
* [fix: api remote caller](https://github.com/juju/juju/pull/22324#top)
* [fix: prevent panic in provisioner](https://github.com/juju/juju/pull/22358#top)

### 🧭 Operator-facing CLI, status, network, and offer output

User-facing output is more accurate and consistent. Fixes include command
output formatting, controller port reporting, relation network data,
`network-get`, offer listing, application-storage size display, hostname
handling, and stable relation information ordering.

* [fix(actions): preserve newlines in exec JSON output](https://github.com/juju/juju/pull/21929#top)
* [feat: relation ingress and egress data for unit settings](https://github.com/juju/juju/pull/22104#top)
* [fix: add controller ports in status output](https://github.com/juju/juju/pull/22134#top)
* [fix: persist Hostname when importing machine](https://github.com/juju/juju/pull/22208#top)
* [fix: list-offers by matching juju 3 behavior](https://github.com/juju/juju/pull/22277#top)
* [fix: network-get for mismatched endpoint and relation](https://github.com/juju/juju/pull/22296#top)
* [fix: application-storage tabular output to use humanize size](https://github.com/juju/juju/pull/22371#top)
* [fix(relation): restore stable relation-info order](https://github.com/juju/juju/pull/22412#top)

## 📘 Summary

`4.0.11` is a broad reliability patch release. It hardens Juju's access and
authentication behavior, closes security dependency gaps, improves the
migration and CMR path from `3.6` to `4.0`, and fixes storage, Kubernetes,
worker, API, and operator-output issues found after `4.0.5`.
