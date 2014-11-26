// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"sync"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
)

// This file contains the core of the gce Environ implementation. You will
// probably not need to change this file very much to begin with; and if you
// never need to add any more fields, you may never need to touch it.
//
// The rest of the implementation is split into environ_instance.go (which
// must be implemented ) and environ_firewall.go (which can be safely
// ignored until you've got an environment bootstrapping successfully).

type environ struct {
	common.SupportsUnitPlacementPolicy

	name string

	lock    sync.Mutex
	ecfg    *environConfig
	storage storage.Storage
}

var _ environs.Environ = (*environ)(nil)

func (env *environ) Name() string {
	return env.name
}

func (*environ) Provider() environs.EnvironProvider {
	return providerInstance
}

func (env *environ) SetConfig(cfg *config.Config) error {
	env.lock.Lock()
	defer env.lock.Unlock()
	ecfg, err := validateConfig(cfg, env.ecfg)
	if err != nil {
		return err
	}
	storage, err := newStorage(ecfg)
	if err != nil {
		return err
	}
	env.ecfg = ecfg
	env.storage = storage
	return nil
}

func (env *environ) getSnapshot() *environ {
	env.lock.Lock()
	clone := *env
	env.lock.Unlock()
	clone.lock = sync.Mutex{}
	return &clone
}

func (env *environ) Config() *config.Config {
	return env.getSnapshot().ecfg.Config
}

func (env *environ) Storage() storage.Storage {
	return env.getSnapshot().storage
}

func (env *environ) Bootstrap(ctx environs.BootstrapContext, params environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	// You can probably ignore this method; the common implementation should work.
	return common.Bootstrap(ctx, env, params)
}

func (env *environ) Destroy() error {
	// You can probably ignore this method; the common implementation should work.
	return common.Destroy(env)
}

// AllocateAddress requests a specific address to be allocated for the
// given instance on the given network.
func (env *environ) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	return nil
}

func (env *environ) ConstraintsValidator() (constraints.Validator, error) {
	return nil, nil
}

func (env *environ) ListNetworks(inst instance.Id) ([]network.BasicInfo, error) {
	return nil, nil
}

func (env *environ) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	return nil
}

// firewall stuff

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) OpenPorts(ports []network.PortRange) error {
	return nil
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) ClosePorts(ports []network.PortRange) error {
	return nil
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (env *environ) Ports() ([]network.PortRange, error) {
	return nil, nil
}

// instance stuff

func (env *environ) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	// Please note that in order to fulfil the demands made of Instances and
	// AllInstances, it is imperative that some environment feature be used to
	// keep track of which instances were actually started by juju.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) AllInstances() ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// Please note that this must *not* return instances that have not been
	// allocated as part of this environment -- if it does, juju will see they
	// are not tracked in state, assume they're stale/rogue, and shut them down.
	// This advice applies even if an instance id passed in corresponds to a
	// real instance that's not part of the environment -- the Environ should
	// treat that no differently to a request for one that does not exist.
	_ = env.getSnapshot()
	return nil, errNotImplemented
}

func (env *environ) StopInstances(instances ...instance.Id) error {
	_ = env.getSnapshot()
	return errNotImplemented
}

func (env *environ) StateServerInstances() ([]instance.Id, error) {
	return nil, nil
}

func (env *environ) SupportedArchitectures() ([]string, error) {
	return arch.AllSupportedArches, nil
}

// SupportNetworks returns whether the environment has support to
// specify networks for services and machines.
func (env *environ) SupportNetworks() bool {
	return false
}

// SupportsUnitAssignment returns an error which, if non-nil, indicates
// that the environment does not support unit placement. If the environment
// does not support unit placement, then machines may not be created
// without units, and units cannot be placed explcitly.
func (env *environ) SupportsUnitPlacement() error {
	return nil
}

// SupportAddressAllocation takes a network.Id and returns a bool
// and an error. The bool indicates whether that network supports
// static ip address allocation.
func (env *environ) SupportAddressAllocation(netId network.Id) (bool, error) {
	return false, nil
}
