// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type environ struct {
	openstackEnviron environs.Environ
}

// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return e.openstackEnviron.Bootstrap(ctx, params)
}

// StartInstance implements environs.Environ.
func (e environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	r, err := e.openstackEnviron.StartInstance(args)
	if err != nil {
		return nil, err
	}
	r.Instance = environInstance{openstackInstance: r.Instance}
	return r, nil
}

// StopInstances implements environs.Environ.
func (e environ) StopInstances(ids ...instance.Id) error {
	return e.openstackEnviron.StopInstances(ids...)
}

// AllInstances implements environs.Environ.
func (e environ) AllInstances() ([]instance.Instance, error) {
	return e.openstackEnviron.AllInstances()
}

// MaintainInstance implements environs.Environ.
func (e environ) MaintainInstance(args environs.StartInstanceParams) error {
	return e.openstackEnviron.MaintainInstance(args)
}

// Config implements environs.Environ.
func (e environ) Config() *config.Config {
	return e.openstackEnviron.Config()
}

// SupportedArchitectures implements environs.Environ.
func (e environ) SupportedArchitectures() ([]string, error) {
	return e.openstackEnviron.SupportedArchitectures()
}

// SupportsUnitPlacement implements environs.Environ.
func (e environ) SupportsUnitPlacement() error {
	return e.openstackEnviron.SupportsUnitPlacement()
}

// ConstraintsValidator implements environs.Environ.
func (e environ) ConstraintsValidator() (constraints.Validator, error) {
	return e.openstackEnviron.ConstraintsValidator()
}

// SetConfig implements environs.Environ.
func (e environ) SetConfig(cfg *config.Config) error {
	return e.openstackEnviron.SetConfig(cfg)
}

// Instances implements environs.Environ.
func (e environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return e.openstackEnviron.Instances(ids)
}

// StateServerInstances implements environs.Environ.
func (e environ) StateServerInstances() ([]instance.Id, error) {
	return e.openstackEnviron.StateServerInstances()
}

// Destroy implements environs.Environ.
func (e environ) Destroy() error {
	return e.openstackEnviron.Destroy()
}

// OpenPorts implements environs.Environ.
func (e environ) OpenPorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("OpenPorts"))
}

// ClosePorts implements environs.Environ.
func (e environ) ClosePorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// Ports implements environs.Environ.
func (e environ) Ports() ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}

// Provider implements environs.Environ.
func (e environ) Provider() environs.EnvironProvider {
	return e.openstackEnviron.Provider()
}

// PrecheckInstance implements environs.Environ.
func (e environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return e.openstackEnviron.PrecheckInstance(series, cons, placement)
}
