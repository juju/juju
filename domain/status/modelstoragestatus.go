// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see licence file for details.

package status

// ModelStorageFilesystemStatus describes a filesystem in the model for the
// purposes of the model status payload. Detachable mirrors 3.6 semantics: a
// filesystem is detachable when it is not bound to a machine, i.e. its
// ownership scope is the model. For volume-backed filesystems the backing
// volume's provision scope takes precedence.
type ModelStorageFilesystemStatus struct {
	// ID is the filesystem identifier, e.g. "data/0".
	ID string

	// ProviderID is the ID of the filesystem from the storage provider. It is
	// empty when the filesystem has not yet been provisioned.
	ProviderID string

	// Status is the current status name of the filesystem, e.g. "attached".
	Status string

	// Detachable is true when the filesystem outlives the unit or machine it
	// is attached to.
	Detachable bool
}

// ModelStorageVolumeStatus describes a volume in the model for the purposes
// of the model status payload. Detachable mirrors 3.6 semantics: a volume is
// detachable when it is not bound to a machine, i.e. it is model-scoped.
type ModelStorageVolumeStatus struct {
	// ID is the volume identifier, e.g. "ebs-0".
	ID string

	// ProviderID is the ID of the volume from the storage provider. It is
	// empty when the volume has not yet been provisioned.
	ProviderID string

	// Status is the current status name of the volume, e.g. "attached".
	Status string

	// Detachable is true when the volume outlives the unit or machine it is
	// attached to.
	Detachable bool
}

// ModelStorageStatus holds the filesystems and volumes of a model as
// required by the model status payload.
type ModelStorageStatus struct {
	Filesystems []ModelStorageFilesystemStatus
	Volumes     []ModelStorageVolumeStatus
}
