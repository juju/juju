// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"sort"
	"strings"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/errors"
	coretools "github.com/juju/juju/internal/tools"
)

type State interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, machine.Name) (string, error)

	// GetModelTargetAgentVersion returns the target agent version for this model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// NamespaceForWatchAgentVersion returns the namespace identifier
	// to watch for the agent version.
	NamespaceForWatchAgentVersion() string

	// SetMachineRunningAgentBinaryVersion sets the running agent version for the machine.
	// A MachineNotFound error will be returned if the machine does not exist.
	SetMachineRunningAgentBinaryVersion(context.Context, string, coreagentbinary.Version) error

	// GetMachineTargetAgentVersion returns the target agent version for the specified machine.
	// The following error types can be expected:
	// - [modelagenterrors.AgentVersionNotFound] - when the agent version does not exist.
	GetMachineTargetAgentVersion(context.Context, string) (coreagentbinary.Version, error)

	// SetUnitRunningAgentBinaryVersion sets the running agent version for the unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	// - [applicationerrors.UnitIsDead] - when the unit is dead.
	// - [coreerrors.NotSupported] - when the architecture is not supported.
	SetUnitRunningAgentBinaryVersion(context.Context, coreunit.UUID, coreagentbinary.Version) error

	// GetUnitTargetAgentVersion returns the target agent version for the specified unit.
	// The following error types can be expected:
	// - [modelagenterrors.AgentVersionNotFound] - when the agent version does not exist.
	GetUnitTargetAgentVersion(context.Context, coreunit.UUID) (coreagentbinary.Version, error)
}

// WatcherFactory provides a factory for constructing new watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service is used to get the target Juju agent version for the current model.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// ProviderWithAgentFinder provides methods used to find agent
// binaries in a provider specific way.
type ProviderWithAgentFinder interface {
	environs.BootstrapEnviron
}

// ProviderService provides the API for working with agent binaries.
type ProviderService struct {
	Service
	providerAgentFinder func(context.Context) (ProviderWithAgentFinder, error)

	toolsFinder func(ctx context.Context, ss envtools.SimplestreamsFetcher, env environs.BootstrapEnviron,
		majorVersion, minorVersion int, streams []string, filter coretools.Filter,
	) (coretools.List, error)
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(
	st State,
	providerAgentFinder providertracker.ProviderGetter[ProviderWithAgentFinder],
	watcherFactory WatcherFactory,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:             st,
			watcherFactory: watcherFactory,
		},
		providerAgentFinder: providerAgentFinder,
		toolsFinder:         envtools.FindTools,
	}
}

// GetMachineTargetAgentVersion reports the target agent version that should be
// running on the provided machine identified by name. The following errors are
// possible:
// - [machineerrors.MachineNotFound]
// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound]
func (s *Service) GetMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetMachineUUID(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return coreagentbinary.Version{}, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"checking if machine %q exists when getting target agent version: %w",
			machineName, err,
		)
	}

	return s.st.GetMachineTargetAgentVersion(ctx, uuid)
}

// GetUnitTargetAgentVersion reports the target agent version that should be
// being run on the provided unit identified by name. The following errors
// are possible:
// - [applicationerrors.UnitNotFound] - When the unit in question does not exist.
//   - [github.com/juju/juju/domain/modelagent/errors.AgentVersionFound] if no
//     agent version record exists.
func (s *Service) GetUnitTargetAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return coreagentbinary.Version{}, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"checking if unit %q exists when getting target agent version: %w",
			unitName, err,
		)
	}

	return s.st.GetUnitTargetAgentVersion(ctx, uuid)
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
//   - [errors.NotValid] if the model ID is not valid;
//   - [github.com/juju/juju/domain/modelagent/errors.AgentVersionFound] if no
//     agent version record exists.
func (s *Service) GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
	return s.st.GetModelTargetAgentVersion(ctx)
}

