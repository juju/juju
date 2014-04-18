// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"labix.org/v2/mgo/bson"
	"launchpad.net/gomaasapi"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/cloudinit"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/network"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/set"
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
	common.NopPrecheckerPolicy
	common.SupportsUnitPlacementPolicy

	name string

	// archMutex gates access to supportedArchitectures
	archMutex sync.Mutex
	// supportedArchitectures caches the architectures
	// for which images can be instantiated.
	supportedArchitectures []string

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

// SupportedArchitectures is specified on the EnvironCapability interface.
func (env *maasEnviron) SupportedArchitectures() ([]string, error) {
	env.archMutex.Lock()
	defer env.archMutex.Unlock()
	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}
	// Create a filter to get all images from our region and for the correct stream.
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Stream: env.Config().ImageStream(),
	})
	var err error
	env.supportedArchitectures, err = common.SupportedArchitectures(env, imageConstraint)
	return env.supportedArchitectures, err
}

// SupportNetworks is specified on the EnvironCapability interface.
func (env *maasEnviron) SupportNetworks() bool {
	caps, err := env.getCapabilities()
	if err != nil {
		logger.Debugf("getCapabilities failed: %v", err)
		return false
	}
	return caps.Contains(capNetworksManagement)
}

const capNetworksManagement = "networks-management"

