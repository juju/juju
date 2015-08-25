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
	"github.com/juju/utils/proxy"
	"github.com/juju/utils/shell"
	"github.com/juju/utils/symlink"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/factory"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/httpstorage"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/service"
	servicecommon "github.com/juju/juju/service/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/terminationworker"
)

// boostrapInstanceId is just the name we give to the bootstrap machine.
// Using "localhost" because it is, and it makes sense.
const bootstrapInstanceId instance.Id = "localhost"

// localEnviron implements Environ.
var _ environs.Environ = (*localEnviron)(nil)

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

// SupportedArchitectures is specified on the EnvironCapability interface.
func (*localEnviron) SupportedArchitectures() ([]string, error) {
	localArch := arch.HostArch()
	return []string{localArch}, nil
}

func (*localEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement != "" {
		return fmt.Errorf("unknown placement directive: %s", placement)
	}
	return nil
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
func (env *localEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (string, string, environs.BootstrapFinalizer, error) {
	if err := ensureNotRoot(); err != nil {
		return "", "", nil, err
	}

	// Make sure there are tools available for the
	// host's architecture and series.
	if _, err := args.AvailableTools.Match(tools.Filter{
		Arch:   arch.HostArch(),
		Series: version.Current.Series,
	}); err != nil {
		return "", "", nil, err
	}

	cfg, err := env.Config().Apply(map[string]interface{}{
		// Record the bootstrap IP, so the containers know where to go for storage.
		"bootstrap-ip": env.bridgeAddress,
	})
	if err == nil {
		err = env.SetConfig(cfg)
	}
	if err != nil {
		logger.Errorf("failed to apply bootstrap-ip to config: %v", err)
		return "", "", nil, err
	}
	return arch.HostArch(), version.Current.Series, env.finishBootstrap, nil
}

// finishBootstrap converts the machine config to cloud-config,
// converts that to a script, and then executes it locally.
func (env *localEnviron) finishBootstrap(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig) error {
	icfg.InstanceId = bootstrapInstanceId
	icfg.DataDir = env.config.rootDir()
	icfg.LogDir = fmt.Sprintf("/var/log/juju-%s", env.config.namespace())
	icfg.CloudInitOutputLog = filepath.Join(icfg.DataDir, "cloud-init-output.log")

	// No JobManageNetworking added in order not to change the network
	// configuration of the user's machine.
	icfg.Jobs = []multiwatcher.MachineJob{multiwatcher.JobManageEnviron}

	icfg.MachineAgentServiceName = env.machineAgentServiceName()
	icfg.AgentEnvironment = map[string]string{
		agent.Namespace:   env.config.namespace(),
		agent.StorageDir:  env.config.storageDir(),
		agent.StorageAddr: env.config.storageAddr(),
		agent.LxcBridge:   env.config.networkBridge(),

		// The local provider only supports a single state server,
		// so we make the oplog size to a small value. This makes
		// the preallocation faster with no disadvantage.
		agent.MongoOplogSize: "1", // 1MB
	}

	if err := instancecfg.FinishInstanceConfig(icfg, env.Config()); err != nil {
		return errors.Trace(err)
	}

	// Since Juju's state machine is currently the host machine
	// for local providers, don't stomp on it.
	cfgAttrs := env.config.AllAttrs()
	if val, ok := cfgAttrs["enable-os-refresh-update"].(bool); !ok {
		logger.Infof("local provider; disabling refreshing OS updates.")
		icfg.EnableOSRefreshUpdate = false
	} else {
		icfg.EnableOSRefreshUpdate = val
	}
	if val, ok := cfgAttrs["enable-os-upgrade"].(bool); !ok {
		logger.Infof("local provider; disabling OS upgrades.")
		icfg.EnableOSUpgrade = false
	} else {
		icfg.EnableOSUpgrade = val
	}

	// don't write proxy or mirror settings for local machine
	icfg.AptProxySettings = proxy.Settings{}
	icfg.ProxySettings = proxy.Settings{}
	icfg.AptMirror = ""

	cloudcfg, err := cloudinit.New(icfg.Series)
	if err != nil {
		return errors.Trace(err)
	}
	cloudcfg.SetSystemUpdate(icfg.EnableOSRefreshUpdate)
	cloudcfg.SetSystemUpgrade(icfg.EnableOSUpgrade)

	// Since rsyslogd is restricted by apparmor to only write to /var/log/**
	// we now provide a symlink to the written file in the local log dir.
	// Also, we leave the old all-machines.log file in
	// /var/log/juju-{{namespace}} until we start the environment again. So
	// potentially remove it at the start of the cloud-init.
	localLogDir := filepath.Join(icfg.DataDir, "log")
	if err := os.RemoveAll(localLogDir); err != nil {
		return errors.Trace(err)
	}
	if err := symlink.New(icfg.LogDir, localLogDir); err != nil {
		return errors.Trace(err)
	}
	if err := os.Remove(icfg.CloudInitOutputLog); err != nil && !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	cloudcfg.AddScripts(
		fmt.Sprintf("rm -fr %s", icfg.LogDir),
		fmt.Sprintf("rm -f /var/spool/rsyslog/machine-0-%s", env.config.namespace()),
	)
	udata, err := cloudconfig.NewUserdataConfig(icfg, cloudcfg)
	if err != nil {
		return errors.Trace(err)
	}
	if err := udata.ConfigureJuju(); err != nil {
		return errors.Trace(err)
	}
	return executeCloudConfig(ctx, icfg, cloudcfg)
}

var executeCloudConfig = func(ctx environs.BootstrapContext, icfg *instancecfg.InstanceConfig, cloudcfg cloudinit.CloudConfig) error {
	// Finally, convert cloud-config to a script and execute it.
	configScript, err := cloudcfg.RenderScript()
	if err != nil {
		return nil
	}
	script := shell.DumpFileOnErrorScript(icfg.CloudInitOutputLog) + configScript
	cmd := exec.Command("sudo", "/bin/bash", "-s")
	cmd.Stdin = strings.NewReader(script)
	cmd.Stdout = ctx.GetStdout()
	cmd.Stderr = ctx.GetStderr()
	return cmd.Run()
}

// StateServerInstances is specified in the Environ interface.
func (env *localEnviron) StateServerInstances() ([]instance.Id, error) {
	agentsDir := filepath.Join(env.config.rootDir(), "agents")
	_, err := os.Stat(agentsDir)
	if os.IsNotExist(err) {
		return nil, environs.ErrNotBootstrapped
	}
	if err != nil {
		return nil, err
	}
	return []instance.Id{bootstrapInstanceId}, nil
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
		return errors.Trace(err)
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
	var imageURLGetter container.ImageURLGetter
	if containerType == instance.LXC {
		if useLxcClone, ok := cfg.LXCUseClone(); ok {
			managerConfig["use-clone"] = fmt.Sprint(useLxcClone)
		}
		if useLxcCloneAufs, ok := cfg.LXCUseCloneAUFS(); ok {
			managerConfig["use-aufs"] = fmt.Sprint(useLxcCloneAufs)
		}
		// For lxc containers, we cache image tarballs in the environment storage, so here
		// we construct a URL getter.
		if uuid, ok := ecfg.UUID(); ok {
			var caCert []byte = nil
			if cert, ok := cfg.CACert(); ok {
				caCert = []byte(cert)
			}
			imageURLGetter = container.NewImageURLGetter(ecfg.stateServerAddr(), uuid, caCert)
		}
	}
	env.containerManager, err = factory.NewContainerManager(
		containerType, managerConfig, imageURLGetter)
	if err != nil {
		return errors.Trace(err)
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
		return errors.Trace(err)
	}
	// Record the network bridge address and create a filestorage.
	if err := env.resolveBridgeAddress(cfg); err != nil {
		return errors.Trace(err)
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

// MaintainInstance is specified in the InstanceBroker interface.
func (*localEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

// StartInstance is specified in the InstanceBroker interface.
func (env *localEnviron) StartInstance(args environs.StartInstanceParams) (*environs.StartInstanceResult, error) {
	if args.InstanceConfig.HasNetworks() {
		return nil, fmt.Errorf("starting instances with networks is not supported yet.")
	}
	series := args.Tools.OneSeries()
	logger.Debugf("StartInstance: %q, %s", args.InstanceConfig.MachineId, series)
	args.InstanceConfig.Tools = args.Tools[0]

	args.InstanceConfig.MachineContainerType = env.config.container()
	logger.Debugf("tools: %#v", args.InstanceConfig.Tools)
	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, env.config.Config); err != nil {
		return nil, err
	}
	// TODO: evaluate the impact of setting the constraints on the
	// instanceConfig for all machines rather than just state server nodes.
	// This limitation is why the constraints are assigned directly here.
	args.InstanceConfig.Constraints = args.Constraints
	args.InstanceConfig.AgentEnvironment[agent.Namespace] = env.config.namespace()
	inst, hardware, err := createContainer(env, args)
	if err != nil {
		return nil, err
	}
	return &environs.StartInstanceResult{
		Instance: inst,
		Hardware: hardware,
	}, nil
}

// Override for testing.
var createContainer = func(env *localEnviron, args environs.StartInstanceParams) (instance.Instance, *instance.HardwareCharacteristics, error) {
	series := args.Tools.OneSeries()
	network := container.BridgeNetworkConfig(env.config.networkBridge(), 0, args.NetworkInfo)
	allowLoopMounts, _ := env.config.AllowLXCLoopMounts()
	isLXC := env.config.container() == instance.LXC
	storage := &container.StorageConfig{
		AllowMount: !isLXC || allowLoopMounts,
	}
	inst, hardware, err := env.containerManager.CreateContainer(args.InstanceConfig, series, network, storage)
	if err != nil {
		return nil, nil, err
	}
	return inst, hardware, nil
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
			juju, "destroy-environment", "-y", "--force", env.Config().Name(),
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
	// Stop the mongo database and machine agent. We log any errors but
	// do not fail, so that remaining "destroy" steps will still happen.
	//
	// We run through twice, since this races with the agent's teardown.
	// We only log errors on the second time through, since if an error
	// occurred, we sould expect it to be due to the service no longer
	// existing.
	for i := 0; i < 2; i++ {
		err = mongoRemoveService(env.config.namespace())
		if err != nil && !errors.IsNotFound(err) && i > 0 {
			logger.Errorf("while stopping mongod: %v", err)
		}
		svc, err := discoverService(env.machineAgentServiceName())
		if err == nil {
			if err := svc.Stop(); err != nil && i > 0 {
				logger.Errorf("while stopping machine agent: %v", err)
			}
			if err := svc.Remove(); err != nil && i > 0 {
				logger.Errorf("while disabling machine agent: %v", err)
			}
		}
	}

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

type agentService interface {
	Stop() error
	Remove() error
}

var mongoRemoveService = func(namespace string) error {
	return mongo.RemoveService(namespace)
}

var discoverService = func(name string) (agentService, error) {
	return service.DiscoverService(name, servicecommon.Conf{})
}

// OpenPorts is specified in the Environ interface.
func (env *localEnviron) OpenPorts(ports []network.PortRange) error {
	return fmt.Errorf("open ports not implemented")
}

// ClosePorts is specified in the Environ interface.
func (env *localEnviron) ClosePorts(ports []network.PortRange) error {
	return fmt.Errorf("close ports not implemented")
}

// Ports is specified in the Environ interface.
func (env *localEnviron) Ports() ([]network.PortRange, error) {
	return nil, nil
}

// Provider is specified in the Environ interface.
func (env *localEnviron) Provider() environs.EnvironProvider {
	return providerInstance
}
