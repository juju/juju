// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/version"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/state"
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

// OperationApplier is the interface that is used to apply operations.
type OperationApplier interface {
	// ApplyOperation applies the given operation.
	ApplyOperation(*state.UpdateUnitOperation) error
}

// CAASDeployerConfig holds the configuration for a CAASDeployer.
type CAASDeployerConfig struct {
	BaseDeployerConfig
	CloudServiceGetter CloudServiceGetter
	OperationApplier   OperationApplier
	UnitPassword       string
}

// Validate validates the configuration.
func (c CAASDeployerConfig) Validate() error {
	if err := c.BaseDeployerConfig.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.CloudServiceGetter == nil {
		return errors.NotValidf("CloudServiceGetter")
	}
	if c.OperationApplier == nil {
		return errors.NotValidf("OperationApplier")
	}
	return nil
}

// CAASDeployer is the interface that is used to deploy the controller charm
// for CAAS workloads.
type CAASDeployer struct {
	baseDeployer
	cloudServiceGetter CloudServiceGetter
	operationApplier   OperationApplier
	unitPassword       string
}

// NewCAASDeployer returns a new ControllerCharmDeployer for CAAS workloads.
func NewCAASDeployer(config CAASDeployerConfig) (*CAASDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &CAASDeployer{
		baseDeployer:       makeBaseDeployer(config.BaseDeployerConfig),
		cloudServiceGetter: config.CloudServiceGetter,
		operationApplier:   config.OperationApplier,
		unitPassword:       config.UnitPassword,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *CAASDeployer) ControllerAddress(context.Context) (string, error) {
	s, err := d.cloudServiceGetter.CloudService(d.controllerConfig.ControllerUUID())
	if err != nil {
		return "", errors.Trace(err)
	}
	hp := network.SpaceAddressesWithPort(s.Addresses(), 0)
	addr := hp.AllMatchingScope(network.ScopeMatchCloudLocal)

	var controllerAddress string
	if len(addr) > 0 {
		controllerAddress = addr[0]
	}
	d.logger.Debugf(context.TODO(), "CAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *CAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	return version.DefaultSupportedLTSBase(), nil
}

// CompleteProcess is called when the bootstrap process is complete.
func (d *CAASDeployer) CompleteProcess(ctx context.Context, controllerUnit Unit) error {
	providerID := controllerProviderID(controllerUnit.UnitTag())
	controllerUnitName, err := coreunit.NewName(controllerUnit.UnitTag().Id())
	if err != nil {
		return errors.Annotatef(err, "parsing controller unit name %q", controllerUnit.UnitTag().Id())
	}
	if err := d.applicationService.UpdateCAASUnit(ctx, controllerUnitName, applicationservice.UpdateCAASUnitParams{
		ProviderID: &providerID,
	}); err != nil {
		return errors.Annotatef(err, "updating controller unit")
	}
	if err := d.passwordService.SetUnitPassword(ctx, controllerUnitName, d.unitPassword); err != nil {
		return errors.Annotate(err, "setting controller unit password")
	}

	// TODO(units) - remove dual write to state
	op := controllerUnit.UpdateOperation(state.UnitUpdateProperties{
		ProviderId: &providerID,
	})

	if err := d.operationApplier.ApplyOperation(op); err != nil {
		return errors.Annotate(err, "cannot update controller unit")
	}

	return nil
}

func controllerProviderID(unitTag names.UnitTag) string {
	return fmt.Sprintf("controller-%d", unitTag.Number())
}
