// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
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
	storageUnlocked    storage.Storage
}

var _ environs.Environ = (*maasEnviron)(nil)
var _ imagemetadata.SupportsCustomSources = (*maasEnviron)(nil)
var _ envtools.SupportsCustomSources = (*maasEnviron)(nil)

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

// Name is specified in the Environ interface.
func (env *maasEnviron) Name() string {
	return env.name
}

// Bootstrap is specified in the Environ interface.
func (env *maasEnviron) Bootstrap(ctx environs.BootstrapContext, cons constraints.Value) error {
	return common.Bootstrap(ctx, env, cons)
}

// StateInfo is specified in the Environ interface.
func (env *maasEnviron) StateInfo() (*state.Info, *api.Info, error) {
	return common.StateInfo(env)
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

	authClient, err := gomaasapi.NewAuthenticatedClient(ecfg.maasServer(), ecfg.maasOAuth(), apiVersion)
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
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		params.Add("tags", strings.Join(*cons.Tags, ","))
	}
	// TODO(bug 1212689): ignore root-disk constraint for now.
	if cons.RootDisk != nil {
		logger.Warningf("ignoring unsupported constraint 'root-disk'")
	}
	if cons.CpuPower != nil {
		logger.Warningf("ignoring unsupported constraint 'cpu-power'")
	}
	return params
}

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode(cons constraints.Value, possibleTools tools.List) (gomaasapi.MAASObject, *tools.Tools, error) {
	acquireParams := convertConstraints(cons)
	acquireParams.Add("agent_name", environ.ecfg().maasAgentName())
	var result gomaasapi.JSONObject
	var err error
	for a := shortAttempt.Start(); a.Next(); {
		client := environ.getMAASClient().GetSubObject("nodes/")
		result, err = client.CallPost("acquire", acquireParams)
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
	return `cat > /etc/network/eth0.config << EOF
iface eth0 inet manual

auto br0
iface br0 inet dhcp
  bridge_ports eth0
EOF
`
}

// linkBridgeInInterfaces adds the file created by createBridgeNetwork to the
// interfaces file.
func linkBridgeInInterfaces() string {
	return `sed -i "s/iface eth0 inet dhcp/source \/etc\/network\/eth0.config/" /etc/network/interfaces`
}

// StartInstance is specified in the InstanceBroker interface.
func (environ *maasEnviron) StartInstance(cons constraints.Value, possibleTools tools.List,
	machineConfig *cloudinit.MachineConfig) (instance.Instance, *instance.HardwareCharacteristics, error) {

	var inst *maasInstance
	var err error
	if node, tools, err := environ.acquireNode(cons, possibleTools); err != nil {
		return nil, nil, fmt.Errorf("cannot run instances: %v", err)
	} else {
		inst = &maasInstance{maasObject: &node, environ: environ}
		machineConfig.Tools = tools
	}
	defer func() {
		if err != nil {
			if err := environ.releaseInstance(inst); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	hostname, err := inst.DNSName()
	if err != nil {
		return nil, nil, err
	}
	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd()
	if err != nil {
		return nil, nil, err
	}
	if err := environs.FinishMachineConfig(machineConfig, environ.Config(), cons); err != nil {
		return nil, nil, err
	}
	// TODO(thumper): 2013-08-28 bug 1217614
	// The machine envronment config values are being moved to the agent config.
	// Explicitly specify that the lxc containers use the network bridge defined above.
	machineConfig.AgentEnvironment[agent.LxcBridge] = "br0"
	userdata, err := environs.ComposeUserData(
		machineConfig,
		runCmd,
		createBridgeNetwork(),
		linkBridgeInInterfaces(),
		"service networking restart",
	)
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	series := possibleTools.OneSeries()
	if err := environ.startNode(*inst.maasObject, series, userdata); err != nil {
		return nil, nil, err
	}
	logger.Debugf("started instance %q", inst.Id())
	// TODO(bug 1193998) - return instance hardware characteristics as well
	return inst, nil, nil
}

// StartInstance is specified in the InstanceBroker interface.
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
	filter.Add("agent_name", environ.ecfg().maasAgentName())
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

	idMap := make(map[instance.Id]instance.Instance)
	for _, instance := range instances {
		idMap[instance.Id()] = instance
	}

	result := make([]instance.Instance, len(ids))
	for index, id := range ids {
		result[index] = idMap[id]
	}

	if len(instances) < len(ids) {
		return result, environs.ErrPartialInstances
	}
	return result, nil
}

// AllInstances returns all the instance.Instance in this provider.
func (environ *maasEnviron) AllInstances() ([]instance.Instance, error) {
	return environ.instances(nil)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() storage.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

func (environ *maasEnviron) Destroy() error {
	return common.Destroy(environ)
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

// GetImageSources returns a list of sources which are used to search for simplestreams image metadata.
func (e *maasEnviron) GetImageSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseImagesPath)}, nil
}

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *maasEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}, nil
}
