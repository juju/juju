// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"launchpad.net/juju-core/agent"
	coreCloudinit "launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/cloudinit/sshinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/factory"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
	"launchpad.net/juju-core/worker/terminationworker"
)

// boostrapInstanceId is just the name we give to the bootstrap machine.
// Using "localhost" because it is, and it makes sense.
const bootstrapInstanceId instance.Id = "localhost"

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

// localEnviron implements SupportsCustomSources.
var _ envtools.SupportsCustomSources = (*localEnviron)(nil)

type localEnviron struct {
	localMutex            sync.Mutex
	config                *environConfig
	name                  string
	sharedStorageListener net.Listener
	storageListener       net.Listener
	containerManager      container.Manager
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *localEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource(e.Storage(), storage.BaseToolsPath)}, nil
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

func (env *localEnviron) rsyslogConfPath() string {
	return fmt.Sprintf("/etc/rsyslog.d/25-juju-%s.conf", env.config.namespace())
}

// PrecheckInstance is specified in the environs.Prechecker interface.
func (*localEnviron) PrecheckInstance(series string, cons constraints.Value) error {
	return nil
}

// PrecheckContainer is specified in the environs.Prechecker interface.
func (*localEnviron) PrecheckContainer(series string, kind instance.ContainerType) error {
	// This check can either go away or be relaxed when the local
	// provider can do nested containers.
	return environs.NewContainersUnsupported("local provider does not support nested containers")
}

func ensureNotRoot() error {
	if checkIfRoot() {
		return fmt.Errorf("bootstrapping a local environment must not be done as root")
	}
	return nil
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	if err := ensureNotRoot(); err != nil {
		return err
	}
	privateKey, err := common.GenerateSystemSSHKey(env)
	if err != nil {
		return err
	}

	// Before we write the agent config file, we need to make sure the
	// instance is saved in the StateInfo.
	stateFileURL, err := bootstrap.CreateStateFile(env.Storage())
	if err != nil {
		return err
	}
	if err := bootstrap.SaveState(env.Storage(), &bootstrap.BootstrapState{
		StateInstances: []instance.Id{bootstrapInstanceId},
	}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		return err
	}

	vers := version.Current
	selectedTools, err := common.EnsureBootstrapTools(env, vers.Series, &vers.Arch)
	if err != nil {
		return err
	}

	mcfg := environs.NewBootstrapMachineConfig(stateFileURL, privateKey)
	mcfg.Tools = selectedTools[0]
	mcfg.DataDir = env.config.rootDir()
	mcfg.LogDir = env.config.logDir()
	mcfg.RsyslogConfPath = env.rsyslogConfPath()
	mcfg.CloudInitOutputLog = filepath.Join(mcfg.LogDir, "cloud-init-output.log")
	mcfg.DisablePackageCommands = true
	mcfg.MachineAgentServiceName = env.machineAgentServiceName()
	mcfg.MongoServiceName = env.mongoServiceName()
	mcfg.AgentEnvironment = map[string]string{
		agent.Namespace:         env.config.namespace(),
		agent.StorageDir:        env.config.storageDir(),
		agent.StorageAddr:       env.config.storageAddr(),
		agent.SharedStorageDir:  env.config.sharedStorageDir(),
		agent.SharedStorageAddr: env.config.sharedStorageAddr(),
	}
	if err := environs.FinishMachineConfig(mcfg, env.Config(), cons); err != nil {
		return err
	}
	// don't write proxy settings for local machine
	mcfg.AptProxySettings = osenv.ProxySettings{}
	mcfg.ProxySettings = osenv.ProxySettings{}
	cloudcfg := coreCloudinit.New()
	if err := cloudinit.ConfigureJuju(mcfg, cloudcfg); err != nil {
		return err
	}
	return finishBootstrap(mcfg, cloudcfg, ctx)
}

