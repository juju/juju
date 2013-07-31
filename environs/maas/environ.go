// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sync"
	"time"

	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

const (
	// We're using v1.0 of the MAAS API.
	apiVersion = "1.0"
)

// A request may fail to due "eventual consistency" semantics, which
// should resolve fairly quickly.  A request may also fail due to a slow
// state transition (for instance an instance taking a while to release
// a security group after termination).  The former failure mode is
// dealt with by shortAttempt, the latter by LongAttempt.
var shortAttempt = utils.AttemptStrategy{
	Total: 5 * time.Second,
	Delay: 200 * time.Millisecond,
}

type maasEnviron struct {
	name string

	// ecfgMutex protects the *Unlocked fields below.
	ecfgMutex sync.Mutex

	ecfgUnlocked       *maasEnvironConfig
	maasClientUnlocked *gomaasapi.MAASObject
	storageUnlocked    environs.Storage
}

var _ environs.Environ = (*maasEnviron)(nil)

func NewEnviron(cfg *config.Config) (*maasEnviron, error) {
	env := new(maasEnviron)
	err := env.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	env.name = cfg.Name()
	env.storageUnlocked = NewStorage(env)
	return env, nil
}

func (env *maasEnviron) Name() string {
	return env.name
}

// startBootstrapNode starts the juju bootstrap node for this environment.
func (env *maasEnviron) startBootstrapNode(cons constraints.Value) (instance.Instance, error) {
	// The bootstrap instance gets machine id "0".  This is not related to
	// instance ids or MAAS system ids.  Juju assigns the machine ID.
	const machineID = "0"

	// Create an empty bootstrap state file so we can get its URL.
	// If will be updated with the instance id and hardware characteristics
	// after the bootstrap instance is started.
	stateFileURL, err := environs.CreateStateFile(env.Storage())
	if err != nil {
		return nil, err
	}

	logger.Debugf("bootstrapping environment %q", env.Name())
	possibleTools, err := environs.FindBootstrapTools(env, cons)
	if err != nil {
		return nil, err
	}
	err = environs.CheckToolsSeries(possibleTools, env.Config().DefaultSeries())
	if err != nil {
		return nil, err
	}

	machineConfig := environs.NewBootstrapMachineConfig(machineID, stateFileURL)
	inst, err := env.internalStartInstance(cons, possibleTools, machineConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot start bootstrap instance: %v", err)
	}
	return inst, nil
}

// Bootstrap is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (env *maasEnviron) Bootstrap(cons constraints.Value) error {

	if err := environs.VerifyBootstrapInit(env, shortAttempt); err != nil {
		return err
	}

	inst, err := env.startBootstrapNode(cons)
	if err != nil {
		return err
	}
	// TODO(wallyworld) add hardware characteristics to BootstrapState
	err = environs.SaveState(
		env.Storage(),
		&environs.BootstrapState{StateInstances: []instance.Id{inst.Id()}})
	if err != nil {
		err2 := env.releaseInstance(inst)
		if err2 != nil {
			// Failure upon failure.  Log it, but return the
			// original error.
			logger.Errorf("cannot release failed bootstrap instance: %v", err2)
		}
		return fmt.Errorf("cannot save state: %v", err)
	}

	// TODO make safe in the case of racing Bootstraps
	// If two Bootstraps are called concurrently, there's
	// no way to make sure that only one succeeds.
	return nil
}

// StateInfo is specified in the Environ interface.
func (env *maasEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return environs.StateInfo(env)
}

// ecfg returns the environment's maasEnvironConfig, and protects it with a
// mutex.
func (env *maasEnviron) ecfg() *maasEnvironConfig {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.ecfgUnlocked
}

// Config is specified in the Environ interface.
func (env *maasEnviron) Config() *config.Config {
	return env.ecfg().Config
}

// SetConfig is specified in the Environ interface.
func (env *maasEnviron) SetConfig(cfg *config.Config) error {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	// The new config has already been validated by itself, but now we
	// validate the transition from the old config to the new.
	var oldCfg *config.Config
	if env.ecfgUnlocked != nil {
		oldCfg = env.ecfgUnlocked.Config
	}
	cfg, err := env.Provider().Validate(cfg, oldCfg)
	if err != nil {
		return err
	}

	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return err
	}

	env.ecfgUnlocked = ecfg

	authClient, err := gomaasapi.NewAuthenticatedClient(ecfg.MAASServer(), ecfg.MAASOAuth(), apiVersion)
	if err != nil {
		return err
	}
	env.maasClientUnlocked = gomaasapi.NewMAAS(*authClient)

	return nil
}

