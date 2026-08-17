(juju4014)=
# Juju 4.0.14
🗓️ 18 Aug 2026

This is a cumulative bug fix release for Juju 4.0, covering changes from
`4.0.12` to `4.0.14`.

## 🎯 Highlights

* **Model migration and CMR are more reliable**: Juju now validates imported
  models before activation, implements source-side cleanup and login
  redirection, and handles leadership, log transfer, synthetic remote units,
  offers, and relation teardown more consistently.
* **Secret handling is more consistent under hooks**: secret creation and
  relation grants are committed atomically with other hook changes, including
  when Kubernetes secret backend RBAC is enabled.
* **Deployment and operator workflows are more predictable**: fixes cover
  charm and model type mismatches, action metadata, zone placement, machine
  constraints, model defaults, status filtering, and machine removal.
* **Bootstrap and provider reliability improves**: fixes apply proxy settings
  earlier, retry transient Kubernetes failures, improve S3 signing, and make
  MAAS, OpenStack Cinder, and controller address cleanup safer.

Full cumulative list of changes:
https://github.com/juju/juju/compare/v4.0.12...v4.0.14

## 🛠️ Fixes

### 🔒 Security and dependency maintenance

Go and cryptography dependencies were updated to address `GO-2026-4550` and
`GO-2026-5856`, including replacement of the deprecated OpenPGP package.

