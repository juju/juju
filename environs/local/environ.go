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

var lxcBridgeName = "lxcbr0"

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by longAttempt.
var shortAttempt = utils.AttemptStrategy{
	Total: 1 * time.Second,
	Delay: 50 * time.Millisecond,
}

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

func (env *localEnviron) mongoServiceName() string {
	return "juju-db-" + env.config.namespace()
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value) error {
	logger.Infof("bootstrapping environment %q", env.name)

	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	if err := environs.VerifyBootstrapInit(env, shortAttempt); err != nil {
		return err
	}

	if err := env.setupLocalMongoService(); err != nil {
		return err
	}

	// Work out the ip address of the lxc bridge, and use that for the mongo config.
	bridgeAddress, err := env.findBridgeAddress()
	if err != nil {
		return err
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, lxcBridgeName)

	// Create a fake machine 0 in state to represent the machine, need an instance id.
	// "localhost" makes sense for that.

	return nil
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
	publicListener, err := createLocalStorageListener(config.sharedStorageDir())
	if err != nil {
		return err
	}

	privateListener, err := createLocalStorageListener(config.storageDir())
	if err != nil {
		publicListener.Close()
		return err
	}
	env.publicListener = publicListener
	env.privateListener = privateListener
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

	logger.Infof("removing service %s", env.mongoServiceName())
	mongo := upstart.NewService(env.mongoServiceName())
	if err := mongo.Remove(); err != nil {
		logger.Errorf("could not remove mongo service: %v", err)
		return err
	}

	return nil
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

func (env *localEnviron) setupLocalMongoService() error {
	journalDir := filepath.Join(env.config.mongoDir(), "journal")
	logger.Debugf("create mongo journal dir: %v", journalDir)
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return err
	}

	logger.Debugf("generate server cert")
	cert, key, err := env.config.GenerateStateServerCertAndKey()
	if err != nil {
		logger.Errorf("failed to generate server cert: %v", err)
		return err
	}
	if err := ioutil.WriteFile(
		env.config.configFile("server.pem"),
		append(cert, key...),
		0600); err != nil {
		logger.Errorf("failed to write server.pem: %v", err)
		return err
	}

	// TODO(thumper): work out how to get the user to sudo bits...
	mongo := upstart.MongoUpstartService(
		env.mongoServiceName(),
		env.config.rootDir(),
		env.config.mongoDir(),
		env.config.StatePort())
	logger.Infof("installing service %s", env.mongoServiceName())
	if err := mongo.Install(); err != nil {
		logger.Errorf("could not install mongo service: %v", err)
		return err
	}
	return nil
}

func (env *localEnviron) findBridgeAddress() (string, error) {

	bridge, err := net.InterfaceByName(lxcBridgeName)
	if err != nil {
		logger.Errorf("cannot find network interface %q: %v", lxcBridgeName, err)
		return "", err
	}
	addrs, err := bridge.Addrs()
	if err != nil {
		logger.Errorf("cannot get addresses for network interface %q: %v", lxcBridgeName, err)
		return "", err
	}
	return utils.GetIPv4Address(addrs)
}
