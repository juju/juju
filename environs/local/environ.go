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
	"launchpad.net/juju-core/environs/agent"
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

func (env *localEnviron) mongoServiceName() string {
	return "juju-db-" + env.config.namespace()
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value) error {
	logger.Infof("bootstrapping environment %q", env.name)

	// TODO(thumper): check that we are running as root

	// TODO(thumper): make sure any cert files are owned by the owner of the folder they are in.
	// $(JUJU_HOME)/local-cert.pem and $(JUJU_HOME)/local-private-key.pem

	// TODO(thumper): check that the constraints don't include "container=lxc" for now.

	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	if err := environs.VerifyBootstrapInit(env, shortAttempt); err != nil {
		return err
	}

	cert, key, err := env.setupLocalMongoService()
	if err != nil {
		return err
	}

	// Work out the ip address of the lxc bridge, and use that for the mongo config.
	bridgeAddress, err := env.findBridgeAddress()
	if err != nil {
		return err
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, lxcBridgeName)

	// Before we write the agent config file, we need to make sure the
	// instance is saved in the StateInfo.
	bootstrapId := instance.Id("localhost")
	if err := environs.SaveState(env.Storage(), &environs.BootstrapState{[]instance.Id{bootstrapId}}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		return err
	}

	// Need to write out the agent file for machine-0 before initializing
	// state, as as part of that process, it will reset the password in the
	// agent file.
	if err := env.writeBootstrapAgentConfFile(cert, key); err != nil {
		return err
	}

	// Have to initialize the state configuration with localhost so we get
	// "special" permissions.
	stateConnection, err := env.initialStateConfiguration("localhost", cons)
	if err != nil {
		return err
	}
	defer stateConnection.Close()

	// TODO(thumper): upload tools into the storage

	// TODO(thumper): start the machine agent for machine-0

	return nil
}

// StateInfo is specified in the Environ interface.
func (env *localEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return environs.StateInfo(env)
}

// Config is specified in the Environ interface.
func (env *localEnviron) Config() *config.Config {
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	return env.config.Config
}

func createLocalStorageListener(dir string) (net.Listener, error) {
	// TODO(thumper): hmm... probably don't want to make the dir here, but
	// instead error if it doesn't exist.
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
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instance.Instance, len(ids))
	for i, id := range ids {
		insts[i] = &localInstance{id, env}
	}
	return insts, nil
}

// AllInstances is specified in the Environ interface.
func (env *localEnviron) AllInstances() (instances []instance.Instance, err error) {
	// TODO(thumper): get all the instances from the container manager
	instances = append(instances, &localInstance{"localhost", env})
	return instances, nil
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

	// TODO(thumper): make sure running as root

	logger.Infof("removing service %s", env.mongoServiceName())
	mongo := upstart.NewService(env.mongoServiceName())
	if err := mongo.Remove(); err != nil {
		logger.Errorf("could not remove mongo service: %v", err)
		return err
	}

	// Remove the rootdir.
	logger.Infof("removing state dir %s", env.config.rootDir())
	if err := os.RemoveAll(env.config.rootDir()); err != nil {
		logger.Errorf("could not remove local state dir: %v", err)
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

// setupLocalMongoService returns the cert and key if there was no error.
func (env *localEnviron) setupLocalMongoService() ([]byte, []byte, error) {
	journalDir := filepath.Join(env.config.mongoDir(), "journal")
	logger.Debugf("create mongo journal dir: %v", journalDir)
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return nil, nil, err
	}

	logger.Debugf("generate server cert")
	cert, key, err := env.config.GenerateStateServerCertAndKey()
	if err != nil {
		logger.Errorf("failed to generate server cert: %v", err)
		return nil, nil, err
	}
	if err := ioutil.WriteFile(
		env.config.configFile("server.pem"),
		append(cert, key...),
		0600); err != nil {
		logger.Errorf("failed to write server.pem: %v", err)
		return nil, nil, err
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
		return nil, nil, err
	}
	return cert, key, nil
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

func (env *localEnviron) writeBootstrapAgentConfFile(cert, key []byte) error {
	info, apiInfo, err := env.StateInfo()
	if err != nil {
		logger.Errorf("failed to get state info to write bootstrap agent file: %v", err)
		return err
	}
	tag := state.MachineTag("0")
	info.Tag = tag
	apiInfo.Tag = tag
	conf := &agent.Conf{
		DataDir:         env.config.rootDir(),
		StateInfo:       info,
		APIInfo:         apiInfo,
		StateServerCert: cert,
		StateServerKey:  key,
		StatePort:       env.config.StatePort(),
		APIPort:         env.config.StatePort(),
		MachineNonce:    state.BootstrapNonce,
	}
	if err := conf.Write(); err != nil {
		logger.Errorf("failed to write bootstrap agent file: %v", err)
		return err
	}
	return nil
}

func (env *localEnviron) initialStateConfiguration(addr string, cons constraints.Value) (*state.State, error) {
	// We don't check the existance of the CACert here as if it wasn't set, we wouldn't get this far.
	cfg := env.config.Config
	caCert, _ := cfg.CACert()
	addr = fmt.Sprintf("%s:%d", addr, cfg.StatePort())
	info := &state.Info{
		Addrs:  []string{addr},
		CACert: caCert,
		// Password: passwordHash,
	}
	timeout := state.DialOpts{10 * time.Second}
	bootstrap, err := environs.BootstrapConfig(cfg)
	if err != nil {
		return nil, err
	}
	st, err := state.Initialize(info, bootstrap, timeout)
	if err != nil {
		logger.Errorf("failed to initialize state: %v", err)
		return nil, err
	}
	logger.Debugf("state initialized")

	passwordHash := utils.PasswordHash(cfg.AdminSecret())
	if err := environs.BootstrapMongoUsers(st, cfg, passwordHash); err != nil {
		st.Close()
		return nil, err
	}
	jobs := []state.MachineJob{state.JobManageEnviron, state.JobManageState}

	if err := environs.ConfigureBootstrapMachine(st, cfg, cons, env.config.rootDir(), jobs); err != nil {
		st.Close()
		return nil, err
	}

	// Return an open state reference.
	return st, nil
}
