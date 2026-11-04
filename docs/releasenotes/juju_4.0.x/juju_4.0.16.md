(juju4016)=
# Juju 4.0.16
🗓️ 22 Oct 2026

This is a bug fix release for Juju 4.0, covering changes from `4.0.15` to
`4.0.16`. It also adds storage provisioning for machine-model subordinate units
and configurable OpenStack root-volume pools.

## 🎯 Highlights

* **Migration from Juju 3.6 preserves more model and relation data**: fixes cover constraints, annotations, historical logs, consumed-offer aliases, relation settings, networks, and secret grants.
* **Secret and offer access checks are tighter**: unit-resource downloads validate the requesting agent, peer relations support secret grants, imported offer permissions are preserved, and revoking consume access suspends affected cross-model relations.
* **Storage provisioning and cleanup are more reliable**: fixes cover repeated storage events, persistent-storage teardown, container-host removal, and Kubernetes workload cleanup. New machine-model subordinate units can provision their requested storage.
* **Deployment and controller operations improve**: fixes cover ARM64 Kubernetes bootstrap, application constraints, local LXD networking, and controller addresses. OpenStack gains configurable Cinder pools for root volumes.

Full list of changes:
https://github.com/juju/juju/compare/v4.0.15...v4.0.16

## 🛠️ Fixes

### 🔒 Secrets, permissions, and resource access

Unit-resource downloads now require a unit or application agent and check that
the requested unit belongs to that agent. Secret-consumer metadata queries are
restricted to consumers in the caller's own application.

Cross-controller secret consumers in the same Kubernetes cluster use the
in-cluster API address when the recorded endpoint is loopback and in-cluster
addressing is preferred.

Secret access can be granted over peer relations. Remote secret lookup selects
the connection holding the grant instead of an arbitrary connection for the
consuming application, fixing access when an application has multiple offer
connections.

Imported offers retain their source permissions without automatically adding
`everyone@external` read access. Revoking offer access below `consume` suspends
the affected user's existing cross-model relations; later unit events without
suspension information no longer resume them accidentally. This does not revoke
credentials issued by the workload itself.

