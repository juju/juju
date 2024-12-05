// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerimageresourcestore

// ContainerImageMetadata contains the access information for an OCI image resource.
type ContainerImageMetadata struct {
	// StorageKey is the key used to look-up the metadata in state.
	StorageKey string

	// RegistryPath holds the image name (including host) of the image in the
	// oci registry.
	RegistryPath string

	// Username holds the username used to gain access to a non-public image.
	Username string

	// Password holds the password used to gain access to a non-public image.
	Password string
}
