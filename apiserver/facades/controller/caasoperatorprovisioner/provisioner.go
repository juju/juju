// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/storagecommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/version"
)

type API struct {
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser

	auth      facade.Authorizer
	resources facade.Resources

	state                   CAASOperatorProvisionerState
	storageProviderRegistry storage.ProviderRegistry
	storagePoolManager      poolmanager.PoolManager
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

	return NewCAASOperatorProvisionerAPI(resources, authorizer, stateShim{ctx.State()}, registry, pm)
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorProvisionerState,
	storageProviderRegistry storage.ProviderRegistry,
	storagePoolManager poolmanager.PoolManager,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, common.ErrPerm
	}
	return &API{
		PasswordChanger:         common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		LifeGetter:              common.NewLifeGetter(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		APIAddresser:            common.NewAPIAddresser(st, resources),
		auth:                    authorizer,
		resources:               resources,
		state:                   st,
		storageProviderRegistry: storageProviderRegistry,
		storagePoolManager:      storagePoolManager,
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

	imagePath := cfg.CAASOperatorImagePath()
	vers := version.Current
	vers.Build = 0
	if imagePath == "" {
		imagePath = fmt.Sprintf("%s/caas-jujud-operator:%s", "jujusolutions", vers.String())
	}
	charmStorageParams, err := charmStorageParams(a.storagePoolManager, a.storageProviderRegistry)
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

	model, err := a.state.Model()
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return params.OperatorProvisioningInfo{}, errors.Trace(err)
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

func charmStorageParams(
	poolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (params.KubernetesFilesystemParams, error) {

	// TODO(caas) - make these configurable via model config
	var pool = caas.OperatorStoragePoolName
	var size uint64 = 1024

	result := params.KubernetesFilesystemParams{
		Size:     size,
		Provider: string(provider.K8s_ProviderType),
	}

	providerType, cfg, err := storagecommon.StoragePoolConfig(pool, poolManager, registry)
	if err != nil && !errors.IsNotFound(err) {
		return params.KubernetesFilesystemParams{}, errors.Trace(err)
	}
	if err == nil {
		result.Provider = string(providerType)
		result.Attributes = cfg.Attrs()
	}
	return result, nil
}
