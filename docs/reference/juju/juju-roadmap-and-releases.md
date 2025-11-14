(juju-roadmap-and-releases)=
# Juju Roadmap & Releases

> See also: {ref}`upgrade-your-deployment`

This document is about our releases of Juju, that is, the `juju` CLI client and the Juju agents.

- We release new minor version (the 'x' of m.x.p) approximately every 3 months.
- Patch releases for supported series are released every month
- Once we release a new major version, the latest minor version of the previous release will become an LTS (Long Term Support) release.
- Minor releases are supported with bug fixes for a period of 4 months from their release date, and a further 2 months of security fixes. LTS releases will receive security fixes for 12 years.
- 4.0 is an exception to the rule, as it is still under development. We plan on releasing beta versions that are content driven and not time.

The rest of this document gives detailed information about each release.


<!--THERE ARE ISSUES WITH THE TARBALL.
```
$ wget https://github.com/juju/juju/archive/refs/tags/juju-2.9.46.zip
$ tar -xf juju-2.9.46.tar.gz
$ cd juju-juju-2.9.46
$ go run version/helper/main.go
3.4-beta1
```
ADD WHEN FIXED.
-->

<!--TEMPLATE
### 🔸 **Juju 4.0.X**
🗓️ <DATE>  <--leave this as TBC until released into stable!

⚙️ Features:
* feat(secrets): handle NotFound errors in secret backend during `RemoveUserSecrets` by @ca-scribner in [#19169](https://github.com/juju/juju/pull/19169)
* feat: open firewall ports for SSH server  proxy by @kian99 in [#19180](https://github.com/juju/juju/pull/19180)

🛠️ Fixes:
* fix: data race in state pool by @SimonRichardson in https://github.com/juju/juju/pull/19816
* fix: associate DNS config with interfaces as appropriate by @manadart in https://github.com/juju/juju/pull/19890

🥳 New Contributors:
* @nsklikas made their first contribution in https://github.com/juju/juju/pull/19821
-->


## * **Juju 4.0**

### ⭐ **Juju 4.0.0**
🗓️ 14 Nov 2025

Juju 4.0.0 introduces a major architectural step toward a relational, Dqlite controller datastore and a simplified 
domain model. It removes long-deprecated surfaces (e.g., series, Pod Spec charms) and tightens CLI behavior around 
bases and deployment safety. Several legacy APIs and workflows are intentionally deferred to the 4.0.x / 4.x cycle.

#### 🎯 Highlights:

* **New controller database architecture (Dqlite-first)**: A relational model backed by Dqlite (high-availability SQLite 
via Raft).
* **API and watcher model refactors**: The AllWatcher is removed from 4.0.
* **Series fully retired in favor of Bases**: 4.0 removes the concept of distribution “series”; bases are the only way 
to specify OS/runtime (the 3.6 docs already steer operations toward bases).
* **Provider rename**: The manual provider type is renamed to unmanaged across the codebase. Update any scripts/CI 
expecting manual.

#### 🔄 Lifecycle upgrade/migrate path clarified

As in 3.6, upgrade-controller does not allow jumping major/minor; 4.0 upgrades are expected via model migration to a 
4.0 controller.  

```{note}
Model migration will appear in next patch versions.
```

#### ⚠️ Breaking changes

* **Controller HA enablement**: Use controller scaling with juju add-unit in the controller model instead of enable-ha. 
The juju-ha-space controller config item is removed in favour of binding the controller application dbcluster endpoint.
* **Default alpha space assumption on MAAS**: There is no MAAS alpha space by default; set model-config 
default-space=<space> or bind endpoints explicitly before deploy.
* **LXD profiles for Kubernetes workloads**: LXD profiles are removed from 4.0.
* **Base/series commands (`juju set-application-base, juju upgrade-machine / machine-upgrade`)**: In-place base or 
series switching via these commands is removed; use charm-defined upgrade flows.
* **SSH Key Management**: Your SSH keys are no longer automatically added to newly created models. After creating a new 
model, you must manually add your SSH keys using the `juju add-ssh-key` command if you want to use `juju ssh` to access 
machines in that model.
* **Kubernetes Pod Spec charms (and `k8s-set / k8s-get` hook commands)**: Pod Spec–based charms no longer run; move to 
modern sidecar charms.
* **Filesystem import**: Importing filesystems is not implemented at GA. Deferred to 4.0.x.
* **Leader settings (`leader-get, leader-set`)**: Leader settings and the associated hook commands are removed.
* **AllWatcher**: Legacy watchers are removed; a lighter event API will replace them.
* **Series → Bases (deploy/add-machine scenarios)**: Series were deprecated in 3.x and are removed in 4.0; bases are
required.
* **Export bundle (`juju export-bundle`)**: Command removed.
* **KVM provider**: KVM support is removed; use LXD with virtual-machine constraint.
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
* **Provider type rename (_manual_ → _unmanaged_)**: Update clouds commands, bootstrap scripts, CI, and docs where 
updated
* **Wait-for**: `juju wait-for` (and subcommands `wait-for model|application|machine|unit`) — removed. In 3.6 this 
family streamed deltas and let you express goal states via a small DSL; it’s no longer available in 4.0. Workarounds: 
poll `juju status --format=json` (optionally with `--watch <interval>`) and evaluate readiness client-side.
* **"private-address" removed**:  it is no longer automatically maintained in relation data. It was a copy 
of "ingres-address", which is the only value that should be used now.

#### 🐛 Known issues / deferred items

* Model migrations (≤3.x → 4.0): not available at GA; planned for 4.0.x
* LXD profiles for k8s workloads: planned for 4.x
* Watcher API replacement: new, lighter event API to land during 4.x
* Server-side status filtering replacement: client-side filtering only for now; a new server-side mechanism is planned for 4.x
* SSH keys are no longer automatically added to newly created models.

#### 📚  Notes for charm authors

* Ensure charms declare the correct bases; behavior that previously relied on series or on `--force-series`
(undeclared bases) is no longer accepted by the client.
* Pod Spec charms must be migrated to modern sidecar patterns to run on 4.0+.
* `leader-get, leader-set` hook tools are removed.
* LXD profiles are removed.

#### 📘 Summary

Juju 4.0.0 represents a significant architectural milestone, delivering a scalable controller foundation built 
on Dqlite while streamlining the operational model through the removal of legacy features. While this major version 
introduces breaking changes that require migration planning, it sets the stage for a more maintainable and performant 
Juju experience. Charm authors and operators should review the breaking changes and migration guidance above to ensure 
a smooth transition to 4.0.0.
