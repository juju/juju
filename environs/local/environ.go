// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"net"
	"sync"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

type localEnviron struct {
	localMutex      sync.Mutex
	config          *environConfig
	name            string
	publicListener  net.Listener
	privateListener net.Listener
}

// Name is specified in the Environ interface.
func (env *localEnviron) Name() string {
	return env.name
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value) error {
	panic("unimplemented")
}

// StateInfo is specified in the Environ interface.
func (env *localEnviron) StateInfo() (*state.Info, *api.Info, error) {
	panic("unimplemented")
}

// Config is specified in the Environ interface.
func (env *localEnviron) Config() *config.Config {
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	return env.config.Clone()
}

// SetConfig is specified in the Environ interface.
func (env *localEnviron) SetConfig(cfg *config.Config) error {
	config, err := provider.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	env.config = config
	env.name = config.Name()
	// Recreate local storage?
	publicListener, err := listen("/var/lib/juju/storage/public", "127.0.0.1", 0)
	if err != nil {
		return err
	}
	privateListener, err := listen("/var/lib/juju/storage/private", "127.0.0.1", 0)
	if err != nil {
		publicListener.Close()
		return err
	}
	env.publicListener = publicListener
	env.privateListener = privateListener
	return nil
}

// StartInstance is specified in the Environ interface.
func (env *localEnviron) StartInstance(machineId, machineNonce string, series string, cons constraints.Value, info *state.Info, apiInfo *api.Info) (instance.Instance, error) {
	panic("unimplemented")
}

// StopInstances is specified in the Environ interface.
func (env *localEnviron) StopInstances([]instance.Instance) error {
	panic("unimplemented")
}

// Instances is specified in the Environ interface.
func (env *localEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	panic("unimplemented")
}

// AllInstances is specified in the Environ interface.
func (env *localEnviron) AllInstances() ([]instance.Instance, error) {
	panic("unimplemented")
}

// Storage is specified in the Environ interface.
func (env *localEnviron) Storage() environs.Storage {
	panic("unimplemented")
}

// PublicStorage is specified in the Environ interface.
func (env *localEnviron) PublicStorage() environs.StorageReader {
	panic("unimplemented")
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy(insts []instance.Instance) error {
	panic("unimplemented")
}

// OpenPorts is specified in the Environ interface.
func (env *localEnviron) OpenPorts(ports []instance.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Environ interface.
func (env *localEnviron) ClosePorts(ports []instance.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Environ interface.
func (env *localEnviron) Ports() ([]instance.Port, error) {
	panic("unimplemented")
}

// Provider is specified in the Environ interface.
func (env *localEnviron) Provider() environs.EnvironProvider {
	panic("unimplemented")
}
