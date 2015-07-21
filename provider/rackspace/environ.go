// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"time"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
)

type environ struct {
	openstackEnviron environs.Environ
}

// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// can't redirect to openstack provider as ussually, because correct environ should be passed for common.Bootstrap
	return common.Bootstrap(ctx, e, params)
}

func isStateServer(mcfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
}

// StartInstance implements environs.Environ.
func (e environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	r, err := e.openstackEnviron.StartInstance(args)
	if err != nil {
		return nil, err
	}
	r.Instance = environInstance{openstackInstance: r.Instance}
	err = e.connectToSsh(args, r.Instance)
	return r, errors.Trace(err)
}

func (e environ) connectToSsh(args environs.StartInstanceParams, inst instance.Instance) error {
	// trying to connect several times, because instance can be not avaliable yet
	var lastError error
	var publicAddr string
	var apiPort int
	var client *common.SshInstanceConfigurator
	for i := 0; i < 10; i++ {
		time.Sleep(5 * time.Second)
		logger.Debugf("Trying to connect to new instance.")
		addresses, err := inst.Addresses()
		if err != nil {
			logger.Debugf(err.Error())
			lastError = err
			goto Sleep
		}
		publicAddr = ""
		for _, addr := range addresses {
			if addr.Scope == network.ScopePublic && addr.Type == network.IPv4Address {
				publicAddr = addr.Value
				break
			}
		}
		if publicAddr == "" {
			goto Sleep
		}
		apiPort = 0
		if isStateServer(args.InstanceConfig) {
			apiPort = args.InstanceConfig.StateServingInfo.APIPort
		}
		client = common.NewSshInstanceConfigurator(publicAddr)
		err = client.DropAllPorts([]int{apiPort, 22}, publicAddr)
		if err != nil {
			logger.Debugf(err.Error())
			lastError = err
			goto Sleep
		} else {
			return nil
		}
	Sleep:

		time.Sleep(5 * time.Second)
	}
	return errors.Trace(lastError)
}

// StopInstances implements environs.Environ.
func (e environ) StopInstances(ids ...instance.Id) error {
	return e.openstackEnviron.StopInstances(ids...)
}

// AllInstances implements environs.Environ.
func (e environ) AllInstances() ([]instance.Instance, error) {
	instances, err := e.openstackEnviron.AllInstances()
	return e.convertInstances(instances, err)
}

func (e environ) convertInstances(instances []instance.Instance, err error) ([]instance.Instance, error) {
	if err != nil {
		return nil, errors.Trace(err)
	}

	res := make([]instance.Instance, 0)
	for _, inst := range instances {
		res = append(res, environInstance{openstackInstance: inst})
	}
	return res, nil
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
	instances, err := e.openstackEnviron.Instances(ids)
	return e.convertInstances(instances, err)
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
