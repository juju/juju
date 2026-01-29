(juju400)=
# Juju 4.0.0
🗓️ 14 Nov 2025

Juju 4.0.0 introduces a major architectural step toward a relational, Dqlite controller datastore and a simplified 
domain model. It removes long-deprecated surfaces (e.g., series, podspec charms) and tightens CLI behavior around 
bases and deployment safety. Several legacy APIs and workflows are intentionally deferred to the `4.0.x` / `4.x` cycle.

#### 🎯 Highlights:

* **New controller database architecture (Dqlite-first)**: A relational model backed by Dqlite (high-availability SQLite 
via Raft).
* **API and watcher model refactors**: The `AllWatcher` is removed from `4.0`.
* **Series fully retired in favor of bases**: 4.0 removes the concept of distribution “series”; bases are the only way 
to specify OS/runtime (the 3.6 docs already steer operations toward bases).
* **Provider rename**: The "Manual" provider type is renamed to "Unmanaged" across the codebase. Update any scripts/CI 
currently using 'manual'.

#### 🔄 Lifecycle upgrade/migrate path clarified

As in `3.6`, upgrade-controller does not allow jumping major/minor; `4.0` upgrades are expected via model migration to a 
`4.0` controller.  

```{note}
Model migration will appear in next patch versions.
```

#### ⚠️ Breaking changes

* **Controller HA enablement**: Use controller scaling with `juju add-unit` in the controller model instead of `enable-ha`. 
The `juju-ha-space` controller config item is removed in favour of binding the controller application `dbcluster` endpoint.
* **Default `alpha` space assumption on MAAS**: There is no MAAS `alpha` space by default; set the default space using the
`default-space=<space>` model config or bind endpoints explicitly before deploy.
* **LXD profiles for Kubernetes workloads**: LXD profiles are removed from `4.0`.
* **Base/series commands (`juju set-application-base, juju upgrade-machine / machine-upgrade`)**: In-place base or 
series switching via these commands is removed; use charm-defined upgrade flows.
* **SSH Key Management**: Your SSH keys are no longer automatically added to newly created models. After creating a new 
model, you must manually add your SSH keys using the `juju add-ssh-key` command if you want to use `juju ssh` to access 
machines in that model.
* **Kubernetes podspec charms (and `k8s-set / k8s-get` hook commands)**: Podspec charms no longer run; move to 
modern sidecar charms.
* **Filesystem import**: Importing filesystems is not implemented at GA. Deferred to 4.0.x.
* **Leader settings (`leader-get, leader-set`)**: Leader settings and the associated hook commands are removed.
* **AllWatcher**: Legacy watchers are removed; a lighter event API will replace them.
* **Series → Bases (`deploy` / `add-machine` scenarios)**: Series were deprecated in `3.x` and are removed in `4.0`; bases are
required.
* **Export bundle (`juju export-bundle`)**: Command removed.
* **KVM provider**: KVM support is removed; use LXD with `virtual-machine` constraint.
* **Ubuntu fan networking**: Fan networking is removed; migrate to alternative networking.
* **Deploy from directory (`juju deploy <local-dir>`)**: Juju no longer packages directories for deploy; use Charmcraft 
to build them, then juju deploy the built artifact.
* **Environment variable**: `JUJU_TARGET_SERIES` is removed from context; use `JUJU_TARGET_BASE`.
* **Safer deploy/refresh switches around bases**
  * `juju deploy --force` no longer allows deploying to a base not declared by the charm. 
  * `juju refresh --force-series` removed; use `--force-base`.
* **Status API filtering**: Server-side `StatusArgs.Patterns[]` filtering is removed (migrate to client-side filtering 
until a replacement arrives).
* **Offers update (`juju offer`)**: Updating an existing offer is removed; use create/remove flows.
* **Provider type rename (`manual` → `unmanaged`)**: Update all relevant cloud commands, bootstrap scripts, CI, and docs.
* **Wait-for**: `juju wait-for` (and subcommands `wait-for model|application|machine|unit`) — removed. In `3.6` this 
family streamed deltas and let you express goal states via a small DSL; it’s no longer available in `4.0`. Workarounds: 
poll `juju status --format=json` and evaluate readiness client-side.
* **Juju status`--watch` flag is dropped**: it is no longer available on commands like `juju status`. The alternative is 
to use any available watcher on the client side. For example: `watch -n 1 --color juju status --color`
* **`private-address` removed**:  it is no longer automatically maintained in relation data. It was a copy
of `ingres-address`, which is the only value that should be used now.
* **`additionalProperties` default changed**: in action parameter configuration, the `additionalProperties` default 
value no longer matches JSONSchema, and is instead `false`. Explicitly include `additionalProperties` rather than relying 
on the default value to have consistency across Juju 3.6 and Juju 4.

#### 🐛 Known issues / deferred items

* Model migrations (`≤3.x` → `4.0`): not available at GA; planned for `4.0.x`
* LXD profiles for Kubernetes workloads: planned for `4.x`
* Watcher API replacement: new, lighter event API to land during `4.x`
* Server-side status filtering replacement: client-side filtering only for now; a new server-side mechanism is planned for 4.x
* SSH keys are no longer automatically added to newly created models.

#### 📚  Notes for charm authors

* Ensure charms declare the correct bases; behavior that previously relied on series or on `--force-series`
(undeclared bases) is no longer accepted by the client.
* Podspec charms must be migrated to modern sidecar patterns to run on `4.0+`.
* `leader-get, leader-set` hook tools are removed.
* LXD profiles are removed.

#### 📘 Summary

Juju `4.0.0` represents a significant architectural milestone, delivering a scalable controller foundation built 
on Dqlite while streamlining the operational model through the removal of legacy features. While this major version 
introduces breaking changes that require migration planning, it sets the stage for a more maintainable and performant 
Juju experience. Charm authors and operators should review the breaking changes and migration guidance above to ensure 
a smooth transition to `4.0.0`.
