---
myst:
  html_meta:
    description: "Guide for Juju 3.6 to 4.1 transition: key behavior changes for charm developers and operators, with examples. Prepare your charms and operations for Juju 4.1 with this comprehensive guide."
---

(upgrade-your-deployment-from-36-to-41)=
# Upgrade your Juju deployment from `3.6` to `4.1`

> **Note:** This guide is checked against the Juju `4.1` source and the `4.0.x` release notes. It will gain more details and examples over time.

This guide lists the main behavior changes when moving from Juju `3.6` to `4.1`.

## Changes for charm developers

### 1. Deploy from a directory is removed (`juju deploy <local-dir>`)

Since `4.0`, Juju does not package a charm directory during deploy. Build the charm first, then deploy the built artifact.

**Juju 3.6**
```bash
juju deploy /my-charm-directory
```

**Juju 4.1**
```bash
# Build the charm artifact first (then deploy the file):
juju deploy ./my-charm.charm
```

### 2. `JUJU_TARGET_SERIES` and `JUJU_TARGET_BASE` are not set

`JUJU_TARGET_SERIES` was removed from the hook context in `4.0`. Juju `4.1` does not set `JUJU_TARGET_BASE` either, because the upgrade-series hooks that used them no longer exist. Do not rely on either variable; to find out the base a charm runs on, use the charm's own environment (for example, `/etc/os-release`).

**Juju 3.6**
```bash
# legacy logic (upgrade-series hooks only)
if [ "$JUJU_TARGET_SERIES" = "focal" ]; then
  echo "target focal"
fi
```

**Juju 4.1**
```bash
# Neither variable is set. Inspect the machine instead:
. /etc/os-release
echo "running on ${ID}@${VERSION_ID}"
```

### 3. Base safety: `deploy --force` is stricter; `refresh --force-series` removed

`juju deploy --force` can no longer deploy onto a base the charm does not declare.
`juju refresh --force-series` is removed; use `--force-base`.

**Juju 3.6**
```bash
# refresh supports --force-series in 3.6
juju refresh myapp --force-series
```

**Juju 4.1**
```bash
juju refresh myapp --force-base
```

### 4. Series → Bases (deploy/add-machine scenarios)

Series were removed in `4.0`; bases are required.

**Juju 3.6**
```
# Some scripts still pass series names (`bionic`/`focal`/…).
```

**Juju 4.1**
```
# Always use bases (example format: `ubuntu@22.04`).
```

### 5. Leader settings removed (`leader-get`, `leader-set`)

Leader settings and the hook tools `leader-get` / `leader-set` were removed in `4.0`.

**Juju 3.6**
```bash
password="$(leader-get db-password)"
leader-set db-password="$new_password"
```

**Juju 4.1**
```
# No `leader-get` / `leader-set`.

# Use a peer relation and store shared data in the peer relation application databag
# (leader writes, everyone reads).
```


### 6. Kubernetes podspec charms no longer run (`k8s-set` / `k8s-get` removed)

Podspec charms stopped working in `4.0`. Move to modern sidecar charms.

**Juju 3.6**
```bash
# legacy podspec pattern (example of removed hook tools)
k8s-set spec-file=pod-spec.yaml
k8s-get --format=json
```

**Juju 4.1**
Podspec charms do not run.
Rewrite as a sidecar charm pattern.

### 7. `private-address` is no longer auto-maintained in relation data

Since `4.0`, Juju no longer maintains `private-address` automatically. It used to be a copy of `ingress-address`.
Use the ingress address instead.

**Juju 3.6**
```bash
# many charms historically read:
relation-get private-address
```

**Juju 4.1**
```bash
# use ingress address info from Juju networking:
network-get --ingress-address <binding-name>
```

### 8. Actions: `additionalProperties` now defaults to `false`

Since `4.0`, action schemas default `additionalProperties` to `false`. 
If your action accepts arbitrary keys, set it explicitly.

**Juju 3.6**
```yaml
my-action:
  description: Example action
  params:
    type: object
    properties:
      reason:
        type: string
  # additionalProperties not set
```

**Juju 4.1**
```yaml
my-action:
  description: Example action
  params:
    type: object
    additionalProperties: true   # only if you want arbitrary keys
    properties:
      reason:
        type: string
```

### 9. Juju defaults to provider-specific storage pools for filesystems

In `3.6`, due to a bug, Juju would also default to the `rootfs` storage pool to
create filesystems, even if a provider-specific storage pool was available.

In `4.0` and `4.1`, if a provider recommends its own storage pool, Juju defaults
to using it, e.g., `ebs` for `aws`, `cinder` for `openstack`, `lxd` for `lxd`.
Providers that do not recommend a pool of their own, for example `maas`, still
default to `rootfs`.

`rootfs` in general shouldn't be used for production deployments, since data is
stored on a machine's root filesystem instead of a separate dedicated entity.

