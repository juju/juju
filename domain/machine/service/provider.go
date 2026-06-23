// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/clock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
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

// ProviderService provides the API for working with machines using the
// underlying provider.
type ProviderService struct {
	Service

	providerGetter providertracker.ProviderGetter[Provider]
}

// NewProviderService creates a new ProviderService.
func NewProviderService(
	st State,
	statusHistory StatusHistory,
	providerGetter providertracker.ProviderGetter[Provider],
	clock clock.Clock, logger logger.Logger,
) *ProviderService {
	return &ProviderService{
		Service: Service{
			st:            st,
			statusHistory: statusHistory,
			clock:         clock,
			logger:        logger,
		},
		providerGetter: providerGetter,
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
	provider, err := s.providerGetter(ctx)
	if err != nil {
		return AddMachineResults{}, errors.Errorf("getting provider for create machine: %w", err)
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
			s.logger.Warningf(ctx, "recording machine status history: %w", err)
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

// mergeMachineAndModelConstraints resolves given application constraints, taking
// into account the model constraints
func (s *ProviderService) mergeMachineAndModelConstraints(ctx context.Context, cons domainconstraints.Constraints) (constraints.Value, error) {
	validator, err := s.constraintsValidator(ctx)
	if err != nil {
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

	// Validate merged constraints to catch unsupported constraints.
	unsupported, err := validator.Validate(mergedCons)
	if err != nil {
		// Should never happens, constraint are validated in merge
		return constraints.Value{}, errors.Capture(err)
	}
	if len(unsupported) > 0 {
		s.logger.Warningf(ctx,
			"unsupported constraints: %v", strings.Join(unsupported, ","))
	}

	return mergedCons, nil
}

// constraintsValidator queries the provider for a constraints validator.
// If the provider doesn't support constraints validation, then we return
// a default validator
func (s *ProviderService) constraintsValidator(ctx context.Context) (constraints.Validator, error) {
	provider, err := s.providerGetter(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	validator, err := provider.ConstraintsValidator(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	} else if validator == nil {
		return constraints.NewValidator(), nil
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
