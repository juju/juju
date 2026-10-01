(juju4015)=
# Juju 4.0.15
🗓️ 23 Sep 2026

This is a bug fix release for Juju 4.0, covering changes from `4.0.14` to
`4.0.15`. It also adds Kubernetes service-link configuration and Google Cloud
storage and image-selection options.

## 🎯 Highlights

* **Storage and removal workflows are more predictable**: Kubernetes filesystem
  import is restored, model destruction handles detached storage, and removal
  commands correctly report which storage will be detached or destroyed.
* **Controller availability and diagnostics improve**: lease expiry continues
  after an HA controller is removed, introspection no longer waits indefinitely
  for a Dqlite leader, and status reporting is more consistent.
* **Secrets and access checks are tighter**: fixes cover secret-backend draining,
  suspended cross-model relations, relation-status authorization, SSH key
  validation, file-storage paths, and certificate verification.
* **Bootstrap, provisioning, and migration are more reliable**: fixes cover
  proxies, controller address selection, model defaults, Kubernetes migration
  metadata, and Azure resource cleanup. Google Cloud gains opt-in Hyperdisk
  support and the `image-id` constraint.

Full list of changes:
https://github.com/juju/juju/compare/v4.0.14...v4.0.15

## 🛠️ Fixes

### 🔒 Security, access, and dependency maintenance

Relation-status updates now validate the authenticated unit and its application's
participation in the relation. SSH key parsing rejects input containing multiple
keys, and file-storage operations reject paths that escape the storage directory.
Certificate verification includes intermediate certificates, avoiding incorrect
trust prompts and connection failures for valid certificate chains.

The release also updates Go to `1.26.6` and refreshes dependencies, including
`golang.org/x/crypto` to `v0.57.0` and `golang.org/x/net` to `v0.59.0`.

