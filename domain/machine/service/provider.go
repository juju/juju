// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	"github.com/juju/clock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	domainconstraints "github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
)

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.BootstrapEnviron
	environs.InstanceTypesFetcher
	environs.InstancePrechecker
}

// LXDProfileProvider represents a provider that can write LXD profiles.
type LXDProfileProvider interface {
	environs.LXDProfiler
}

// ProviderService provides the API for working with machines using the
// underlying provider.
type ProviderService struct {
	Service

	providerGetter           providertracker.ProviderGetter[Provider]
	lxdProfileProviderGetter providertracker.ProviderGetter[LXDProfileProvider]
}

// NewProviderService creates a new ProviderService.
func NewProviderService(
	st State,
	statusHistory StatusHistory,
	providerGetter providertracker.ProviderGetter[Provider],
	lxdProfileProviderGetter providertracker.ProviderGetter[LXDProfileProvider],
	clock clock.Clock, logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:            st,
			statusHistory: statusHistory,
			clock:         clock,
			logger:        logger,
		},
		providerGetter:           providerGetter,
		lxdProfileProviderGetter: lxdProfileProviderGetter,
	}
}

// AddMachine creates the net node and machines if required, depending
// on the placement.
// It returns the net node UUID for the machine and a list of child
// machine names that were created as part of the placement.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] if the parent machine (for container
// placement) does not exist.
func (s *ProviderService) AddMachine(ctx context.Context, args domainmachine.AddMachineArgs) (AddMachineResults, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return AddMachineResults{}, errors.Errorf("getting provider for create machine: %w", err)
	}
	os, err := encodeOSType(args.Platform.OSType)
	if err != nil {
		return AddMachineResults{}, errors.Capture(err)
	}
	base, err := base.ParseBase(os, args.Platform.Channel)
	if err != nil {
		return AddMachineResults{}, errors.Capture(err)
	}
	mergedCons, err := s.mergeMachineAndModelConstraints(ctx, args.Constraints)
	if err != nil {
		return AddMachineResults{}, errors.Capture(err)
	}
	precheckInstanceParams := environs.PrecheckInstanceParams{
		Base:        base,
		Constraints: mergedCons,
	}
	// We only precheck placement directive with the provider if the placement
	// type is provider.
	if args.Directive.Type == deployment.PlacementTypeProvider {
		precheckInstanceParams.Placement = args.Directive.Directive
	}
	if err := provider.PrecheckInstance(ctx, precheckInstanceParams); err != nil {
		return AddMachineResults{}, errors.Errorf("prechecking instance for create machine: %w", err)
	}

	_, machineNames, err := s.st.AddMachine(ctx, args)
	if err != nil {
		return AddMachineResults{}, errors.Capture(err)
	}

	for _, machineName := range machineNames {
		if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, machineName, s.clock); err != nil {
			s.logger.Infof(ctx, "failed recording machine status history: %w", err)
		}
	}

	res := AddMachineResults{
		MachineName: machineNames[0],
	}
	if len(machineNames) > 1 {
		res.ChildMachineName = &machineNames[1]
	}
	return res, nil
}

func encodeOSType(ostype deployment.OSType) (string, error) {
	switch ostype {
	case deployment.Ubuntu:
		return base.UbuntuOS, nil
	default:
		return "", errors.Errorf("unknown os type %q", ostype)
	}
}

func (s *ProviderService) mergeMachineAndModelConstraints(ctx context.Context, cons domainconstraints.Constraints) (constraints.Value, error) {
	// If the provider doesn't support constraints validation, then we can
	// just return the zero value.
	validator, err := s.constraintsValidator(ctx)
	if err != nil || validator == nil {
		return constraints.Value{}, errors.Capture(err)
	}

	modelCons, err := s.st.GetModelConstraints(ctx)
	if err != nil && !errors.Is(err, modelerrors.ConstraintsNotFound) {
		return constraints.Value{}, errors.Errorf("retrieving model constraints constraints: %w	", err)
	}

	mergedCons, err := validator.Merge(domainconstraints.EncodeConstraints(modelCons), domainconstraints.EncodeConstraints(cons))
	if err != nil {
		return constraints.Value{}, errors.Errorf("merging application and model constraints: %w", err)
	}

	// Always ensure that we snapshot the constraint's architecture when adding
	// the machine. If no architecture in the constraints, then look at
	// the model constraints. If no architecture is found in the model, use the
	// default architecture (amd64).
	snapshotCons := mergedCons
	if !snapshotCons.HasArch() {
		a := constraints.ArchOrDefault(snapshotCons, nil)
		snapshotCons.Arch = &a
	}

	return snapshotCons, nil
}

