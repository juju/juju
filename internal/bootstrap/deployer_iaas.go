// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// IAASDeployerConfig holds the configuration for a IAASDeployer.
type IAASDeployerConfig struct {
	BaseDeployerConfig
	ApplicationService IAASApplicationService
	MachineGetter      MachineGetter
}

// Validate validates the configuration.
func (c IAASDeployerConfig) Validate() error {
	if err := c.BaseDeployerConfig.Validate(); err != nil {
		return errors.Capture(err)
	}
	if c.ApplicationService == nil {
		return jujuerrors.NotValidf("ApplicationService")
	}
	if c.MachineGetter == nil {
		return jujuerrors.NotValidf("MachineGetter")
	}
	return nil
}

// IAASDeployer is the interface that is used to deploy the controller charm
// for IAAS workloads.
type IAASDeployer struct {
	baseDeployer
	applicationService IAASApplicationService
	machineGetter      MachineGetter
}

// NewIAASDeployer returns a new ControllerCharmDeployer for IAAS workloads.
func NewIAASDeployer(config IAASDeployerConfig) (*IAASDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return &IAASDeployer{
		baseDeployer:       makeBaseDeployer(config.BaseDeployerConfig),
		applicationService: config.ApplicationService,
		machineGetter:      config.MachineGetter,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *IAASDeployer) ControllerAddress(ctx context.Context) (string, error) {
	m, err := d.machineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return "", errors.Capture(err)
	}

	pa, err := m.PublicAddress()
	if err != nil && !network.IsNoAddressError(err) {
		return "", errors.Capture(err)
	}
	var controllerAddress string
	if err == nil {
		controllerAddress = pa.Value
	}
	d.logger.Debugf(ctx, "IAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *IAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	m, err := d.machineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return corebase.Base{}, errors.Capture(err)
	}

	machineBase := m.Base()
	return corebase.ParseBase(machineBase.OS, machineBase.Channel)
}

// AddIAASControllerApplication adds the IAAS controller application.
func (b *IAASDeployer) AddIAASControllerApplication(ctx context.Context, info DeployCharmInfo, controllerAddress string) (coreunit.Name, error) {
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
	if _, err := b.applicationService.CreateIAASApplication(ctx,
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
		return "", errors.Errorf("creating IAAS controller application: %w", err)
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
func (d *IAASDeployer) CompleteProcess(ctx context.Context, controllerUnit coreunit.Name) error {
	return nil
}
