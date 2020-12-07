// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constants

import (
	"github.com/juju/juju/storage"
)

const (
	// ECSProviderType is the provider type for AWS ECS.
	ECSProviderType = "ecs"

	// StorageProviderType defines the Juju storage type which can be used
	// to provision storage on caas models.
	StorageProviderType = storage.ProviderType("ecs")
)
