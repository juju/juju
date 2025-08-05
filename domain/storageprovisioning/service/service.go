// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/errors"
)

// Service provides the interface for provisioning storage within a model.
type Service struct {
	watcherFactory WatcherFactory

	st State
}

// State is the accumulation of all the requirements [Service] has for
// persisting and watching changes to storage instances in the model.
type State interface {
	FilesystemState
	VolumeState

	// CheckMachineIsDead checks to see if a machine is dead, returning true
	// when the life of the machine is dead.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided UUID.
	CheckMachineIsDead(context.Context, coremachine.UUID) (bool, error)

	// GetMachineNetNodeUUID retrieves the net node UUID associated with provided
	// machine.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided UUID.
	GetMachineNetNodeUUID(context.Context, coremachine.UUID) (domainnetwork.NetNodeUUID, error)

	// GetUnitNetNodeUUID returns the node UUID associated with the supplied
	// unit.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when no
	// unit exists for the supplied unit UUID.
	GetUnitNetNodeUUID(context.Context, coreunit.UUID) (domainnetwork.NetNodeUUID, error)

	// NamespaceForWatchMachineCloudInstance returns the change stream namespace
	// for watching machine cloud instance changes.
	NamespaceForWatchMachineCloudInstance() string

	// GetStorageResourceTagInfoForApplication returns information required to
	// build resource tags for storage created for the given application.
	GetStorageResourceTagInfoForApplication(context.Context, application.ID, string) (storageprovisioning.ResourceTagInfo, error)
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNamespaceWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. Change-log events will be emitted only if the filter
	// accepts them, and dispatching the notifications via the Changes channel. A
	// filter option is required, though additional filter options can be provided.
	NewNamespaceWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// NewService creates a new [Service] instance with the provided state for
// provisioning storage instances in the model.
func NewService(st State, wf WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: wf,
	}
}

// WatchMachineCloudInstance returns a watcher that fires when a machine's cloud
// instance info is changed.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// UUID.
// - [machineerrors.MachineIsDead] when the machine is dead meaning it is about
// to go away.
func (s *Service) WatchMachineCloudInstance(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	dead, err := s.st.CheckMachineIsDead(ctx, machineUUID)
	if err != nil {
		return nil, errors.Errorf("checking if machine is dead: %w", err)
	}

	if dead {
		return nil, errors.Errorf("machine %q is dead", machineUUID).Add(
			machineerrors.MachineIsDead,
		)
	}

	ns := s.st.NamespaceForWatchMachineCloudInstance()
	filter := eventsource.PredicateFilter(ns, changestream.All,
		eventsource.EqualsPredicate(machineUUID.String()))
	return s.watcherFactory.NewNotifyWatcher(ctx, filter)
}

// GetStorageResourceTagsForApplication returns the storage resource tags for
// the given application. These tags are used when creating a resource in an
// environ.
func (s *Service) GetStorageResourceTagsForApplication(
	ctx context.Context, appUUID application.ID,
) (map[string]string, error) {
	if err := appUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	info, err := s.st.GetStorageResourceTagInfoForApplication(
		ctx, appUUID, config.ResourceTagsKey)
	if err != nil {
		return nil, errors.Errorf(
			"getting filesystem templates for app %q: %w", appUUID, err,
		)
	}

	resourceTags := map[string]string{}
	// Resource tags as defined in model config are space separated key-value
	// pairs, where the key and value are separated by an equals sign.
	for pair := range strings.SplitSeq(info.BaseResourceTags, " ") {
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			return nil, errors.Errorf("malformed resource tag %q", pair)
		}
		if strings.HasPrefix(key, tags.JujuTagPrefix) {
			continue
		}
		resourceTags[key] = value
	}
	resourceTags[tags.JujuController] = info.ControllerUUID
	resourceTags[tags.JujuModel] = info.ModelUUID
	resourceTags[tags.JujuStorageOwner] = info.ApplicationName

	return resourceTags, nil
}
