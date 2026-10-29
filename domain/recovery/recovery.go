// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"time"

	"github.com/juju/juju/core/semversion"
)

// ApplicationInfo describes one archived workload application, used for
// recovery substrate inventories on Kubernetes: the model's namespace must
// still hold the application's workload objects.
type ApplicationInfo struct {
	// UUID is the application's logical identity, preserved by recovery.
	// On Kubernetes it also derives the storage unique id annotation the
	// surviving StatefulSet must carry: a mismatching annotation would be
	// deleted and recreated at the first reconcile.
	UUID string

	// Name is the application's name; on Kubernetes it is also the name
	// of its StatefulSet in the model namespace.
	Name string

	// Units lists the archived unit ordinals of the application. Pods
	// churn by design and are recreated by the StatefulSet controller,
	// so they are reported only as scale context, never checked.
	Units []string

	// FilesystemProviderIDs lists the kubernetes persistent volume claim
	// names the archived storage filesystems record, one per attached
	// volume. A claim that no longer exists comes back empty at the
	// first reconcile, so it is reported as missing substrate.
	FilesystemProviderIDs []string
}

// ModelInfo describes a single model recorded in a backup archive.
type ModelInfo struct {
	// UUID is the model's logical identity, preserved by recovery.
	UUID string

	// Name is the model's name.
	Name string

	// ModelType is "iaas" or "caas".
	ModelType string

	// CloudName is the name of the model's cloud.
	CloudName string

	// CloudType is the provider family of the model's cloud, for example
	// "lxd", "ec2" or "kubernetes".
	CloudType string

	// Applications lists the archived Kubernetes workload applications
	// of the model. It is filled only for caas models and excludes the
	// controller application: the controller namespace is disposable
	// bootstrap output, not surviving substrate. The inventory powers the
	// read-only substrate check; it never gates recovery on its own.
	Applications []ApplicationInfo
}

// ArchiveInfo is the validated summary of a controller backup archive.
// It is produced before anything is provisioned and drives the recovery
// preflight checks.
type ArchiveInfo struct {
	// AgentVersion is the exact agent version of the source controller,
	// read from the archive metadata (the manifest).
	AgentVersion semversion.Number

	// ControllerUUID is the source controller's logical identity.
	ControllerUUID string

	// ControllerName is the source controller's name as recorded in the
	// controller configuration. Kubernetes recovers must bootstrap with
	// this name so the controller namespace keeps its source name. It is
	// empty when the archive does not record it.
	ControllerName string

	// ControllerModelUUID is the source controller model's identity.
	ControllerModelUUID string

	// CACert and CAPrivateKey are the source controller's CA material.
	// The replacement is bootstrapped with them, so surviving agents keep
	// trusting the controller through source trust.
	CACert       string
	CAPrivateKey string

	// HANodes is the number of controller nodes the source had. Recovery
	// always produces a single-node replacement; values above one mean
	// the remaining controller machines load as dead rows.
	HANodes int64

	// BackupFinished is when the source backup completed, telling the
	// operator how much state drift to expect.
	BackupFinished time.Time

	// CloudName is the name of the controller model's cloud.
	CloudName string

	// CloudType is the provider family of the controller model's cloud.
	// Recovery requires the replacement to use the same provider family.
	CloudType string

	// Models is the full model inventory of the source controller,
	// including the controller model.
	Models []ModelInfo

	// Checksum is the archive's SHA-256 checksum, hex encoded, computed
	// while streaming the archive.
	Checksum string

	// Size is the archive file's size in bytes.
	Size int64
}

// MachinePatch carries the replacement machine's physical facts, known to
// bootstrap because it just created them.
type MachinePatch struct {
	// MachineName is the name of the archived controller machine the
	// replacement maps onto. It is resolved from the loaded dump by the
	// recovery stage; the caller need not set it.
	MachineName string

	// InstanceID is the replacement's actual provider instance ID. It
	// always overwrites the archived value.
	InstanceID string

	// DisplayName is the replacement's provider display name. It always
	// overwrites the archived value; empty is a genuine value on providers
	// that do not set one.
	DisplayName string

	// Arch is the replacement's architecture.
	Arch string

	// MemMB, Cores and RootDiskMB are the replacement's observed
	// hardware. Zero values leave the archived observations in place.
	MemMB      uint64
	Cores      uint64
	RootDiskMB uint64
}
