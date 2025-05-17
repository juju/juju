// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	jujuerrors "github.com/juju/errors"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// CloudService is the interface that is used to get the cloud service
// for the controller.
type CloudService interface {
	Addresses() network.SpaceAddresses
}

// CloudServiceGetter is the interface that is used to get the cloud service
// for the controller.
type CloudServiceGetter interface {
	CloudService(string) (CloudService, error)
}

// CAASDeployerConfig holds the configuration for a CAASDeployer.
type CAASDeployerConfig struct {
	BaseDeployerConfig
	ApplicationService CAASApplicationService
	CloudServiceGetter CloudServiceGetter
	UnitPassword       string
}

// Validate validates the configuration.
func (c CAASDeployerConfig) Validate() error {
	if err := c.BaseDeployerConfig.Validate(); err != nil {
		return errors.Capture(err)
	}
	if c.ApplicationService == nil {
		return jujuerrors.NotValidf("ApplicationService")
	}
	if c.CloudServiceGetter == nil {
		return jujuerrors.NotValidf("CloudServiceGetter")
	}
	return nil
}

// CAASDeployer is the interface that is used to deploy the controller charm
// for CAAS workloads.
type CAASDeployer struct {
	baseDeployer
	applicationService CAASApplicationService
	cloudServiceGetter CloudServiceGetter
	unitPassword       string
}

// NewCAASDeployer returns a new ControllerCharmDeployer for CAAS workloads.
func NewCAASDeployer(config CAASDeployerConfig) (*CAASDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return &CAASDeployer{
		baseDeployer:       makeBaseDeployer(config.BaseDeployerConfig),
		applicationService: config.ApplicationService,
		cloudServiceGetter: config.CloudServiceGetter,
		unitPassword:       config.UnitPassword,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *CAASDeployer) ControllerAddress(ctx context.Context) (string, error) {
	s, err := d.cloudServiceGetter.CloudService(d.controllerConfig.ControllerUUID())
	if err != nil {
		return "", errors.Capture(err)
	}
	hp := network.SpaceAddressesWithPort(s.Addresses(), 0)
	addr := hp.AllMatchingScope(network.ScopeMatchCloudLocal)

	var controllerAddress string
	if len(addr) > 0 {
		controllerAddress = addr[0]
	}
	d.logger.Debugf(ctx, "CAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *CAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	return version.DefaultSupportedLTSBase(), nil
}

// AddCAASControllerApplication adds the CAAS controller application.
func (b *CAASDeployer) AddCAASControllerApplication(ctx context.Context, info DeployCharmInfo, controllerAddress string) (coreunit.Name, error) {
	if err := info.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	origin := *info.Origin

	cfg, err := b.createCharmSettings(controllerAddress)
	if err != nil {
		return "", errors.Errorf("creating charm settings: %w", err)
	}

	// DownloadInfo is not required for local charms, so we only set it if
	// it's not nil.
	if info.URL.Schema == charm.Local.String() && info.DownloadInfo != nil {
		return "", errors.New("download info should not be set for local charms")
	}

	var downloadInfo *applicationcharm.DownloadInfo
	if info.DownloadInfo != nil {
		downloadInfo = &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceBootstrap,
			CharmhubIdentifier: info.DownloadInfo.CharmhubIdentifier,
			DownloadURL:        info.DownloadInfo.DownloadURL,
			DownloadSize:       info.DownloadInfo.DownloadSize,
		}
	}
	if _, err := b.applicationService.CreateCAASApplication(ctx,
		bootstrap.ControllerApplicationName,
		info.Charm,
		origin,
		applicationservice.AddApplicationArgs{
			ReferenceName:        bootstrap.ControllerCharmName,
			CharmStoragePath:     info.ArchivePath,
			CharmObjectStoreUUID: info.ObjectStoreUUID,
			DownloadInfo:         downloadInfo,
			ApplicationConfig:    cfg,
			ApplicationSettings: domainapplication.ApplicationSettings{
				Trust: true,
			},
			ApplicationStatus: &status.StatusInfo{
				Status: status.Unset,
				Since:  ptr(b.clock.Now()),
			},
		},
		applicationservice.AddUnitArg{},
	); err != nil {
		return "", errors.Errorf("creating CAAS controller application: %w", err)
	}

	// We can deduce that the unit name must be controller/0 since we're
	// currently bootstrapping the controller, so this unit is the first unit
	// to be created.
	unitName, err := coreunit.NewNameFromParts(bootstrap.ControllerApplicationName, 0)
	if err != nil {
		return "", errors.Errorf("creating unit name %q: %w", bootstrap.ControllerApplicationName, err)
	}
	return unitName, nil
}

// CompleteProcess is called when the bootstrap process is complete.
func (d *CAASDeployer) CompleteProcess(ctx context.Context, controllerUnit coreunit.Name) error {
	providerID := controllerProviderID(controllerUnit)
	if err := d.applicationService.UpdateCAASUnit(ctx, controllerUnit, applicationservice.UpdateCAASUnitParams{
		ProviderID: &providerID,
	}); err != nil {
		return errors.Errorf("updating controller unit: %w", err)
	}
	if err := d.passwordService.SetUnitPassword(ctx, controllerUnit, d.unitPassword); err != nil {
		return errors.Errorf("setting controller unit password: %w", err)
	}
	return nil
}

func controllerProviderID(name coreunit.Name) string {
	return fmt.Sprintf("controller-%d", name.Number())
}