func (s *ProviderService) constraintsValidator(ctx context.Context) (constraints.Validator, error) {
	provider, err := s.providerGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		// Not validating constraints, as the provider doesn't support it.
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	validator, err := provider.ConstraintsValidator(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return validator, nil
}

// GetBootstrapEnviron returns the bootstrap environ.
//
// Note: Use only for [environs.ImageMetadataSources] or [environs.FindTools]
// which are also used during bootstrap before the controller instance is
// created.
func (s *ProviderService) GetBootstrapEnviron(ctx context.Context) (environs.BootstrapEnviron, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

// GetInstanceTypesFetcher returns the instance types fetcher.
func (s *ProviderService) GetInstanceTypesFetcher(ctx context.Context) (environs.InstanceTypesFetcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return provider, nil
}

// UpdateLXDProfiles writes LXD Profiles to LXC for applications on the
// given machine if the providers supports it. A slice of profile names
// is returned. If the provider does not support LXDProfiles, no error
// is returned.
func (s *ProviderService) UpdateLXDProfiles(ctx context.Context, modelName, machineID string) ([]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.lxdProfileProviderGetter(ctx)
	if errors.Is(err, coreerrors.NotSupported) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Errorf("getting provider: %w", err)
	}

	profileArgs, err := s.Service.st.GetLXDProfilesForMachine(ctx, machineID)
	if err != nil {
		return nil, errors.Errorf("updating LXD profiles in the model for machine %s: %w", machineID, err)
	}

	// TODO (lxdprofiles)
	// We cannot guarantee that there is only one service per model, nor
	// one environ per service. There have been past issues with attempting
	// write the same profile at the same time. Previously this code
	// had a lock, but the services must not contain state.
	var pNames []string
	for _, arg := range profileArgs {
		pName := lxdprofile.Name(modelName, arg.ApplicationName, arg.CharmRevision)
		profile, err := decodeLXDProfile(arg.LXDProfile)
		if err != nil {
			return nil, errors.Errorf("decoding LXD profile for machine %s: %w", machineID, err)
		}
		if err := provider.MaybeWriteLXDProfile(pName, profile); err != nil {
			return nil, errors.Capture(err)
		}
		pNames = append(pNames, pName)
	}
	return pNames, nil
}

func decodeLXDProfile(profile []byte) (lxdprofile.Profile, error) {
	if len(profile) == 0 {
		return lxdprofile.Profile{}, nil
	}

	var result lxdProfile
	if err := json.Unmarshal(profile, &result); err != nil {
		return lxdprofile.Profile{}, errors.Errorf("unmarshal lxd profile: %w", err)
	}

	return lxdprofile.Profile{
		Config:      result.Config,
		Description: result.Description,
		Devices:     result.Devices,
	}, nil
}

type lxdProfile struct {
	Config      map[string]string            `json:"config,omitempty"`
	Description string                       `json:"description,omitempty"`
	Devices     map[string]map[string]string `json:"devices,omitempty"`
}

// NewNoopProvider returns a no-op provider that implements the Provider
// interface.
func NewNoopProvider() Provider {
	return &noopProvider{}
}

type noopProvider struct {
	Provider
}

// PrecheckInstance is a no-op implementation of the environs.InstancePrechecker
// interface. It does not perform any pre-checks on the instance.
func (p *noopProvider) PrecheckInstance(ctx context.Context, params environs.PrecheckInstanceParams) error {
	return nil
}

// ConstraintsValidator is a no-op implementation of the
// environs.ConstraintsValidator interface. It returns a new constraints
// validator without any specific constraints or vocabulary.
func (p *noopProvider) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}
