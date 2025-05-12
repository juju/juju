// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/errors"
)

// MachineService defines the methods that the credential service assumes from
// the Machine service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID machine.UUID) (string, error)
}

// MachineState provides access to all machines.
type MachineState interface {
	// AllMachines returns all machines in the model.
	AllMachines() ([]Machine, error)
}

// Machine defines machine methods needed for the check.
type Machine interface {
	// IsManual returns true if the machine was manually provisioned.
	IsManual() (bool, error)

	// IsContainer returns true if the machine is a container.
	IsContainer() bool

	// Id returns the machine id.
	Id() string
}

// CloudProvider defines methods needed from the cloud provider to perform the check.
type CloudProvider interface {
	// AllInstances returns all instances currently known to the cloud provider.
	AllInstances(ctx context.Context) ([]instances.Instance, error)
}

// CredentialValidationContext provides access to artefacts needed to
// validate a credential for a given model.
type CredentialValidationContext struct {
	ControllerUUID string

	Config         *config.Config
	MachineState   MachineState
	MachineService MachineService

	ModelType coremodel.ModelType
	Cloud     cloud.Cloud
	Region    string
}

// CredentialValidator instances check that a given credential is
// valid for any models which want to use it.
type CredentialValidator interface {
	Validate(
		ctx context.Context,
		validationContext CredentialValidationContext,
		credentialKey corecredential.Key,
		credential *cloud.Credential,
		checkCloudInstances bool,
	) ([]error, error)
}

type defaultCredentialValidator struct{}

// NewCredentialValidator returns the credential validator used in production.
func NewCredentialValidator() CredentialValidator {
	return defaultCredentialValidator{}
}

// Validate checks if a new cloud credential could be valid for a model whose
// details are defined in the context.
func (v defaultCredentialValidator) Validate(
	ctx context.Context,
	validationContext CredentialValidationContext,
	key corecredential.Key,
	cred *cloud.Credential,
	checkCloudInstances bool,
) (machineErrors []error, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := key.Validate(); err != nil {
		return nil, errors.Errorf("credential %w", err)
	}

	openParams, err := v.buildOpenParams(validationContext, key, cred)
	if err != nil {
		return nil, errors.Capture(err)
	}
	switch validationContext.ModelType {
	case coremodel.CAAS:
		return checkCAASModelCredential(ctx, openParams)
	case coremodel.IAAS:
		return checkIAASModelCredential(ctx, validationContext.MachineState, validationContext.MachineService, openParams, checkCloudInstances)
	default:
		return nil, errors.Errorf("model type %q %w", validationContext.ModelType, coreerrors.NotSupported)
	}
}

// TODO (stickupkid): This should be removed with haste.
// Instead the provider factory should allow you to get a provider without a
// credential validator.
func checkCAASModelCredential(ctx context.Context, brokerParams environs.OpenParams) ([]error, error) {
	broker, err := newCAASBroker(ctx, brokerParams, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, errors.Capture(err)
	}

	if err = broker.CheckCloudCredentials(ctx); err != nil {
		return nil, errors.Capture(err)
	}
	return nil, nil
}

// TODO (stickupkid): This should be removed with haste.
// Instead the provider factory should allow you to get a provider without a
// credential validator.
func checkIAASModelCredential(ctx context.Context, machineState MachineState, machineService MachineService, openParams environs.OpenParams, checkCloudInstances bool) ([]error, error) {
	env, err := newEnv(ctx, openParams, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, errors.Capture(err)
	}
	// We only check persisted machines vs known cloud instances.
	// In the future, this check may be extended to other cloud resources,
	// entities and operation-level authorisations such as interfaces,
	// ability to CRUD storage, etc.
	return checkMachineInstances(ctx, machineState, machineService, env, checkCloudInstances)
}

