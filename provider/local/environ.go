// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/juju/errors"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/agent/mongo"
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
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/arch"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils/proxy"
	"launchpad.net/juju-core/utils/shell"
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
	common.SupportsUnitPlacementPolicy

	localMutex       sync.Mutex
	config           *environConfig
	name             string
	bridgeAddress    string
	localStorage     storage.Storage
	storageListener  net.Listener
	containerManager container.Manager
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *localEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}, nil
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (*localEnviron) SupportedArchitectures() ([]string, error) {
	localArch := arch.HostArch()
	return []string{localArch}, nil
}

// SupportNetworks is specified on the EnvironCapability interface.
func (*localEnviron) SupportNetworks() bool {
	return false
}

func (*localEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		return fmt.Errorf("unknown placement directive: %s", placement)
	}
	return nil
}

// Name is specified in the Environ interface.
func (env *localEnviron) Name() string {
	return env.name
}

func (env *localEnviron) machineAgentServiceName() string {
	return "juju-agent-" + env.config.namespace()
}

func ensureNotRoot() error {
	if checkIfRoot() {
		return fmt.Errorf("bootstrapping a local environment must not be done as root")
	}
	return nil
}

// Bootstrap is specified in the Environ interface.
func (env *localEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) error {
	if err := ensureNotRoot(); err != nil {
		return err
	}
	privateKey, err := common.GenerateSystemSSHKey(env)
	if err != nil {
		return err
	}

	// Before we write the agent config file, we need to make sure the
	// instance is saved in the StateInfo.
	if err := bootstrap.SaveState(env.Storage(), &bootstrap.BootstrapState{
		StateInstances: []instance.Id{bootstrapInstanceId},
	}); err != nil {
		logger.Errorf("failed to save state instances: %v", err)
		return err
	}

	vers := version.Current
	selectedTools, err := common.EnsureBootstrapTools(ctx, env, vers.Series, &vers.Arch)
	if err != nil {
		return err
	}

	// Record the bootstrap IP, so the containers know where to go for storage.
	cfg, err := env.Config().Apply(map[string]interface{}{
		"bootstrap-ip": env.bridgeAddress,
	})
	if err == nil {
		err = env.SetConfig(cfg)
	}
	if err != nil {
		logger.Errorf("failed to apply bootstrap-ip to config: %v", err)
		return err
	}

	mcfg := environs.NewBootstrapMachineConfig(privateKey)
	mcfg.InstanceId = bootstrapInstanceId
	mcfg.Tools = selectedTools[0]
	mcfg.DataDir = env.config.rootDir()
	mcfg.LogDir = fmt.Sprintf("/var/log/juju-%s", env.config.namespace())
	mcfg.Jobs = []params.MachineJob{params.JobManageEnviron}
	mcfg.CloudInitOutputLog = filepath.Join(mcfg.DataDir, "cloud-init-output.log")
	mcfg.DisablePackageCommands = true
	mcfg.MachineAgentServiceName = env.machineAgentServiceName()
	mcfg.AgentEnvironment = map[string]string{
		agent.Namespace:   env.config.namespace(),
		agent.StorageDir:  env.config.storageDir(),
		agent.StorageAddr: env.config.storageAddr(),
	}
	if err := environs.FinishMachineConfig(mcfg, cfg, args.Constraints); err != nil {
		return err
	}
	// don't write proxy settings for local machine
	mcfg.AptProxySettings = proxy.Settings{}
	mcfg.ProxySettings = proxy.Settings{}
	cloudcfg := coreCloudinit.New()
	// Since rsyslogd is restricted by apparmor to only write to /var/log/**
	// we now provide a symlink to the written file in the local log dir.
	// Also, we leave the old all-machines.log file in
	// /var/log/juju-{{namespace}} until we start the environment again. So
	// potentially remove it at the start of the cloud-init.
	localLogDir := filepath.Join(mcfg.DataDir, "log")
	if err := os.RemoveAll(localLogDir); err != nil {
		return err
	}
	if err := os.Symlink(mcfg.LogDir, localLogDir); err != nil {
		return err
	}
	if err := os.Remove(mcfg.CloudInitOutputLog); err != nil && !os.IsNotExist(err) {
		return err
	}
	cloudcfg.AddScripts(
		fmt.Sprintf("rm -fr %s", mcfg.LogDir),
		fmt.Sprintf("rm -f /var/spool/rsyslog/machine-0-%s", env.config.namespace()),
	)
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
	configScript, err := sshinit.ConfigureScript(cloudcfg)
	if err != nil {
		return nil
	}
	script := shell.DumpFileOnErrorScript(mcfg.CloudInitOutputLog) + configScript
	cmd := exec.Command("sudo", "/bin/bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = ctx.GetStdout()
	cmd.Stderr = ctx.GetStderr()
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
	containerType := ecfg.container()
	managerConfig := container.ManagerConfig{
		container.ConfigName:   env.config.namespace(),
		container.ConfigLogDir: env.config.logDir(),
	}
	if containerType == instance.LXC {
		if useLxcClone, ok := cfg.LXCUseClone(); ok {
			managerConfig["use-clone"] = fmt.Sprint(useLxcClone)
		}
		if useLxcCloneAufs, ok := cfg.LXCUseCloneAUFS(); ok {
			managerConfig["use-aufs"] = fmt.Sprint(useLxcCloneAufs)
		}
	}
	env.containerManager, err = factory.NewContainerManager(
		containerType, managerConfig)
	if err != nil {
		return err
	}

	// When the localEnviron value is created on the client
	// side, the bootstrap-ip attribute will not exist,
	// because it is only set *within* the running
	// environment, not in the configuration created by
	// Prepare.
	//
	// When bootstrapIPAddress returns a non-empty string,
	// we know we are running server-side and thus must use
	// httpstorage.
	if addr := ecfg.bootstrapIPAddress(); addr != "" {
		env.bridgeAddress = addr
		return nil
	}
	// If we get to here, it is because we haven't yet bootstrapped an
	// environment, and saved the config in it, or we are running a command
	// from the command line, so it is ok to work on the assumption that we
	// have direct access to the directories.
	if err := env.config.createDirs(); err != nil {
		return err
	}
	// Record the network bridge address and create a filestorage.
	if err := env.resolveBridgeAddress(cfg); err != nil {
		return err
	}
	return env.setLocalStorage()
}

// resolveBridgeAddress finishes up the setup of the environment in
// situations where there is no machine agent running yet.
func (env *localEnviron) resolveBridgeAddress(cfg *config.Config) error {
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
		return fmt.Errorf("cannot find address of network-bridge: %q: %v", networkBridge, err)
	}
	logger.Debugf("found %q as address for %q", bridgeAddress, networkBridge)
	env.bridgeAddress = bridgeAddress
	return nil
}

