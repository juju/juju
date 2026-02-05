// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	corestorage "github.com/juju/juju/core/storage"
)

// StorageService defines a service for storage related behaviour.
type StorageService struct {
	st             State
	clock          clock.Clock
	registryGetter corestorage.ModelStorageRegistryGetter
}
