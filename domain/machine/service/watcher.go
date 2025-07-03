// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// WatchableService is a service that can be watched for updates to machine
// values.
type WatchableService struct {
	ProviderService
	watcherFactory WatcherFactory
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNamespaceWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	providerGetter providertracker.ProviderGetter[Provider],
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{
				st:            st,
				statusHistory: statusHistory,
				clock:         clock,
				logger:        logger,
			},
			providerGetter: providerGetter,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchMachineLife returns a watcher that observes the changes to life of one
// machine.
func (s *WatchableService) WatchMachineLife(ctx context.Context, machineName machine.Name) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table := s.st.NamespaceForMachineLife()
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			table,
			changestream.All,
			eventsource.EqualsPredicate(machineName.String()),
		),
	)
}

// WatchMachineContainerLife returns a watcher that observes machine container
// life changes.
func (s *WatchableService) WatchMachineContainerLife(ctx context.Context, parentMachineName machine.Name) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := parentMachineName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	// We only support watching containers using the parent machine name.
	// Watching a container using a container name i.e. "0/lxd/0" is not
	// supported. We don't support grand parenting.
	if parentMachineName.IsContainer() {
		return nil, errors.Errorf("watching a container using %q is not supported", parentMachineName.String())
	}

	// We only support LXD containers for now, so we need to use this to be
	// able to filter the changes.
	containerType, err := s.getContainerTypeOrUseDefault(ctx, parentMachineName, instance.LXD)
	if err != nil {
		return nil, errors.Capture(err)
	}

	prefix := parentMachineName.Parent().String() + "/" + string(containerType) + "/"

	table, stmt, arg := s.st.InitialMachineContainerLifeStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges(stmt, arg(prefix)),
		eventsource.PredicateFilter(
			table,
			changestream.All,
			func(changed string) bool {
				// We only want to watch containers, so we filter out
				// any changes that are not for containers.
				if !strings.Contains(changed, "/") {
					return false
				}

				// We also filter out any changes that are not for the
				// parent machine.

				return strings.HasPrefix(changed, prefix)
			},
		),
	)
}

func (s *WatchableService) getContainerTypeOrUseDefault(
	ctx context.Context, parentMachineName machine.Name, defaultType instance.ContainerType,
) (instance.ContainerType, error) {
	uuid, err := s.st.GetMachineUUID(ctx, parentMachineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return defaultType, nil
	} else if err != nil {
		return defaultType, errors.Capture(err)
	}

	containerTypes, err := s.st.GetSupportedContainersTypes(ctx, uuid)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return defaultType, nil
	} else if err != nil {
		return defaultType, errors.Capture(err)
	} else if len(containerTypes) == 0 {
		return defaultType, errors.Errorf("no container types supported for machine %q", parentMachineName.String())
	} else if len(containerTypes) > 1 {
		return defaultType, errors.Errorf("multiple container types (%v) supported for machine %q",
			strings.Join(containerTypes, ", "), parentMachineName.String())
	}

	return instance.ContainerType(containerTypes[0]), nil
}

// WatchModelMachines watches for additions or updates to non-container
// machines. It is used by workers that need to factor life value changes,
// and so does not factor machine removals, which are considered to be
// after their transition to the dead state.
// It emits machine names rather than UUIDs.
func (s *WatchableService) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table, stmt := s.st.InitialWatchModelMachinesStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		eventsource.PredicateFilter(table, changestream.All,
			func(changed string) bool {
				return !strings.Contains(changed, "/")
			},
		),
	)
}

// WatchModelMachineLifeAndStartTimes returns a string watcher that emits machine names
// for changes to machine life or agent start times.
func (s *WatchableService) WatchModelMachineLifeAndStartTimes(ctx context.Context) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table, stmt := s.st.InitialWatchModelMachineLifeAndStartTimesStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchMachineCloudInstances returns a NotifyWatcher that is subscribed to
// the changes in the machine_cloud_instance table in the model, for the given
// machine UUID.
func (s *WatchableService) WatchMachineCloudInstances(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineCloudInstance(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID.String()),
		),
	)
}

// WatchLXDProfiles returns a NotifyWatcher that is subscribed to the changes in
// the machine_cloud_instance table in the model, for the given machine UUID.
// Note: Sometime in the future, this watcher could react to logical changes
// fired from `SetAppliedLXDProfileNames()` instead of the `machine_lxd_profile`
// table, which could become noisy.
func (s *WatchableService) WatchLXDProfiles(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineLXDProfiles(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID.String()),
		),
	)
}

// WatchMachineReboot returns a NotifyWatcher that is subscribed to
// the changes in the machine_requires_reboot table in the model.
// It raises an event whenever the machine uuid or its parent is added to the reboot table.
func (s *WatchableService) WatchMachineReboot(ctx context.Context, uuid machine.UUID) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuids, err := s.machineToCareForReboot(ctx, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}
	machines := set.NewStrings(transform.Slice(uuids, func(u machine.UUID) string { return u.String() })...)
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineReboot(),
			changestream.All,
			func(s string) bool {
				return machines.Contains(s)
			},
		),
	)
}

func (s *WatchableService) machineToCareForReboot(ctx context.Context, uuid machine.UUID) ([]machine.UUID, error) {
	parentUUID, err := s.st.GetMachineParentUUID(ctx, uuid)
	if err != nil && !errors.Is(err, machineerrors.MachineHasNoParent) {
		return nil, errors.Capture(err)
	}
	if errors.Is(err, machineerrors.MachineHasNoParent) {
		return []machine.UUID{uuid}, nil
	}
	return []machine.UUID{uuid, parentUUID}, nil
}
