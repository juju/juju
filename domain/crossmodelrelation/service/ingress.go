// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/errors"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	internalerrors "github.com/juju/juju/internal/errors"
)

// WatchEgressAddressesForRelation returns a watcher that notifies when
// addresses, from which connections will originate to the provider side of the
// relation, change.
// Each event contains the entire set of addresses which the provider side is
// required to allow for access from the other side of the relation.
func (s *Service) WatchEgressAddressesForRelation(ctx context.Context, relationUUID corerelation.UUID) (watcher.StringsWatcher, error) {
	return nil, internalerrors.Errorf("crossmodelrelation.WatchEgressAddressesForRelation").Add(errors.NotImplemented)
}

// WatchIngressAddressesForRelation returns a watcher that notifies when
// addresses, from which connections will originate for the relation, change.
// Each event contains the entire set of addresses which are required
// for ingress into this model from the other requirer side of the relation.
func (c *Service) WatchIngressAddressesForRelation(ctx context.Context, relationUUID corerelation.UUID) (watcher.StringsWatcher, error) {
	return nil, internalerrors.Errorf("crossmodelrelation.WatchIngressAddressesForRelation").Add(errors.NotImplemented)
}
