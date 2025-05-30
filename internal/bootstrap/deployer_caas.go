// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/caas"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	domainapplication "github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
)

// ServiceManager provides the API to manipulate services.
type ServiceManager interface {
	// GetService returns the service for the specified application.
	GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error)
}

// CAASDeployerConfig holds the configuration for a CAASDeployer.
type CAASDeployerConfig struct {
	BaseDeployerConfig
	ApplicationService CAASApplicationService
	ServiceManager     ServiceManager
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
	if c.ServiceManager == nil {
		return jujuerrors.NotValidf("ServiceManager")
	}
	return nil
}

// CAASDeployer is the interface that is used to deploy the controller charm
// for CAAS workloads.
type CAASDeployer struct {
	baseDeployer
	applicationService CAASApplicationService
	serviceManager     ServiceManager
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
		serviceManager:     config.ServiceManager,
		unitPassword:       config.UnitPassword,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *CAASDeployer) ControllerAddress(ctx context.Context) (string, error) {
	addrs, err := d.getK8sServiceAddresses(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var controllerAddress string
	if addr, ok := addrs.OneMatchingScope(network.ScopeMatchCloudLocal); ok {
		controllerAddress = addr.Value
	}

	d.logger.Debugf(ctx, "CAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// getK8sServiceAddresses returns the addresses of the k8s service from the k8s
// broker.
func (d *CAASDeployer) getK8sServiceAddresses(ctx context.Context) (network.ProviderAddresses, error) {
	// Retrieve the k8s service from the k8s broker.
	svc, err := d.serviceManager.GetService(ctx, k8sconstants.JujuControllerStackName, true)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if len(svc.Addresses) == 0 {
		// this should never happen because we have already checked in k8s controller bootstrap stacker.
		return nil, jujuerrors.NotProvisionedf("k8s controller service %q address", svc.Id)
	}

	return svc.Addresses, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *CAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	return version.DefaultSupportedLTSBase(), nil
}

// AddCAASControllerApplication adds the CAAS controller application.
func (b *CAASDeployer) AddCAASControllerApplication(ctx context.Context, info DeployCharmInfo, controllerAddress string) error {
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
		return errors.Errorf("creating CAAS controller application: %w", err)
	}

	return nil
}

// CompleteCAASProcess is called when the bootstrap process is complete.
func (d *CAASDeployer) CompleteCAASProcess(ctx context.Context) error {
	// We can deduce that the unit name must be controller/0 since we're
	// currently bootstrapping the controller, so this unit is the first unit
	// to be created.
	controllerUnit, err := coreunit.NewNameFromParts(bootstrap.ControllerApplicationName, 0)
	if err != nil {
		return errors.Errorf("creating unit name %q: %w", bootstrap.ControllerApplicationName, err)
	}

	providerID := controllerProviderID(controllerUnit)
	if err := d.applicationService.UpdateCAASUnit(ctx, controllerUnit, applicationservice.UpdateCAASUnitParams{
		ProviderID: &providerID,
	}); err != nil {
		return errors.Errorf("updating controller unit: %w", err)
	}
	if err := d.passwordService.SetUnitPassword(ctx, controllerUnit, d.unitPassword); err != nil {
		return errors.Errorf("setting controller unit password: %w", err)
	}

	// Insert the k8s service with its addresses.
	addrs, err := d.getK8sServiceAddresses(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	d.logger.Debugf(ctx, "creating cloud service for k8s controller %q", controllerProviderID(controllerUnit))
	err = d.applicationService.UpdateCloudService(ctx, bootstrap.ControllerApplicationName, controllerProviderID(controllerUnit), addrs)
	if err != nil {
		return errors.Capture(err)
	}
	d.logger.Debugf(ctx, "created cloud service with addresses %v for controller", addrs)

	return nil
}

func controllerProviderID(name coreunit.Name) string {
	return fmt.Sprintf("controller-%d", name.Number())
}
