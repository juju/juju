---
myst:
  html_meta:
    description: "Guide for Juju 3.6 to 4.0 transition: key behavior changes for charm developers and operators, with examples. Prepare your charms and operations for Juju 4.0 with this comprehensive guide."
---

(upgrade-your-deployment-from-36-to-40)=
# Upgrade your Juju deployment from `3.6` to `4.0`

> **Disclaimer:** this is v1 of the document. We will update it with more details and examples over time.

This guide lists the main behavior changes when moving from Juju `3.6` to `4.0`.

## Changes for charm developers

### 1. Deploy from a directory is removed (`juju deploy <local-dir>`)

In `4.0` Juju does not package a charm directory during deploy. Build the charm first, then deploy the built artifact.

**Juju 3.6**
```bash
juju deploy /my-charm-directory
```

**Juju 4.0**
```bash
# Build the charm artifact first (then deploy the file):
juju deploy ./my-charm.charm
```

### 2. `JUJU_TARGET_SERIES` removed; use `JUJU_TARGET_BASE`

Hook context no longer provides `JUJU_TARGET_SERIES`. Use `JUJU_TARGET_BASE` (example format: `ubuntu@22.04`).

**Juju 3.6**
```bash
# legacy logic
if [ "$JUJU_TARGET_SERIES" = "focal" ]; then
  echo "target focal"
fi
```

**Juju 4.0**
```bash
# new logic
case "$JUJU_TARGET_BASE" in
  ubuntu@22.04) echo "target ubuntu 22.04" ;;
  ubuntu@24.04) echo "target ubuntu 24.04" ;;
esac
```

### 3. Base safety: `deploy --force` is stricter; `refresh --force-series` removed

`juju deploy --force` can no longer deploy onto a base the charm does not declare.
`juju refresh --force-series` is removed; use `--force-base`.

**Juju 3.6**
```bash
# refresh supports --force-series in 3.6
juju refresh myapp --force-series
```

**Juju 4.0**
```bash
juju refresh myapp --force-base
```

### 4. Series → Bases (deploy/add-machine scenarios)

Series are removed in `4.0`; bases are required.

**Juju 3.6**
```
# Some scripts still pass series names (`bionic`/`focal`/…).
```

**Juju 4.0**
```
# Always use bases (example format: `ubuntu@22.04`).
```

### 5. Leader settings removed (`leader-get`, `leader-set`)

Leader settings and the hook tools `leader-get` / `leader-set` are removed in `4.0`.

**Juju 3.6**
```bash
password="$(leader-get db-password)"
leader-set db-password="$new_password"
```

**Juju 4.0**
```
# No `leader-get` / `leader-set`.

# Use a peer relation and store shared data in the peer relation application databag
# (leader writes, everyone reads).
```


### 6. Kubernetes podspec charms no longer run (`k8s-set` / `k8s-get` removed)

Podspec charms stop working in `4.0`. Move to modern sidecar charms.

**Juju 3.6**
```bash
# legacy podspec pattern (example of removed hook tools)
k8s-set spec-file=pod-spec.yaml
k8s-get --format=json
```

**Juju 4.0**
Podspec charms do not run.
Rewrite as a sidecar charm pattern.

### 7. `private-address` is no longer auto-maintained in relation data

Juju no longer maintains `private-address` automatically. It used to be a copy of `ingres-address`. 
Use ingress address instead.

**Juju 3.6**
```bash
# many charms historically read:
relation-get private-address
```

**Juju 4.0**
```bash
# use ingress address info from Juju networking:
network-get --ingress-address <binding-name>
```

### 8. Actions: `additionalProperties` now defaults to `false`

In `4.0`, action schemas default `additionalProperties` to `false`. 
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

**Juju 4.0**
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

## Changes for Juju operators

### 1. Ubuntu fan networking removed

Fan networking is removed in `4.0`. If you rely on Juju-managed fan overlay addresses, 
migrate to other networking before migrating to `4.0`.

**Juju 3.6**
```
# Fan networking can be configured/used in `3.6` models.
```

**Juju 4.0**
```
# No Juju-managed fan networking in 4.0.
# Plan an alternative networking approach before migration.
```

### 2. MAAS: no default alpha space assumption

On MAAS, don’t assume an alpha space exists by default. Set `default-space` or bind endpoints before deploy.

**Juju 3.6**
```
# “alpha space exists” assumption may have worked in some setups.
```

**Juju 4.0**
```bash
# set model default space
juju model-config default-space=myspace
```

### 3. SSH Key Management changed: keys not auto-added to new models

After creating a new model, add SSH keys manually if you want `juju ssh` to work.

**Juju 3.6**
```
# Your user SSH key was automatically added to the model by default.
```

**Juju 4.0**
```bash
juju add-model mymodel
juju add-ssh-key "ssh-ed25519 AAAA... yourkeycomment"
```

