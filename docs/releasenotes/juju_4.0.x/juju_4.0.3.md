(juju403)=
# Juju 4.0.3
🗓️ 4 Mar 2026

## 🎯 Highlights

* **`4.0.3` is the cumulative patch release from `4.0.1` to `4.0.3`**: `4.0.2` was not promoted, so this release carries the full stability and migration work from both patch versions.
* **Model migration coverage expands significantly**: Juju model config, machines, subnets, filesystems, volumes, and storage attachments imports have all been reworked.
* **CMR reliability improves end-to-end**: importing remote consumers/offerers, relation units, and duplicate remote applications is now safer and more consistent.
* **Storage and object-store behavior is more robust under load**: fixes target retries, leaked readers, spin-lock behavior, and deletion safety for de-duplicated blobs.
* **Lifecycle and operator UX is tightened up**: better removal ordering and foreign key (FK) cleanup, clearer action error output, and more accurate offer visibility.

## ⚠️ Breaking changes

N/A.

* **Upgrade note**: transition behavior for storage and offers is now explicitly documented; migrating models with multiple SAAS entries for one offer may collapse to a single SAAS entry, and orphaned storage may appear as storage instances to support cleanup flows.
  [feat: add transition information for storage and offers](https://github.com/juju/juju/pull/21832#top)

## 🚀 Features (key changes)

N/A. This is a patch release focused on migration completeness, reliability, and operational correctness rather than new top-level product features.

Key changes include:

* **Model migration now imports storage attachments with storage instances**: attachment records are imported as part of the same storage migration flow.
  [feat: import storage attachments alongside storage instances](https://github.com/juju/juju/pull/21846#top)

* **CMR migration merges duplicate remote applications**: duplicate remote applications are now merged during import.
  [feat: merging duplicate remote applications](https://github.com/juju/juju/pull/21785#top)

* **Storage listing works through the client facade**: storage instances, filesystems, and volumes are listed through the storage client path.
  [feat: storage list client](https://github.com/juju/juju/pull/21615#top)

* **`network-get` supports providers without spaces**: networking data is now returned correctly for providers that do not implement space support.
  [fix: network-get for providers without space support](https://github.com/juju/juju/pull/21754#top)

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

## 📘 Summary

`4.0.3` is a reliability-focused release: it finishes major chunks of the 3.6→4.0 migration path, hardens CMR and storage/object-store behavior in real workloads, and improves day-2 operator visibility and lifecycle safety across offers, actions, and cloud cleanup workflows.
