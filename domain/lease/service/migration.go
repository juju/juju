// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/errors"
)

// MigrationState is an interface that provides methods to get the application
// leadership.
type MigrationState interface {
	// GetApplicationLeadershipForModel returns the leadership information for
	// the model applications
	GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (map[string]string, error)
}

// MigrationService provides the lease read capabilities.
type MigrationService struct {
	st MigrationState
}

// NewMigrationService creates a new MigrationService.
func NewMigrationService(st MigrationState) *MigrationService {
	return &MigrationService{
		st: st,
	}
}

// GetApplicationLeadershipForModel returns the leadership information for the
// model applications.
func (s *MigrationService) GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (_ map[string]string, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return s.st.GetApplicationLeadershipForModel(ctx, modelUUID)
}