// SetMachineReportedAgentVersion sets the reported agent version for the
// supplied machine name. Reported agent version is the version that the agent
// binary on this machine has reported it is running.
//
// The following errors are possible:
// - [coreerrors.NotValid] if the reportedVersion is not valid or the machine
// name is not valid.
// - [coreerrors.NotSupported] if the architecture is not supported.
// - [machineerrors.MachineNotFound] when the machine does not exist.
// - [machineerrors.MachineDead] when the machine is dead.
func (s *Service) SetMachineReportedAgentVersion(
	ctx context.Context,
	machineName machine.Name,
	reportedVersion coreagentbinary.Version,
) error {
	if err := machineName.Validate(); err != nil {
		return errors.Errorf("setting reported agent version for machine: %w", err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	machineUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Errorf(
			"getting machine UUID for machine %q: %w",
			machineName,
			err,
		)
	}

	if err := s.st.SetMachineRunningAgentBinaryVersion(ctx, machineUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting machine %q reported agent version (%s) in state: %w",
			machineUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// SetUnitReportedAgentVersion sets the reported agent version for the
// supplied unit name. Reported agent version is the version that the agent
// binary on this unit has reported it is running.
//
// The following errors are possible:
// - [coreerrors.NotValid] - when the reportedVersion is not valid.
// - [coreerrors.NotSupported] - when the architecture is not supported.
// - [applicationerrors.UnitNotFound] - when the unit does not exist.
// - [applicationerrors.UnitIsDead] - when the unit is dead.
func (s *Service) SetUnitReportedAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
	reportedVersion coreagentbinary.Version,
) error {
	if err := unitName.Validate(); err != nil {
		return errors.Errorf("unit name %q is not valid: %w", unitName, err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Errorf(
			"getting unit UUID for unit %q: %w",
			unitName,
			err,
		)
	}

	if err := s.st.SetUnitRunningAgentBinaryVersion(ctx, unitUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting unit %q reported agent version (%s) in state: %w",
			unitUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// WatchMachineTargetAgentVersion is responsible for watching the target agent
// version for machine and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [machineerrors.NotFound] - When no machine exists for the provided name.
func (s *Service) WatchMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (watcher.NotifyWatcher, error) {
	_, err := s.st.GetMachineUUID(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if machine %q exists when watching target agent version: %w", machineName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, errors.Errorf("getting watcher for machine %q model target agent version: %w", machineName, err)
	}
	return w, nil
}

// WatchUnitTargetAgentVersion is responsible for watching the target agent
// version for unit and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [applicationerrors.UnitNotFound] - When no unit exists for the provided name.
func (s *Service) WatchUnitTargetAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
) (watcher.NotifyWatcher, error) {
	_, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return nil, errors.Errorf("checking if unit %q exists when watching target agent version: %w", unitName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, errors.Errorf("getting watcher for unit %q model target agent version: %w", unitName, err)
	}
	return w, nil
}

// WatchModelTargetAgentVersion is responsible for watching the target agent
// version of this model and reporting when a change has happened in the
// version.
func (s *Service) WatchModelTargetAgentVersion(ctx context.Context) (watcher.NotifyWatcher, error) {
	w, err := s.watcherFactory.NewNotifyWatcher(
		eventsource.NamespaceFilter(s.st.NamespaceForWatchAgentVersion(), changestream.All),
	)
	if err != nil {
		return nil, errors.Errorf("creating watcher for agent version: %w", err)
	}
	return w, nil
}

// FindAgents calls findMatchingTools and then rewrites the URLs
// using the provided ToolsURLsGetter function.
func (s *ProviderService) FindAgents(ctx context.Context, args modelagent.FindAgentsParams) (coretools.List, error) {
	list, err := s.findMatchingAgents(ctx, args)
	if err != nil {
		return nil, err
	}

	// Rewrite the URLs so they point at the API servers. If the
	// tools are not in tools storage, then the API server will
	// download and cache them if the client requests that version.
	var fullList coretools.List
	for _, baseTools := range list {
		urls, err := args.ToolsURLsGetter(ctx, baseTools.Version)
		if err != nil {
			return nil, err
		}
		for _, url := range urls {
			tools := *baseTools
			tools.URL = url
			fullList = append(fullList, &tools)
		}
	}
	return fullList, nil
}

// findMatchingAgents searches agent storage and simplestreams for agents
// matching the given parameters.
// If an exact match is specified (number, ostype and arch) and is found in
// agent storage, then simplestreams will not be searched.
func (s *ProviderService) findMatchingAgents(ctx context.Context, args modelagent.FindAgentsParams) (result coretools.List, _ error) {
	exactMatch := args.Number != semversion.Zero && args.Arch != ""

	storageList, err := s.matchingStorageAgent(args)
	if err != nil && !errors.Is(err, coretools.ErrNoMatches) {
		return nil, err
	}
	if len(storageList) > 0 && exactMatch {
		return storageList, nil
	}

	// Look for tools in simplestreams too, but don't replace
	// any versions found in storage.
	provider, err := s.providerAgentFinder(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return storageList, nil
	} else if err != nil {
		return nil, errors.Errorf("getting agent finder: %w", err)
	}
	cfg := provider.Config()

	filter := toolsFilter(args)
	requestedStream := cfg.AgentStream()
	if args.AgentStream != "" {
		requestedStream = args.AgentStream
	}

	streams := envtools.PreferredStreams(&args.Number, cfg.Development(), requestedStream)
	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	majorVersion := args.Number.Major
	minorVersion := args.Number.Minor
	if args.Number == semversion.Zero {
		majorVersion = args.MajorVersion
		minorVersion = args.MinorVersion
	}
	simplestreamsList, err := s.toolsFinder(ctx, ss,
		provider, majorVersion, minorVersion, streams, filter,
	)
	if len(storageList) == 0 && err != nil {
		return nil, err
	}

	list := storageList
	found := make(map[semversion.Binary]bool)
	for _, tools := range storageList {
		found[tools.Version] = true
	}
	for _, tools := range simplestreamsList {
		if !found[tools.Version] {
			list = append(list, tools)
		}
	}
	sort.Sort(list)
	return list, nil
}

// matchingStorageAgent returns a coretools.List, with an entry for each
// metadata entry in the agent storage that matches the given parameters.
func (s *ProviderService) matchingStorageAgent(args modelagent.FindAgentsParams) (coretools.List, error) {
	allMetadata, err := args.AgentStorage.AllMetadata()
	if err != nil {
		return nil, err
	}
	list := make(coretools.List, len(allMetadata))
	for i, m := range allMetadata {
		vers, err := semversion.ParseBinary(m.Version)
		if err != nil {
			return nil, errors.Errorf(
				"unexpected bad version %q of agent binary in storage: %w",
				m.Version, err,
			)
		}
		list[i] = &coretools.Tools{
			Version: vers,
			Size:    m.Size,
			SHA256:  m.SHA256,
		}
	}
	list, err = list.Match(toolsFilter(args))
	if err != nil {
		return nil, err
	}
	// Return early if we are doing an exact match.
	if args.Number != semversion.Zero {
		if len(list) == 0 {
			return nil, coretools.ErrNoMatches
		}
		return list, nil
	}
	// At this point, we are matching just on major or minor version
	// rather than an exact match.
	var matching coretools.List
	for _, tools := range list {
		if tools.Version.Major != args.MajorVersion {
			continue
		}
		if args.MinorVersion > 0 && tools.Version.Minor != args.MinorVersion {
			continue
		}
		matching = append(matching, tools)
	}
	if len(matching) == 0 {
		return nil, coretools.ErrNoMatches
	}
	return matching, nil
}

func toolsFilter(args modelagent.FindAgentsParams) coretools.Filter {
	return coretools.Filter{
		Number: args.Number,
		Arch:   args.Arch,
		OSType: strings.ToLower(ostype.Ubuntu.String()),
	}
}
