// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/juju/clock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	domainconstraints "github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/statushistory"
)

const reprovisioningStatusMessage = "reprovisioning requested"

// Provider represents an underlying cloud provider.
type Provider interface {
	environs.BootstrapEnviron
	environs.InstanceLister
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
	clock clock.Clock,
	logger logger.Logger,
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

// ReprovisionMachine validates that the machine identified by name is eligible
// for reprovisioning, then applies the split-brain prevention liveness gates.
func (s *ProviderService) ReprovisionMachine(ctx context.Context, machineName machine.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	instanceID, err := s.validateReprovisionMachine(ctx, machineName)
	if err != nil {
		return errors.Capture(err)
	}

	present, err := s.st.IsMachineAgentPresent(ctx, machineName)
	if err != nil {
		return errors.Errorf("checking machine %q agent presence: %w", machineName, err)
	}
	if present {
		return errors.Errorf("machine %q: %w", machineName, machineerrors.MachineAgentPresent)
	}

	provider, err := s.providerGetter(ctx)
	if err != nil {
		return errors.Errorf("getting provider for machine %q reprovisioning: %w", machineName, err)
	}
	instances, err := provider.Instances(ctx, []instance.Id{instanceID})
	if err != nil && !errors.Is(err, environs.ErrNoInstances) && !errors.Is(err, environs.ErrPartialInstances) {
		return errors.Errorf("checking provider instance %q for machine %q: %w", instanceID, machineName, err)
	}

	// If the provider returns no instance, there is nothing to do.
	if len(instances) == 0 || instances[0] == nil {
		return nil
	}

	// If the provider reports that the instance is running, then there isn't
	// anything we should do.
	providerStatus := instances[0].Status(ctx)
	if providerStatus.Status == corestatus.Running {
		return errors.Errorf("machine %q instance %q: %w", machineName, instanceID, machineerrors.MachineProviderInstanceRunning)
	}
	return s.detachLostMachineCloudInstance(ctx, machineName, instanceID)
}

// detachLostMachineCloudInstance atomically clears provider-observed state for
// a lost machine instance and moves the machine back to pending. The expected
// instance ID prevents detaching a replacement that appeared after the
// provider liveness check.
func (s *ProviderService) detachLostMachineCloudInstance(
	ctx context.Context,
	machineName machine.Name,
	expectedInstanceID instance.Id,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	statusData := map[string]any{
		"old-instance-id": expectedInstanceID.String(),
	}
	encodedStatusData, err := json.Marshal(statusData)
	if err != nil {
		return errors.Errorf("encoding reprovisioning status data: %w", err)
	}

	now := s.clock.Now().UTC()
	if err := s.st.DetachLostMachineCloudInstance(
		ctx, machineName.String(), expectedInstanceID.String(), reprovisioningStatusMessage,
		encodedStatusData, now,
	); err != nil {
		return errors.Errorf("detaching lost cloud instance for machine %q: %w", machineName, err)
	}

	statusInfo := corestatus.StatusInfo{
		Status:  corestatus.Pending,
		Message: reprovisioningStatusMessage,
		Data:    statusData,
		Since:   &now,
	}
	for _, namespace := range []statushistory.Namespace{
		domainstatus.MachineNamespace.WithID(machineName.String()),
		domainstatus.MachineInstanceNamespace.WithID(machineName.String()),
	} {
		if err := s.statusHistory.RecordStatus(ctx, namespace, statusInfo); err != nil {
			s.logger.Warningf(ctx, "recording reprovisioning status history: %w", err)
		}
	}

	return nil
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
		// Should never happen; constraints are validated during merge.
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
