// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import domainstorage "github.com/juju/juju/domain/storage"

type StoragePoolToCreate struct {
	Attributes   map[string]any
	Name         string
	ProviderType domainstorage.ProviderType
}
