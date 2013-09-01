// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"launchpad.net/juju-core/agent"
	agenttools "launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// lxcBridgeName is the name of the network interface that the local provider
// uses to determine the ip address to use for machine-0 such that the
// containers being created are able to communicate with it simply.
const lxcBridgeName = "lxcbr0"

// boostrapInstanceId is just the name we give to the bootstrap machine.
// Using "localhost" because it is, and it makes sense.
const boostrapInstanceId = "localhost"

// upstartScriptLocation is parameterised purely for testing purposes as we
// don't really want to be installing and starting scripts as root for
// testing.
var upstartScriptLocation = "/etc/init"

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

type localEnviron struct {
	localMutex            sync.Mutex
	config                *environConfig
	name                  string
	sharedStorageListener net.Listener
	storageListener       net.Listener
	containerManager      lxc.ContainerManager
}

// Name is specified in the Environ interface.
func (env *localEnviron) Name() string {
	return env.name
}

func (env *localEnviron) mongoServiceName() string {
	return "juju-db-" + env.config.namespace()
}

func (env *localEnviron) machineAgentServiceName() string {
	return "juju-agent-" + env.config.namespace()
}

// ensureCertOwner checks to make sure that the cert files created
// by the bootstrap command are owned by the user and not root.
func (env *localEnviron) ensureCertOwner() error {
	files := []string{
		config.JujuHomePath(env.name + "-cert.pem"),
		config.JujuHomePath(env.name + "-private-key.pem"),
	}

	uid, gid, err := sudoCallerIds()
	if err != nil {
		return err
	}
	if uid != 0 || gid != 0 {
		for _, filename := range files {
			if err := os.Chown(filename, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(cons constraints.Value, possibleTools tools.List, machineID string) error {
	if !env.config.runningAsRoot {
		return fmt.Errorf("bootstrapping a local environment must be done as root")
	}
	if err := env.config.createDirs(); err != nil {
		logger.Errorf("failed to create necessary directories: %v", err)
		return err
	}

	if err := env.ensureCertOwner(); err != nil {
		logger.Errorf("failed to reassign ownership of the certs to the user: %v", err)
		return err
	}
	// TODO(thumper): check that the constraints don't include "container=lxc" for now.

	cert, key, err := env.setupLocalMongoService()
	if err != nil {
		return err
	}

	// Before we write the agent config file, we need to make sure the
	// instance is saved in the StateInfo.
	bootstrapId := instance.Id(boostrapInstanceId)
	if err := environs.SaveState(env.Storage(), &environs.BootstrapState{StateInstances: []instance.Id{bootstrapId}}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		return err
	}

	// Need to write out the agent file for machine-0 before initializing
	// state, as as part of that process, it will reset the password in the
	// agent file.
	agentConfig, err := env.writeBootstrapAgentConfFile(env.config.AdminSecret(), cert, key)
	if err != nil {
		return err
	}

	// Have to initialize the state configuration with localhost so we get
	// "special" permissions.
	stateConnection, err := env.initialStateConfiguration(agentConfig, cons)
	if err != nil {
		return err
	}
	defer stateConnection.Close()

	return env.setupLocalMachineAgent(cons, possibleTools)
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

func createLocalStorageListener(dir, address string) (net.Listener, error) {
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("storage directory %q does not exist, bootstrap first", dir)
	} else if err != nil {
		return nil, err
	} else if !info.Mode().IsDir() {
		return nil, fmt.Errorf("%q exists but is not a directory (and it needs to be)", dir)
	}
	return localstorage.Serve(address, dir)
}

// SetConfig is specified in the Environ interface.
func (env *localEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := provider.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	env.config = ecfg
	env.name = ecfg.Name()

	env.containerManager = lxc.NewContainerManager(
		lxc.ManagerConfig{
			Name:   env.config.namespace(),
			LogDir: env.config.logDir(),
		})

	// Here is the end of normal config setting.
	if ecfg.bootstrapped() {
		return nil
	}
	return env.bootstrapAddressAndStorage(cfg)
}

// bootstrapAddressAndStorage finishes up the setup of the environment in
// situations where there is no machine agent running yet.
func (env *localEnviron) bootstrapAddressAndStorage(cfg *config.Config) error {
	// If we get to here, it is because we haven't yet bootstrapped an
	// environment, and saved the config in it, or we are running a command
	// from the command line, so it is ok to work on the assumption that we
	// have direct access to the directories.
	if err := env.config.createDirs(); err != nil {
		return err
	}

	bridgeAddress, err := env.findBridgeAddress()
	if err != nil {
		return err
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, lxcBridgeName)
	cfg, err = cfg.Apply(map[string]interface{}{
		"bootstrap-ip": bridgeAddress,
	})
	if err != nil {
		logger.Errorf("failed to apply new addresses to config: %v", err)
		return err
	}
	config, err := provider.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.config = config

	return env.setupLocalStorage()
}

// setupLocalStorage looks to see if there is someone listening on the storage
// address port.  If there is we assume that it is ours and all is good.  If
// there is no one listening on that port, create listeners for both storage
// and the shared storage for the duration of the commands execution.
func (env *localEnviron) setupLocalStorage() error {
	// Try to listen to the storageAddress.
	logger.Debugf("checking %s to see if machine agent running storage listener", env.config.storageAddr())
	connection, err := net.Dial("tcp", env.config.storageAddr())
	if err != nil {
		logger.Debugf("nope, start some")
		// These listeners are part of the environment structure so as to remain
		// referenced for the duration of the open environment.  This is only for
		// environs that have been created due to a user command.
		env.storageListener, err = createLocalStorageListener(env.config.storageDir(), env.config.storageAddr())
		if err != nil {
			return err
		}
		env.sharedStorageListener, err = createLocalStorageListener(env.config.sharedStorageDir(), env.config.sharedStorageAddr())
		if err != nil {
			return err
		}
	} else {
		logger.Debugf("yes, don't start local storage listeners")
		connection.Close()
	}
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	machineId := machineConfig.MachineId
	series := possibleTools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", machineId, series)
	agenttools := possibleTools[0]
	logger.Debugf("tools: %#v", agenttools)

	network := lxc.DefaultNetworkConfig()
	inst, err := env.containerManager.StartContainer(
		machineId, series, machineConfig.MachineNonce, network,
		agenttools, env.config.Config,
		machineConfig.StateInfo, machineConfig.APIInfo)
	if err != nil {
		return nil, nil, err
	}
	// TODO(thumper): return some hardware characteristics.
	return inst, nil, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StopInstances(instances []instance.Instance) error {
	for _, inst := range instances {
		if inst.Id() == boostrapInstanceId {
			return fmt.Errorf("cannot stop the bootstrap instance")
		}
		if err := env.containerManager.StopContainer(inst); err != nil {
			return err
		}
	}
	return nil
}

// Instances is specified in the Environ interface.
func (env *localEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	// NOTE: do we actually care about checking the existance of the instances?
	// I posit that here we don't really care, and that we are only called with
	// instance ids that we know exist.
	if len(ids) == 0 {
		return nil, nil
	}
	insts := make([]instance.Instance, len(ids))
	for i, id := range ids {
		insts[i] = &localInstance{id, env}
	}
	return insts, nil
}

// AllInstances is specified in the InstanceBroker interface.
func (env *localEnviron) AllInstances() (instances []instance.Instance, err error) {
	instances = append(instances, &localInstance{boostrapInstanceId, env})
	// Add in all the containers as well.
	lxcInstances, err := env.containerManager.ListContainers()
	if err != nil {
		return nil, err
	}
	for _, inst := range lxcInstances {
		instances = append(instances, &localInstance{inst.Id(), env})
	}
	return instances, nil
}

// Storage is specified in the Environ interface.
func (env *localEnviron) Storage() environs.Storage {
	return localstorage.Client(env.config.storageAddr())
}

// PublicStorage is specified in the Environ interface.
func (env *localEnviron) PublicStorage() environs.StorageReader {
	return localstorage.Client(env.config.sharedStorageAddr())
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy(insts []instance.Instance) error {
	if !env.config.runningAsRoot {
		return fmt.Errorf("destroying a local environment must be done as root")
	}
	// Kill all running instances.
	containers, err := env.containerManager.ListContainers()
	if err != nil {
		return err
	}
	for _, inst := range containers {
		if err := env.containerManager.StopContainer(inst); err != nil {
			return err
		}
	}

	logger.Infof("removing service %s", env.machineAgentServiceName())
	machineAgent := upstart.NewService(env.machineAgentServiceName())
	machineAgent.InitDir = upstartScriptLocation
	if err := machineAgent.Remove(); err != nil {
		logger.Errorf("could not remove machine agent service: %v", err)
		return err
	}

	logger.Infof("removing service %s", env.mongoServiceName())
	mongo := upstart.NewService(env.mongoServiceName())
	mongo.InitDir = upstartScriptLocation
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
	return fmt.Errorf("open ports not implemented")
}

// ClosePorts is specified in the Environ interface.
func (env *localEnviron) ClosePorts(ports []instance.Port) error {
	return fmt.Errorf("close ports not implemented")
}

// Ports is specified in the Environ interface.
func (env *localEnviron) Ports() ([]instance.Port, error) {
	return nil, nil
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

	mongo := upstart.MongoUpstartService(
		env.mongoServiceName(),
		env.config.rootDir(),
		env.config.mongoDir(),
		env.config.StatePort())
	mongo.InitDir = upstartScriptLocation
	logger.Infof("installing service %s to %s", env.mongoServiceName(), mongo.InitDir)
	if err := mongo.Install(); err != nil {
		logger.Errorf("could not install mongo service: %v", err)
		return nil, nil, err
	}
	return cert, key, nil
}

func (env *localEnviron) setupLocalMachineAgent(cons constraints.Value, possibleTools tools.List) error {
	dataDir := env.config.rootDir()
	// unpack the first tools into the agent dir.
	agentTools := possibleTools[0]
	logger.Debugf("tools: %#v", agentTools)
	// brutally abuse our knowledge of storage to directly open the file
	toolsUrl, err := url.Parse(agentTools.URL)
	if err != nil {
		return err
	}
	toolsLocation := filepath.Join(env.config.storageDir(), toolsUrl.Path)
	logger.Infof("tools location: %v", toolsLocation)
	toolsFile, err := os.Open(toolsLocation)
	defer toolsFile.Close()
	// Again, brutally abuse our knowledge here.

	// The tools that possible bootstrap tools are based on the
	// default series in the config.  However we are running potentially on a
	// different series.  When the machine agent is started, it will be
	// looking based on the current series, so we need to override the series
	// returned in the tools to be the current series.
	agentTools.Version.Series = version.CurrentSeries()
	err = agenttools.UnpackTools(dataDir, agentTools, toolsFile)

	machineId := "0" // Always machine 0
	tag := names.MachineTag(machineId)
	toolsDir := agenttools.SharedToolsDir(dataDir, agentTools.Version)
	logDir := env.config.logDir()
	logConfig := env.config.LoggingConfig()
	machineEnvironment := map[string]string{
		"USER": env.config.user,
		"HOME": os.Getenv("HOME"),
	}
	agentService := upstart.MachineAgentUpstartService(
		env.machineAgentServiceName(),
		toolsDir, dataDir, logDir, tag, machineId, logConfig, machineEnvironment)

	agentService.InitDir = upstartScriptLocation
	logger.Infof("installing service %s to %s", env.machineAgentServiceName(), agentService.InitDir)
	if err := agentService.Install(); err != nil {
		logger.Errorf("could not install machine agent service: %v", err)
		return err
	}
	return nil
}

func (env *localEnviron) findBridgeAddress() (string, error) {
	return getAddressForInterface(lxcBridgeName)
}

func (env *localEnviron) writeBootstrapAgentConfFile(secret string, cert, key []byte) (agent.Config, error) {
	tag := names.MachineTag("0")
	passwordHash := utils.PasswordHash(secret)
	// We don't check the existance of the CACert here as if it wasn't set, we
	// wouldn't get this far.
	cfg := env.config.Config
	caCert, _ := cfg.CACert()
	agentValues := map[string]string{
		agent.ProviderType:      env.config.Type(),
		agent.StorageDir:        env.config.storageDir(),
		agent.StorageAddr:       env.config.storageAddr(),
		agent.SharedStorageDir:  env.config.sharedStorageDir(),
		agent.SharedStorageAddr: env.config.sharedStorageAddr(),
	}
	// NOTE: the state address HAS to be localhost, otherwise the mongo
	// initialization fails.  There is some magic code somewhere in the mongo
	// connection code that treats connections from localhost as special, and
	// will raise unauthorized errors during the initialization if the caller
	// is not connected from localhost.
	stateAddress := fmt.Sprintf("localhost:%d", cfg.StatePort())
	apiAddress := fmt.Sprintf("localhost:%d", cfg.APIPort())
	config, err := agent.NewStateMachineConfig(
		agent.StateMachineConfigParams{
			AgentConfigParams: agent.AgentConfigParams{
				DataDir:        env.config.rootDir(),
				Tag:            tag,
				Password:       passwordHash,
				Nonce:          state.BootstrapNonce,
				StateAddresses: []string{stateAddress},
				APIAddresses:   []string{apiAddress},
				CACert:         caCert,
				Values:         agentValues,
			},
			StateServerCert: cert,
			StateServerKey:  key,
			StatePort:       cfg.StatePort(),
			APIPort:         cfg.APIPort(),
		})
	if err != nil {
		return nil, err
	}
	if err := config.Write(); err != nil {
		logger.Errorf("failed to write bootstrap agent file: %v", err)
		return nil, err
	}
	return config, nil
}

func (env *localEnviron) initialStateConfiguration(agentConfig agent.Config, cons constraints.Value) (*state.State, error) {
	timeout := state.DialOpts{60 * time.Second}
	bootstrapCfg, err := environs.BootstrapConfig(env.config.Config)
	if err != nil {
		return nil, err
	}
	st, err := agent.InitialStateConfiguration(agentConfig, bootstrapCfg, timeout)
	if err != nil {
		return nil, err
	}

	jobs := []state.MachineJob{state.JobManageEnviron, state.JobManageState}

	if err := bootstrap.ConfigureBootstrapMachine(
		st, cons, env.config.rootDir(), jobs, instance.Id(boostrapInstanceId), instance.HardwareCharacteristics{}); err != nil {
		st.Close()
		return nil, err
	}

	// Return an open state reference.
	return st, nil
}
