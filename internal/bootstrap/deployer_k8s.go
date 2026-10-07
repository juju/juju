// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	jujuerrors "github.com/juju/errors"

	"github.com/juju/juju/caas"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	domainapplication "github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/errors"
)

// ServiceManager provides the API to manipulate services.
type ServiceManager interface {
	// GetService returns the service for the specified application.
	GetService(ctx context.Context, appName string, includeClusterIP bool) (*caas.Service, error)
}

// K8sDeployerConfig holds the configuration for a K8sDeployer.
type K8sDeployerConfig struct {
	BaseDeployerConfig
	ApplicationService K8sApplicationService
	ServiceManager     ServiceManager
	UnitPassword       string
	ControllerFQDN     string
}

// Validate validates the configuration.
func (c K8sDeployerConfig) Validate() error {
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

// K8sDeployer deploys the controller charm for K8s workloads.
type K8sDeployer struct {
	baseDeployer
	applicationService K8sApplicationService
	serviceManager     ServiceManager
	unitPassword       string
	controllerFQDN     string
}

var _ ControllerCharmDeployer = (*K8sDeployer)(nil)

// NewK8sDeployer returns a new ControllerCharmDeployer for K8s workloads.
func NewK8sDeployer(config K8sDeployerConfig) (*K8sDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	return &K8sDeployer{
		baseDeployer:       makeBaseDeployer(config.BaseDeployerConfig),
		applicationService: config.ApplicationService,
		serviceManager:     config.ServiceManager,
		unitPassword:       config.UnitPassword,
		controllerFQDN:     config.ControllerFQDN,
	}, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *K8sDeployer) ControllerCharmBase() (corebase.Base, error) {
	return version.DefaultSupportedLTSBase(), nil
}

// EnsureControllerApplication creates the K8s controller application if needed
// and completes its unit, service and exposure setup, even if it already exists.
func (b *K8sDeployer) EnsureControllerApplication(ctx context.Context, info DeployCharmInfo) error {
	if err := info.Validate(); err != nil {
		return errors.Capture(err)
	}

	origin := *info.Origin
	controllerConstraints, err := normalizeControllerConstraints(b.constraints, origin.Platform.Architecture)
	if err != nil {
		return err
	}

	cfg, err := b.createCharmSettings()
	if err != nil {
		return errors.Errorf("creating charm settings: %w", err)
	}

	downloadInfo, err := b.controllerDownloadInfo(info.URL.Schema, info.DownloadInfo)
	if err != nil {
		return errors.Errorf("creating download info: %w", err)
	}

	unitArg := applicationservice.AddUnitArg{}

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
				Since:  new(b.clock.Now()),
			},
			Constraints:  controllerConstraints,
			IsController: true,
		},
		unitArg,
	); err != nil && !errors.Is(err, applicationerrors.ApplicationAlreadyExists) {
		return errors.Errorf("creating K8s controller application: %w", err)
	}

	if err := b.completeControllerApplication(ctx); err != nil {
		return errors.Errorf("completing K8s controller application: %w", err)
	}

	return b.ensureControllerApplicationExposed(ctx)
}

// normalizeControllerConstraints follows application-domain architecture
// normalization: an explicit constraint must match the charm platform, and an
// absent constraint uses the selected charm architecture.
func normalizeControllerConstraints(cons constraints.Value, charmArch string) (constraints.Value, error) {
	if cons.HasArch() && charmArch != "" && *cons.Arch != charmArch {
		return constraints.Value{}, errors.Errorf("arch %q in constraints does not match controller charm platform arch %q",
			*cons.Arch, charmArch)
	}
	if !cons.HasArch() {
		cons.Arch = &charmArch
	}
	return cons, nil
}

func (d *K8sDeployer) completeControllerApplication(ctx context.Context) error {
	// We can deduce that the unit name must be controller/0 since we're
	// currently bootstrapping the controller, so this unit is the first unit
	// to be created.
	controllerUnit, err := coreunit.NewNameFromParts(bootstrap.ControllerApplicationName, 0)
	if err != nil {
		return errors.Errorf("creating unit name %q: %w", bootstrap.ControllerApplicationName, err)
	}

	providerID := controllerProviderID(controllerUnit)
	updateParams := applicationservice.UpdateCAASUnitParams{
		ProviderID: &providerID,
	}
	// Persist the controller pod's stable FQDN as the unit's network identity,
	// in the same flow that upserts the k8s pod (provider id).
	if d.controllerFQDN != "" {
		fqdn := d.controllerFQDN
		updateParams.FQDN = &fqdn
	}
	if err := d.applicationService.UpdateCAASUnit(ctx, controllerUnit, updateParams); err != nil {
		return errors.Errorf("updating controller unit: %w", err)
	}
	if err := d.passwordService.SetUnitPassword(ctx, controllerUnit, d.unitPassword); err != nil {
		return errors.Errorf("setting controller unit password: %w", err)
	}

	// Persist the API Service's actual UID and addresses, separately from the
	// controller pod's provider ID and stable Dqlite identity above.
	svc, err := d.serviceManager.GetService(ctx, bootstrap.ControllerApplicationName, true)
	if err != nil {
		return errors.Errorf("getting controller API service: %w", err)
	}
	if svc == nil || svc.Id == "" {
		return errors.New("controller API service has not been provisioned")
	}
	err = d.applicationService.UpdateK8sService(ctx, bootstrap.ControllerApplicationName, svc.Id, svc.Addresses)
	if err != nil {
		return errors.Capture(err)
	}
	d.logger.Debugf(ctx, "recorded controller API service %q with addresses %v", svc.Id, svc.Addresses)

	return nil
}

func controllerProviderID(name coreunit.Name) string {
	return fmt.Sprintf("controller-%d", name.Number())
}
