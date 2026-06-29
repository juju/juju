// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/docker"
	provisionertypes "github.com/juju/juju/internal/worker/caasapplicationprovisioner/types"
)

// ProvisionerApplicationService provides the application service methods
// needed by the provisioner facade shim.
type ProvisionerApplicationService interface {
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)
	IsCharmAvailable(ctx context.Context, locator applicationcharm.CharmLocator) (bool, error)
	GetApplicationUUIDByName(ctx context.Context, name string) (coreapplication.UUID, error)
	GetApplicationConstraints(ctx context.Context, id coreapplication.UUID) (constraints.Value, error)
	GetApplicationScale(ctx context.Context, appName string) (int, error)
	GetApplicationCharmOrigin(ctx context.Context, name string) (corecharm.Origin, error)
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.UUID) (int, error)
	GetApplicationTrustSetting(ctx context.Context, appName string) (bool, error)
	GetDeviceConstraints(ctx context.Context, name string) (map[string]devices.Constraints, error)
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
	WatchApplication(ctx context.Context, appName string) (watcher.NotifyWatcher, error)
}

// ProvisionerControllerConfigService provides controller configuration.
type ProvisionerControllerConfigService interface {
	ControllerConfig(ctx context.Context) (controller.Config, error)
	WatchControllerConfig(ctx context.Context) (watcher.StringsWatcher, error)
}

// ProvisionerControllerNodeService provides controller node information.
type ProvisionerControllerNodeService interface {
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
	WatchControllerAPIAddresses(ctx context.Context) (watcher.NotifyWatcher, error)
}

// ProvisionerModelConfigService provides model configuration.
type ProvisionerModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
	Watch(ctx context.Context) (watcher.StringsWatcher, error)
}

// ProvisionerModelInfoService provides model information and constraint
// resolution.
type ProvisionerModelInfoService interface {
	GetModelInfo(ctx context.Context) (coremodel.ModelInfo, error)
	ResolveConstraints(ctx context.Context, cons constraints.Value) (constraints.Value, error)
}

// ProvisionerRemovalService provides unit removal operations.
type ProvisionerRemovalService interface {
	MarkUnitAsDead(ctx context.Context, unitUUID coreunit.UUID) error
	RemoveUnit(ctx context.Context, unitUUID coreunit.UUID, destroyStorage bool, force bool, wait time.Duration) (removal.UUID, error)
}

// provisionerFacadeShim implements CAASProvisionerFacade using domain
// services instead of an API facade client.
type provisionerFacadeShim struct {
	appSvc        ProvisionerApplicationService
	ctrlConfigSvc ProvisionerControllerConfigService
	ctrlNodeSvc   ProvisionerControllerNodeService
	modelCfgSvc   ProvisionerModelConfigService
	modelInfoSvc  ProvisionerModelInfoService
	removalSvc    ProvisionerRemovalService
}

// ProvisioningInfo implements CAASProvisionerFacade by aggregating data
// from multiple domain services, matching the server-side
// provisioningInfo implementation.
func (s *provisionerFacadeShim) ProvisioningInfo(ctx context.Context, appName string) (provisionertypes.ProvisioningInfo, error) {
	locator, err := s.appSvc.GetCharmLocatorByApplicationName(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting charm locator for application %q", appName)
	}

	isAvailable, err := s.appSvc.IsCharmAvailable(ctx, locator)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "checking charm availability for %v", locator)
	} else if !isAvailable {
		return provisionertypes.ProvisioningInfo{}, errors.NotProvisionedf("charm for application %q", appName)
	}

	cfg, err := s.ctrlConfigSvc.ControllerConfig(ctx)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting controller config")
	}

	modelConfig, err := s.modelCfgSvc.ModelConfig(ctx)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting model config")
	}

	modelInfo, err := s.modelInfoSvc.GetModelInfo(ctx)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting model info")
	}

	deviceConstraints, err := s.appSvc.GetDeviceConstraints(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting device constraints")
	}
	var deviceParams []devices.KubernetesDeviceParams
	for _, d := range deviceConstraints {
		deviceParams = append(deviceParams, devices.KubernetesDeviceParams{
			Type:       d.Type,
			Count:      int64(d.Count),
			Attributes: d.Attributes,
		})
	}

	appID, err := s.appSvc.GetApplicationUUIDByName(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting application UUID for %q", appName)
	}

	cons, err := s.appSvc.GetApplicationConstraints(ctx, appID)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting application constraints for %q", appName)
	}
	mergedCons, err := s.modelInfoSvc.ResolveConstraints(ctx, cons)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "resolving constraints")
	}

	resourceTags := tags.ResourceTags(
		names.NewModelTag(modelInfo.UUID.String()),
		names.NewControllerTag(cfg.ControllerUUID()),
		modelConfig,
	)

	vers, ok := modelConfig.AgentVersion()
	if !ok {
		return provisionertypes.ProvisioningInfo{}, errors.NewNotValid(nil,
			"agent version is missing in model config",
		)
	}

	addrs, err := s.ctrlNodeSvc.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting API addresses")
	}

	caCert, _ := cfg.CACert()

	scale, err := s.appSvc.GetApplicationScale(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting application scale for %q", appName)
	}

	origin, err := s.appSvc.GetApplicationCharmOrigin(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting charm origin for %q", appName)
	}

	parsedBase, err := base.ParseBase(origin.Platform.OS, origin.Platform.Channel)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "parsing base from charm origin")
	}

	modelImage := cfg.CAASImageRepo() + "/" + podcfg.JujudOCIName
	modelImageRepo, err := podcfg.RecoverRepoFromOperatorPath(modelImage)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "recovering repo from operator path")
	}

	modelImagePath, err := podcfg.GetJujuOCIImagePathFromModelRepo(modelImageRepo, vers)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "getting OCI image path")
	}

	imageRepoDetails, err := docker.NewImageRepoDetails(modelImageRepo)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotate(err, "parsing image repo details")
	}

	charmModifiedVersion, err := s.appSvc.GetCharmModifiedVersion(ctx, appID)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting charm modified version for %q", appName)
	}

	trustSetting, err := s.appSvc.GetApplicationTrustSetting(ctx, appName)
	if err != nil {
		return provisionertypes.ProvisioningInfo{}, errors.Annotatef(err, "getting trust setting for %q", appName)
	}

	return provisionertypes.ProvisioningInfo{
		Version:              vers,
		APIAddresses:         addrs,
		CACert:               caCert,
		Tags:                 resourceTags,
		Devices:              deviceParams,
		Constraints:          mergedCons,
		Base:                 parsedBase,
		ImageDetails:         convertToDockerImageDetails(docker.ConvertToResourceImageDetails(imageRepoDetails), modelImagePath),
		CharmModifiedVersion: charmModifiedVersion,
		Trust:                trustSetting,
		Scale:                scale,
	}, nil
}

