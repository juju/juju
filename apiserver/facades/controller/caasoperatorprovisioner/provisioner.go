// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.apiserver.caasoperatorprovisioner")

type API struct {
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser

	auth      facade.Authorizer
	resources facade.Resources

	state              CAASOperatorProvisionerState
	storagePoolManager poolmanager.PoolManager
	registry           storage.ProviderRegistry
}

// NewStateCAASOperatorProvisionerAPI provides the signature required for facade registration.
func NewStateCAASOperatorProvisionerAPI(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	registry := stateenvirons.NewStorageProviderRegistry(broker)
	pm := poolmanager.New(state.NewStateSettings(ctx.State()), registry)

	return NewCAASOperatorProvisionerAPI(resources, authorizer, stateShim{ctx.State()}, pm, registry)
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	st CAASOperatorProvisionerState,
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
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
		registry:           registry,
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
func (a *API) OperatorProvisioningInfo(args params.Entities) (params.OperatorProvisioningInfoResults, error) {
	var result params.OperatorProvisioningInfoResults
	cfg, err := a.state.ControllerConfig()
	if err != nil {
		return result, err
	}

	model, err := a.state.Model()
	if err != nil {
		return result, errors.Trace(err)
	}
	modelConfig, err := model.ModelConfig()
	if err != nil {
		return result, errors.Trace(err)
	}

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return result, errors.NewNotValid(nil,
			fmt.Sprintf("agent version is missing in model config %q", modelConfig.Name()),
		)
	}

	resourceTags := tags.ResourceTags(
		names.NewModelTag(model.UUID()),
		names.NewControllerTag(cfg.ControllerUUID()),
		modelConfig,
	)

	imagePath := podcfg.GetJujuOCIImagePath(cfg, vers.ToPatch(), version.OfficialBuild)
	apiAddresses, err := a.APIAddresses()
	if err == nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return result, errors.Annotatef(err, "getting api addresses")
	}

	oneProvisioningInfo := func(storageRequired bool) params.OperatorProvisioningInfo {
		var charmStorageParams *params.KubernetesFilesystemParams
		storageClassName, _ := modelConfig.AllAttrs()[provider.OperatorStorageKey].(string)
		if storageRequired {
			if storageClassName == "" {
				return params.OperatorProvisioningInfo{
					Error: common.ServerError(errors.New("no operator storage defined")),
				}
			} else {
				charmStorageParams, err = CharmStorageParams(cfg.ControllerUUID(), storageClassName, modelConfig, "", a.storagePoolManager, a.registry)
				if err != nil {
					return params.OperatorProvisioningInfo{
						Error: common.ServerError(errors.Annotatef(err, "getting operator storage parameters")),
					}
				}
				charmStorageParams.Tags = resourceTags
			}
		}
		return params.OperatorProvisioningInfo{
			ImagePath:    imagePath,
			Version:      vers,
			APIAddresses: apiAddresses.Result,
			CharmStorage: charmStorageParams,
			Tags:         resourceTags,
		}
	}
	result.Results = make([]params.OperatorProvisioningInfo, len(args.Entities))
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		app, err := a.state.Application(appName.Id())
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		ch, _, err := app.Charm()
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		needStorage := provider.RequireOperatorStorage(ch.Meta().MinJujuVersion)
		logger.Debugf("application %s has min-juju-version=%v, so charm storage is %v",
			appName.String(), ch.Meta().MinJujuVersion, needStorage)
		result.Results[i] = oneProvisioningInfo(needStorage)
	}
	return result, nil
}

// IssueOperatorCertificate issues an x509 certificate for use by the specified application operator.
func (a *API) IssueOperatorCertificate(args params.Entities) (params.IssueOperatorCertificateResults, error) {
	cfg, err := a.state.ControllerConfig()
	if err != nil {
		return params.IssueOperatorCertificateResults{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()

	si, err := a.state.StateServingInfo()
	if err != nil {
		return params.IssueOperatorCertificateResults{}, errors.Trace(err)
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey([]byte(caCert),
		[]byte(si.CAPrivateKey))
	if err != nil {
		return params.IssueOperatorCertificateResults{}, errors.Trace(err)
	}

	res := params.IssueOperatorCertificateResults{
		Results: make([]params.IssueOperatorCertificateResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		appTag, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			res.Results[i] = params.IssueOperatorCertificateResult{
				Error: common.ServerError(err),
			}
			continue
		}

		leaf, err := authority.LeafRequestForGroup(appTag.Name).
			AddDNSNames(appTag.Name).
			Commit()

		if err != nil {
			res.Results[i] = params.IssueOperatorCertificateResult{
				Error: common.ServerError(err),
			}
			continue
		}

		cert, privateKey, err := leaf.ToPemParts()
		if err != nil {
			res.Results[i] = params.IssueOperatorCertificateResult{
				Error: common.ServerError(err),
			}
			continue
		}

		res.Results[i] = params.IssueOperatorCertificateResult{
			CACert:     caCert,
			Cert:       string(cert),
			PrivateKey: string(privateKey),
		}
	}

	return res, nil
}

// CharmStorageParams returns filesystem parameters needed
// to provision storage used for a charm operator or workload.
func CharmStorageParams(
	controllerUUID string,
	storageClassName string,
	modelCfg *config.Config,
	poolName string,
	poolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (*params.KubernetesFilesystemParams, error) {
	// The defaults here are for operator storage.
	// Workload storage will override these elsewhere.
	var size uint64 = 1024
	tags := tags.ResourceTags(
		names.NewModelTag(modelCfg.UUID()),
		names.NewControllerTag(controllerUUID),
		modelCfg,
	)

	result := &params.KubernetesFilesystemParams{
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

	providerType, attrs, err := poolStorageProvider(poolManager, registry, maybePoolName)
	if err != nil && (!errors.IsNotFound(err) || poolName != "") {
		return nil, errors.Trace(err)
	}
	if err == nil {
		result.Provider = string(providerType)
		if len(attrs) > 0 {
			result.Attributes = attrs
		}
	}
	if _, ok := result.Attributes[provider.StorageClass]; !ok && result.Provider == string(provider.K8s_ProviderType) {
		result.Attributes[provider.StorageClass] = storageClassName
	}
	return result, nil
}

func poolStorageProvider(poolManager poolmanager.PoolManager, registry storage.ProviderRegistry, poolName string) (storage.ProviderType, map[string]interface{}, error) {
	pool, err := poolManager.Get(poolName)
	if errors.IsNotFound(err) {
		// If there's no pool called poolName, maybe a provider type
		// has been specified directly.
		providerType := storage.ProviderType(poolName)
		provider, err1 := registry.StorageProvider(providerType)
		if err1 != nil {
			// The name can't be resolved as a storage provider type,
			// so return the original "pool not found" error.
			return "", nil, errors.Trace(err)
		}
		if !provider.Supports(storage.StorageKindFilesystem) {
			return "", nil, errors.NotValidf("storage provider %q", providerType)
		}
		return providerType, nil, nil
	} else if err != nil {
		return "", nil, errors.Trace(err)
	}
	providerType := pool.Provider()
	return providerType, pool.Attrs(), nil
}
