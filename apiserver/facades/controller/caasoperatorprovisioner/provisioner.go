// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorprovisioner

import (
	"fmt"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	charmscommon "github.com/juju/juju/apiserver/common/charms"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/cloudconfig/podcfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/docker"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/pki"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
)

var logger = loggo.GetLogger("juju.apiserver.caasoperatorprovisioner")

type APIGroup struct {
	charmInfoAPI    *charmscommon.CharmInfoAPI
	appCharmInfoAPI *charmscommon.ApplicationCharmInfoAPI
	*API
}

// CharmInfo returns information about the requested charm.
func (a *APIGroup) CharmInfo(args params.CharmURL) (params.Charm, error) {
	return a.charmInfoAPI.CharmInfo(args)
}

// ApplicationCharmInfo returns information about an application's charm.
func (a *APIGroup) ApplicationCharmInfo(args params.Entity) (params.Charm, error) {
	return a.appCharmInfoAPI.ApplicationCharmInfo(args)
}

// TODO (manadart 2020-10-21): Remove the ModelUUID method
// from the next version of this facade.

// API is CAAS operator provisioner API facade.
type API struct {
	*common.PasswordChanger
	*common.LifeGetter
	*common.APIAddresser

	auth      facade.Authorizer
	resources facade.Resources

	ctrlState          CAASControllerState
	state              CAASOperatorProvisionerState
	storagePoolManager poolmanager.PoolManager
	registry           storage.ProviderRegistry
}

// NewCAASOperatorProvisionerAPI returns a new CAAS operator provisioner API facade.
func NewCAASOperatorProvisionerAPI(
	resources facade.Resources,
	authorizer facade.Authorizer,
	ctrlSt CAASControllerState,
	st CAASOperatorProvisionerState,
	storagePoolManager poolmanager.PoolManager,
	registry storage.ProviderRegistry,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		PasswordChanger:    common.NewPasswordChanger(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		LifeGetter:         common.NewLifeGetter(st, common.AuthFuncForTagKind(names.ApplicationTagKind)),
		APIAddresser:       common.NewAPIAddresser(ctrlSt, resources),
		auth:               authorizer,
		resources:          resources,
		ctrlState:          ctrlSt,
		state:              st,
		storagePoolManager: storagePoolManager,
		registry:           registry,
	}, nil
}