// setLocalStorage creates a filestorage so tools can
// be synced and so forth without having a machine agent
// running.
func (env *localEnviron) setLocalStorage() error {
	storage, err := filestorage.NewFileStorageWriter(env.config.storageDir())
	if err != nil {
		return err
	}
	env.localStorage = storage
	return nil
}

var unsupportedConstraints = []string{
	constraints.CpuCores,
	constraints.CpuPower,
	constraints.InstanceType,
	constraints.Tags,
}

// ConstraintsValidator is defined on the Environs interface.
func (env *localEnviron) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := env.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	return validator, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StartInstance(args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, []network.Info, error) {
	if args.MachineConfig.HasNetworks() {
		return nil, nil, nil, fmt.Errorf("starting instances with networks is not supported yet.")
	}
	series := args.Tools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", args.MachineConfig.MachineId, series)
	args.MachineConfig.Tools = args.Tools[0]
	args.MachineConfig.MachineContainerType = env.config.container()
	logger.Debugf("tools: %#v", args.MachineConfig.Tools)
	network := container.BridgeNetworkConfig(env.config.networkBridge())
	if err := environs.FinishMachineConfig(args.MachineConfig, env.config.Config, args.Constraints); err != nil {
		return nil, nil, nil, err
	}
	// TODO: evaluate the impact of setting the contstraints on the
	// machineConfig for all machines rather than just state server nodes.
	// This limiation is why the constraints are assigned directly here.
	args.MachineConfig.Constraints = args.Constraints
	args.MachineConfig.AgentEnvironment[agent.Namespace] = env.config.namespace()
	inst, hardware, err := env.containerManager.CreateContainer(args.MachineConfig, series, network)
	if err != nil {
		return nil, nil, nil, err
	}
	return inst, hardware, nil, nil
}