* [build(deps): bump go package and github action versions](https://github.com/juju/juju/pull/22828#top)

### 🔁 Migration and CMR correctness

This release implements target-side validation and source-side cleanup, and
fixes several migration and cross-model relation (CMR) correctness issues.
Validation covers secret backends and references, relation-unit consistency,
cloud credential reachability, and agent binary availability. A failure leaves
the imported model gated and abortable.

These changes harden specific migration phases and prechecks. They do not
establish complete end-to-end validation of every migration scenario.
Migration from a `4.0` controller to another `4.0` controller remains
unsupported.

* [feat: model migration validation](https://github.com/juju/juju/pull/22945#top)
* [feat: implement source-side REAP with durable login redirection for 4.x model migration](https://github.com/juju/juju/pull/22817#top)
* [fix: restore controller-machine access to the /log endpoint](https://github.com/juju/juju/pull/22965#top)
* [fix(migration): correct swapped lease key fields in leadership import](https://github.com/juju/juju/pull/22774#top)
* [fix(migration): ignore synthetic CMR units in version prechecks](https://github.com/juju/juju/pull/22927#top)
* [fix(migration): exclude synthetic CMR units from agent-version and status queries](https://github.com/juju/juju/pull/22972#top)
* [fix(cmr): make offer creation idempotent for identical offers](https://github.com/juju/juju/pull/22955#top)
* [fix: decrement offer connection count](https://github.com/juju/juju/pull/22997#top)
* [fix(cmr): ignore stale relation settings changes](https://github.com/juju/juju/pull/22914#top)

Redeploying a bundle with an unchanged offer now succeeds, but changing an
existing offer remains unsupported.

### 🔐 Secrets and transaction consistency

Secret creation and grants made by charms are now coordinated with the rest
of a hook's committed changes. Server-side secret ID reservations also allow
Kubernetes backend RBAC to authorize new charm secrets without allowing units
to claim arbitrary IDs.

* [feat: move secret creates into CommitHookChanges transaction](https://github.com/juju/juju/pull/22663#top)
* [fix: pass new secret IDs to backend config for charm secret RBAC](https://github.com/juju/juju/pull/22751#top)
* [fix(unitstate): grant pending secrets during hook commit](https://github.com/juju/juju/pull/23022#top)

### 🧭 Deployment, actions, constraints, and status

Deployment and lifecycle operations now report invalid charm and model
combinations earlier, make action metadata available during deployment, and
retain the settings required for subsequent provisioning and automation.
Kubernetes charms deployed to machine models are rejected unless `--force` is
used, while machine charms deployed to Kubernetes models produce a warning.
Bundle deployments are not covered by this mismatch check.

* [feat: handle charm and model type mismatch at deploy](https://github.com/juju/juju/pull/22773#top)
* [feat: fetch actions yaml at deploy time](https://github.com/juju/juju/pull/22890#top)
* [fix: support zone placement for local charm deploys](https://github.com/juju/juju/pull/22878#top)
* [fix(machine): persist merged model constraints on add-machine](https://github.com/juju/juju/pull/22936#top)
* [fix(model-defaults): preserve empty defaults and safe comparisons](https://github.com/juju/juju/pull/22792#top)
* [fix: app leader pattern matching in `juju status`](https://github.com/juju/juju/pull/22920#top)
* [fix: constraint errors when removing machines with storage](https://github.com/juju/juju/pull/22740#top)

### 🗃️ Providers, Kubernetes, networking, and object storage

Controller proxy settings are now applied before bootstrap makes outbound
requests. Kubernetes deployment retries transient proxy failures and pod loss,
while provider fixes improve MAAS VM ownership, OpenStack Cinder cleanup, and
controller API address selection.

S3 signing regions are derived from common AWS endpoint forms. The optional
`object-store-s3-region` controller setting takes precedence. If no region can
be derived for an object store using static credentials, Juju uses a
placeholder signing region and logs a warning; S3-compatible stores that
validate the region should set it explicitly.

* [fix: proxy updater sync issue](https://github.com/juju/juju/pull/22894#top)
* [fix: handle proxy connection errors](https://github.com/juju/juju/pull/22831#top)
* [refactor: retry pod failures during bootstrap](https://github.com/juju/juju/pull/22966#top)
* [fix(s3client): derive signing region from endpoint for S3 object store](https://github.com/juju/juju/pull/22790#top)
* [fix: tag maas vms created by juju and delete only them](https://github.com/juju/juju/pull/22700#top)
* [(3.6) fix(openstack): delete orphaned cinder attachments during volume destroy](https://github.com/juju/juju/pull/22753#top)
* [feat: exclude virtual Ethernet devices for API addresses](https://github.com/juju/juju/pull/22767#top)

### 🧱 Agent, API, and workload behavior

Pebble polling now includes notices created by non-root workload users, so
charms receive the corresponding `pebble-custom-notice` hooks. The Juju client
also reports its version on the main API WebSocket connection, giving JIMM the
metadata needed to apply compatibility rules in mixed controller fleets. Juju
controllers themselves currently ignore this header.

* [fix(uniter): include non-root user notices in pebbleNoticer polling](https://github.com/juju/juju/pull/22684#top)
* [feat(api): report client version on the main API WebSocket dial](https://github.com/juju/juju/pull/22794#top)

## 📘 Documentation

Documentation updates add user-secret lifecycle guidance and improve command
examples, limits, cloud references, controller configuration details, and
tutorial links.

* [docs: extend secret lifecycle section to cover user-secret lifecycle](https://github.com/juju/juju/pull/22529#top)
* [docs: add resource revision example to refresh help text](https://github.com/juju/juju/pull/22827#top)
* [docs: fix integrate cross-model relation help text](https://github.com/juju/juju/pull/22822#top)
* [docs: mention add-k8s --storage flag for setting default storage class](https://github.com/juju/juju/pull/22803#top)
* [docs: add 5 MiB size limit note for config values read from file](https://github.com/juju/juju/pull/22808#top)
* [docs: document 10,000 line maximum for debug-log --lines/--limit](https://github.com/juju/juju/pull/22825#top)
* [docs: fix caas-image-repo post-bootstrap changeability](https://github.com/juju/juju/pull/22812#top)
* [docs: 4.0 update cloud ref](https://github.com/juju/juju/pull/22765#top)
* [docs(tutorial): fix cloud-init URL to use juju/juju not canonical/juju](https://github.com/juju/juju/pull/22866#top)

## 📘 Summary

`4.0.14` is a cumulative reliability patch release. It hardens specific model
migration and CMR phases, makes hook-driven secret changes more atomic,
improves deployment and status workflows, and strengthens bootstrap,
Kubernetes, S3, MAAS, OpenStack, networking, and workload behavior since
`4.0.12`.