Filesystems from these pools can sometimes have subtly different properties to
`rootfs` filesystems. Charm developers should ensure their charms are not
locked-in to the `rootfs` storage pools.

To replicate the `3.6` behaviour, explicitly set the storage pool to `rootfs` in
the storage directive. E.g.
```
$ juju deploy postgresql --storage pgdata=rootfs,10G
```

## Changes for Juju operators

### 1. Ubuntu fan networking removed

Fan networking was removed in `4.0`. If you rely on Juju-managed fan overlay addresses,
migrate to other networking before migrating to `4.1`. Model migration refuses models that have `fan-config` set or `container-networking-method=fan`.

**Juju 3.6**
```
# Fan networking can be configured/used in `3.6` models.
```

**Juju 4.1**
```
# No Juju-managed fan networking in 4.1.
# Plan an alternative networking approach before migration.
```

### 2. MAAS: no default alpha space assumption

Since `4.0`, on MAAS, don’t assume an alpha space exists by default. Set `default-space` or bind endpoints before deploy.

**Juju 3.6**
```
# “alpha space exists” assumption may have worked in some setups.
```

**Juju 4.1**
```bash
# set model default space
juju model-config default-space=myspace
```

### 3. SSH Key Management changed: keys not auto-added to new models

Since `4.0`, after creating a new model, add SSH keys manually if you want `juju ssh` to work.

**Juju 3.6**
```
# Your user SSH key was automatically added to the model by default.
```

**Juju 4.1**
```bash
juju add-model mymodel
juju add-ssh-key "ssh-ed25519 AAAA... yourkeycomment"
```

### 4. Status filtering is limited to exact names

