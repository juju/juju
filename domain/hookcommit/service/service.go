// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v5"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/hookcommit"
	"github.com/juju/juju/internal/errors"
)

type State interface {
	GetAppUnitUUID(ctx context.Context, unitName string) (coreapplication.ID, coreunit.UUID, error)
	CommitHookChanges(ctx context.Context, changes hookcommit.CommitHookChangesArgs) error
}

type DeleteSecretState interface {
	DeleteSecret(ctx domain.AtomicContext, uri *secrets.URI, revs []int) error
}

type Service struct {
	domain.LeaseService

	st            State
	clock         clock.Clock
	logger        logger.Logger
	secretDeleter DeleteSecretState
}

func NewService(st State, deleteSecretState DeleteSecretState, logger logger.Logger) *Service {
	return &Service{
		st:            st,
		clock:         clock.WallClock,
		logger:        logger,
		secretDeleter: deleteSecretState,
	}
}

func (s *Service) CommitHookChanges(ctx context.Context, unitName string, changes hookcommit.CommitHookChangesParams) error {
	for _, p := range changes.SecretCreates {
		if err := s.validateSecretParams(p.UpdateCharmSecretParams); err != nil {
			return err
		}
	}
	for _, p := range changes.SecretUpdates {
		if err := s.validateSecretParams(p); err != nil {
			return err
		}
	}

	appUUID, unitUUID, err := s.st.GetAppUnitUUID(ctx, unitName)
	if err != nil {
		return err
	}

	// Leadership is needed if any application owned secrets are being manipulated.
	requireLeadership := false

	changesArgs := hookcommit.CommitHookChangesArgs{
		UnitUUID:        unitUUID,
		ApplicationUUID: appUUID,

		// Fill in values...
	}
	if !requireLeadership {
		return s.st.CommitHookChanges(ctx, changesArgs)
	}

	appName, _ := names.UnitApplication(unitName)
	return s.WithLease(ctx, appName, unitName, func(ctx context.Context) error {
		return s.st.CommitHookChanges(ctx, changesArgs)
	})
}

func (s *Service) validateSecretParams(params hookcommit.UpdateCharmSecretParams) error {
	if len(params.Data) > 0 && params.ValueRef != nil {
		return errors.New("must specify either content or a value reference but not both")
	}
	return nil
}
