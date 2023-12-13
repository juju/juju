// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
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

// CAASDeployer is the interface that is used to deploy the controller charm
// for CAAS workloads.
type CAASDeployer struct {
	baseDeployer
	CloudServiceGetter CloudServiceGetter
	OperationApplier   OperationApplier
	UnitPassword       string
}

// NewCAASDeployer returns a new ControllerCharmDeployer for CAAS workloads.
func NewCAASDeployer() *CAASDeployer {
	return &CAASDeployer{}
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *CAASDeployer) ControllerAddress(context.Context) (string, error) {
	s, err := d.CloudServiceGetter.CloudService(d.controllerConfig.ControllerUUID())
	if err != nil {
		return "", errors.Trace(err)
	}
	hp := network.SpaceAddressesWithPort(s.Addresses(), 0)
	addr := hp.AllMatchingScope(network.ScopeMatchCloudLocal)

	var controllerAddress string
	if len(addr) > 0 {
		controllerAddress = addr[0]
	}
	d.logger.Debugf("CAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *CAASDeployer) ControllerCharmBase() corebase.Base {
	return version.DefaultSupportedLTSBase()
}

// CompleteProcess is called when the bootstrap process is complete.
func (d *CAASDeployer) CompleteProcess(ctx context.Context, controllerUnit ControllerUnit) error {
	providerID := fmt.Sprintf("controller-%d", controllerUnit.UnitTag().Number())
	op := controllerUnit.UpdateOperation(state.UnitUpdateProperties{
		ProviderId: &providerID,
	})

	if err := d.OperationApplier.ApplyOperation(op); err != nil {
		return errors.Annotate(err, "cannot update controller unit")
	}

	if err := controllerUnit.SetPassword(d.UnitPassword); err != nil {
		return errors.Annotate(err, "cannot set password for controller unit")
	}

	return nil
}