Server-side status filtering via `StatusArgs.Patterns[]` was removed in `4.0.0` and restored in the `4.0.x` series (from `4.0.8`). In `4.1`, `juju status` accepts selectors again, with these limits:
- Matching occurs only on applications, units and machines.
- Filtering is based on complete matches - no wildcards or globbing. Examples:
  - `postgresql` (application)
  - `postgresql/2` (unit)
  - `postgresql/leader` (the application's leader unit)
  - `0/lxd/1` (machine)

**Juju 3.6**
```bash
juju status 'postgres*'
```

**Juju 4.1**
```bash
# exact names only
juju status postgresql
juju status postgresql/2
```

### 5. `juju show-unit` relation `application-data` now uses the local side (was remote in `3.6`)

Since `4.0`, `juju show-unit <unit>` reports relation `application-data` set by the unit leader in the local application data bag. This differs from `3.6`, where the displayed `application-data` was set by the unit leader on the remote side of the relation. If automation previously parsed `application-data` from `juju show-unit wordpress/0` to inspect a related application's data, query a unit on the other side of that relation instead, for example `juju show-unit mysql/0`, or otherwise update the logic to use the local-side orientation.

**Juju 3.6**

```bash
# `application-data` is from the remote/opposite application.
juju show-unit wordpress/0
```

**Juju 4.1**

```bash
# `application-data` is from wordpress's local application.
juju show-unit wordpress/0

# Query a related unit if your automation needs the opposite side.
juju show-unit mysql/0
```

### 6. Offers can’t be “updated” in place via `juju offer`

Re-running `juju offer` to change an existing offer was removed in `4.0`. Use remove + create flows.

**Juju 3.6**
```bash
juju offer myapp:mysql hosted-mysql
```

**Juju 4.1**
```bash
juju remove-offer hosted-mysql
juju offer myapp:mysql hosted-mysql
```

### 7. Provider type rename: `manual` → `unmanaged`

The provider name “Manual” was renamed to “Unmanaged” in `4.0`. Update scripts, tests, docs.

**Juju 3.6**
```
# cloud/provider type: `manual`
```

**Juju 4.1**
```
# cloud/provider type: `unmanaged`
```

### 8. Base/series commands removed: `upgrade-machine` and `set-application-base`

In-place base/series switching via these commands was removed in `4.0`.
Plan base changes as “move/redeploy” workflows instead of mutating machines.

**Juju 3.6**
```bash
juju upgrade-machine 3 prepare ubuntu@18.04
juju upgrade-machine 3 complete

juju set-application-base myapp ubuntu@20.04
```

**Juju 4.1**
```
# These commands are removed.
# Use new machines on the new base, migrate workload, then remove old machines.
```

### 9. Controller HA: `enable-ha` removed; `juju-ha-space` removed (use binding)

`enable-ha` was removed in `4.0`; scale the controller like a normal application with `juju add-unit`.
Controller config `juju-ha-space` was removed; bind the controller application `dbcluster` endpoint instead.

**Juju 3.6**
```bash
juju enable-ha -n 3
```

**Juju 4.1**
```bash
# scale controller units in the controller model:
juju add-unit -m controller controller -n 2
```

### 10. LXD profiles removed (Kubernetes workloads)

LXD profiles were removed for Kubernetes workloads in `4.0`.

**Juju 3.6**
```
# Deploy could involve LXD profile handling/validation in some scenarios.
```

**Juju 4.1**
```
# No LXD profiles for Kubernetes workloads.
# Do not depend on LXD profile behavior in charm operations.
```

### 11. KVM provider removed; use LXD + “virtual-machine” constraint

KVM support was removed in `4.0`. Use LXD and a VM constraint instead.

**Juju 3.6**
```bash
juju add-machine kvm:1
```

**Juju 4.1**
```bash
# example pattern from release notes:
juju add-machine lxd:1 --constraints virt-type=virtual-machine
```

### 12. `juju wait-for` removed (scripts/CI must change)

`juju wait-for` and subcommands were removed in `4.0`. Use status polling and check readiness yourself.

**Juju 3.6**
```bash
juju wait-for application myapp --timeout=10m
```

**Juju 4.1**
```bash
# poll status JSON, then evaluate readiness client-side
juju status --format=json
```

### 13. `juju export-bundle` no longer works

`juju export-bundle` still exists in the CLI, but since `4.0` the controller rejects it with a not-implemented error ("Juju 4.0 doesn't support exporting bundles").

**Juju 3.6**
```bash
juju export-bundle > bundle.yaml
```

**Juju 4.1**
```
# The command fails. Please use Juju Terraform Provider plans.
```

### 14. `juju status --watch` flag is dropped

Since `4.0`, operators can’t use `juju status` to watch the changes (via polling) using this command.

**Juju 3.6**
```bash
juju status --watch=1s
```

**Juju 4.1**
```bash
# Client-side watcher (recommended portable pattern):
watch --color -n 1 juju status --color

# or any thirdparty watcher. For example
# viddy github: https://github.com/sachaos/viddy 
viddy juju status
```

### 15. Volume-backed storage pools are preferred by default

Where possible, Juju defaults to use volume-backed storage pools to create filesystems.

**Juju 3.6**
```bash
juju deploy postgresql --storage pgdata=10G
# This will result in a `rootfs` filesystem
```

**Juju 4.1**
```bash
juju deploy postgresql --storage pgdata=10G
# This will result in `ebs`, `azure`, `gce`, `cinder`, `lxd`, etc. volume-backed filesystems for the providers `aws`, `azure`, `gce`, `openstack`, `lxd`, etc. respectively
# (providers that recommend no pool of their own, such as `maas`, still get `rootfs`)

# To replicate the 3.6 behaviour, set the storage pool to `rootfs` in the storage directive
juju deploy postgresql --storage pgdata=rootfs,10G
```

### 16. Spaces are explicit

Juju `3.6` and prior treated the `alpha` space as a space agnostic request in some cases.

Juju `4.0` and `4.1` treat spaces used for relation endpoint bindings of configuration settings as explicit requests in all 
cases. This means that any time there is a space _other_ than the `alpha` space, care should be taken to set an 
appropriate value for `default-space` in model configuration or choose a suitable space for all bindings.

This is most relevant to MAAS, where Juju detects spaces on the cloud. This invariably means that (default) `alpha` 
spaces does not contain any subnets.

### 17. Cross-model integration offers can only be consumed once

Juju `3.6` allowed operators to consume offers multiple times with different names in order to end up with multiple SAAS
entities for the same offer.

Juju `4.0` and `4.1` allow a maximum of one SAAS per remote offer in a consuming model.

If importing multiple SAAS entities for the same offer, Juju will unify them into a single SAAS.

### 18. Backups are not available

Dqlite-based backups are not delivered yet. In `4.0` and `4.1`, `juju create-backup` fails with a not-implemented error.

**Juju 3.6**
```bash
juju create-backup
```

**Juju 4.1**
```
# The command fails ("Dqlite-based backups not implemented").
# Plan for controller recovery without `juju create-backup`.
```

### 19. `juju ssh` and `juju scp` go through the controller by default

New in `4.1`: `juju ssh` and `juju scp` proxy the connection through the controller's SSH server (`ssh-server-port`, default `17022`, which must be open on the controller machine). Connecting to nested containers running on machines is not supported this way. Use `--direct` to connect directly, as in `3.6`; this is required for Juju 3 controllers.

**Juju 3.6**
```bash
juju ssh 0
```

**Juju 4.1**
```bash
# proxied through the controller SSH server (default)
juju ssh 0

# connect directly, as in 3.6
juju ssh --direct 0
```

### 20. Bootstrap no longer adds your SSH keys

New in `4.1`: `juju bootstrap` no longer reads the public keys in `~/.ssh` (or creates a key in the Juju home directory) to authorize them on the controller. Provide keys explicitly with `--config authorized-keys` or `--config authorized-keys-path`, or add them afterwards with `juju add-ssh-key`.

**Juju 3.6**
```bash
juju bootstrap mycloud mycontroller
# Your default SSH public keys were added automatically.
```

**Juju 4.1**
```bash
juju bootstrap mycloud mycontroller --config authorized-keys-path=~/.ssh/id_ed25519.pub
```