// getMAASClient returns a MAAS client object to use for a request, in a
// lock-protected fashion.
func (env *maasEnviron) getMAASClient() *gomaasapi.MAASObject {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()

	return env.maasClientUnlocked
}

// convertConstraints converts the given constraints into an url.Values
// object suitable to pass to MAAS when acquiring a node.
// CpuPower is ignored because it cannot translated into something
// meaningful for MAAS right now.
func convertConstraints(cons constraints.Value) url.Values {
	params := url.Values{}
	if cons.Arch != nil {
		params.Add("arch", *cons.Arch)
	}
	if cons.CpuCores != nil {
		params.Add("cpu_count", fmt.Sprintf("%d", *cons.CpuCores))
	}
	if cons.Mem != nil {
		params.Add("mem", fmt.Sprintf("%d", *cons.Mem))
	}
	if cons.CpuPower != nil {
		logger.Warningf("ignoring unsupported constraint 'cpu-power'")
	}
	return params
}

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode(cons constraints.Value, possibleTools tools.List) (gomaasapi.MAASObject, *tools.Tools, error) {
	constraintsParams := convertConstraints(cons)
	var result gomaasapi.JSONObject
	var err error
	for a := shortAttempt.Start(); a.Next(); {
		client := environ.getMAASClient().GetSubObject("nodes/")
		result, err = client.CallPost("acquire", constraintsParams)
		if err == nil {
			break
		}
	}
	if err != nil {
		return gomaasapi.MAASObject{}, nil, err
	}
	node, err := result.GetMAASObject()
	if err != nil {
		msg := fmt.Errorf("unexpected result from 'acquire' on MAAS API: %v", err)
		return gomaasapi.MAASObject{}, nil, msg
	}
	tools := possibleTools[0]
	logger.Warningf("picked arbitrary tools %q", tools)
	return node, tools, nil
}

// startNode installs and boots a node.
func (environ *maasEnviron) startNode(node gomaasapi.MAASObject, series string, userdata []byte) error {
	userDataParam := base64.StdEncoding.EncodeToString(userdata)
	params := url.Values{
		"distro_series": {series},
		"user_data":     {userDataParam},
	}
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	err := fmt.Errorf("(no error)")
	for a := shortAttempt.Start(); a.Next() && err != nil; {
		_, err = node.CallPost("start", params)
	}
	return err
}

// createBridgeNetwork returns a string representing the upstart command to
// create a bridged eth0.
func createBridgeNetwork() string {
	return `cat > /etc/network/interfaces << EOF
auto lo
iface lo inet loopback

auto eth0
iface eth0 inet manual

auto br0
iface br0 inet dhcp
  bridge_ports eth0
EOF
`
}

