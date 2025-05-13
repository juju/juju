// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence
// methods specific to unit removal.
type UnitState interface {
	// UnitExists returns true if a unit exists with the input name.
	UnitExists(ctx context.Context, unitUUID string) (bool, error)

	// EnsureUnitNotAlive ensures that there is no unit
	// identified by the input name, that is still alive.
	EnsureUnitNotAlive(ctx context.Context, unitUUID string) error

	// UnitScheduleRemoval schedules a removal job for the unit with the
	// input name, qualified with the input force boolean.
	UnitScheduleRemoval(ctx context.Context, removalUUID, unitUUID string, force bool, when time.Time) error
}

// RemoveUnit checks if a unit with the input name exists.
// If it does, the unit is guaranteed after this call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// unit. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveUnit(
	ctx context.Context,
	unitUUID unit.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	exists, err := s.st.UnitExists(ctx, unitUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if unit %q exists: %w", unitUUID, err)
	} else if !exists {
		return "", errors.Errorf("unit %q does not exist", unitUUID).Add(applicationerrors.UnitNotFound)
	}

	// Ensure the unit is not alive.
	if err := s.st.EnsureUnitNotAlive(ctx, unitUUID.String()); err != nil {
		return "", errors.Errorf("unit %q: %w", unitUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the unit if the normal destruction
			// workflows complete within the the wait duration.
			if _, err := s.unitScheduleRemoval(ctx, unitUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of unit %q", unitUUID.String())
			wait = 0
		}
	}

	jobUUID, err := s.unitScheduleRemoval(ctx, unitUUID, force, wait)
	return jobUUID, errors.Capture(err)
}

func (s *Service) unitScheduleRemoval(
	ctx context.Context, unitUUID unit.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.UnitScheduleRemoval(
		ctx, jobUUID.String(), unitUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("unit %q: %w", unitUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for unit %q", jobUUID, unitUUID)
	return jobUUID, nil
}
