// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
)

// RemoteRelationState describes retrieval and persistence
// methods specific to remote relation removal.
type RemoteRelationState interface {
}

// RemoveRelation checks if a relation with the input UUID exists.
// If it does, the relation is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
func (s *Service) RemoveRemoteRelation(
	ctx context.Context, relUUID corerelation.UUID) error {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return nil
}
