// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package null

import (
	"errors"
	"path"
	"sync"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/manual"
	"launchpad.net/juju-core/environs/sshstorage"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
)

const (
	// TODO(axw) make this configurable?
	dataDir = "/var/lib/juju"

	// storageSubdir is the subdirectory of
	// dataDir in which storage will be located.
	storageSubdir = "storage"

	// storageTmpSubdir is the subdirectory of
	// dataDir in which temporary storage will
	// be located.
	storageTmpSubdir = "storage-tmp"
)

type nullEnviron struct {
	cfg      *environConfig
	cfgmutex sync.Mutex
}

var errNoStartInstance = errors.New("null provider cannot start instances")
var errNoStopInstance = errors.New("null provider cannot stop instances")

func (*nullEnviron) StartInstance(constraints.Value, tools.List, *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {
	return nil, nil, errNoStartInstance
}

func (*nullEnviron) StopInstances([]instance.Instance) error {
	return errNoStopInstance
}

func (e *nullEnviron) AllInstances() ([]instance.Instance, error) {
	return e.Instances([]instance.Id{manual.BootstrapInstanceId})
}

func (e *nullEnviron) envConfig() (cfg *environConfig) {
	e.cfgmutex.Lock()
	cfg = e.cfg
	e.cfgmutex.Unlock()
	return cfg
}

func (e *nullEnviron) Config() *config.Config {
	return e.envConfig().Config
}

func (e *nullEnviron) Name() string {
	return e.envConfig().Name()
}

func (e *nullEnviron) Bootstrap(_ constraints.Value, possibleTools tools.List, machineID string) error {
	return manual.Bootstrap(manual.BootstrapArgs{
		Host:          e.envConfig().sshHost(),
		DataDir:       dataDir,
		Environ:       e,
		MachineId:     machineID,
		PossibleTools: possibleTools,
	})
}

func (e *nullEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return provider.StateInfo(e)
}

func (e *nullEnviron) SetConfig(cfg *config.Config) error {
	e.cfgmutex.Lock()
	defer e.cfgmutex.Unlock()
	envConfig, err := nullProvider{}.validate(cfg, e.cfg.Config)
	if err != nil {
		return err
	}
	e.cfg = envConfig
	return nil
}

// Implements environs.Environ.
//
// This method will only ever return an Instance for the Id
// environ/manual.BootstrapInstanceId. If any others are
// specified, then ErrPartialInstances or ErrNoInstances
// will result.
func (e *nullEnviron) Instances(ids []instance.Id) (instances []instance.Instance, err error) {
	instances = make([]instance.Instance, len(ids))
	var found bool
	for i, id := range ids {
		if id == manual.BootstrapInstanceId {
			instances[i] = nullBootstrapInstance{e.envConfig().bootstrapHost()}
			found = true
		} else {
			err = environs.ErrPartialInstances
		}
	}
	if !found {
		err = environs.ErrNoInstances
	}
	return instances, err
}

// Implements environs/bootstrap.BootstrapStorage.
func (e *nullEnviron) BootstrapStorage() (storage.Storage, error) {
	cfg := e.envConfig()
	storageDir := e.StorageDir()
	storageTmpdir := path.Join(dataDir, storageTmpSubdir)
	return sshstorage.NewSSHStorage(cfg.sshHost(), storageDir, storageTmpdir)
}

func (e *nullEnviron) Storage() storage.Storage {
	return httpstorage.Client(e.envConfig().storageAddr())
}

func (e *nullEnviron) PublicStorage() storage.StorageReader {
	return environs.EmptyStorage
}

func (e *nullEnviron) Destroy() error {
	return errors.New("null provider destruction is not implemented yet")
}

func (e *nullEnviron) OpenPorts(ports []instance.Port) error {
	return nil
}

func (e *nullEnviron) ClosePorts(ports []instance.Port) error {
	return nil
}

func (e *nullEnviron) Ports() ([]instance.Port, error) {
	return []instance.Port{}, nil
}

func (*nullEnviron) Provider() environs.EnvironProvider {
	return nullProvider{}
}

func (e *nullEnviron) StorageAddr() string {
	return e.envConfig().storageListenAddr()
}

func (e *nullEnviron) StorageDir() string {
	return path.Join(dataDir, storageSubdir)
}

func (e *nullEnviron) SharedStorageAddr() string {
	return ""
}

func (e *nullEnviron) SharedStorageDir() string {
	return ""
}