* [fix(apiserver): authorise unit resource downloads](https://github.com/juju/juju/pull/23360#top)
* [fix(secret): support granting secret access over peer relations](https://github.com/juju/juju/pull/23388#top)
* [fix(crossmodelsecrets): resolve consuming application by secret grant](https://github.com/juju/juju/pull/23587#top)
* [fix: preserve imported offer ACLs](https://github.com/juju/juju/pull/23545#top)
* [fix(cmr): suspend cross-model relations when revoking offer consume permission](https://github.com/juju/juju/pull/23358#top)

### 🔁 Model migration and historical logs

Migration from Juju 3.6 imports spaces before model constraints that reference
them, avoiding `SpaceNotFound` errors. Annotations on models, machines,
applications, and units are also imported. External users referenced only by
offer permissions are created without requiring them to be model members.

Historical log transfer tracks progress for the migrating model instead of
using newly arriving target-agent logs as the resume point. Transferred logs
are written to the correct model.

Kubernetes model workers stay paused while a target import is active, avoiding
premature changes to shared Kubernetes resources before migration commits.
Remote secret clients follow a migrated offering model to its new controller
and update the saved model location.

These changes improve specific migration paths and data handling. Migration
from a `4.0` controller to another `4.0` controller remains unsupported. Fan
networking is not supported in Juju 4.0; the test-suite changes do not add
support or automatically change production models using it.

* [fix: import model constraints after spaces during migration](https://github.com/juju/juju/pull/23379#top)
* [fix(migration): import annotations from 3.6 models](https://github.com/juju/juju/pull/23383#top)
* [fix(modelmigration): create offer-ACL-only external users on import](https://github.com/juju/juju/pull/23455#top)
* [fix(migration): preserve historical logs during model transfer](https://github.com/juju/juju/pull/23489#top)
* [fix(migration): gate caas model workers on migration-inactive flag](https://github.com/juju/juju/pull/23590#top)
* [fix(CMR): follow controller redirects for remote secrets and model lookups](https://github.com/juju/juju/pull/23479#top)

### 🔗 Cross-model relation import and removal

Cross-model relation import preserves relation IDs, status, suspension state,
endpoint settings, unit settings and scope membership, and ingress and egress
networks, including explicit `--via` overrides. Consumed offers can be imported
before a relation has been created.

When consumed-offer aliases are merged, their relation tokens and unit settings
are retained. Offering-side migration imports every connection of a legacy
consumer proxy, including the additional connections' relation data, networks,
and secret grants.

Removing one relation to a shared consumed offer no longer departs synthetic
units still used by another relation. Application, consumed-offer, and model
removal use the appropriate CMR cleanup paths, and dying consumed offers reject
new relations. Stale workers cannot update a replacement remote application's
status by reusing its name. A missing remote offer triggers local cleanup and
stops its worker instead of causing repeated restarts.

* [fix(crossmodelrelation): import consumed offers without relations](https://github.com/juju/juju/pull/23372#top)
* [feat(crossmodelrelation): import relation networks](https://github.com/juju/juju/pull/23367#top)
* [fix(crossmodelrelation): preserve identity and state of imported consumer relations](https://github.com/juju/juju/pull/23368#top)
* [fix(relation): preserve tokens and unit settings across consume aliases](https://github.com/juju/juju/pull/23380#top)
* [fix(crossmodelrelation): import every offer connection of a legacy consumer proxy](https://github.com/juju/juju/pull/23406#top)
* [fix(relation): import relation data of remote application consumers](https://github.com/juju/juju/pull/23395#top)
* [fix(migration): import relation data of multi-connection consumer proxies](https://github.com/juju/juju/pull/23510#top)
* [fix(crossmodelrelation): import networks and secret grants of additional offer connections](https://github.com/juju/juju/pull/23559#top)
* [fix(cmr): atomically remove synthetic units from shared relations](https://github.com/juju/juju/pull/23601#top)
* [fix(cmr): address CMR forced-removal integrity defects](https://github.com/juju/juju/pull/23596#top)

### 🗃️ Storage provisioning and removal

Block-storage attachment locations fall back to the device name when no stable
`/dev/disk/by-id` link is available. This allows devices such as loop-backed
volumes to be reported as provisioned.

New machine-model subordinate units provision storage from their application's
storage directives on the principal unit's machine. This does not retroactively
provision storage for existing subordinate units. Reattaching a provisioned
volume passes its availability-zone information into machine creation.

The storage provisioner deduplicates repeated watcher results and updates
matching pending create and attach operations without scheduling duplicate work
or resetting their retry timing.

Model and controller destruction distinguish model-scoped storage from
machine-scoped storage and report remaining volumes and filesystems. Controller
destruction stops retrying indefinitely when persistent-storage reports
disagree. Model-scoped persistent storage still requires an explicit destroy
or release choice.

Container hosts receive removal jobs during model teardown, allowing cleanup to
finish after their containers are removed. Kubernetes application workers also
complete provider cleanup when forced removal deletes application records
mid-operation, avoiding leftover workloads that can block PVC deletion.

* [fix(storage): fall back to device name for block storage location](https://github.com/juju/juju/pull/23303#top)
* [feat(relation): provision storage for IAAS subordinate units](https://github.com/juju/juju/pull/23446#top)
* [fix(storage): restore 3.6 semantics for destroy with persistent storage and volume-attach AZ placement](https://github.com/juju/juju/pull/23319#top)
* [fix: deduplicate storage provisioner operations](https://github.com/juju/juju/pull/23616#top)
* [fix(removal): unblock model teardown on container hosts](https://github.com/juju/juju/pull/23473#top)
* [fix(worker): finish CAAS k8s cleanup when app is removed mid-flight](https://github.com/juju/juju/pull/23390#top)

### ☁️ Bootstrap, constraints, providers, and charm resources

Kubernetes controller bootstrap uses the selected controller charm architecture,
including ARM64, and reports conflicting architecture constraints before
creating the application. Model creation opens the provider before saving the
model, returning provider-open errors directly.

Application deployment stores explicit application constraints plus the
resolved architecture rather than copying model defaults into the application
record. Kubernetes scaling refreshes the application specification before
adding units so they use updated constraints. This does not resize existing
pods or add support for unsupported Kubernetes constraints.

Local LXD container networking can use the default bridge when its subnet is
not registered in a Juju space, without bridging the host's primary interface.
The local-mode fallback does not enforce host-space constraints; the provider
networking method is required for those checks. Synthetic Kubernetes network
devices use an empty interface name instead of descriptive placeholder text.

OpenStack supports a Cinder storage-pool name in `root-disk-source`, with
configurable volume type, tag, and disk bus for root volumes.

Resource-kind lookup matches both the resource name and charm, avoiding file
resources being interpreted as container-image metadata when a charm declares
multiple resources.

* [fix(bootstrap): support arm64 k8s controller bootstrap](https://github.com/juju/juju/pull/23307#top)
* [fix: provider should be validated before adding a model](https://github.com/juju/juju/pull/23524#top)
* [fix: restore 3.6 application constraints behaviour (#23462)](https://github.com/juju/juju/pull/23578#top)
* [fix(network): use the default LXD bridge for local container networking](https://github.com/juju/juju/pull/23472#top)
* [fix(network-get): remove placeholder string from k8s dummy NIC names](https://github.com/juju/juju/pull/23292#top)
* [fix(resource): wrong kind for charms with several resources](https://github.com/juju/juju/pull/23558#top)

### 🧱 Controller connectivity and API behavior

Unrelated controller configuration updates no longer clear the stored API
port. Cross-model connection details include the controller's configured public
DNS address before its raw API addresses.

Controller API and SSH-server ports are managed on controller instances rather
than the model firewall, allowing controller expose CIDRs to restrict access.
This is not a default-deny change: unexposed controllers, and expose rules that
resolve to no CIDRs, retain access from all networks.

Missing model-user records return a typed not-found error instead of a
malformed nil-error message.

* [fix(controllerconfig): only update api-port when it is set or removed](https://github.com/juju/juju/pull/23314#top)
* [fix: add dns-name if configured to consume details](https://github.com/juju/juju/pull/23323#top)
* [fix(firewaller): keep controller ports off model firewall](https://github.com/juju/juju/pull/23362#top)
* [fix(model): don't format nil err when model user not found](https://github.com/juju/juju/pull/23322#top)

## 📘 Summary

`4.0.16` improves migration data handling, cross-model relations, secret and
offer access, and storage provisioning and cleanup since `4.0.15`. It also fixes
bootstrap, constraints, networking, controller connectivity, and resource
handling, and adds storage support for new machine-model subordinate units and
configurable OpenStack root-volume pools.
