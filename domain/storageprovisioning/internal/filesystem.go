// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// FilesystemTemplate represents the information available within the model's
// state for informing the creation of a Kubernetes PVC template/Pod template.
type FilesystemTemplate struct {
	// StorageName is the name of the storage as defined in the charm for this
	// application.
	StorageName string

	// Count is the number of filesystem(s) to mount for this storage.
	Count int

	// MaxCount is the maxium number of filesystems for this storage.
	MaxCount int

	// SizeMiB is the number of mebibytes to allocate for this filesystem or
	// each of these filesystems.
	SizeMiB uint64

	// ProviderType is the name of the provider to be used to provision this
	// filesystem(s).
	ProviderType string

	// ReadOnly is true if this filesystem(s) or the mount should be read-only.
	ReadOnly bool

	// Location is a path to hint where the filesystem(s) should be mounted for
	// the charm to access. It is not the exact path the filesystem(s) will be
	// mounted.
	Location string

	// Attributes are a set of key value pairs that are supplied to the provider
	// or provisioner to facilitate this filesystem(s).
	Attributes map[string]string
}
