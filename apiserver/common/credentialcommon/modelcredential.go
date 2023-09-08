// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ValidateExistingModelCredential checks if the cloud credential that a given model uses is valid for it.
func ValidateExistingModelCredential(
	backend PersistentBackend, credentialService common.CredentialService,
	callCtx context.ProviderCallContext, cld cloud.Cloud, checkCloudInstances bool,
) (params.ErrorResults, error) {
	model, err := backend.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	credentialTag, isSet := model.CloudCredentialTag()
	if !isSet {
		return params.ErrorResults{}, nil
	}

	storedCredential, err := credentialService.CloudCredential(callCtx, credentialTag)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if storedCredential.Invalid {
		return params.ErrorResults{}, errors.NotValidf("credential %q", storedCredential.Label)
	}
	credential := cloud.NewCredential(storedCredential.AuthType(), storedCredential.Attributes())
	return ValidateNewModelCredential(backend, callCtx, credentialTag, &credential, cld, checkCloudInstances)
}

// ValidateNewModelCredential checks if a new cloud credential could be valid for a given model.
func ValidateNewModelCredential(
	backend PersistentBackend, callCtx context.ProviderCallContext,
	credentialTag names.CloudCredentialTag, credential *cloud.Credential, cld cloud.Cloud, checkCloudInstances bool,
) (params.ErrorResults, error) {
	openParams, err := buildOpenParams(callCtx, backend, cld, credentialTag, credential)
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
		return checkIAASModelCredential(openParams, backend, callCtx, checkCloudInstances)
	default:
		return params.ErrorResults{}, errors.NotSupportedf("model type %q", model.Type())
	}
}

func checkCAASModelCredential(brokerParams environs.OpenParams) (params.ErrorResults, error) {
	broker, err := newCAASBroker(stdcontext.TODO(), brokerParams)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	if err = broker.CheckCloudCredentials(); err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	return params.ErrorResults{}, nil
}

func checkIAASModelCredential(openParams environs.OpenParams, backend PersistentBackend, callCtx context.ProviderCallContext, checkCloudInstances bool) (params.ErrorResults, error) {
	env, err := newEnv(callCtx, openParams)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	// We only check persisted machines vs known cloud instances.
	// In the future, this check may be extended to other cloud resources,
	// entities and operation-level authorisations such as interfaces,
	// ability to CRUD storage, etc.
	return checkMachineInstances(backend, env, callCtx, checkCloudInstances)
}

// checkMachineInstances compares model machines from state with
// the ones reported by the provider using supplied credential.
// This only makes sense for non-k8s providers.
func checkMachineInstances(backend PersistentBackend, provider CloudProvider, callCtx context.ProviderCallContext, checkCloudInstances bool) (params.ErrorResults, error) {
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
		return params.ErrorResult{Error: apiservererrors.ServerError(received)}
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

	// From here, there 2 ways of checking whether the credential is valid:
	// 1. Can we reach all cloud instances that machines know about?
	// 2. Can we cross examine all machines we know about with all the instances we can reach
	// and ensure that they correspond 1:1.
	// Second check (2) is more useful for model migration, for example, since we want to know if
	// we have moved the known universe correctly. However, it is a but redundant if we just care about
	// credential validity since the first check (1) addresses all our concerns.

	instanceIds := set.NewStrings()
	for _, instance := range instances {
		id := string(instance.Id())
		instanceIds.Add(id)
		if checkCloudInstances {
			if _, found := machinesByInstance[id]; !found {
				results = append(results, serverError(errors.Errorf("no machine with instance %q", id)))
			}
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

func buildOpenParams(
	ctx context.ProviderCallContext,
	backend PersistentBackend, modelCloud cloud.Cloud,
	credentialTag names.CloudCredentialTag, credential *cloud.Credential,
) (environs.OpenParams, error) {
	fail := func(original error) (environs.OpenParams, error) {
		return environs.OpenParams{}, original
	}

	model, err := backend.Model()
	if err != nil {
		return fail(errors.Trace(err))
	}

	err = validateCloudCredential(modelCloud, credentialTag)
	if err != nil {
		return fail(errors.Trace(err))
	}

	tempCloudSpec, err := environscloudspec.MakeCloudSpec(modelCloud, model.CloudRegion(), credential)
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

// validateCloudCredential validates the given cloud credential
// name against the provided cloud definition and credentials.
func validateCloudCredential(
	cld cloud.Cloud,
	cloudCredential names.CloudCredentialTag,
) error {
	if cloudCredential != (names.CloudCredentialTag{}) {
		if cloudCredential.Cloud().Id() != cld.Name {
			return errors.NotValidf("credential %q", cloudCredential.Id())
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
		return errors.NotValidf("missing CloudCredential")
	}
	return nil
}