### 4. Status API filtering removed (`StatusArgs.Patterns[]`)

Server-side status filtering via `StatusArgs.Patterns[]` is removed. 
API clients must fetch full status and filter client-side.

**Juju 3.6 (API consumer idea)**
```go
// pseudo-example
args := StatusArgs{ Patterns: []string{"mysql"} }
st, err := client.Status(args)
```

**Juju 4.0**
```go
// pseudo-example
st, err := client.Status(StatusArgs{})
// filter locally
```

### 5. Offers can’t be “updated” in place via `juju offer`

Re-running `juju offer` to change an existing offer is removed. Use remove + create flows.

**Juju 3.6**
```bash
juju offer myapp:mysql hosted-mysql
```

**Juju 4.0**
```bash
juju remove-offer hosted-mysql
juju offer myapp:mysql hosted-mysql
```

### 6. Provider type rename: `manual` → `unmanaged`

The provider name “Manual” is renamed to “Unmanaged”. Update scripts, tests, docs.

**Juju 3.6**
```
# cloud/provider type: `manual`
```

**Juju 4.0**
```
# cloud/provider type: `unmanaged`
```

### 7. Base/series commands removed: `upgrade-machine` and `set-application-base`

In-place base/series switching via these commands is removed in `4.0`. 
Plan base changes as “move/redeploy” workflows instead of mutating machines.

**Juju 3.6**
```bash
juju upgrade-machine 3 prepare ubuntu@18.04
juju upgrade-machine 3 complete

juju set-application-base myapp ubuntu@20.04
```

**Juju 4.0**
```
# These commands are removed.
# Use new machines on the new base, migrate workload, then remove old machines.
```

### 8. Controller HA: `enable-ha` removed; `juju-ha-space` removed (use binding)

`enable-ha` is removed; scale the controller like a normal application with `juju add-unit`.
Controller config `juju-ha-space` is removed; bind the controller application `dbcluster` endpoint instead.

**Juju 3.6**
```bash
juju enable-ha -n 3
```

**Juju 4.0**
```bash
# scale controller units in the controller model:
juju add-unit -m controller controller -n 2
```

### 9. LXD profiles removed (Kubernetes workloads)

LXD profiles are removed for Kubernetes workloads in `4.0`.

**Juju 3.6**
```
# Deploy could involve LXD profile handling/validation in some scenarios.
```

**Juju 4.0**
```
# No LXD profiles for Kubernetes workloads.
# Do not depend on LXD profile behavior in charm operations.
```

### 10. KVM provider removed; use LXD + “virtual-machine” constraint

KVM support is removed. Use LXD and a VM constraint instead.

**Juju 3.6**
```bash
juju add-machine kvm:1
```

**Juju 4.0**
```bash
# example pattern from release notes:
juju add-machine lxd:1 --constraints virt-type=virtual-machine
```

### 11. `juju status --watch` is dropped

`juju wait-for` and subcommands are removed in `4.0`. Use status polling and check readiness yourself.

**Juju 3.6**
```bash
juju wait-for application myapp --timeout=10m
```

**Juju 4.0**
```bash
# poll JSON status, then evaluate readiness client-side
juju status --format=json
```

### 12. `juju wait-for` removed (scripts/CI must change)

`juju wait-for` and subcommands are removed in `4.0`. Use status polling and check readiness yourself.

**Juju 3.6**
```bash
juju wait-for application myapp --timeout=10m
```

**Juju 4.0**
```bash
# poll status JSON, then evaluate readiness client-side
juju status --format=json
```

### 13. `juju export-bundle` removed

The `juju status` command cannot be used to watch status anymore. Please use the third-party `watch`.

**Juju 3.6**
```bash
juju export-bundle > bundle.yaml
```

**Juju 4.0**
```
# Command removed. Please use Juju Terraform Provider plans.
```

### 14. `juju status --watch` flag is dropped

Operators can’t use `juju status` to watch the changes (via polling) using this command in `4.0`.

**Juju 3.6**
```bash
juju status --watch=1s
```

**Juju 4.0**
```bash
# Client-side watcher (recommended portable pattern):
watch --color -n 1 juju status --color

# or any thirdparty watcher. For example
# viddy github: https://github.com/sachaos/viddy 
viddy juju status
```

### 15. Importing orphaned volumes or file systems creates new storage instances

If a storage volume or file system with no associated storage instance is migrated to `4.0`, a storage instance will be
created. These will be of the form `orphaned/x`. The purpose of such storage instances is to allow the operator to 
delete the linked volume or file system and its cloud resources should they desire.

### 16. Cross-model integration offers can only be consumed once

Juju `3.6` allowed operators to consume offers multiple times with different names in order to end up with multiple SAAS
entities for the same offer.

Juju `4.0` allows a maximum of one SAAS per remote offer in a consuming model.

If importing multiple SAAS entities for the same offer, Juju `4.0` will unify them into a single SAAS.