// FilesystemProvisioningInfo implements CAASProvisionerFacade. The
// server-side implementation is a stub that returns empty data; the worker
// already fetches filesystem data through StorageProvisioningService.
func (s *provisionerFacadeShim) FilesystemProvisioningInfo(_ context.Context, _ string) (provisionertypes.FilesystemProvisioningInfo, error) {
	return provisionertypes.FilesystemProvisioningInfo{}, nil
}

// RemoveUnit implements CAASProvisionerFacade by delegating to domain
// services, matching the server-side Remove implementation.
func (s *provisionerFacadeShim) RemoveUnit(ctx context.Context, unitName string) error {
	name, err := coreunit.NewName(unitName)
	if err != nil {
		return errors.NotValidf("unit name %q", unitName)
	}

	unitUUID, err := s.appSvc.GetUnitUUID(ctx, name)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "getting unit UUID for %q", unitName)
	}

	if err := s.removalSvc.MarkUnitAsDead(ctx, unitUUID); err != nil {
		return errors.Annotatef(err, "marking unit %q as dead", unitName)
	}

	_, err = s.removalSvc.RemoveUnit(ctx, unitUUID, false, false, 0)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	}
	return errors.Annotatef(err, "removing unit %q", unitName)
}

// WatchProvisioningInfo implements CAASProvisionerFacade by composing
// four watchers into a multi-watcher, matching the server-side
// watchProvisioningInfo implementation.
func (s *provisionerFacadeShim) WatchProvisioningInfo(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appWatcher, err := s.appSvc.WatchApplication(ctx, appName)
	if err != nil {
		return nil, errors.Annotatef(err, "watching application %q", appName)
	}

	ctrlConfigWatcher, err := s.ctrlConfigSvc.WatchControllerConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching controller config")
	}
	ctrlConfigNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(ctrlConfigWatcher)
	if err != nil {
		return nil, errors.Annotate(err, "creating controller config notify watcher")
	}

	ctrlAPIAddrWatcher, err := s.ctrlNodeSvc.WatchControllerAPIAddresses(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching controller API addresses")
	}

	modelCfgWatcher, err := s.modelCfgSvc.Watch(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "watching model config")
	}
	modelCfgNotifyWatcher, err := eventsource.NewStringsNotifyWatcher(modelCfgWatcher)
	if err != nil {
		return nil, errors.Annotate(err, "creating model config notify watcher")
	}

	return eventsource.NewMultiNotifyWatcher(ctx,
		appWatcher,
		ctrlConfigNotifyWatcher,
		ctrlAPIAddrWatcher,
		modelCfgNotifyWatcher,
	)
}

// DestroyUnits implements CAASProvisionerFacade by delegating to domain
// services, matching the server-side destroyUnit implementation.
func (s *provisionerFacadeShim) DestroyUnits(ctx context.Context, unitNames []string) error {
	for _, unitName := range unitNames {
		name, err := coreunit.NewName(unitName)
		if err != nil {
			return errors.NotValidf("unit name %q", unitName)
		}

		unitUUID, err := s.appSvc.GetUnitUUID(ctx, name)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "getting unit UUID for %q", unitName)
		}

		_, err = s.removalSvc.RemoveUnit(ctx, unitUUID, false, false, 0)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			continue
		} else if err != nil {
			return errors.Annotatef(err, "destroying unit %q", unitName)
		}
	}
	return nil
}

// convertToDockerImageDetails converts resource image repo details to
// DockerImageDetails with the given registry path.
func convertToDockerImageDetails(repoDetails resource.ImageRepoDetails, registryPath string) resource.DockerImageDetails {
	return resource.DockerImageDetails{
		RegistryPath:     registryPath,
		ImageRepoDetails: repoDetails,
	}
}
