// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
)

// IAASDeployer is the interface that is used to deploy the controller charm
// for IAAS workloads.
type IAASDeployer struct {
	baseDeployer
	MachineGetter MachineGetter
}

// NewIAASDeployer returns a new ControllerCharmDeployer for IAAS workloads.
func NewIAASDeployer() *IAASDeployer {
	return &IAASDeployer{}
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *IAASDeployer) ControllerAddress(context.Context) (string, error) {
	m, err := d.MachineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return "", errors.Trace(err)
	}

	pa, err := m.PublicAddress()
	if err != nil && !network.IsNoAddressError(err) {
		return "", errors.Trace(err)
	}
	var controllerAddress string
	if err == nil {
		controllerAddress = pa.Value
	}
	d.logger.Debugf("IAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *IAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	m, err := d.MachineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return corebase.Empty, errors.Trace(err)
	}

	machineBase := m.Base()
	return corebase.ParseBase(machineBase.OS, machineBase.Channel)
}

// CompleteProcess is called when the bootstrap process is complete.
func (d *IAASDeployer) CompleteProcess(ctx context.Context, controllerUnit ControllerUnit) error {
	m, err := d.MachineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return errors.Trace(err)
	}

	if err := controllerUnit.AssignToMachine(m); err != nil {
		return errors.Annotate(err, "cannot assign controller unit to machine")
	}
	return nil
}
