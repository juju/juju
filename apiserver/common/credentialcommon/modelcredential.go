// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
)

// ValidateModelCredential checks if a cloud credential is valid for a model.
func ValidateModelCredential(persisted PersistedCloudEntitiesBackend, provider CloudProvider, callCtx context.ProviderCallContext) (params.ErrorResults, error) {
	// We only check persisted machines vs known cloud instances.
	// In the future, this check may be extended to other cloud resources,
	// entities and operation-level authorisations such as interfaces,
	// ability to CRUD storage, etc.
	return CheckMachineInstances(persisted, provider, callCtx)
}

// ValidateNewModelCredential checks if a new cloud credential can be valid
// for a given model.
// Note that this call does not validate credential against the cloud of the model.
func ValidateNewModelCredential(backend PersistedModelBackend, newEnv NewEnvironFunc, callCtx context.ProviderCallContext, credential *cloud.Credential) (params.ErrorResults, error) {
	model, err := backend.Model()
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}

	modelCloud, err := backend.Cloud(model.Cloud())
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}
	tempCloudSpec, err := environs.MakeCloudSpec(modelCloud, model.CloudRegion(), credential)
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}

	cfg, err := model.Config()
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}
	tempOpenParams := environs.OpenParams{
		Cloud:  tempCloudSpec,
		Config: cfg,
	}
	env, err := newEnv(tempOpenParams)
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}

	return ValidateModelCredential(backend, env, callCtx)
}

// CheckMachineInstances compares model machines from state with
// the ones reported by the provider using supplied credential.
func CheckMachineInstances(backend PersistedCloudEntitiesBackend, provider CloudProvider, callCtx context.ProviderCallContext) (params.ErrorResults, error) {
	// Get machines from state
	machines, err := backend.AllMachines()
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}
	machinesByInstance := make(map[string]string)
	for _, machine := range machines {
		if machine.IsContainer() {
			// Containers don't correspond to instances at the
			// provider level.
			continue
		}
		if manual, err := machine.IsManual(); err != nil {
			return failCredentialValidation(errors.Trace(err))
		} else if manual {
			continue
		}
		instanceId, err := machine.InstanceId()
		if err != nil {
			// TODO (anastasiamac 2018-08-21) do we really want to fail all processing here or just an error result against this machine and keep going?
			return failCredentialValidation(errors.Annotatef(err, "getting instance id for machine %s", machine.Id()))
		}
		machinesByInstance[string(instanceId)] = machine.Id()
	}

	// Check can see all machines' instances
	instances, err := provider.AllInstances(callCtx)
	if err != nil {
		return failCredentialValidation(errors.Trace(err))
	}

	var results []params.ErrorResult

	errorResult := func(format string, args ...interface{}) params.ErrorResult {
		return params.ErrorResult{Error: common.ServerError(errors.Errorf(format, args...))}
	}

	instanceIds := set.NewStrings()
	for _, instance := range instances {
		id := string(instance.Id())
		instanceIds.Add(id)
		if _, found := machinesByInstance[id]; !found {
			results = append(results, errorResult("no machine with instance %q", id))
		}
	}

	for instanceId, name := range machinesByInstance {
		if !instanceIds.Contains(instanceId) {
			results = append(results, errorResult(
				"couldn't find instance %q for machine %s", instanceId, name))
		}
	}

	return params.ErrorResults{Results: results}, nil
}

func failCredentialValidation(original error) (params.ErrorResults, error) {
	return params.ErrorResults{}, original
}

// NewEnvironFunc defines function that obtains new Environ.
type NewEnvironFunc func(args environs.OpenParams) (environs.Environ, error)
