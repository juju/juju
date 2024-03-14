// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialcommon

import (
	stdcontext "context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ValidateExistingModelCredential checks if the cloud credential that a given
// model uses is valid for it. For IAAS models, if the modelMigrationCheck is
// disabled, then it will not perform the mapping of the instances on the clouud
// to the machines on the model, and deem the credential valid if it can be used
// to just access the instances on the cloud. Otherwise the instances will be
// mapped against the machines on the model. Furthermore, normally it's valid to
// have more instances than machines, but if the checkCloudInstances is enabled,
// then a 1:1 mapping is expected to deem the credential valid.
func ValidateExistingModelCredential(
	backend PersistentBackend,
	callCtx context.ProviderCallContext,
	checkCloudInstances bool,
	modelMigrationCheck bool) (params.ErrorResults, error) {
	model, err := backend.Model()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	credentialTag, isSet := model.CloudCredentialTag()
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
	return ValidateNewModelCredential(backend, callCtx, credentialTag,
		&credential, checkCloudInstances, modelMigrationCheck)
}

// ValidateNewModelCredential checks if a new cloud credential could be valid
// for a given model. For IAAS models, if the modelMigrationCheck is disabled,
// then it will not perform the mapping of the instances on the clouud to the
// machines on the model, and deem the credential valid if it can be used to
// just access the instances on the cloud. Otherwise the instances will be
// mapped against the machines on the model. Furthermore, normally it's valid to
// have more instances than machines, but if the checkCloudInstances is enabled,
// then a 1:1 mapping is expected to deem the credential valid.
func ValidateNewModelCredential(
	backend PersistentBackend,
	callCtx context.ProviderCallContext,
	credentialTag names.CloudCredentialTag,
	credential *cloud.Credential,
	checkCloudInstances bool,
	modelMigrationCheck bool) (params.ErrorResults, error) {
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
		return checkIAASModelCredential(openParams, backend, callCtx,
			checkCloudInstances, modelMigrationCheck)
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

// checkIAASModelCredential checks if the cloud credential that a given model
// uses is valid for it. if the modelMigrationCheck is disabled, then it will
// not perform the mapping of the instances on the clouud to the machines on the
// model, and deem the credential valid if it can be used to just access the
// instances on the cloud. Otherwise the instances will be mapped against the
// machines on the model. Furthermore, normally it's valid to have more
// instances than machines, but if the checkCloudInstances is enabled, then a
// 1:1 mapping is expected to deem the credential valid.
func checkIAASModelCredential(
	openParams environs.OpenParams,
	backend PersistentBackend,
	callCtx context.ProviderCallContext,
	checkCloudInstances bool,
	modelMigrationCheck bool) (params.ErrorResults, error) {
	env, err := newEnv(callCtx, openParams)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	// Check that we can see all machines' instances regardless of their state
	// as perceived by the cloud, i.e. this call will return all non-terminated
	// instances.
	instances, err := env.AllInstances(callCtx)
	// If we're not performing this check for model migrations; then being able
	// to get the instances is proof enough that the credential is valid
	// (authenticated, authorization is a different concern), no need to check
	// the mapping between instances and machines.
	if err != nil {
		return params.ErrorResults{Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(errors.Annotate(err, "receiving instances from provider"))}},
		}, errors.Trace(err)
	}

	if !modelMigrationCheck {
		return params.ErrorResults{}, nil
	}

	// We only check persisted machines vs known cloud instances. In the future,
	// this check may be extended to other cloud resources, entities and
	// operation-level authorisations such as interfaces, ability to CRUD
	// storage, etc.
	return checkMachineInstances(backend, checkCloudInstances, instances)
}

// checkMachineInstances compares model machines from state with the ones
// reported by the provider using supplied credential. This only makes sense for
// non-k8s providers.
func checkMachineInstances(
	backend PersistentBackend,
	checkCloudInstances bool,
	instances []instances.Instance) (params.ErrorResults, error) {
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
			results = append(results, serverError(errors.Annotatef(err,
				"getting instance id for machine %s", machine.Id())))
			continue
		}
		machinesByInstance[string(instanceId)] = machine.Id()
	}

	// From here, we cross examine all machines we know about with all the
	// instances we can reach and ensure that they correspond 1:1. This is
	// useful for model migration, for example, since we want to know if we have
	// moved the known universe correctly.
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
	backend PersistentBackend,
	credentialTag names.CloudCredentialTag,
	credential *cloud.Credential) (environs.OpenParams, error) {
	fail := func(original error) (environs.OpenParams, error) {
		return environs.OpenParams{}, original
	}

	model, err := backend.Model()
	if err != nil {
		return fail(errors.Trace(err))
	}

	modelCloud, err := backend.Cloud(model.CloudName())
	if err != nil {
		return fail(errors.Trace(err))
	}

	err = model.ValidateCloudCredential(credentialTag, *credential)
	if err != nil {
		return fail(errors.Trace(err))
	}

	tempCloudSpec, err := environscloudspec.MakeCloudSpec(modelCloud,
		model.CloudRegion(), credential)
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
