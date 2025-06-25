// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	coreos "github.com/juju/juju/core/os"
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
}

// Validate validates the configuration.
func (c IAASDeployerConfig) Validate() error {
	if err := c.BaseDeployerConfig.Validate(); err != nil {
		return errors.Capture(err)
	}
	if c.ApplicationService == nil {
		return jujuerrors.NotValidf("ApplicationService")
	}
	return nil
}

// IAASDeployer is the interface that is used to deploy the controller charm
// for IAAS workloads.
type IAASDeployer struct {
	baseDeployer
	applicationService IAASApplicationService
}

// NewIAASDeployer returns a new ControllerCharmDeployer for IAAS workloads.
func NewIAASDeployer(config IAASDeployerConfig) (*IAASDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	return &IAASDeployer{
		baseDeployer:       makeBaseDeployer(config.BaseDeployerConfig),
		applicationService: config.ApplicationService,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
// This address is retrieved from the database, since the machine information
// is available already.
func (d *IAASDeployer) ControllerAddress(ctx context.Context) (string, error) {
	// TODO (stickupkid): We need to ask the provider for the instance address.
	var controllerAddress string
	d.logger.Debugf(ctx, "IAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *IAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	base, err := coreos.HostBase()
	if err != nil {
		return corebase.Base{}, errors.Errorf("getting host base: %w", err)
	}

	return corebase.ParseBase(base.OS, base.Channel.String())
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
			Constraints:  b.constraints,
			IsController: true,
		},
		applicationservice.AddIAASUnitArg{
			Nonce: ptr(agent.BootstrapNonce),
		},
	); err != nil {
		return errors.Errorf("creating IAAS controller application: %w", err)
	}

	return nil
}
