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
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (s *ProviderService) CreateMachine(ctx context.Context, machineName machine.Name, nonce *string) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Machine name must not contain container child names, so they must be
	// singular machine name.
	if machineName.IsContainer() {
		return "", errors.Errorf("machine name %q cannot be a container", machineName)
	}

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return "", errors.Errorf("getting provider for machine %q: %w", machineName, err)
	}
	if err := provider.PrecheckInstance(ctx, environs.PrecheckInstanceParams{}); err != nil {
		return "", errors.Errorf("prechecking instance for machine %q: %w", machineName, err)
	}

	// Make new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q: %w", machineName, err)
	}
	err = s.st.CreateMachine(ctx, machineName, nodeUUID, machineUUID, nonce)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q: %w", machineName, err)
	}

	if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, machineName, s.clock); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, nil
}

// CreateMachineWithParent creates the specified machine with the specified
// parent.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
// It returns a MachineNotFound error if the parent machine does not exist.
func (s *ProviderService) CreateMachineWithParent(ctx context.Context, machineName, parentName machine.Name) (machine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Machine names must not contain container child names, so they must be
	// singular machine name.
	if machineName.IsContainer() {
		return "", errors.Errorf("machine name %q cannot be a container", machineName)
	} else if parentName.IsContainer() {
		return "", errors.Errorf("parent machine name %q cannot be a container", parentName)
	}

	// The machine name then becomes, the <parent name>/scope/<machine name>.
	// TODO (stickupkid): Use the placement directive to determine the
	// the scope.
	name, err := parentName.NamedChild("lxd", machineName.String())
	if err != nil {
		return "", errors.Errorf("creating machine name from parent %q and machine %q: %w", parentName, machineName, err)
	}
	if err := name.Validate(); err != nil {
		return "", errors.Errorf("validating machine name %q: %w", name, err)
	}

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return "", errors.Errorf("getting provider for machine %q: %w", name, err)
	}
	if err := provider.PrecheckInstance(ctx, environs.PrecheckInstanceParams{}); err != nil {
		return "", errors.Errorf("prechecking instance for machine %q: %w", name, err)
	}

	// Make new UUIDs for the net-node and the machine.
	// We want to do this in the service layer so that if retries are invoked at
	// the state layer we don't keep regenerating.
	nodeUUID, machineUUID, err := createUUIDs()
	if err != nil {
		return "", errors.Errorf("creating machine %q: %w", name, err)
	}
	_, err = s.st.CreateMachineWithParent(ctx, name, nodeUUID, machineUUID)
	if err != nil {
		return machineUUID, errors.Errorf("creating machine %q: %w", name, err)
	}

	if err := recordCreateMachineStatusHistory(ctx, s.statusHistory, name, s.clock); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return machineUUID, nil
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
