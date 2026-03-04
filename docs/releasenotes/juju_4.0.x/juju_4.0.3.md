(juju403)=
# Juju 4.0.3
🗓️ 4 Mar 2026

## 🎯 Highlights

* **`4.0.3` is the cumulative patch release from `4.0.1` to `4.0.3`**: `4.0.2` was not promoted, so this release carries the full stability and migration work from both patch trains.
* **Model migration coverage expands significantly**: Juju model config, machines, subnets, filesystems, volumes, and storage attachments imports has been reworked.
* **CMR reliability improved end-to-end**: importing remote consumers/offerers, relation units, and duplicate remote applications is now safer and more consistent.
* **Storage and object-store behavior is more robust under load**: fixes target retries, leaked readers, spin-lock behavior, and deletion safety for de-duplicated blobs.
* **Lifecycle and operator UX tightened up**: better removal ordering and FK cleanup, clearer action error output, and more accurate offer visibility.

## ⚠️ Breaking changes

N/A

* **Upgrade note**: transition behavior for storage and offers is now explicitly documented; migrating models with multiple SAAS entries for one offer may collapse to a single SAAS entry, and orphaned storage may appear as storage instances to support cleanup flows.
  [feat: add transition information for storage and offers](https://github.com/juju/juju/pull/21832#top)

## 🚀 Features (key changes)

This is a patch release focused on migration completeness, reliability, and operational correctness rather than new top-level product features.

### 🧭 Migration/import pipeline maturity

* **Model config import is provider-aware** and now validated with dedicated integration coverage.
  [test: add integration tests for model config import](https://github.com/juju/juju/pull/21688#top)
* **Machine import is completed** (placement, constraints, and containers), reducing migration gaps.
  [feat: finish machine import](https://github.com/juju/juju/pull/21583#top)
* **Migration now handles `/32` devices without pre-existing subnets** by creating the needed subnet records.
  [fix: enable migration of /32 devices without subnets](https://github.com/juju/juju/pull/21707#top)
* **Storage import coverage now includes the full chain**:
  storage instances, filesystems, volumes, filesystem attachments, and storage attachments.
  [feat: import storage instances](https://github.com/juju/juju/pull/21732#top),
  [feat: import filesystems](https://github.com/juju/juju/pull/21767#top),
  [feat: import storage volumes](https://github.com/juju/juju/pull/21787#top),
  [feat(storage): import filesystem attachments for IAAS during model migration](https://github.com/juju/juju/pull/21835#top),
  [feat: import storage attachments alongside storage instances](https://github.com/juju/juju/pull/21846#top)
* **CMR migration support broadens** with remote consumer import, duplicate remote-app merging, and improved offerer/relation import.
  [feat: import remote application consumer](https://github.com/juju/juju/pull/21710#top),
  [feat: merging duplicate remote applications](https://github.com/juju/juju/pull/21785#top),
  [fix: import offerers applications and relations](https://github.com/juju/juju/pull/21840#top)

### 💾 Storage and status UX improvements

* **`juju storage` list behavior is restored/improved** for storage instances, filesystems, and volumes through the storage client facade.
  [feat: storage list client](https://github.com/juju/juju/pull/21615#top)
* **Charm refresh now reconciles storage directives correctly**, including user overrides and state updates.
  [feat: update storage directives during setcharm call useroverride and state](https://github.com/juju/juju/pull/21742#top)

### 🌐 Networking and operations clarity

* **`network-get` works correctly for providers without space support**, improving behavior on substrates like vSphere.
  [fix: network-get for providers without space support](https://github.com/juju/juju/pull/21754#top)
* **Operations filtering/querying is clearer and safer** through static SQL for `GetOperations`.
  [refactor(operation): use static SQL for GetOperations query](https://github.com/juju/juju/pull/21827#top)

Full cumulative list of changes: https://github.com/juju/juju/compare/v4.0.1...v4.0.3

## 🛠️ Fixes

### 🔁 Migration/CMR correctness

* [feat: allow model migration to be reentrant](https://github.com/juju/juju/pull/21714#top)
* [fix: relation unit watcher for remote app with no units](https://github.com/juju/juju/pull/21836#top)
* [fix: ensure we watch all relation units](https://github.com/juju/juju/pull/21828#top)
* [fix: nil pointer panics during caas migration](https://github.com/juju/juju/pull/21842#top)
* [fix(remoterelationconsumer): clean up orphaned offer_connection when local relation is removed during registration](https://github.com/juju/juju/pull/21802#top)

### 🗃️ Object store, resources, and firewaller stability

* [fix: prevent getting empty resources](https://github.com/juju/juju/pull/21864#top)
* [fix: only delete unreferenced objects from store](https://github.com/juju/juju/pull/21870#top)
* [fix: close all file object store readers](https://github.com/juju/juju/pull/21861#top)
* [fix: prevent spin-lock in object store scoped context](https://github.com/juju/juju/pull/21857#top)
* [fix: retries and not found errors in the remote objectstore](https://github.com/juju/juju/pull/21874#top)
* [feat: handle firewaller duplicate changes](https://github.com/juju/juju/pull/21859#top)

### 🧱 Lifecycle, offers, and cloud lifecycle safety

* [fix(removal): clean up FK references on app, unit, and relation delete](https://github.com/juju/juju/pull/21813#top)
* [fix(cloud): cascade credential deletion when removing a cloud](https://github.com/juju/juju/pull/21794#top)
* [fix(offers): show correct list of users in show-offer command](https://github.com/juju/juju/pull/21806#top)
* [fix(offer): use charm reference_name instead of metadata name in v_offer_detail view](https://github.com/juju/juju/pull/21792#top)
* [fix(actions): show correct message when task has error outcome](https://github.com/juju/juju/pull/21752#top)

### 🔒 Hardening and dependency updates

* [fix(domain): enforce UTC for database timestamp fields](https://github.com/juju/juju/pull/21773#top)
* [chore: update SQLair dependency](https://github.com/juju/juju/pull/21814#top)
* [fix(govuln): fix GO-2026-4559 bumping golang.org/x/net](https://github.com/juju/juju/commit/3ce1dee6890d4885929014c17b17ee98ef6064cd)

## 📘 Summary

`4.0.3` is a reliability-heavy release: it finishes major chunks of the 3.6→4.0 migration path, hardens CMR and storage/object-store behavior in real workloads, and improves day-2 operator visibility and lifecycle safety across offers, actions, and cloud cleanup workflows.