* [fix(uniter): authorise the caller in SetRelationStatus](https://github.com/juju/juju/pull/23000#top)
* [fix(ssh): reject public key data describing more than one key](https://github.com/juju/juju/pull/22994#top)
* [fix: contain filestorage names within the storage directory](https://github.com/juju/juju/pull/22682#top)
* [chore: port intermediate ca_cert fix to 4.0](https://github.com/juju/juju/pull/23190#top)
* [chore: upgrade go version](https://github.com/juju/juju/pull/23042#top)
* [chore(deps): bump golang.org/x/crypto to v0.56.0](https://github.com/juju/juju/pull/23211#top)
* [fix: check if application is controller](https://github.com/juju/juju/pull/23308#top) — also includes the final dependency updates.

### 🔐 Secrets and cross-model access

Secret draining now grants access to secrets that have not yet reached the
destination backend. Vault drain tokens can create secrets as well as update
existing ones. Cross-model secret access is denied as soon as a relation is
marked suspended, rather than waiting for the relation hooks to complete.
Remote secret retrieval also avoids an unnecessary retry delay when macaroon
authentication needs renewal.

* [fix: allow vault drain token to create secrets when draining from internal backend](https://github.com/juju/juju/pull/23048#top)
* [fix(secrets): grant drain workers access to secrets not yet on the target backend](https://github.com/juju/juju/pull/23215#top)
* [fix(cmr): check relation suspended flag for cross model secret access](https://github.com/juju/juju/pull/23285#top)
* [fix: GetRemoteSecretContentInfo](https://github.com/juju/juju/pull/22973#top)

### 🗃️ Storage import, status, and removal

Kubernetes `juju import-filesystem` can import PersistentVolumes again, including
forced import of Juju-managed claims with ownership checks and protection against
concurrent claim changes and duplicate imports.

Model destruction now accounts for detached storage and respects the
`--destroy-storage` and `--release-storage` choices. Without an explicit choice,
persistent storage is checked before changing the model's lifecycle state.
Controller destruction waits for hosted models to be removed before tearing down
the controller, preventing orphaned Kubernetes model namespaces.

Removal commands and their dry runs again report which attached storage will be
detached or destroyed. Classification uses ownership scope, including volume-less
filesystems and unprovisioned volumes; `--destroy-storage` overrides detachment.
Storage status correctly reports volume persistence, and `add-storage` preserves
the configured size when no size override is supplied.

* [fix(kubernetes): implement filesystem import](https://github.com/juju/juju/pull/23056#top)
* [fix(removal): make destroy-model handle detached storage JUJU-10264](https://github.com/juju/juju/pull/23107#top)
* [fix(cli): wait for hosted models to be removed before destroying the controller](https://github.com/juju/juju/pull/23065#top)
* [feat(application): restore storage removal classification on destroy](https://github.com/juju/juju/pull/23205#top)
* [fix(storage): classify removal by ownership scope rather than persistence](https://github.com/juju/juju/pull/23274#top)
* [fix(storage): thread persistent flag through storage instance status chain](https://github.com/juju/juju/pull/22958#top)
* [fix(application): fix test-charm-storage-aws for juju 4.0](https://github.com/juju/juju/pull/23192#top) — also fixes the zero-size `add-storage` regression.
* [fix: concurrent unit removal with storage](https://github.com/juju/juju/pull/23085#top)

### 🧱 Controller availability, status, and diagnostics

The lease-expiry worker now runs on every controller node. Removing the node
running singular workers therefore no longer stops lease expiry and prevents
another node from taking over those workers.

Introspection uses bounded contexts when gathering dependency-engine and Dqlite
leader information, reporting a leader lookup error rather than hanging
indefinitely. This improves diagnostics; it does not introduce a new HA health
monitoring or automatic recovery system.

Application status messages are deterministic when units share the highest
severity: the leader is preferred when it has that severity, otherwise the
lowest-numbered matching unit is selected. Machine agent versions are restored
to full status output, provider instance status is recorded before network
updates, and controller-originated connections no longer interfere with model
machine-presence tracking.

* [fix: run lease-expiry worker on all controller nodes](https://github.com/juju/juju/pull/23145#top)
* [fix(tests): wait for voter quorum and correct HA teardown wait](https://github.com/juju/juju/pull/23197#top) — includes the runtime introspection fixes.
* [fix(domain/status): make derived application status deterministic](https://github.com/juju/juju/pull/23239#top)
* [fix: machine agent version in full status and simplestream upgrade tests](https://github.com/juju/juju/pull/23293#top)
* [fix: set polled instance status early](https://github.com/juju/juju/pull/23243#top)
* [fix: ignore controller connections when factoring presence](https://github.com/juju/juju/pull/23247#top)
* [fix: repeated context wrapping on transaction retries](https://github.com/juju/juju/pull/23157#top)
* [fix: wait for configchange.socket before deployer starts on every machine](https://github.com/juju/juju/pull/23122#top)
* [fix(controlsocket): use fresh password for GetUserByAuth on UserAlreadyExists](https://github.com/juju/juju/pull/23143#top)

### 🔁 Migration and API connection lifecycle

Migration fixes exclude synthetic CMR relations from inappropriate local-unit
validation, preserve SQL null values in model exports, and tolerate differences
in controller-local Kubernetes `rbac-id` metadata while keeping authentication
attribute comparisons strict. Credential mismatch errors no longer include
credential values.

Kubernetes migration metadata no longer requires agent binaries in the object
store, since those agents are distributed in OCI images, and expected migration
participants are derived from unit agents rather than removed per-application
operators. Model removal closes associated API connections without the startup
race that could crash the controller.

These changes harden specific migration phases and data handling. Migration
from a `4.0` controller to another `4.0` controller remains unsupported; the
changes do not establish support for that path.

* [fex: ignore cmr units for model migration readiness](https://github.com/juju/juju/pull/23094#top)
* [fix(modelmigration): preserve null values in model exports](https://github.com/juju/juju/pull/23283#top)
* [fix(credential): allow k8s migration with differing rbac IDs](https://github.com/juju/juju/pull/23275#top)
* [fix(modelagent): skip agent binary store check for CAAS models](https://github.com/juju/juju/pull/23219#top)
* [fix(modelmigration): report CAAS migration minions from unit agents only](https://github.com/juju/juju/pull/23220#top)
* [fix(apiserver): close model connections when removed](https://github.com/juju/juju/pull/23099#top)
* [fix(apiserver): close model connections from their own goroutine](https://github.com/juju/juju/pull/23177#top)
* [fix: return NotFound for destroy models](https://github.com/juju/juju/pull/23104#top)

### 🧭 Deployment, refresh, relations, and model configuration

Refreshing machine charms removes empty directories left behind by the previous
revision, avoiding stale Python package metadata that can break hooks.
Subordinate relations no longer create units recursively, and deleted relations
cannot remain in, or be restored to, committed unit relation state.

Model creation honors agent-stream defaults without leaking agent metadata into
model configuration or skipping default storage-pool and provider-resource
creation. Legacy `authorized-keys` model configuration sent by `3.x` clients is
ignored rather than rejected; SSH keys remain separately managed in Juju 4.

CLI errors are clearer for duplicate integrations, endpoint limits, and local
files inaccessible to the confined snap. Ordinary applications named
`controller` can be unexposed; the actual controller application remains protected.

* [fix(uniter): remove empty directories left behind by charm upgrades](https://github.com/juju/juju/pull/23204#top)
* [fix(relation): stop recursive subordinate unit deployment](https://github.com/juju/juju/pull/23185#top)
* [fix: remove deleted relations from unit relation state](https://github.com/juju/juju/pull/23276#top)
* [fix: agent-stream and agent-version keys could leak to the model config if model-defaults were provided during bootstrap](https://github.com/juju/juju/pull/23090#top)
* [fix(model): consolidate CreateModel variants to fix missing storage pool seeding](https://github.com/juju/juju/pull/23171#top)
* [chore: Merge model 3.x compatibility 4.x to 4.0](https://github.com/juju/juju/pull/23111#top)
* [fix(relation): map endpoint quota and relation-exists errors to wire codes](https://github.com/juju/juju/pull/23114#top)
* [feat(cli): provide a clearer error message when deploying or using resources outside of confinement](https://github.com/juju/juju/pull/22527#top)
* [fix: check if application is controller](https://github.com/juju/juju/pull/23308#top)

### ☁️ Providers, networking, and Kubernetes configuration

Google Cloud supports additional disk types, including opt-in Hyperdisk, and the
`image-id` constraint with image compatibility checks. Hyperdisk availability
still depends on the selected machine family. Azure image lookup recognizes
Ubuntu `26.04` as an LTS, and controller destruction cleans up Juju-owned
resources in customer-provided resource groups without deleting the group itself.

Kubernetes bootstrap supplies proxy environment variables before downloading the
controller charm. OpenStack upgrade prechecks retain proxy transport settings,
and machine provisioning receives the configured OS update and upgrade settings.

Controller API address selection honors `juju-mgmt-space`, including VETH
addresses. Without a configured management space, non-VETH addresses are
preferred, with VETH addresses retained when they are the only candidates.

Kubernetes models gain `enable-service-links`, defaulting to `true`. Setting
`juju model-config enable-service-links=false` disables application service-link
environment-variable injection in generated workload pod specifications. This
retains the existing behavior by default; it does not provide network isolation
or remove the mandatory Kubernetes API service variables.

* [feat(gce): add support for new disk storage types eg hyperdisk](https://github.com/juju/juju/pull/22993#top)
* [feat: add support for image-id constraint on google cloud](https://github.com/juju/juju/pull/23012#top)
* [fix: add 26.04 image support to Azure](https://github.com/juju/juju/pull/23063#top)
* [fix(azure): destroy-controller fails to delete resources in shared resource groups](https://github.com/juju/juju/pull/23035#top)
* [fix: propagate proxy envvars](https://github.com/juju/juju/pull/23132#top)
* [fix(openstack): preserve proxy transport settings](https://github.com/juju/juju/pull/23080#top)
* [feat: ensure we pass update and upgrade config](https://github.com/juju/juju/pull/23210#top)
* [fix: API addresses from virtual Ethernet devices](https://github.com/juju/juju/pull/23039#top)
* [feat(kubernetes): add enable-service-links model config to control K8s service env var injection](https://github.com/juju/juju/pull/23070#top)

## 📘 Documentation

Documentation adds the minimum vSphere privileges needed to operate Juju and
explains the orientation of application data in `show-unit` output during
upgrades. Cloud references also document the new Google Cloud options and
Kubernetes service-link setting described above.

* [docs(cloud): add minimal vsphere permissions reference](https://github.com/juju/juju/pull/23112#top)
* [docs(upgrade): document show-unit application-data orientation](https://github.com/juju/juju/pull/22842#top)

## 📘 Summary

`4.0.15` strengthens storage lifecycle handling, controller failover and
diagnostics, secret access, and deployment maintenance. It also improves
migration data handling, model configuration, proxy support, and cloud-provider
behavior since `4.0.14`, while adding targeted Google Cloud and Kubernetes
configuration options.