// checkMachineInstances compares model machines from state with
// the ones reported by the provider using supplied credential.
// This only makes sense for non-k8s providers.
func checkMachineInstances(ctx context.Context, machineState MachineState, machineService MachineService, provider CloudProvider, checkCloudInstances bool) ([]error, error) {
	// Get machines from state
	machines, err := machineState.AllMachines()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []error

	machinesByInstance := make(map[string]string)
	for _, m := range machines {
		if m.IsContainer() {
			// Containers don't correspond to instances at the
			// provider level.
			continue
		}
		if manual, err := m.IsManual(); err != nil {
			return nil, errors.Capture(err)
		} else if manual {
			continue
		}
		machineUUID, err := machineService.GetMachineUUID(ctx, machine.Name(m.Id()))
		if err != nil {
			return nil, errors.Capture(err)
		}
		instanceId, err := machineService.InstanceID(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			// Skip over this machine; we wouldn't expect the cloud
			// to know about it.
			continue
		} else if err != nil {
			results = append(results, errors.Errorf("getting instance id for machine %s: %w", m.Id(), err))
			continue
		}
		machinesByInstance[instanceId] = m.Id()
	}

	// Check that we can see all machines' instances regardless of their state as perceived by the cloud, i.e.
	// this call will return all non-terminated instances.
	instances, err := provider.AllInstances(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// From here, there 2 ways of checking whether the credential is valid:
	// 1. Can we reach all cloud instances that machines know about?
	// 2. Can we cross examine all machines we know about with all the instances we can reach
	// and ensure that they correspond 1:1.
	// Second check (2) is more useful for model migration, for example, since we want to know if
	// we have moved the known universe correctly. However, it is a but redundant if we just care about
	// credential validity since the first check (1) addresses all our concerns.

	instanceIds := set.NewStrings()
	for _, instance := range instances {
		id := instance.Id().String()
		instanceIds.Add(id)
		if checkCloudInstances {
			if _, found := machinesByInstance[id]; !found {
				results = append(results, errors.Errorf("no machine with instance %q", id))
			}
		}
	}

	for instanceId, name := range machinesByInstance {
		if !instanceIds.Contains(instanceId) {
			results = append(results, errors.Errorf("couldn't find instance %q for machine %s", instanceId, name))
		}
	}

	return results, nil
}

var (
	newEnv        = environs.New
	newCAASBroker = caas.New
)

func (v defaultCredentialValidator) buildOpenParams(
	ctx CredentialValidationContext, credentialKey corecredential.Key, credential *cloud.Credential,
) (environs.OpenParams, error) {
	fail := func(original error) (environs.OpenParams, error) {
		return environs.OpenParams{}, original
	}

	err := v.validateCloudCredential(ctx.Cloud, credentialKey)
	if err != nil {
		return fail(errors.Capture(err))
	}

	tempCloudSpec, err := environscloudspec.MakeCloudSpec(ctx.Cloud, ctx.Region, credential)
	if err != nil {
		return fail(errors.Capture(err))
	}

	return environs.OpenParams{
		ControllerUUID: ctx.ControllerUUID,
		Cloud:          tempCloudSpec,
		Config:         ctx.Config,
	}, nil
}

// validateCloudCredential validates the given cloud credential
// name against the provided cloud definition and credentials.
func (v defaultCredentialValidator) validateCloudCredential(
	cld cloud.Cloud,
	credentialKey corecredential.Key,
) error {
	if !credentialKey.IsZero() {
		if credentialKey.Cloud != cld.Name {
			return errors.Errorf("credential %q %w", credentialKey, coreerrors.NotValid)
		}
		return nil
	}
	var hasEmptyAuth bool
	for _, authType := range cld.AuthTypes {
		if authType != cloud.EmptyAuthType {
			continue
		}
		hasEmptyAuth = true
		break
	}
	if !hasEmptyAuth {
		return errors.Errorf("missing CloudCredential %w", coreerrors.NotValid)
	}
	return nil
}
