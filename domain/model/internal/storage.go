// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	domainstorage "github.com/juju/juju/domain/storage"
)

// CreateModelDefaultStoragePoolArg represents the arguments required for establishing
// a new model storage pool.
//
// This argument is used to see the model with the builtin and default storage
// pools that exist.
type CreateModelDefaultStoragePoolArg struct {
	Attributes map[string]string
	Name       string
	Origin     domainstorage.StoragePoolOrigin
	Type       string
	UUID       domainstorage.StoragePoolUUID
}
