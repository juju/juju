// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

type localEnviron struct {
	localMutex            sync.Mutex
	config                *environConfig
	name                  string
	sharedStorageListener net.Listener
	storageListener       net.Listener
}

// Name is specified in the Environ interface.
func (env *localEnviron) Name() string {
	return env.name
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value) error {
	return fmt.Errorf("not implemented")
}

// StateInfo is specified in the Environ interface.
func (env *localEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

// Config is specified in the Environ interface.
func (env *localEnviron) Config() *config.Config {
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	return env.config.Config
}

func createLocalStorageListener(dir string) (net.Listener, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.Errorf("failed to make directory for storage at %s: %v", dir, err)
		return nil, err
	}
	return localstorage.Serve("localhost:0", dir)
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
	// Well... this works fine as long as the config has set from the clients
	// local machine.
	sharedStorageListener, err := createLocalStorageListener(config.sharedStorageDir())
	if err != nil {
		return err
	}

	storageListener, err := createLocalStorageListener(config.storageDir())
	if err != nil {
		sharedStorageListener.Close()
		return err
	}
	if env.sharedStorageListener != nil {
		env.sharedStorageListener.Close()
	}
	if env.storageListener != nil {
		env.storageListener.Close()
	}
	env.sharedStorageListener = sharedStorageListener
	env.storageListener = storageListener
	return nil
}

// StartInstance is specified in the Environ interface.
func (env *localEnviron) StartInstance(
	machineId, machineNonce, series string,
	cons constraints.Value,
	info *state.Info,
	apiInfo *api.Info,
) (instance.Instance, *instance.HardwareCharacteristics, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

// StopInstances is specified in the Environ interface.
func (env *localEnviron) StopInstances([]instance.Instance) error {
	return fmt.Errorf("not implemented")
}

// Instances is specified in the Environ interface.
func (env *localEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	return nil, fmt.Errorf("not implemented")
}

// AllInstances is specified in the Environ interface.
func (env *localEnviron) AllInstances() ([]instance.Instance, error) {
	return nil, fmt.Errorf("not implemented")
}

// Storage is specified in the Environ interface.
func (env *localEnviron) Storage() environs.Storage {
	return localstorage.Client(env.privateListener.Addr().String())
}

// PublicStorage is specified in the Environ interface.
func (env *localEnviron) PublicStorage() environs.StorageReader {
	return localstorage.Client(env.publicListener.Addr().String())
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy(insts []instance.Instance) error {
	return fmt.Errorf("not implemented")
}

// OpenPorts is specified in the Environ interface.
func (env *localEnviron) OpenPorts(ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// ClosePorts is specified in the Environ interface.
func (env *localEnviron) ClosePorts(ports []instance.Port) error {
	return fmt.Errorf("not implemented")
}

// Ports is specified in the Environ interface.
func (env *localEnviron) Ports() ([]instance.Port, error) {
	return nil, fmt.Errorf("not implemented")
}

// Provider is specified in the Environ interface.
func (env *localEnviron) Provider() environs.EnvironProvider {
	return &provider
}
