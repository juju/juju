// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
)

// IAASDeployerConfig holds the configuration for a IAASDeployer.
type IAASDeployerConfig struct {
	BaseDeployerConfig
	MachineGetter MachineGetter
}

// Validate validates the configuration.
func (c IAASDeployerConfig) Validate() error {
	if err := c.BaseDeployerConfig.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.MachineGetter == nil {
		return errors.NotValidf("MachineGetter")
	}
	return nil
}

// IAASDeployer is the interface that is used to deploy the controller charm
// for IAAS workloads.
type IAASDeployer struct {
	baseDeployer
	machineGetter MachineGetter
}

// NewIAASDeployer returns a new ControllerCharmDeployer for IAAS workloads.
func NewIAASDeployer(config IAASDeployerConfig) (*IAASDeployer, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &IAASDeployer{
		baseDeployer:  makeBaseDeployer(config.BaseDeployerConfig),
		machineGetter: config.MachineGetter,
	}, nil
}

// ControllerAddress returns the address of the controller that should be
// used.
func (d *IAASDeployer) ControllerAddress(ctx context.Context) (string, error) {
	m, err := d.machineGetter.Machine(agent.BootstrapControllerId)
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
	d.logger.Debugf(ctx, "IAAS controller address %v", controllerAddress)
	return controllerAddress, nil
}

// ControllerCharmBase returns the base used for deploying the controller
// charm.
func (d *IAASDeployer) ControllerCharmBase() (corebase.Base, error) {
	m, err := d.machineGetter.Machine(agent.BootstrapControllerId)
	if err != nil {
		return corebase.Base{}, errors.Trace(err)
	}

	machineBase := m.Base()
	return corebase.ParseBase(machineBase.OS, machineBase.Channel)
}

// CompleteProcess is called when the bootstrap process is complete.
func (d *IAASDeployer) CompleteProcess(ctx context.Context, controllerUnit coreunit.Name) error {
	return nil
}
