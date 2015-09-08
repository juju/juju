// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
<<<<<<< HEAD
<<<<<<< HEAD
	"time"

	"github.com/juju/juju/cloudconfig/instancecfg"
<<<<<<< HEAD
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	jujuos "github.com/juju/juju/juju/os"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/version"
)

type environ struct {
	environs.Environ
}

<<<<<<< HEAD
// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// can't redirect to openstack provider as ussually, because correct environ should be passed for common.Bootstrap
	return common.Bootstrap(ctx, e, params)
=======
var bootstrap = common.Bootstrap

// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// can't redirect to openstack provider as ussually, because correct environ should be passed for common.Bootstrap
	return bootstrap(ctx, e, params)
>>>>>>> More review comments implemented
}

func isStateServer(mcfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
=======
=======
	"time"
>>>>>>> working version of rackspace provider

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
=======
>>>>>>> review comments implemented
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/version"
)

type environ struct {
	environs.Environ
}

// Bootstrap implements environs.Environ.
func (e environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
<<<<<<< HEAD
	return e.openstackEnviron.Bootstrap(ctx, params)
>>>>>>> modifications to opestack provider applied
=======
	// can't redirect to openstack provider as ussually, because correct environ should be passed for common.Bootstrap
	return common.Bootstrap(ctx, e, params)
}

func isStateServer(mcfg *instancecfg.InstanceConfig) bool {
	return multiwatcher.AnyJobNeedsState(mcfg.Jobs...)
>>>>>>> working version of rackspace provider
}

// StartInstance implements environs.Environ.
func (e environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
<<<<<<< HEAD
<<<<<<< HEAD
	os, err := version.GetOSFromSeries(args.Tools.OneSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if os == jujuos.Windows && args.InstanceConfig.Config.FirewallMode() != config.FwNone {
		return nil, errors.Errorf("rackspace provider doesn't support firewalls for windows instances")
<<<<<<< HEAD

	}
	r, err := e.Environ.StartInstance(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r.Instance = environInstance{Instance: r.Instance}
	if args.InstanceConfig.Config.FirewallMode() != config.FwNone {
		err = e.connectToSsh(args, r.Instance)
	}
	return r, errors.Trace(err)
}

var newInstanceConfigurator = common.NewSshInstanceConfigurator

// connectToSsh creates new InstanceConfigurator and calls  DropAllPorts method.
// In order to do this it needs to wait until ip address becomes avaliable.
// Dropiing all ports is required  to implement firewall functionality: by default all ports should be closed,
// and only when we  expose some service, we will open all required ports.
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
<<<<<<< HEAD
		client = common.NewSshInstanceConfigurator(publicAddr)
=======
		client = newInstanceConfigurator(publicAddr)
>>>>>>> More review comments implemented
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
=======
	r, err := e.openstackEnviron.StartInstance(args)
=======
	os, err := version.GetOSFromSeries(args.Tools.OneSeries())
>>>>>>> review comments implemented
	if err != nil {
		return nil, errors.Trace(err)
	}
	if os == version.Windows && args.InstanceConfig.Config.FirewallMode() != config.FwNone {
		return nil, errors.Errorf("Rackspace provider don't support firawall mode other then none for windows instances.")
=======
>>>>>>> More review comments implemented

	}
	r, err := e.Environ.StartInstance(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r.Instance = environInstance{Instance: r.Instance}
	if args.InstanceConfig.Config.FirewallMode() != config.FwNone {
		err = e.connectToSsh(args, r.Instance)
	}
	return r, errors.Trace(err)
}

var newInstanceConfigurator = common.NewSshInstanceConfigurator

// connectToSsh creates new InstanceConfigurator and calls  DropAllPorts method.
// In order to do this it needs to wait until ip address becomes avaliable.
// Dropiing all ports is required  to implement firewall functionality: by default all ports should be closed,
// and only when we  expose some service, we will open all required ports.
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
<<<<<<< HEAD
		client = common.NewSshInstanceConfigurator(publicAddr)
=======
		client = newInstanceConfigurator(publicAddr)
>>>>>>> More review comments implemented
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

<<<<<<< HEAD
// StopInstances implements environs.Environ.
func (e environ) StopInstances(ids ...instance.Id) error {
	return e.openstackEnviron.StopInstances(ids...)
>>>>>>> modifications to opestack provider applied
}

// AllInstances implements environs.Environ.
func (e environ) AllInstances() ([]instance.Instance, error) {
<<<<<<< HEAD
<<<<<<< HEAD
	instances, err := e.Environ.AllInstances()
	res, err := e.convertInstances(instances, err)
	return res, errors.Trace(err)
=======
	instances, err := e.openstackEnviron.AllInstances()
	return e.convertInstances(instances, err)
>>>>>>> working version of rackspace provider
=======
// AllInstances implements environs.Environ.
func (e environ) AllInstances() ([]instance.Instance, error) {
	instances, err := e.Environ.AllInstances()
	res, err := e.convertInstances(instances, err)
	return res, errors.Trace(err)
>>>>>>> review comments implemented
}

func (e environ) convertInstances(instances []instance.Instance, err error) ([]instance.Instance, error) {
	if err != nil {
<<<<<<< HEAD
<<<<<<< HEAD
		return nil, err
=======
		return nil, errors.Trace(err)
>>>>>>> working version of rackspace provider
=======
		return nil, err
>>>>>>> review comments implemented
	}

	res := make([]instance.Instance, 0)
	for _, inst := range instances {
<<<<<<< HEAD
<<<<<<< HEAD
		res = append(res, environInstance{inst})
	}
	return res, nil
=======
	return e.openstackEnviron.AllInstances()
=======
		res = append(res, environInstance{openstackInstance: inst})
=======
		res = append(res, environInstance{inst})
>>>>>>> review comments implemented
	}
	return res, nil
>>>>>>> working version of rackspace provider
}

<<<<<<< HEAD
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
>>>>>>> modifications to opestack provider applied
}

// Instances implements environs.Environ.
func (e environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
<<<<<<< HEAD
<<<<<<< HEAD
	instances, err := e.Environ.Instances(ids)
	res, err := e.convertInstances(instances, err)
	return res, errors.Trace(err)
=======
	return e.openstackEnviron.Instances(ids)
=======
	instances, err := e.openstackEnviron.Instances(ids)
	return e.convertInstances(instances, err)
>>>>>>> working version of rackspace provider
}

// StateServerInstances implements environs.Environ.
func (e environ) StateServerInstances() ([]instance.Id, error) {
	return e.openstackEnviron.StateServerInstances()
}

// Destroy implements environs.Environ.
func (e environ) Destroy() error {
	return e.openstackEnviron.Destroy()
>>>>>>> modifications to opestack provider applied
=======
// Instances implements environs.Environ.
func (e environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	instances, err := e.Environ.Instances(ids)
	res, err := e.convertInstances(instances, err)
	return res, errors.Trace(err)
>>>>>>> review comments implemented
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
<<<<<<< HEAD
<<<<<<< HEAD
	return &providerInstance
=======
	return e.openstackEnviron.Provider()
}

// PrecheckInstance implements environs.Environ.
func (e environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return e.openstackEnviron.PrecheckInstance(series, cons, placement)
>>>>>>> modifications to opestack provider applied
=======
	return &providerInstance
>>>>>>> review comments implemented
}
