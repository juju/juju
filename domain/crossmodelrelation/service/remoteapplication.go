// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/crossmodelrelation"
)

// GetRemoteApplicationConsumers returns the current state of all remote
// application consumers in the local model.
func (s *Service) GetRemoteApplicationConsumers(context.Context) ([]crossmodelrelation.RemoteApplicationConsumer, error) {
	return nil, errors.NotImplemented
}

// GetRemoteApplicationOfferers returns all application proxies for offers
// consumed in this model.
func (s *Service) GetRemoteApplicationOfferers(context.Context) ([]crossmodelrelation.RemoteApplicationOfferer, error) {
	return nil, errors.NotImplemented
}
