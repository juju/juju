// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// ApplicationState describes retrieval and persistence
// methods specific to application removal.
type ApplicationState interface {
	// ApplicationExists returns true if a application exists with the input
	// application UUID.
	ApplicationExists(ctx context.Context, appUUID string) (bool, error)

	// EnsureApplicationNotAliveCascade ensures that there is no application
	// identified by the input application UUID, that is still alive. If the
	// application has units, they are also guaranteed to be no longer alive,
	// cascading. The affected unit UUIDs are returned. If the units are also
	// the last ones on their machines, it will cascade and the machines are
	// also set to dying. The affected machine UUIDs are returned.
	EnsureApplicationNotAliveCascade(ctx context.Context, appUUID string) (unitUUIDs, machineUUIDs []string, err error)

	// ApplicationScheduleRemoval schedules a removal job for the application
	// with the input application UUID, qualified with the input force boolean.
	ApplicationScheduleRemoval(ctx context.Context, removalUUID, appUUID string, force bool, when time.Time) error

	// GetApplicationLife returns the life of the application with the input
	// UUID.
	GetApplicationLife(ctx context.Context, appUUID string) (life.Life, error)

	// DeleteApplication removes a application from the database completely.
	DeleteApplication(ctx context.Context, appUUID string) error
}

// RemoveApplication checks if a application with the input application UUID
// exists. If it does, the application is guaranteed after this call to be:
//   - No longer alive.
//   - Removed or scheduled to be removed with the input force qualification.
//   - If the application has units, the units are also guaranteed to be no
//     longer alive and scheduled for removal.
//
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// application. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveApplication(
	ctx context.Context,
	appUUID coreapplication.ID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.st.ApplicationExists(ctx, appUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if application %q exists: %w", appUUID, err)
	} else if !exists {
		return "", errors.Errorf("application %q does not exist", appUUID).Add(applicationerrors.ApplicationNotFound)
	}

	// Ensure the application is not alive.
	unitUUIDs, machineUUIDs, err := s.st.EnsureApplicationNotAliveCascade(ctx, appUUID.String())
	if err != nil {
		return "", errors.Errorf("application %q: %w", appUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the application if the normal destruction
			// workflows complete within the wait duration.
			if _, err := s.applicationScheduleRemoval(ctx, appUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of application %q", appUUID.String())
			wait = 0
		}
	}

	appJobUUID, err := s.applicationScheduleRemoval(ctx, appUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Ensure that the application units and machines are removed as well.
	if len(unitUUIDs) > 0 {
		// If there are any units that transitioned from alive to dying or dead,
		// we need to schedule their removal as well.
		s.logger.Infof(ctx, "application has units %v, scheduling removal", unitUUIDs)

		s.removeUnits(ctx, unitUUIDs, force, wait)
	}

	if len(machineUUIDs) > 0 {
		// If there are any machines that transitioned from alive to dying or
		// dead, we need to schedule their removal as well.
		s.logger.Infof(ctx, "application has machines %v, scheduling removal", machineUUIDs)

		s.removeMachines(ctx, machineUUIDs, force, wait)
	}

	return appJobUUID, nil
}

func (s *Service) applicationScheduleRemoval(
	ctx context.Context, appUUID coreapplication.ID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.st.ApplicationScheduleRemoval(
		ctx, jobUUID.String(), appUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("application %q: %w", appUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for application %q", jobUUID, appUUID)
	return jobUUID, nil
}

func (s *Service) removeUnits(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, unitUUID := range uuids {
		if _, err := s.RemoveUnit(ctx, unit.UUID(unitUUID), force, wait); errors.Is(err, applicationerrors.UnitNotFound) {
			// There could be a chance that the unit has already been removed
			// by another process. We can safely ignore this error and
			// continue with the next unit.
			continue
		} else if err != nil {
			// If the unit fails to be scheduled for removal, we log out the
			// error. The application and the units are already transitioned to
			// dying and there is no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of unit %q: %v", unitUUID, err)
		}
	}
}

func (s *Service) removeMachines(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, machineUUID := range uuids {
		if _, err := s.RemoveMachine(ctx, machine.UUID(machineUUID), force, wait); errors.Is(err, machineerrors.MachineNotFound) {
			// There could be a chance that the machine has already been removed
			// by another process. We can safely ignore this error and
			// continue with the next machine.
			continue
		} else if err != nil {
			// If the machine fails to be scheduled for removal, we log out the
			// error. The application and the units are already transitioned to
			// dying and there is no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of machine %q: %v", machineUUID, err)
		}
	}
}

// processApplicationRemovalJob deletes an application if it is dying.
// Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processApplicationRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.ApplicationJob {
		return errors.Errorf("job type: %q not valid for application removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.st.GetApplicationLife(ctx, job.EntityUUID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting application %q life: %w", job.EntityUUID, err)
	}

	if l == life.Alive {
		return errors.Errorf("application %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if err := s.st.DeleteApplication(ctx, job.EntityUUID); errors.Is(err, applicationerrors.ApplicationNotFound) {
		// The application has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting application %q: %w", job.EntityUUID, err)
	}
	return nil
}