// StopInstances is specified in the InstanceBroker interface.
func (env *localEnviron) StopInstances(ids ...instance.Id) error {
	for _, id := range ids {
		if id == bootstrapInstanceId {
			return fmt.Errorf("cannot stop the bootstrap instance")
		}
		if err := env.containerManager.DestroyContainer(id); err != nil {
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

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network. This is not supported on the
// local provider.
func (*localEnviron) AllocateAddress(_ instance.Id, _ network.Id) (instance.Address, error) {
	return instance.Address{}, errors.NotSupportedf("AllocateAddress")
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
	// localStorage is non-nil if we're running from the CLI
	if env.localStorage != nil {
		return env.localStorage
	}
	return httpstorage.Client(env.config.storageAddr())
}

// Destroy is specified in the Environ interface.
func (env *localEnviron) Destroy() error {
	// If bootstrap failed, for example because the user
	// lacks sudo rights, then the agents won't have been
	// installed. If that's the case, we can just remove
	// the data-dir and exit.
	agentsDir := filepath.Join(env.config.rootDir(), "agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		// If we can't remove the root dir, then continue
		// and attempt as root anyway.
		if os.RemoveAll(env.config.rootDir()) == nil {
			return nil
		}
	}
	if !checkIfRoot() {
		juju, err := exec.LookPath(os.Args[0])
		if err != nil {
			return err
		}
		args := []string{
			"env", osenv.JujuHomeEnvKey + "=" + osenv.JujuHome(),
			juju, "destroy-environment", "-y", "--force", env.Name(),
		}
		cmd := exec.Command("sudo", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	// Kill all running instances. This must be done as
	// root, or listing/stopping containers will fail.
	containers, err := env.containerManager.ListContainers()
	if err != nil {
		return err
	}
	for _, inst := range containers {
		if err := env.containerManager.DestroyContainer(inst.Id()); err != nil {
			return err
		}
	}
	cmd := exec.Command(
		"pkill",
		fmt.Sprintf("-%d", terminationworker.TerminationSignal),
		"-f", filepath.Join(regexp.QuoteMeta(env.config.rootDir()), ".*", "jujud"),
	)
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			// Exit status 1 means no processes were matched:
			// we don't consider this an error here.
			if err.ProcessState.Sys().(syscall.WaitStatus).ExitStatus() != 1 {
				return errors.Annotate(err, "failed to kill jujud")
			}
		}
	}
	// Stop the mongo database and machine agent. It's possible that the
	// service doesn't exist or is not running, so don't check the error.
	mongo.RemoveService(env.config.namespace())
	upstart.NewService(env.machineAgentServiceName()).StopAndRemove()

	// Finally, remove the data-dir.
	if err := os.RemoveAll(env.config.rootDir()); err != nil && !os.IsNotExist(err) {
		// Before we return the error, just check to see if the directory is
		// there. There is a race condition with the agent with the removing
		// of the directory, and due to a bug
		// (https://code.google.com/p/go/issues/detail?id=7776) the
		// os.IsNotExist error isn't always returned.
		if _, statErr := os.Stat(env.config.rootDir()); os.IsNotExist(statErr) {
			return nil
		}
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
	return providerInstance
}
