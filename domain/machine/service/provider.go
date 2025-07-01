// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	domainmachine "github.com/juju/juju/domain/machine"
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

// CreateMachine creates the specified machine. The nonce is an optional
// parameter and is used only used during bootstrapping to ensure that
// the machine is created with a unique name.
func (s *ProviderService) CreateMachine(ctx context.Context, args CreateMachineArgs) (machine.UUID, machine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return "", "", errors.Errorf("getting provider for create machine: %w", err)
	}
	if err := provider.PrecheckInstance(ctx, environs.PrecheckInstanceParams{}); err != nil {
		return "", "", errors.Errorf("prechecking instance for create machine: %w", err)
	}

	// Make new UUIDs for the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	stateArgs, err := s.makeCreateMachineArgs(args, machineUUID)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	machineName, err := s.st.CreateMachine(ctx, stateArgs)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, machineName, s.clock); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, machineName, nil
}

// CreateMachineWithParent creates the specified machine with the specified
// parent.
// It returns a MachineNotFound error if the parent machine does not exist.
func (s *ProviderService) CreateMachineWithParent(ctx context.Context, args CreateMachineArgs, parentName machine.Name) (machine.UUID, machine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return "", "", errors.Errorf("getting provider for create machine with parent %q: %w", parentName, err)
	}
	if err := provider.PrecheckInstance(ctx, environs.PrecheckInstanceParams{}); err != nil {
		return "", "", errors.Errorf("prechecking instance for create machine with parent %q: %w", parentName, err)
	}

	// Make new UUIDs for the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	machineUUID, err := machine.NewUUID()
	if err != nil {
		return "", "", errors.Capture(err)
	}
	parentUUID, err := s.st.GetMachineUUID(ctx, parentName)
	if err != nil {
		return "", "", errors.Errorf("getting parent UUID for machine %q: %w", parentName, err)
	}
	stateArgs, err := s.makeCreateMachineArgs(args, machineUUID)
	if err != nil {
		return "", "", errors.Capture(err)
	}
	machineName, err := s.st.CreateMachineWithParent(ctx, stateArgs, parentUUID.String())
	if err != nil {
		return "", "", errors.Errorf("creating machine with parent %q: %w", parentName, err)
	}

	if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, machineName, s.clock); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, machineName, nil
}

func (s *ProviderService) makeCreateMachineArgs(args CreateMachineArgs, machineUUID machine.UUID) (domainmachine.CreateMachineArgs, error) {
	return domainmachine.CreateMachineArgs{
		Nonce:       args.Nonce,
		MachineUUID: machineUUID,
		Platform:    args.Platform,
		Directive:   args.Directive,
		Constraints: args.Constraints,
	}, nil
}

// GetBootstrapEnviron returns the bootstrap environ.
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
