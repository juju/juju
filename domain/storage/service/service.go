// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/juju/internal/storage"
)

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// State defines an interface for interacting with the underlying state.
type State interface {
	StoragePoolState
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	*StoragePoolService
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger Logger, registry storage.ProviderRegistry) *Service {
	return &Service{
		StoragePoolService: &StoragePoolService{
			st:       st,
			logger:   logger,
			registry: registry,
		},
	}
}
