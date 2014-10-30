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

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
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

// Bootstrap is specified in the Environ interface.
func (env *maasEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	return common.Bootstrap(ctx, env, args)
}

// StateServerInstances is specified in the Environ interface.
func (env *maasEnviron) StateServerInstances() ([]instance.Id, error) {
	return common.ProviderStateInstances(env, env.Storage())
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
	bootImages, err := env.allBootImages()
	if err != nil {
		logger.Debugf("error querying boot-images: %v", err)
		logger.Debugf("falling back to listing nodes")
		supportedArchitectures, err := env.nodeArchitectures()
		if err != nil {
			return nil, err
		}
		env.supportedArchitectures = supportedArchitectures
	} else {
		var architectures set.Strings
		for _, image := range bootImages {
			architectures.Add(image.architecture)
		}
		env.supportedArchitectures = architectures.SortedValues()
	}
	return env.supportedArchitectures, nil
}

// allBootImages queries MAAS for all of the boot-images across
// all registered nodegroups.
func (env *maasEnviron) allBootImages() ([]bootImage, error) {
	nodegroups, err := env.getNodegroups()
	if err != nil {
		return nil, err
	}
	var allBootImages []bootImage
	var seen set.Strings
	for _, nodegroup := range nodegroups {
		bootImages, err := env.nodegroupBootImages(nodegroup)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get boot images for nodegroup %v", nodegroup)
		}
		for _, image := range bootImages {
			str := fmt.Sprint(image)
			if seen.Contains(str) {
				continue
			}
			seen.Add(str)
			allBootImages = append(allBootImages, image)
		}
	}
	return allBootImages, nil
}

// getNodegroups returns the UUID corresponding to each nodegroup
// in the MAAS installation.
func (env *maasEnviron) getNodegroups() ([]string, error) {
	nodegroupsListing := env.getMAASClient().GetSubObject("nodegroups")
	nodegroupsResult, err := nodegroupsListing.CallGet("list", nil)
	if err != nil {
		return nil, err
	}
	list, err := nodegroupsResult.GetArray()
	if err != nil {
		return nil, err
	}
	nodegroups := make([]string, len(list))
	for i, obj := range list {
		nodegroup, err := obj.GetMap()
		if err != nil {
			return nil, err
		}
		uuid, err := nodegroup["uuid"].GetString()
		if err != nil {
			return nil, err
		}
		nodegroups[i] = uuid
	}
	return nodegroups, nil
}

type bootImage struct {
	architecture string
	release      string
}

// nodegroupBootImages returns the set of boot-images for the specified nodegroup.
func (env *maasEnviron) nodegroupBootImages(nodegroupUUID string) ([]bootImage, error) {
	nodegroupObject := env.getMAASClient().GetSubObject("nodegroups").GetSubObject(nodegroupUUID)
	bootImagesObject := nodegroupObject.GetSubObject("boot-images/")
	result, err := bootImagesObject.CallGet("", nil)
	if err != nil {
		return nil, err
	}
	list, err := result.GetArray()
	if err != nil {
		return nil, err
	}
	var bootImages []bootImage
	for _, obj := range list {
		bootimage, err := obj.GetMap()
		if err != nil {
			return nil, err
		}
		arch, err := bootimage["architecture"].GetString()
		if err != nil {
			return nil, err
		}
		release, err := bootimage["release"].GetString()
		if err != nil {
			return nil, err
		}
		bootImages = append(bootImages, bootImage{
			architecture: arch,
			release:      release,
		})
	}
	return bootImages, nil
}

