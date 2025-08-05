// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/internal/errors"
)

// Provider describes methods for interacting with the provider.
type Provider interface {
	// ReleaseContainerAddresses releases the previously allocated
	// addresses matching the interface details passed in.
	ReleaseContainerAddresses(ctx context.Context, interfaces []string) error

	// Destroy shuts down all known machines and destroys the rest of the
	// known environment.
	Destroy(ctx context.Context) error
}

// ModelDBState describes retrieval and persistence methods for entity removal
// in the model database.
type ModelDBState interface {
	RelationState
	UnitState
	ApplicationState
	MachineState
	ModelState

	// GetAllJobs returns all removal jobs.
	GetAllJobs(ctx context.Context) ([]removal.Job, error)

	// DeleteJob deletes a removal record under the assumption
	// that it was executed successfully.
	DeleteJob(ctx context.Context, jUUID string) error
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for changes to the
	// input table name that match the input mask.
	NewUUIDsWatcher(
		ctx context.Context,
		tableName, summary string,
		changeMask changestream.ChangeType,
	) (watcher.StringsWatcher, error)

	// NewNamespaceMapperWatcher returns a new watcher that receives changes from
	// the input base watcher's db/queue.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// Service provides the API for working with entity removal.
type Service struct {
	controllerState ControllerDBState
	modelState      ModelDBState

	leadershipRevoker leadership.Revoker
	provider          providertracker.ProviderGetter[Provider]

	modelUUID model.UUID

	clock  clock.Clock
	logger logger.Logger
}

// GetAllJobs returns all removal jobs.
func (s *Service) GetAllJobs(ctx context.Context) ([]removal.Job, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	jobs, err := s.modelState.GetAllJobs(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return jobs, nil
}

// ExecuteJob runs the appropriate removal logic for the input job.
// If the job is determined to have run successfully, we ensure that
// no removal job with the same UUID exists in the database.
func (s *Service) ExecuteJob(ctx context.Context, job removal.Job) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	var err error
	switch job.RemovalType {
	case removal.RelationJob:
		err = s.processRelationRemovalJob(ctx, job)

	case removal.UnitJob:
		err = s.processUnitRemovalJob(ctx, job)

	case removal.ApplicationJob:
		err = s.processApplicationRemovalJob(ctx, job)

	case removal.MachineJob:
		err = s.processMachineRemovalJob(ctx, job)

	case removal.ModelJob:
		err = s.processModelJob(ctx, job)

	default:
		err = errors.Errorf("removal job type %q not supported", job.RemovalType).Add(
			removalerrors.RemovalJobTypeNotSupported)
	}

	if errors.Is(err, removalerrors.RemovalJobIncomplete) {
		s.logger.Debugf(ctx, "removal job for %s %q incomplete: %v", job.RemovalType, job.EntityUUID, err)
		return nil
	}
	if err != nil && !errors.Is(err, removalerrors.RemovalModelRemoved) {
		return errors.Capture(err)
	}

	if err := s.modelState.DeleteJob(ctx, job.UUID.String()); err != nil {
		return errors.Errorf("completing removal %q: %w", job.UUID.String(), err)
	}

	// The model was removed successfully, it's now up to listeners to ensure
	// that everything else is cleaned up. That's outside of the scope of the
	// removal service (delete DB for example).
	if errors.Is(err, removalerrors.RemovalModelRemoved) {
		s.logger.Infof(ctx, "removal job for %s %q completed successfully", job.RemovalType, job.EntityUUID)
		return err
	}

	return nil
}

func (s *Service) removeUnits(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, unitUUID := range uuids {
		if _, err := s.RemoveUnit(ctx, unit.UUID(unitUUID), force, wait); errors.Is(err, applicationerrors.UnitNotFound) {
			// There could be a chance that the unit has already been removed by
			// another process. We can safely ignore this error and continue
			// with the next unit.
			continue
		} else if err != nil {
			// If the unit fails to be scheduled for removal, we log out the
			// error. The units are already transitioned to dying and there is
			// no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of unit %q: %v", unitUUID, err)
		}
	}
}

func (s *Service) removeMachines(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, machineUUID := range uuids {
		if _, err := s.RemoveMachine(ctx, machine.UUID(machineUUID), force, wait); errors.Is(err, machineerrors.MachineNotFound) {
			// There could be a chance that the machine has already been removed
			// by another process. We can safely ignore this error and continue
			// with the next machine.
			continue
		} else if err != nil {
			// If the machine fails to be scheduled for removal, we log out the
			// error. The machines are already transitioned to dying and there
			// is no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of machine %q: %v", machineUUID, err)
		}
	}
}

func (s *Service) removeRelations(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, relationUUID := range uuids {
		if _, err := s.RemoveRelation(ctx, relation.UUID(relationUUID), force, wait); errors.Is(err, relationerrors.RelationNotFound) {
			// There could be a chance that the relation has already been
			// removed by another process. We can safely ignore this error and
			// continue with the next relation.
			continue
		} else if err != nil {
			// If the unit fails to be scheduled for removal, we log out the
			// error. The relations are already transitioned to dying and there
			// is no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of relation %q: %v", relationUUID, err)
		}
	}
}

func (s *Service) removeApplications(ctx context.Context, uuids []string, force bool, wait time.Duration) {
	for _, applicationUUID := range uuids {
		if _, err := s.RemoveApplication(ctx, application.ID(applicationUUID), force, wait); errors.Is(err, applicationerrors.ApplicationNotFound) {
			// There could be a chance that the application has already been
			// removed by another process. We can safely ignore this error and
			// continue with the next application.
			continue
		} else if err != nil {
			// If the unit fails to be scheduled for removal, we log out the
			// error. The applications are already transitioned to dying and
			// there is no way to transition them back to alive.
			s.logger.Errorf(ctx, "scheduling removal of application %q: %v", applicationUUID, err)
		}
	}
}

// WatchableService provides the API for working with entity removal,
// including the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService creates a new WatchableService
// for working with entity removal.
func NewWatchableService(
	controllerState ControllerDBState,
	modelState ModelDBState,
	watcherFactory WatcherFactory,
	leadershipRevoker leadership.Revoker,
	provider providertracker.ProviderGetter[Provider],
	modelUUID model.UUID,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			controllerState:   controllerState,
			modelState:        modelState,
			leadershipRevoker: leadershipRevoker,
			provider:          provider,
			modelUUID:         modelUUID,
			clock:             clock,
			logger:            logger,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchRemovals watches for scheduled removal jobs.
// The returned watcher emits the UUIDs of any inserted or updated jobs.
func (s *WatchableService) WatchRemovals(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	w, err := s.watcherFactory.NewUUIDsWatcher(
		ctx,
		"removals watcher",
		s.modelState.NamespaceForWatchRemovals(),
		changestream.Changed,
	)
	if err != nil {
		return nil, errors.Errorf("creating watcher for removals: %w", err)
	}
	return w, nil
}

// WatchEntityRemovals watches for scheduled removal jobs for specific entities.
func (s *WatchableService) WatchEntityRemovals(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	initialQuery, filterNames := s.modelState.NamespaceForWatchEntityRemovals()

	if len(filterNames) == 0 {
		return nil, errors.Errorf("no filter names provided for entity removals watcher")
	}

	var filters []eventsource.FilterOption
	for name := range filterNames {
		filters = append(filters, eventsource.NamespaceFilter(name, changestream.All))
	}

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		"entity removals watcher",
		func(ctx context.Context, ce []changestream.ChangeEvent) ([]string, error) {
			var results []string
			for _, c := range ce {
				name, ok := filterNames[c.Namespace()]
				if !ok {
					return nil, errors.Errorf("unknown namespace %q in entity removals watcher", c.Namespace())
				}

				entityLife, err := s.getEntityLife(ctx, name, c.Changed())
				if errors.IsOneOf(err,
					relationerrors.RelationNotFound,
					applicationerrors.UnitNotFound,
					applicationerrors.ApplicationNotFound,
					machineerrors.MachineNotFound,
					modelerrors.NotFound,
				) {
					continue
				} else if err != nil {
					return nil, errors.Errorf("getting life for %s %q: %w", name, c.Changed(), err)
				}
				if entityLife == life.Alive {
					// If the entity is still alive, we don't emit it.
					continue
				}

				results = append(results, name+":"+c.Changed())
			}
			return results, nil
		},
		filters[0],
		filters[1:]...,
	)
	if err != nil {
		return nil, errors.Errorf("creating watcher for entity removals: %w", err)
	}
	return w, nil
}

func (s *WatchableService) getEntityLife(ctx context.Context, entityType, entityUUID string) (life.Life, error) {
	switch entityType {
	case "relation":
		return s.modelState.GetRelationLife(ctx, entityUUID)
	case "unit":
		return s.modelState.GetUnitLife(ctx, entityUUID)
	case "machine":
		return s.modelState.GetMachineLife(ctx, entityUUID)
	case "model":
		return s.modelState.GetModelLife(ctx, entityUUID)
	case "application":
		return s.modelState.GetApplicationLife(ctx, entityUUID)
	default:
		return -1, errors.Errorf("unknown entity type %q", entityType)
	}
}