// internalStartInstance allocates and starts a MAAS node.  It is used both
// for the implementation of StartInstance, and to initialize the bootstrap
// node.
// The instance will be set up for the same series for which you pass tools.
// All tools in possibleTools must be for the same series.
// machineConfig will be filled out with further details, but should contain
// MachineID, MachineNonce, StateInfo, and APIInfo.
// TODO(bug 1199847): Some of this work can be shared between providers.
func (environ *maasEnviron) internalStartInstance(cons constraints.Value, possibleTools tools.List, machineConfig *cloudinit.MachineConfig) (_ *maasInstance, err error) {
	series := possibleTools.Series()
	if len(series) != 1 {
		panic(fmt.Errorf("should have gotten tools for one series, got %v", series))
	}
	var instance *maasInstance
	if node, tools, err := environ.acquireNode(cons, possibleTools); err != nil {
		return nil, fmt.Errorf("cannot run instances: %v", err)
	} else {
		instance = &maasInstance{&node, environ}
		machineConfig.Tools = tools
	}
	defer func() {
		if err != nil {
			if err := environ.releaseInstance(instance); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	hostname, err := instance.DNSName()
	if err != nil {
		return nil, err
	}
	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd()
	if err != nil {
		return nil, err
	}
	if err := environs.FinishMachineConfig(machineConfig, environ.Config(), cons); err != nil {
		return nil, err
	}
	// Explicitly specify that the lxc containers use the network bridge defined above.
	machineConfig.MachineEnvironment[osenv.JujuLxcBridge] = "br0"
	userdata, err := environs.ComposeUserData(
		machineConfig,
		runCmd,
		createBridgeNetwork(),
		"service networking restart",
	)
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	if err := environ.startNode(*instance.maasObject, series[0], userdata); err != nil {
		return nil, err
	}
	logger.Debugf("started instance %q", instance.Id())
	return instance, nil
}

// StartInstance is specified in the Environ interface.
// TODO(bug 1199847): This work can be shared between providers.
func (environ *maasEnviron) StartInstance(machineID, machineNonce string, series string, cons constraints.Value,
	stateInfo *state.Info, apiInfo *api.Info) (instance.Instance, *instance.HardwareCharacteristics, error) {
	possibleTools, err := environs.FindInstanceTools(environ, series, cons)
	if err != nil {
		return nil, nil, err
	}
	err = environs.CheckToolsSeries(possibleTools, series)
	if err != nil {
		return nil, nil, err
	}
	machineConfig := environs.NewMachineConfig(machineID, machineNonce, stateInfo, apiInfo)
	// TODO(bug 1193998) - return instance hardware characteristics as well
	inst, err := environ.internalStartInstance(cons, possibleTools, machineConfig)
	return inst, nil, err
}

// StopInstances is specified in the Environ interface.
func (environ *maasEnviron) StopInstances(instances []instance.Instance) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(instances) == 0 {
		return nil
	}
	// Tell MAAS to release each of the instances.  If there are errors,
	// return only the first one (but release all instances regardless).
	// Note that releasing instances also turns them off.
	var firstErr error
	for _, instance := range instances {
		err := environ.releaseInstance(instance)
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// releaseInstance releases a single instance.
func (environ *maasEnviron) releaseInstance(inst instance.Instance) error {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	_, err := maasObj.CallPost("release", nil)
	if err != nil {
		logger.Debugf("error releasing instance %v", maasInst)
	}
	return err
}

// instances calls the MAAS API to list nodes.  The "ids" slice is a filter for
// specific instance IDs.  Due to how this works in the HTTP API, an empty
// "ids" matches all instances (not none as you might expect).
func (environ *maasEnviron) instances(ids []instance.Id) ([]instance.Instance, error) {
	nodeListing := environ.getMAASClient().GetSubObject("nodes")
	filter := getSystemIdValues(ids)
	listNodeObjects, err := nodeListing.CallGet("list", filter)
	if err != nil {
		return nil, err
	}
	listNodes, err := listNodeObjects.GetArray()
	if err != nil {
		return nil, err
	}
	instances := make([]instance.Instance, len(listNodes))
	for index, nodeObj := range listNodes {
		node, err := nodeObj.GetMAASObject()
		if err != nil {
			return nil, err
		}
		instances[index] = &maasInstance{
			maasObject: &node,
			environ:    environ,
		}
	}
	return instances, nil
}

// Instances returns the instance.Instance objects corresponding to the given
// slice of instance.Id.  The error is ErrNoInstances if no instances
// were found.
func (environ *maasEnviron) Instances(ids []instance.Id) ([]instance.Instance, error) {
	if len(ids) == 0 {
		// This would be treated as "return all instances" below, so
		// treat it as a special case.
		// The interface requires us to return this particular error
		// if no instances were found.
		return nil, environs.ErrNoInstances
	}
	instances, err := environ.instances(ids)
	if err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return nil, environs.ErrNoInstances
	}
	if len(ids) != len(instances) {
		return instances, environs.ErrPartialInstances
	}
	return instances, nil
}

// AllInstances returns all the instance.Instance in this provider.
func (environ *maasEnviron) AllInstances() ([]instance.Instance, error) {
	return environ.instances(nil)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() environs.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

// PublicStorage is defined by the Environ interface.
func (env *maasEnviron) PublicStorage() environs.StorageReader {
	// MAAS does not have a shared storage.
	return environs.EmptyStorage
}

func (environ *maasEnviron) Destroy(ensureInsts []instance.Instance) error {
	logger.Debugf("destroying environment %q", environ.name)
	insts, err := environ.AllInstances()
	if err != nil {
		return fmt.Errorf("cannot get instances: %v", err)
	}
	found := make(map[instance.Id]bool)
	for _, inst := range insts {
		found[inst.Id()] = true
	}

	// Add any instances we've been told about but haven't yet shown
	// up in the instance list.
	for _, inst := range ensureInsts {
		id := inst.Id()
		if !found[id] {
			insts = append(insts, inst)
			found[id] = true
		}
	}
	err = environ.StopInstances(insts)
	if err != nil {
		return err
	}

	return environ.Storage().RemoveAll()
}

// MAAS does not do firewalling so these port methods do nothing.
func (*maasEnviron) OpenPorts([]instance.Port) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (*maasEnviron) ClosePorts([]instance.Port) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (*maasEnviron) Ports() ([]instance.Port, error) {
	logger.Debugf("unimplemented Ports() called")
	return []instance.Port{}, nil
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	return &providerInstance
}
