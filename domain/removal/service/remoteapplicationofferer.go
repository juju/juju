// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	"github.com/juju/juju/core/trace"
	crossmodelrelationerrors "github.com/juju/juju/domain/crossmodelrelation/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

// RemoteApplicationOffererState describes retrieval and persistence methods for
// remote application offerers in the model database.
type RemoteApplicationOffererState interface {
	// GetRemoteApplicationOfferer returns true if a remote application exists
	// with the input UUID.
	RemoteApplicationOffererExists(ctx context.Context, rUUID string) (bool, error)

	// EnsureRemoteApplicationOffererNotAliveCascade ensures that there is no
	// remote application offerer identified by the input UUID, that is still
	// alive.
	// NOTE: We do not cascade down to (synthetic) applications or units, since
	// they are synthetic so can be removed directly without worrying about
	// their life
	EnsureRemoteApplicationOffererNotAliveCascade(
		ctx context.Context, rUUID string,
	) (internal.CascadedRemoteApplicationOffererLives, error)

	// RemoteApplicationOffererScheduleRemoval schedules a removal job for the
	// remote application offerer with the input UUID, qualified with the input
	// force boolean.
	RemoteApplicationOffererScheduleRemoval(
		ctx context.Context, removalUUID, rUUID string, force bool, when time.Time,
	) error

	// GetRemoteApplicationOffererLife returns the life of the remote application
	// offerer with the input UUID.
	GetRemoteApplicationOffererLife(ctx context.Context, rUUID string) (life.Life, error)

	// DeleteRemoteApplicationOfferer removes a remote application offerer from
	// the database completely. This also removes the synthetic application,
	// unit and charm associated with the remote application offerer.
	DeleteRemoteApplicationOfferer(ctx context.Context, rUUID string) error

	// GetRemoteApplicationOffererUUIDByApplicationUUID returns the remote
	// application offerer UUID associated with the input application UUID.
	GetRemoteApplicationOffererUUIDByApplicationUUID(
		ctx context.Context, appUUID string,
	) (string, error)
}

// RemoveRemoteApplicationOfferer checks if a remote application with the input
// UUID exists. If it does, the remote application is guaranteed after this
// call to be:
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// remote application. This duration is ignored if the force argument is false.
// The UUID for the scheduled removal job is returned.
func (s *Service) RemoveRemoteApplicationOfferer(
	ctx context.Context,
	remoteAppOffererUUID coreremoteapplication.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	exists, err := s.modelState.RemoteApplicationOffererExists(ctx, remoteAppOffererUUID.String())
	if err != nil {
		return "", errors.Errorf("checking if remote application offerer %q exists: %w", remoteAppOffererUUID, err)
	}
	if !exists {
		return "", errors.Errorf("remote application offerer %q does not exist", remoteAppOffererUUID).
			Add(crossmodelrelationerrors.RemoteApplicationNotFound)
	}

	cascaded, err := s.modelState.EnsureRemoteApplicationOffererNotAliveCascade(ctx, remoteAppOffererUUID.String())
	if err != nil {
		return "", errors.Errorf("remote application offerer %q: %w", remoteAppOffererUUID, err)
	}

	if force {
		if wait > 0 {
			// If we have been supplied with the force flag *and* a wait time,
			// schedule a normal removal job immediately. This will cause the
			// earliest removal of the remote application offerer if the normal
			// destruction workflows complete within the wait duration.
			if _, err := s.remoteApplicationOffererScheduleRemoval(ctx, remoteAppOffererUUID, false, 0); err != nil {
				return "", errors.Capture(err)
			}
		}
	} else {
		if wait > 0 {
			s.logger.Infof(ctx, "ignoring wait duration for non-forced removal of remote application offerer %q", remoteAppOffererUUID)
			wait = 0
		}
	}

	appJobUUID, err := s.remoteApplicationOffererScheduleRemoval(ctx, remoteAppOffererUUID, force, wait)
	if err != nil {
		return "", errors.Capture(err)
	}

	if cascaded.IsEmpty() {
		return appJobUUID, nil
	}

	for _, r := range cascaded.RelationUUIDs {
		if _, err := s.relationScheduleRemoval(ctx, relation.UUID(r), force, wait); err != nil {
			return "", errors.Capture(err)
		}
	}

	return appJobUUID, nil
}

// RemoveRemoteApplicationOffererByApplicationUUID checks if a remote
// application offerer associated with the input application UUID exists. If it
// does, the remote application offerer is guaranteed after this call to be:
//
// - No longer alive.
// - Removed or scheduled to be removed with the input force qualification.
//
// The input wait duration is the time that we will give for the normal
// life-cycle advancement and removal to finish before forcefully removing the
// remote application offerer. This duration is ignored if the force argument is
// false. The UUID for the scheduled removal job is returned.
func (s *Service) RemoveRemoteApplicationOffererByApplicationUUID(
	ctx context.Context,
	applicationUUID coreapplication.UUID,
	force bool,
	wait time.Duration,
) (removal.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	remoteAppOffererUUID, err := s.modelState.GetRemoteApplicationOffererUUIDByApplicationUUID(
		ctx, applicationUUID.String(),
	)
	if err != nil {
		return "", errors.Errorf("getting remote application offerer for application %q: %w", applicationUUID, err)
	}

	return s.RemoveRemoteApplicationOfferer(ctx, coreremoteapplication.UUID(remoteAppOffererUUID), force, wait)
}

func (s *Service) remoteApplicationOffererScheduleRemoval(
	ctx context.Context, remoteAppOffererUUID coreremoteapplication.UUID, force bool, wait time.Duration,
) (removal.UUID, error) {
	jobUUID, err := removal.NewUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	if err := s.modelState.RemoteApplicationOffererScheduleRemoval(
		ctx, jobUUID.String(), remoteAppOffererUUID.String(), force, s.clock.Now().UTC().Add(wait),
	); err != nil {
		return "", errors.Errorf("remote application offerer %q: %w", remoteAppOffererUUID, err)
	}

	s.logger.Infof(ctx, "scheduled removal job %q for remote application offerer %q", jobUUID, remoteAppOffererUUID)
	return jobUUID, nil
}

// processRemoteApplicationOffererRemovalJob deletes a remote application offerer
// if it is dying. Note that we do not need transactionality here:
//   - Life can only advance - it cannot become alive if dying or dead.
func (s *Service) processRemoteApplicationOffererRemovalJob(ctx context.Context, job removal.Job) error {
	if job.RemovalType != removal.RemoteApplicationOffererJob {
		return errors.Errorf("job type: %q not valid for remote application offerer removal", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotValid)
	}

	l, err := s.modelState.GetRemoteApplicationOffererLife(ctx, job.EntityUUID)
	if errors.Is(err, crossmodelrelationerrors.RemoteApplicationNotFound) {
		// The remote application offerer has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("getting remote application offerer %q life: %w", job.EntityUUID, err)
	} else if l == life.Alive {
		return errors.Errorf("remote application offerer %q is alive", job.EntityUUID).Add(removalerrors.EntityStillAlive)
	}

	if err := s.modelState.DeleteRemoteApplicationOfferer(ctx, job.EntityUUID); errors.Is(err, crossmodelrelationerrors.RemoteApplicationNotFound) {
		// The remote application offerer has already been removed.
		// Indicate success so that this job will be deleted.
		return nil
	} else if err != nil {
		return errors.Errorf("deleting remote application offerer %q: %w", job.EntityUUID, err)
	}

	return nil
}