// nodeArchitectures returns the architectures of all
// available nodes in the system.
//
// Note: this should only be used if we cannot query
// boot-images.
func (env *maasEnviron) nodeArchitectures() ([]string, error) {
	filter := make(url.Values)
	filter.Add("status", gomaasapi.NodeStatusDeclared)
	filter.Add("status", gomaasapi.NodeStatusCommissioning)
	filter.Add("status", gomaasapi.NodeStatusReady)
	filter.Add("status", gomaasapi.NodeStatusReserved)
	filter.Add("status", gomaasapi.NodeStatusAllocated)
	allInstances, err := env.instances(filter)
	if err != nil {
		return nil, err
	}
	var architectures set.Strings
	for _, inst := range allInstances {
		inst := inst.(*maasInstance)
		arch, _, err := inst.architecture()
		if err != nil {
			return nil, err
		}
		architectures.Add(arch)
	}
	return architectures.SortedValues(), nil
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

func (env *maasEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	// We treat all placement directives as maas-name.
	return nil
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
			if err, ok := err.(*gomaasapi.ServerError); ok && err.StatusCode == 404 {
				return caps, fmt.Errorf("MAAS does not support version info")
			}
			return caps, err
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
		// Note: Juju and MAAS use the same architecture names.
		// MAAS also accepts a subarchitecture (e.g. "highbank"
		// for ARM), which defaults to "generic" if unspecified.
		params.Add("arch", *cons.Arch)
	}
	if cons.CpuCores != nil {
		params.Add("cpu_count", fmt.Sprintf("%d", *cons.CpuCores))
	}
	if cons.Mem != nil {
		params.Add("mem", fmt.Sprintf("%d", *cons.Mem))
	}
	if cons.Tags != nil && len(*cons.Tags) > 0 {
		tags, notTags := parseTags(*cons.Tags)
		if len(tags) > 0 {
			params.Add("tags", strings.Join(tags, ","))
		}
		if len(notTags) > 0 {
			params.Add("not_tags", strings.Join(notTags, ","))
		}
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

// parseTags parses a tags constraints, splitting it into a positive
// and negative tags to pass to MAAS. Positive tags have no prefix,
// negative tags have a "^" prefix. All spaces inside the rawTags are
// stripped before parsing.
func parseTags(rawTags []string) (tags, notTags []string) {
	for _, tag := range rawTags {
		tag = strings.Replace(tag, " ", "", -1)
		if len(tag) == 0 {
			continue
		}
		if strings.HasPrefix(tag, "^") {
			notTags = append(notTags, strings.TrimPrefix(tag, "^"))
		} else {
			tags = append(tags, tag)
		}
	}
	return tags, notTags
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
func (environ *maasEnviron) acquireNode(nodeName string, cons constraints.Value, includeNetworks, excludeNetworks []string) (gomaasapi.MAASObject, error) {
	acquireParams := convertConstraints(cons)
	addNetworks(acquireParams, includeNetworks, excludeNetworks)
	acquireParams.Add("agent_name", environ.ecfg().maasAgentName())
	if nodeName != "" {
		acquireParams.Add("name", nodeName)
	} else if cons.Arch == nil {
		// TODO(axw) 2014-08-18 #1358219
		// We should be requesting preferred
		// architectures if unspecified, like
		// in the other providers.
		//
		// This is slightly complicated in MAAS
		// as there are a finite number of each
		// architecture; preference may also
		// conflict with other constraints, such
		// as tags. Thus, a preference becomes a
		// demand (which may fail) if not handled
		// properly.
		logger.Warningf(
			"no architecture was specified, acquiring an arbitrary node",
		)
	}

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
		return gomaasapi.MAASObject{}, err
	}
	node, err := result.GetMAASObject()
	if err != nil {
		err := errors.Annotate(err, "unexpected result from 'acquire' on MAAS API")
		return gomaasapi.MAASObject{}, err
	}
	return node, nil
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

// restoreInterfacesFiles returns a string representing the upstart command to
// revert MAAS changes to interfaces file.
func restoreInterfacesFiles(iface string) string {
	return fmt.Sprintf(`mkdir -p etc/network/interfaces.d
cat > /etc/network/interfaces.d/%s.cfg << EOF
# The primary network interface
auto %s
iface %s inet dhcp
EOF
sed -i '/auto %s/{N;s/auto %s\niface %s inet dhcp//}' /etc/network/interfaces
cat >> /etc/network/interfaces << EOF
# Source interfaces
# Please check /etc/network/interfaces.d before changing this file
# as interfaces may have been defined in /etc/network/interfaces.d
# NOTE: the primary ethernet device is defined in
# /etc/network/interfaces.d/%s.cfg
# See LP: #1262951
source /etc/network/interfaces.d/*.cfg
EOF
`, iface, iface, iface, iface, iface, iface, iface)
}

// createBridgeNetwork returns a string representing the upstart command to
// create a bridged interface.
func createBridgeNetwork(iface string) string {
	return fmt.Sprintf(`cat > /etc/network/interfaces.d/br0.cfg << EOF
auto br0
iface br0 inet dhcp
  bridge_ports %s
EOF
sed -i 's/iface %s inet dhcp/iface %s inet manual/' /etc/network/interfaces.d/%s.cfg
`, iface, iface, iface, iface)
}

var unsupportedConstraints = []string{
	constraints.CpuPower,
	constraints.InstanceType,
}

// ConstraintsValidator is defined on the Environs interface.
func (environ *maasEnviron) ConstraintsValidator() (constraints.Validator, error) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported(unsupportedConstraints)
	supportedArches, err := environ.SupportedArchitectures()
	if err != nil {
		return nil, err
	}
	validator.RegisterVocabulary(constraints.Arch, supportedArches)
	return validator, nil
}

// setupNetworks prepares a []network.Info for the given instance, but
// only interfaces on networks in networksToEnable will be configured
// on the machine.
func (environ *maasEnviron) setupNetworks(inst instance.Instance, networksToEnable set.Strings) ([]network.Info, error) {
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
		disabled := !networksToEnable.Contains(netw.Name)
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
					Disabled:      disabled,
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
	var err error
	nodeName := args.Placement
	requestedNetworks := args.MachineConfig.Networks
	includeNetworks := append(args.Constraints.IncludeNetworks(), requestedNetworks...)
	excludeNetworks := args.Constraints.ExcludeNetworks()
	node, err := environ.acquireNode(
		nodeName,
		args.Constraints,
		includeNetworks,
		excludeNetworks,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot run instances: %v", err)
	}
	inst := &maasInstance{maasObject: &node, environ: environ}
	defer func() {
		if err != nil {
			if err := environ.StopInstances(inst.Id()); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	hc, err := inst.hardwareCharacteristics()
	if err != nil {
		return nil, nil, nil, err
	}

	selectedTools, err := args.Tools.Match(tools.Filter{
		Arch: *hc.Arch,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	args.MachineConfig.Tools = selectedTools[0]

	var networkInfo []network.Info
	if args.MachineConfig.HasNetworks() {
		networkInfo, err = environ.setupNetworks(inst, set.NewStrings(requestedNetworks...))
		if err != nil {
			return nil, nil, nil, err
		}
	}

	hostname, err := inst.hostname()
	if err != nil {
		return nil, nil, nil, err
	}
	if err := environs.FinishMachineConfig(args.MachineConfig, environ.Config()); err != nil {
		return nil, nil, nil, err
	}
	// TODO(thumper): 2013-08-28 bug 1217614
	// The machine envronment config values are being moved to the agent config.
	// Explicitly specify that the lxc containers use the network bridge defined above.
	args.MachineConfig.AgentEnvironment[agent.LxcBridge] = "br0"

	iface := environ.ecfg().networkBridge()
	series := args.MachineConfig.Tools.Version.Series

	cloudcfg, err := environ.newCloudinitConfig(hostname, iface, series)
	if err != nil {
		return nil, nil, nil, err
	}
	userdata, err := environs.ComposeUserData(args.MachineConfig, cloudcfg)
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, nil, nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	if err := environ.startNode(*inst.maasObject, series, userdata); err != nil {
		return nil, nil, nil, err
	}
	logger.Debugf("started instance %q", inst.Id())

	if params.AnyJobNeedsState(args.MachineConfig.Jobs...) {
		if err := common.AddStateInstance(environ.Storage(), inst.Id()); err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}

	return inst, hc, networkInfo, nil
}

// newCloudinitConfig creates a cloudinit.Config structure
// suitable as a base for initialising a MAAS node.
func (environ *maasEnviron) newCloudinitConfig(hostname, iface, series string) (*cloudinit.Config, error) {
	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd(series)

	if err != nil {
		return nil, err
	}

	cloudcfg := cloudinit.New()
	operatingSystem, err := version.GetOSFromSeries(series)
	if err != nil {
		return nil, err
	}

	switch operatingSystem {
	case version.Windows:
		cloudcfg.AddScripts(
			runCmd,
		)
	case version.Ubuntu:
		cloudcfg.SetAptUpdate(true)
		if on, set := environ.Config().DisableNetworkManagement(); on && set {
			logger.Infof("network management disabled - setting up br0, eth0 disabled")
			cloudcfg.AddScripts("set -xe", runCmd)
		} else {
			cloudcfg.AddPackage("bridge-utils")
			cloudcfg.AddScripts(
				"set -xe",
				runCmd,
				fmt.Sprintf("ifdown %s", iface),
				restoreInterfacesFiles(iface),
				createBridgeNetwork(iface),
				"ifup br0",
			)
		}
	}
	return cloudcfg, nil
}

// StopInstances is specified in the InstanceBroker interface.
func (environ *maasEnviron) StopInstances(ids ...instance.Id) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(ids) == 0 {
		return nil
	}
	// TODO(axw) 2014-05-13 #1319016
	// Nodes that have been removed out of band will cause
	// the release call to fail. We should parse the error
	// returned from MAAS and retry, or otherwise request
	// an enhancement to MAAS to ignore unknown node IDs.
	nodes := environ.getMAASClient().GetSubObject("nodes")
	_, err := nodes.CallPost("release", getSystemIdValues("nodes", ids))
	if err != nil {
		return errors.Annotate(err, "cannot not release nodes")
	}
	return common.RemoveStateInstances(environ.Storage(), ids...)
}

// acquireInstances calls the MAAS API to list acquired nodes.
//
// The "ids" slice is a filter for specific instance IDs.
// Due to how this works in the HTTP API, an empty "ids"
// matches all instances (not none as you might expect).
func (environ *maasEnviron) acquiredInstances(ids []instance.Id) ([]instance.Instance, error) {
	filter := getSystemIdValues("id", ids)
	filter.Add("agent_name", environ.ecfg().maasAgentName())
	return environ.instances(filter)
}

// instances calls the MAAS API to list nodes matching the given filter.
func (environ *maasEnviron) instances(filter url.Values) ([]instance.Instance, error) {
	nodeListing := environ.getMAASClient().GetSubObject("nodes")
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
	instances, err := environ.acquiredInstances(ids)
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

// AllocateAddress requests a new address to be allocated for the
// given instance on the given network. This is not implemented on the
// MAAS provider yet.
func (*maasEnviron) AllocateAddress(_ instance.Id, _ network.Id) (network.Address, error) {
	// TODO(dimitern) 2014-05-06 bug #1316627
	// Once MAAS API allows allocating an address,
	// implement this using the API.
	return network.Address{}, errors.NotImplementedf("AllocateAddress")
}

// ListNetworks returns basic information about all networks known
// by the provider for the environment. They may be unknown to juju
// yet (i.e. when called initially or when a new network was created).
// This is not implemented by the MAAS provider yet.
func (*maasEnviron) ListNetworks() ([]network.BasicInfo, error) {
	return nil, errors.NotImplementedf("ListNetworks")
}

// AllInstances returns all the instance.Instance in this provider.
func (environ *maasEnviron) AllInstances() ([]instance.Instance, error) {
	return environ.acquiredInstances(nil)
}

// Storage is defined by the Environ interface.
func (env *maasEnviron) Storage() storage.Storage {
	env.ecfgMutex.Lock()
	defer env.ecfgMutex.Unlock()
	return env.storageUnlocked
}

func (environ *maasEnviron) Destroy() error {
	if err := common.Destroy(environ); err != nil {
		return errors.Trace(err)
	}
	return environ.Storage().RemoveAll()
}

// MAAS does not do firewalling so these port methods do nothing.
func (*maasEnviron) OpenPorts([]network.PortRange) error {
	logger.Debugf("unimplemented OpenPorts() called")
	return nil
}

func (*maasEnviron) ClosePorts([]network.PortRange) error {
	logger.Debugf("unimplemented ClosePorts() called")
	return nil
}

func (*maasEnviron) Ports() ([]network.PortRange, error) {
	logger.Debugf("unimplemented Ports() called")
	return nil, nil
}

func (*maasEnviron) Provider() environs.EnvironProvider {
	return &providerInstance
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