// getCapabilities asks the MAAS server for its capabilities, if
// supported by the server.
func (env *maasEnviron) getCapabilities() (caps set.Strings, err error) {
	var result gomaasapi.JSONObject
	caps = set.NewStrings()

	for a := shortAttempt.Start(); a.Next(); {
		client := env.getMAASClient().GetSubObject("version/")
		result, err = client.CallGet("", nil)
		if err != nil {
			err0, ok := err.(*gomaasapi.ServerError)
			if ok && err0.StatusCode == 404 {
				return caps, fmt.Errorf("MAAS does not support version info")
			} else {
				return caps, err
			}
			continue
		}
	}
	if err != nil {
		return caps, err
	}
	info, err := result.GetMap()
	if err != nil {
		return caps, err
	}
	capsObj, ok := info["capabilities"]
	if !ok {
		return caps, fmt.Errorf("MAAS does not report capabilities")
	}
	items, err := capsObj.GetArray()
	if err != nil {
		return caps, err
	}
	for _, item := range items {
		val, err := item.GetString()
		if err != nil {
			return set.NewStrings(), err
		}
		caps.Add(val)
	}
	return caps, nil
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

// addNetworks converts networks include/exclude information into
// url.Values object suitable to pass to MAAS when acquiring a node.
func addNetworks(params url.Values, includeNetworks, excludeNetworks []string) {
	// Network Inclusion/Exclusion setup
	if len(includeNetworks) > 0 {
		for _, name := range includeNetworks {
			params.Add("networks", name)
		}
	}
	if len(excludeNetworks) > 0 {
		for _, name := range excludeNetworks {
			params.Add("not_networks", name)
		}
	}
}

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode(cons constraints.Value, includeNetworks, excludeNetworks []string, possibleTools tools.List) (gomaasapi.MAASObject, *tools.Tools, error) {
	acquireParams := convertConstraints(cons)
	addNetworks(acquireParams, includeNetworks, excludeNetworks)
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
	logger.Warningf("picked arbitrary tools %v", tools)
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

// setupNetworks prepares a []network.Info for the given instance.
func (environ *maasEnviron) setupNetworks(inst instance.Instance) ([]network.Info, error) {
	// Get the instance network interfaces first.
	interfaces, err := environ.getInstanceNetworkInterfaces(inst)
	if err != nil {
		return nil, fmt.Errorf("getInstanceNetworkInterfaces failed: %v", err)
	}
	logger.Debugf("node %q has network interfaces %v", inst.Id(), interfaces)
	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, fmt.Errorf("getInstanceNetworks failed: %v", err)
	}
	logger.Debugf("node %q has networks %v", inst.Id(), networks)
	var tempNetworkInfo []network.Info
	for _, netw := range networks {
		netCIDR := &net.IPNet{
			IP:   net.ParseIP(netw.IP),
			Mask: net.IPMask(net.ParseIP(netw.Mask)),
		}
		macs, err := environ.getNetworkMACs(netw.Name)
		if err != nil {
			return nil, fmt.Errorf("getNetworkMACs failed: %v", err)
		}
		logger.Debugf("network %q has MACs: %v", netw.Name, macs)
		for _, mac := range macs {
			if interfaceName, ok := interfaces[mac]; ok {
				tempNetworkInfo = append(tempNetworkInfo, network.Info{
					MACAddress:    mac,
					InterfaceName: interfaceName,
					CIDR:          netCIDR.String(),
					VLANTag:       netw.VLANTag,
					ProviderId:    network.Id(netw.Name),
					NetworkName:   netw.Name,
					IsVirtual:     netw.VLANTag > 0,
				})
			}
		}
	}
	// Verify we filled-in everything for all networks/interfaces
	// and drop incomplete records.
	var networkInfo []network.Info
	for _, info := range tempNetworkInfo {
		if info.ProviderId == "" || info.NetworkName == "" || info.CIDR == "" {
			logger.Warningf("ignoring network interface %q: missing network information", info.InterfaceName)
			continue
		}
		if info.MACAddress == "" || info.InterfaceName == "" {
			logger.Warningf("ignoring network %q: missing network interface information", info.ProviderId)
			continue
		}
		networkInfo = append(networkInfo, info)
	}
	logger.Debugf("node %q network information: %#v", inst.Id(), networkInfo)
	return networkInfo, nil
}

// StartInstance is specified in the InstanceBroker interface.
func (environ *maasEnviron) StartInstance(args environs.StartInstanceParams) (
	instance.Instance, *instance.HardwareCharacteristics, []network.Info, error,
) {
	var inst *maasInstance
	var err error
	node, tools, err := environ.acquireNode(
		args.Constraints,
		args.MachineConfig.IncludeNetworks,
		args.MachineConfig.ExcludeNetworks,
		args.Tools)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot run instances: %v", err)
	} else {
		inst = &maasInstance{maasObject: &node, environ: environ}
		args.MachineConfig.Tools = tools
	}
	defer func() {
		if err != nil {
			if err := environ.releaseInstance(inst); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()
	var networkInfo []network.Info
	if args.MachineConfig.HasNetworks() {
		networkInfo, err = environ.setupNetworks(inst)
		if err != nil {
			return nil, nil, nil, err
		}
	}

	hostname, err := inst.DNSName()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := environs.FinishMachineConfig(args.MachineConfig, environ.Config(), args.Constraints); err != nil {
		return nil, nil, nil, err
	}
	// TODO(thumper): 2013-08-28 bug 1217614
	// The machine envronment config values are being moved to the agent config.
	// Explicitly specify that the lxc containers use the network bridge defined above.
	args.MachineConfig.AgentEnvironment[agent.LxcBridge] = "br0"
	cloudcfg, err := newCloudinitConfig(hostname, networkInfo)
	if err != nil {
		return nil, nil, nil, err
	}
	userdata, err := environs.ComposeUserData(args.MachineConfig, cloudcfg)
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, nil, nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	series := args.Tools.OneSeries()
	if err := environ.startNode(*inst.maasObject, series, userdata); err != nil {
		return nil, nil, nil, err
	}
	logger.Debugf("started instance %q", inst.Id())
	// TODO(bug 1193998) - return instance hardware characteristics as well
	return inst, nil, networkInfo, nil
}

// newCloudinitConfig creates a cloudinit.Config structure
// suitable as a base for initialising a MAAS node.
func newCloudinitConfig(hostname string, networkInfo []network.Info) (*cloudinit.Config, error) {
	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd()
	if err != nil {
		return nil, err
	}
	cloudcfg := cloudinit.New()
	cloudcfg.SetAptUpdate(true)
	cloudcfg.AddPackage("bridge-utils")
	cloudcfg.AddScripts(
		"set -xe",
		runCmd,
		"ifdown eth0",
		createBridgeNetwork(),
		linkBridgeInInterfaces(),
		"ifup br0",
	)
	setupNetworksOnBoot(cloudcfg, networkInfo)
	return cloudcfg, nil
}

// setupNetworksOnBoot prepares a script to enable and start all given
// networks on boot.
func setupNetworksOnBoot(cloudcfg *cloudinit.Config, networkInfo []network.Info) {
	const ifaceConfig = `cat >> /etc/network/interfaces << EOF

auto %s
iface %s inet dhcp
EOF
`
	// We need the vlan package for the vconfig command.
	cloudcfg.AddPackage("vlan")

	script := func(line string, args ...interface{}) {
		cloudcfg.AddScripts(fmt.Sprintf(line, args...))
	}
	// Because eth0 is already configured in the br0 bridge, we
	// don't want to break that.
	configured := set.NewStrings("eth0")

	// In order to support VLANs, we need to include 8021q module
	// configure vconfig's set_name_type
	script("modprobe 8021q")
	script("sh -c 'grep -q 8021q /etc/modules || echo 8021q >> /etc/modules'")
	script("vconfig set_name_type DEV_PLUS_VID_NO_PAD")
	// Now prepare each interface configuration
	for _, info := range networkInfo {
		if !configured.Contains(info.InterfaceName) {
			// Register and bring up the physical interface.
			script(ifaceConfig, info.InterfaceName, info.InterfaceName)
			script("ifup %s", info.InterfaceName)
			configured.Add(info.InterfaceName)
		}
		if info.VLANTag > 0 {
			// We have a VLAN and need to create and register it after
			// its parent interface was brought up.
			script("vconfig add %s %d", info.InterfaceName, info.VLANTag)
			vlan := fmt.Sprintf("%s.%d", info.InterfaceName, info.VLANTag)
			script(ifaceConfig, vlan, vlan)
			script("ifup %s", vlan)
		}
	}
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

// networkDetails holds information about a MAAS network.
type networkDetails struct {
	Name        string
	IP          string
	Mask        string
	VLANTag     int
	Description string
}

// getInstanceNetworks returns a list of all MAAS networks for a given node.
func (environ *maasEnviron) getInstanceNetworks(inst instance.Instance) ([]networkDetails, error) {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	client := environ.getMAASClient().GetSubObject("networks")
	nodeId, err := maasObj.GetField("system_id")
	if err != nil {
		return nil, err
	}
	params := url.Values{"node": {nodeId}}
	json, err := client.CallGet("", params)
	if err != nil {
		return nil, err
	}
	jsonNets, err := json.GetArray()
	if err != nil {
		return nil, err
	}

	networks := make([]networkDetails, len(jsonNets))
	for i, jsonNet := range jsonNets {
		fields, err := jsonNet.GetMap()
		if err != nil {
			return nil, err
		}
		name, err := fields["name"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get name: %v", err)
		}
		ip, err := fields["ip"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get ip: %v", err)
		}
		netmask, err := fields["netmask"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get netmask: %v", err)
		}
		vlanTag := 0
		vlanTagField, ok := fields["vlan_tag"]
		if ok && !vlanTagField.IsNil() {
			// vlan_tag is optional, so assume it's 0 when missing or nil.
			vlanTagFloat, err := vlanTagField.GetFloat64()
			if err != nil {
				return nil, fmt.Errorf("cannot get vlan_tag: %v", err)
			}
			vlanTag = int(vlanTagFloat)
		}
		description, err := fields["description"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get description: %v", err)
		}

		networks[i] = networkDetails{
			Name:        name,
			IP:          ip,
			Mask:        netmask,
			VLANTag:     vlanTag,
			Description: description,
		}
	}
	return networks, nil
}

// getNetworkMACs returns all MAC addresses connected to the given
// network.
func (environ *maasEnviron) getNetworkMACs(networkName string) ([]string, error) {
	client := environ.getMAASClient().GetSubObject("networks").GetSubObject(networkName)
	json, err := client.CallGet("list_connected_macs", nil)
	if err != nil {
		return nil, err
	}
	jsonMACs, err := json.GetArray()
	if err != nil {
		return nil, err
	}

	macs := make([]string, len(jsonMACs))
	for i, jsonMAC := range jsonMACs {
		fields, err := jsonMAC.GetMap()
		if err != nil {
			return nil, err
		}
		macAddress, err := fields["mac_address"].GetString()
		if err != nil {
			return nil, fmt.Errorf("cannot get mac_address: %v", err)
		}
		macs[i] = macAddress
	}
	return macs, nil
}

// getInstanceNetworkInterfaces returns a map of interface MAC address
// to name for each network interface of the given instance, as
// discovered during the commissioning phase.
func (environ *maasEnviron) getInstanceNetworkInterfaces(inst instance.Instance) (map[string]string, error) {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	result, err := maasObj.CallGet("details", nil)
	if err != nil {
		return nil, err
	}
	// Get the node's lldp / lshw details discovered at commissioning.
	data, err := result.GetBytes()
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := bson.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	lshwData, ok := parsed["lshw"]
	if !ok {
		return nil, fmt.Errorf("no hardware information available for node %q", inst.Id())
	}
	lshwXML, ok := lshwData.([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid hardware information for node %q", inst.Id())
	}
	// Now we have the lshw XML data, parse it to extract and return NICs.
	return extractInterfaces(inst, lshwXML)
}

// extractInterfaces parses the XML output of lswh and extracts all
// network interfaces, returing a map MAC address to interface name.
func extractInterfaces(inst instance.Instance, lshwXML []byte) (map[string]string, error) {
	type Node struct {
		Id          string `xml:"id,attr"`
		Description string `xml:"description"`
		Serial      string `xml:"serial"`
		LogicalName string `xml:"logicalname"`
		Children    []Node `xml:"node"`
	}
	type List struct {
		Nodes []Node `xml:"node"`
	}
	var lshw List
	if err := xml.Unmarshal(lshwXML, &lshw); err != nil {
		return nil, fmt.Errorf("cannot parse lshw XML details for node %q: %v", inst.Id(), err)
	}
	interfaces := make(map[string]string)
	var processNodes func(nodes []Node)
	processNodes = func(nodes []Node) {
		for _, node := range nodes {
			if strings.HasPrefix(node.Id, "network") {
				interfaces[node.Serial] = node.LogicalName
			}
			processNodes(node.Children)
		}
	}
	processNodes(lshw.Nodes)
	return interfaces, nil
}
