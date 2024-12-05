// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

type containerImageMetadataStorageKey struct {
	StorageKey string `db:"storage_key"`
}

type containerImageMetadata struct {
	StorageKey   string `db:"storage_key"`
	RegistryPath string `db:"registry_path"`
	UserName     string `db:"username"`
	Password     string `db:"password"`
}
