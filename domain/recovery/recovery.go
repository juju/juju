// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package recovery

import (
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/errors"
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

	// Applications lists the archived CAAS workload applications of the
	// model. It is filled only for caas models and excludes the
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

// CheckAgentVersion enforces the exact-version recovery gate: an archive
// is only ever recovered onto the same agent version it was taken from.
// The official build number (the ".1" in "4.1-beta3.1") distinguishes
// released packaging of the same version, not the dump format or the
// schema; the gate ignores it, mirroring the CAAS agent-version
// comparison.
func (i *ArchiveInfo) CheckAgentVersion(current semversion.Number) error {
	archived := i.AgentVersion
	archived.Build = 0
	current.Build = 0
	if archived.Compare(current) != 0 {
		return errors.Errorf(
			"archive was created by agent version %s but this binary is %s; "+
				"recovery requires the exact same version", i.AgentVersion, current)
	}
	return nil
}

// ModelFamily returns the single model type ("iaas" or "caas") shared by
// every model in the archive. A source controller with mixed model types
// is unsupported and rejected.
func (i *ArchiveInfo) ModelFamily() (string, error) {
	if len(i.Models) == 0 {
		return "", errors.New("archive records no models")
	}
	family := i.Models[0].ModelType
	for _, m := range i.Models[1:] {
		if m.ModelType != family {
			return "", errors.Errorf(
				"source controller has mixed model types: model %q is %s, expected %s",
				m.Name, m.ModelType, family)
		}
	}
	return family, nil
}

// CheckProviderFamily enforces the same-provider-family rule: the
// replacement controller must be bootstrapped on a cloud of the same
// provider type as the source controller model's cloud.
func (i *ArchiveInfo) CheckProviderFamily(targetCloudType string) error {
	if i.CloudType != targetCloudType {
		return errors.Errorf(
			"archive comes from a %q controller but the bootstrap cloud is %q; "+
				"recovery requires the same provider family", i.CloudType, targetCloudType)
	}
	return nil
}