// WatchApplications starts a StringsWatcher to watch applications
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
	logger.Infof("alvin OperatorProvisingInfo called")
	var result params.OperatorProvisioningInfoResults
	cfg, err := a.ctrlState.ControllerConfig()
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

	imageRepo, exists := modelConfig.CAASImageRepo()
	if !exists {
		imageRepo = controller.CAASImageRepo
		logger.Infof("alvin CAASImageRepo: %q", imageRepo)

		if imageRepo == "" {
			imageRepo = podcfg.JujudOCINamespace
		}
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(imageRepo)
	if err != nil {
		return result, errors.Annotatef(err, "parsing %s", controller.CAASImageRepo)
	}
	logger.Infof("alvin2 OperatorProvisioningInfo model: %#v", model)
	logger.Infof("alvin2 OperatorProvisioningInfo modelConfig: %#v", modelConfig)
	registryPath, err := podcfg.GetJujuOCIImagePath(cfg, modelConfig, vers)
	if err != nil {
		return result, errors.Trace(err)
	}
	imageInfo := params.NewDockerImageInfo(imageRepoDetails, registryPath)
	logger.Tracef("image info %v", imageInfo)

	// PodSpec charms now use focal as the operator base until PodSpec is removed.
	baseRegistryPath, err := podcfg.ImageForBase(imageRepoDetails.Repository, charm.Base{
		Name:    "ubuntu",
		Channel: charm.Channel{Track: "20.04", Risk: charm.Stable},
	})
	if err != nil {
		return result, errors.Trace(err)
	}
	baseImageInfo := params.NewDockerImageInfo(imageRepoDetails, baseRegistryPath)
	logger.Tracef("base image info %v", baseImageInfo)

	apiAddresses, err := a.APIAddresses()
	if err == nil && apiAddresses.Error != nil {
		err = apiAddresses.Error
	}
	if err != nil {
		return result, errors.Annotatef(err, "getting api addresses")
	}

	oneProvisioningInfo := func(storageRequired bool) params.OperatorProvisioningInfo {
		var charmStorageParams *params.KubernetesFilesystemParams
		storageClassName, _ := modelConfig.AllAttrs()[k8sconstants.OperatorStorageKey].(string)
		if storageRequired {
			if storageClassName == "" {
				return params.OperatorProvisioningInfo{
					Error: apiservererrors.ServerError(errors.New("no operator storage defined")),
				}
			} else {
				charmStorageParams, err = CharmStorageParams(cfg.ControllerUUID(), storageClassName, modelConfig, "", a.storagePoolManager, a.registry)
				if err != nil {
					return params.OperatorProvisioningInfo{
						Error: apiservererrors.ServerError(errors.Annotatef(err, "getting operator storage parameters")),
					}
				}
				charmStorageParams.Tags = resourceTags
			}
		}
		return params.OperatorProvisioningInfo{
			ImageDetails:     imageInfo,
			BaseImageDetails: baseImageInfo,
			Version:          vers,
			APIAddresses:     apiAddresses.Result,
			CharmStorage:     charmStorageParams,
			Tags:             resourceTags,
		}
	}
	result.Results = make([]params.OperatorProvisioningInfo, len(args.Entities))
	for i, entity := range args.Entities {
		appName, err := names.ParseApplicationTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		app, err := a.state.Application(appName.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		ch, _, err := app.Charm()
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		needStorage := provider.RequireOperatorStorage(ch)
		logger.Debugf("application %s has min-juju-version=%v, so charm storage is %v",
			appName.String(), ch.Meta().MinJujuVersion, needStorage)
		result.Results[i] = oneProvisioningInfo(needStorage)
	}
	return result, nil
}

// IssueOperatorCertificate issues an x509 certificate for use by the specified application operator.
func (a *API) IssueOperatorCertificate(args params.Entities) (params.IssueOperatorCertificateResults, error) {
	cfg, err := a.ctrlState.ControllerConfig()
	if err != nil {
		return params.IssueOperatorCertificateResults{}, errors.Trace(err)
	}
	caCert, _ := cfg.CACert()

	si, err := a.ctrlState.StateServingInfo()
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
				Error: apiservererrors.ServerError(err),
			}
			continue
		}

		leaf, err := authority.LeafRequestForGroup(appTag.Name).
			AddDNSNames(appTag.Name).
			Commit()

		if err != nil {
			res.Results[i] = params.IssueOperatorCertificateResult{
				Error: apiservererrors.ServerError(err),
			}
			continue
		}

		cert, privateKey, err := leaf.ToPemParts()
		if err != nil {
			res.Results[i] = params.IssueOperatorCertificateResult{
				Error: apiservererrors.ServerError(err),
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

// ModelUUID returns the model UUID that this facade is used to operate.
// It is implemented here directly as a result of removing it from
// embedded APIAddresser *without* bumping the facade version.
// It should be blanked when this facade version is next incremented.
func (a *API) ModelUUID() params.StringResult {
	m, err := a.state.Model()
	if err != nil {
		return params.StringResult{Error: apiservererrors.ServerError(err)}
	}
	return params.StringResult{Result: m.UUID()}
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
		Provider:    string(k8sconstants.StorageProviderType),
		Tags:        tags,
		Attributes:  make(map[string]interface{}),
	}

	// The storage key value from the model config might correspond
	// to a storage pool, unless there's been a specific storage pool
	// requested.
	// First, blank out the fallback pool name used in previous
	// versions of Juju.
	if poolName == string(k8sconstants.StorageProviderType) {
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
	if _, ok := result.Attributes[k8sconstants.StorageClass]; !ok && result.Provider == string(k8sconstants.StorageProviderType) {
		result.Attributes[k8sconstants.StorageClass] = storageClassName
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