// finishBootstrap converts the machine config to cloud-config,
// converts that to a script, and then executes it locally.
//
// mcfg is supplied for testing purposes.
var finishBootstrap = func(mcfg *cloudinit.MachineConfig, cloudcfg *coreCloudinit.Config, ctx environs.BootstrapContext) error {
	script, err := sshinit.ConfigureScript(cloudcfg)
	if err != nil {
		return nil
	}
	cmd := exec.Command("sudo", "/bin/bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = ctx.Stdout()
	cmd.Stderr = ctx.Stderr()
	return cmd.Run()
}

// StateInfo is specified in the Environ interface.
func (env *localEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
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
	storage, err := filestorage.NewFileStorageWriter(dir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return nil, err
	}
	return httpstorage.Serve(address, storage)
}

// SetConfig is specified in the Environ interface.
func (env *localEnviron) SetConfig(cfg *config.Config) error {
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	env.localMutex.Lock()
	defer env.localMutex.Unlock()
	env.config = ecfg
	env.name = ecfg.Name()

	env.containerManager, err = factory.NewContainerManager(
		ecfg.container(),
		container.ManagerConfig{
			Name:   env.config.namespace(),
			LogDir: env.config.logDir(),
		})
	if err != nil {
		return err
	}

	// Here is the end of normal config setting.
	if ecfg.bootstrapped() {
		return nil
	}
	if err := ensureNotRoot(); err != nil {
		return err
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

	// We need the provider config to get the network bridge.
	config, err := providerInstance.newConfig(cfg)
	if err != nil {
		logger.Errorf("failed to create new environ config: %v", err)
		return err
	}
	networkBridge := config.networkBridge()
	bridgeAddress, err := getAddressForInterface(networkBridge)
	if err != nil {
		logger.Infof("configure a different bridge using 'network-bridge' in the config file")
		return fmt.Errorf("cannot find address of network-bridge: %q", networkBridge)
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, networkBridge)
	cfg, err = cfg.Apply(map[string]interface{}{
		"bootstrap-ip": bridgeAddress,
	})
	if err != nil {
		logger.Errorf("failed to apply new addresses to config: %v", err)
		return err
	}
	// Now recreate the config based on the settings with the bootstrap id.
	config, err = providerInstance.newConfig(cfg)
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

	series := possibleTools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", machineConfig.MachineId, series)
	machineConfig.Tools = possibleTools[0]
	machineConfig.MachineContainerType = env.config.container()
	logger.Debugf("tools: %#v", machineConfig.Tools)
	network := container.BridgeNetworkConfig(env.config.networkBridge())
	if err := environs.FinishMachineConfig(machineConfig, env.config.Config, cons); err != nil {
		return nil, nil, err
	}
	// TODO: evaluate the impact of setting the contstraints on the
	// machineConfig for all machines rather than just state server nodes.
	// This limiation is why the constraints are assigned directly here.
	machineConfig.Constraints = cons
	machineConfig.AgentEnvironment[agent.Namespace] = env.config.namespace()
	inst, hardware, err := env.containerManager.StartContainer(machineConfig, series, network)
	if err != nil {
		return nil, nil, err
	}
	return inst, hardware, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StopInstances(instances []instance.Instance) error {
	for _, inst := range instances {
		if inst.Id() == bootstrapInstanceId {
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
	if len(ids) == 0 {
		return nil, nil
	}
	insts, err := env.AllInstances()
	if err != nil {
		return nil, err
	}
	allInstances := make(map[instance.Id]instance.Instance)
	for _, inst := range insts {
		allInstances[inst.Id()] = inst
	}
	var found int
	insts = make([]instance.Instance, len(ids))
	for i, id := range ids {
		if inst, ok := allInstances[id]; ok {
			insts[i] = inst
			found++
		}
	}
	if found == 0 {
		insts, err = nil, environs.ErrNoInstances
	} else if found < len(ids) {
		err = environs.ErrPartialInstances
	} else {
		err = nil
	}
	return insts, err
}

// AllInstances is specified in the InstanceBroker interface.
func (env *localEnviron) AllInstances() (instances []instance.Instance, err error) {
	instances = append(instances, &localInstance{bootstrapInstanceId, env})
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
func (env *localEnviron) Storage() storage.Storage {
	return httpstorage.Client(env.config.storageAddr())
}

// Implements environs.BootstrapStorager.
func (env *localEnviron) EnableBootstrapStorage() error {
	return env.setupLocalStorage()
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy() error {
	// Kill all running instances. This must be done as
	// root, or listing/stopping containers will fail.
	if !checkIfRoot() {
		juju, err := exec.LookPath(os.Args[0])
		if err != nil {
			return err
		}
		args := append([]string{juju}, os.Args[1:]...)
		args = append(args, "-y")
		cmd := exec.Command("sudo", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	} else {
		containers, err := env.containerManager.ListContainers()
		if err != nil {
			return err
		}
		for _, inst := range containers {
			if err := env.containerManager.StopContainer(inst); err != nil {
				return err
			}
		}
		cmd := exec.Command(
			"pkill",
			fmt.Sprintf("-%d", terminationworker.TerminationSignal),
			"jujud",
		)
		return cmd.Run()
	}
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
	return providerInstance
}
