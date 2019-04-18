// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage/poolmanager"
)

type API struct {
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser

	auth      facade.Authorizer
	resources facade.Resources

	state              CAASOperatorProvisionerState
	storagePoolManager poolmanager.PoolManager
}

// NewStateCAASOperatorProvisionerAPI provides the signature required for facade registration.
func NewStateCAASOperatorProvisionerAPI(ctx facade.Context) (*API, error) {

	authorizer := ctx.Auth()
	resources := ctx.Resources()

	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(ctx.State())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(ctx.State()), registry)

	return NewCAASOperatorProvisionerAPI(resources, authorizer, stateShim{ctx.State()}, pm)
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorProvisionerState,
	storagePoolManager poolmanager.PoolManager,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		PasswordChanger:    common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		LifeGetter:         common.NewLifeGetter(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		APIAddresser:       common.NewAPIAddresser(st, resources),
		auth:               authorizer,
		resources:          resources,
		state:              st,
		storagePoolManager: storagePoolManager,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch CAAS applications
// deployed to this model.
func (a *API) WatchApplications() (params.StringsWatchResult, error) {
	watch := a.state.WatchApplications()
	// Consume the initial event and forward it to the result.
	if changes, ok := <-watch.Changes(); ok {
		return params.StringsWatchResult{
			StringsWatcherId: a.resources.Register(watch),
			Changes:          changes,
		}, nil
	}
	return params.StringsWatchResult{}, watcher.EnsureErr(watch)
}

// OperatorProvisioningInfo returns the info needed to provision an operator.
func (a *API) OperatorProvisioningInfo() (params.OperatorProvisioningInfo, error) {
	cfg, err := a.state.ControllerConfig()
	if err != nil {
		return params.OperatorProvisioningInfo{}, err
	}

	model, err := a.state.Model()
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Trace(err)
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return params.OperatorProvisioningInfo{}, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in model config %q", modelConfig.Name()),
		)
	}
	vers.Build = 0

	imagePath := podcfg.GetJujuOCIImagePath(cfg, vers)
	storageClassName, _ := modelConfig.AllAttrs()[provider.OperatorStorageKey].(string)
	if storageClassName == "" {
		return params.OperatorProvisioningInfo{}, errors.New("no operator storage class defined")
	}
	charmStorageParams, err := CharmStorageParams(cfg.ControllerUUID(), storageClassName, modelConfig, "", a.storagePoolManager)
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Annotatef(err, "getting operator storage parameters")
	}
	apiAddresses, err := a.APIAddresses()
	if err == nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Annotatef(err, "getting api addresses")
	}

	resourceTags := tags.ResourceTags(
		names.NewModelTag(model.UUID()),
		names.NewControllerTag(cfg.ControllerUUID()),
		modelConfig,
	)
	charmStorageParams.Tags = resourceTags

	return params.OperatorProvisioningInfo{
		ImagePath:    imagePath,
		Version:      vers,
		APIAddresses: apiAddresses.Result,
		CharmStorage: charmStorageParams,
		Tags:         resourceTags,
	}, nil
}

// CharmStorageParams returns filesystem parameters needed
// to provision storage used for a charm operator or workload.
func CharmStorageParams(
	controllerUUID string,
	storageClassName string,
	modelCfg *config.Config,
	poolName string,
	poolManager poolmanager.PoolManager,
) (params.KubernetesFilesystemParams, error) {
	// The defaults here are for operator storage.
	// Workload storage will override these elsewhere.
	var size uint64 = 1024
	tags := tags.ResourceTags(
		names.NewModelTag(modelCfg.UUID()),
		names.NewControllerTag(controllerUUID),
		modelCfg,
	)

	result := params.KubernetesFilesystemParams{
		StorageName: "charm",
		Size:        size,
		Provider:    string(provider.K8s_ProviderType),
		Tags:        tags,
		Attributes:  make(map[string]interface{}),
	}

	// The storage key value from the model config might correspond
	// to a storage pool, unless there's been a specific storage pool
	// requested.
	// First, blank out the fallback pool name used in previous
	// versions of Juju.
	if poolName == string(provider.K8s_ProviderType) {
		poolName = ""
	}
	maybePoolName := poolName
	if maybePoolName == "" {
		maybePoolName = storageClassName
	}

	pool, err := poolManager.Get(maybePoolName)
	if err != nil && (!errors.IsNotFound(err) || poolName != "") {
		return params.KubernetesFilesystemParams{}, errors.Trace(err)
	}
	if err == nil {
		result.Provider = string(pool.Provider())
		result.Attributes = pool.Attrs()
	}
	if _, ok := result.Attributes[provider.StorageClass]; !ok {
		result.Attributes[provider.StorageClass] = storageClassName
	}
	return result, nil
}
