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
	domainapplication "github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
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
func (b *IAASDeployer) AddIAASControllerApplication(ctx context.Context, info DeployCharmInfo, controllerAddress string) error {
	if err := info.Validate(); err != nil {
		return errors.Capture(err)
	}

	origin := *info.Origin

	cfg, err := b.createCharmSettings(controllerAddress)
	if err != nil {
		return errors.Errorf("creating charm settings: %w", err)
	}

	downloadInfo, err := b.controllerDownloadInfo(info.URL.Schema, info.DownloadInfo)
	if err != nil {
		return errors.Errorf("creating download info: %w", err)
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
		return errors.Errorf("creating IAAS controller application: %w", err)
	}

	return nil
}
