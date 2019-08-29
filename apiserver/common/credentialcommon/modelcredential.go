// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state"
)

// ValidateExistingModelCredential checks if the cloud credential that a given model uses is valid for it.
func ValidateExistingModelCredential(backend PersistentBackend, callCtx context.ProviderCallContext) (params.ErrorResults, error) {
	model, err := backend.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	credentialTag, isSet := model.CloudCredential()
	if !isSet {
		return params.ErrorResults{}, nil
	}

	storedCredential, err := backend.CloudCredential(credentialTag)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if !storedCredential.IsValid() {
		return params.ErrorResults{}, errors.NotValidf("credential %q", storedCredential.Name)
	}
	credential := cloud.NewCredential(cloud.AuthType(storedCredential.AuthType), storedCredential.Attributes)
	return ValidateNewModelCredential(backend, callCtx, credentialTag, &credential)
}

// ValidateNewModelCredential checks if a new cloud credential could be valid for a given model.
func ValidateNewModelCredential(backend PersistentBackend, callCtx context.ProviderCallContext, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (params.ErrorResults, error) {
	openParams, err := buildOpenParams(backend, credentialTag, credential)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	model, err := backend.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	switch model.Type() {
	case state.ModelTypeCAAS:
		return checkCAASModelCredential(openParams)
	case state.ModelTypeIAAS:
		return checkIAASModelCredential(openParams, backend, callCtx)
	default:
		return params.ErrorResults{}, errors.NotSupportedf("model type %q", model.Type())
	}
}

func checkCAASModelCredential(brokerParams environs.OpenParams) (params.ErrorResults, error) {
	broker, err := newCAASBroker(brokerParams)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	_, err = broker.Namespaces()
	if err != nil {
		// If this call could not be made with provided credential, we know that the credential is invalid.
		return params.ErrorResults{}, errors.Trace(err)
	}
	return params.ErrorResults{}, nil
}

func checkIAASModelCredential(openParams environs.OpenParams, backend PersistentBackend, callCtx context.ProviderCallContext) (params.ErrorResults, error) {
	env, err := newEnv(openParams)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	// We only check persisted machines vs known cloud instances.
	// In the future, this check may be extended to other cloud resources,
	// entities and operation-level authorisations such as interfaces,
	// ability to CRUD storage, etc.
	return checkMachineInstances(backend, env, callCtx)
}

// checkMachineInstances compares model machines from state with
// the ones reported by the provider using supplied credential.
// This only makes sense for non-k8s providers.
func checkMachineInstances(backend PersistentBackend, provider CloudProvider, callCtx context.ProviderCallContext) (params.ErrorResults, error) {
	fail := func(original error) (params.ErrorResults, error) {
		return params.ErrorResults{}, original
	}

	// Get machines from state
	machines, err := backend.AllMachines()
	if err != nil {
		return fail(errors.Trace(err))
	}

	var results []params.ErrorResult

	serverError := func(received error) params.ErrorResult {
		return params.ErrorResult{Error: common.ServerError(received)}
	}

	machinesByInstance := make(map[string]string)
	for _, machine := range machines {
		if machine.IsContainer() {
			// Containers don't correspond to instances at the
			// provider level.
			continue
		}
		if manual, err := machine.IsManual(); err != nil {
			return fail(errors.Trace(err))
		} else if manual {
			continue
		}
		instanceId, err := machine.InstanceId()
		if errors.IsNotProvisioned(err) {
			// Skip over this machine; we wouldn't expect the cloud
			// to know about it.
			continue
		} else if err != nil {
			results = append(results, serverError(errors.Annotatef(err, "getting instance id for machine %s", machine.Id())))
			continue
		}
		machinesByInstance[string(instanceId)] = machine.Id()
	}

	// Check that we can see all machines' instances regardless of their state as perceived by the cloud, i.e.
	// this call will return all non-terminated instances.
	instances, err := provider.AllInstances(callCtx)
	if err != nil {
		return fail(errors.Trace(err))
	}

	instanceIds := set.NewStrings()
	for _, instance := range instances {
		id := string(instance.Id())
		instanceIds.Add(id)
		if _, found := machinesByInstance[id]; !found {
			results = append(results, serverError(errors.Errorf("no machine with instance %q", id)))
		}
	}

	for instanceId, name := range machinesByInstance {
		if !instanceIds.Contains(instanceId) {
			results = append(results, serverError(errors.Errorf("couldn't find instance %q for machine %s", instanceId, name)))
		}
	}

	return params.ErrorResults{Results: results}, nil
}

var (
	newEnv        = environs.New
	newCAASBroker = caas.New
)

func buildOpenParams(backend PersistentBackend, credentialTag names.CloudCredentialTag, credential *cloud.Credential) (environs.OpenParams, error) {
	fail := func(original error) (environs.OpenParams, error) {
		return environs.OpenParams{}, original
	}

	model, err := backend.Model()
	if err != nil {
		return fail(errors.Trace(err))
	}

	modelCloud, err := backend.Cloud(model.Cloud())
	if err != nil {
		return fail(errors.Trace(err))
	}

	err = model.ValidateCloudCredential(credentialTag, *credential)
	if err != nil {
		return fail(errors.Trace(err))
	}

	tempCloudSpec, err := environs.MakeCloudSpec(modelCloud, model.CloudRegion(), credential)
	if err != nil {
		return fail(errors.Trace(err))
	}

	cfg, err := model.Config()
	if err != nil {
		return fail(errors.Trace(err))
	}

	controllerConfig, err := backend.ControllerConfig()
	if err != nil {
		return fail(errors.Trace(err))
	}
	return environs.OpenParams{
		ControllerUUID: controllerConfig.ControllerUUID(),
		Cloud:          tempCloudSpec,
		Config:         cfg,
	}, nil
}
