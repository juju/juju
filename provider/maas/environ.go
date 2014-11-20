// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
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

var (
	ReleaseNodes     = releaseNodes
	ReserveIPAddress = reserveIPAddress
	ReleaseIPAddress = releaseIPAddress
)

func releaseNodes(nodes gomaasapi.MAASObject, ids url.Values) error {
	_, err := nodes.CallPost("release", ids)
	return err
}

func reserveIPAddress(ipaddresses gomaasapi.MAASObject, cidr string, addr network.Address) error {
	params := url.Values{}
	params.Add("network", cidr)
	params.Add("ip", addr.Value)
	_, err := ipaddresses.CallPost("reserve", params)
	return err
}

func releaseIPAddress(ipaddresses gomaasapi.MAASObject, addr network.Address) error {
	params := url.Values{}
	params.Add("ip", addr.Value)
	_, err := ipaddresses.CallPost("release", params)
	return err
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

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone
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
	// Override the network bridge device used for both LXC and KVM
	// containers, because we'll be creating juju-br0 at bootstrap
	// time.
	args.ContainerBridgeName = environs.DefaultBridgeName
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
		architectures := make(set.Strings)
		for _, image := range bootImages {
			architectures.Add(image.architecture)
		}
		env.supportedArchitectures = architectures.SortedValues()
	}
	return env.supportedArchitectures, nil
}

// SupportAddressAllocation is specified on the EnvironCapability interface.
func (env *maasEnviron) SupportAddressAllocation(netId network.Id) (bool, error) {
	caps, err := env.getCapabilities()
	if err != nil {
		return false, errors.Annotatef(err, "getCapabilities failed")
	}
	return caps.Contains(capStaticIPAddresses), nil
}

// allBootImages queries MAAS for all of the boot-images across
// all registered nodegroups.
func (env *maasEnviron) allBootImages() ([]bootImage, error) {
	nodegroups, err := env.getNodegroups()
	if err != nil {
		return nil, err
	}
	var allBootImages []bootImage
	seen := make(set.Strings)
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
	architectures := make(set.Strings)
	for _, inst := range allInstances {
		inst := inst.(*maasInstance)
		arch, _, err := inst.architecture()
		if err != nil {
			return nil, err
		}
		architectures.Add(arch)
	}
	// TODO(dfc) why is this sorted
	return architectures.SortedValues(), nil
}

type maasAvailabilityZone struct {
	name string
}

func (z maasAvailabilityZone) Name() string {
	return z.name
}

func (z maasAvailabilityZone) Available() bool {
	// MAAS' physical zone attributes only include name and description;
	// there is no concept of availability.
	return true
}

// AvailabilityZones returns a slice of availability zones
// for the configured region.
func (e *maasEnviron) AvailabilityZones() ([]common.AvailabilityZone, error) {
	e.availabilityZonesMutex.Lock()
	defer e.availabilityZonesMutex.Unlock()
	if e.availabilityZones == nil {
		zonesObject := e.getMAASClient().GetSubObject("zones")
		result, err := zonesObject.CallGet("", nil)
		if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == http.StatusNotFound {
			return nil, errors.NewNotImplemented(nil, "the MAAS server does not support zones")
		}
		if err != nil {
			return nil, errors.Annotate(err, "cannot query ")
		}
		list, err := result.GetArray()
		if err != nil {
			return nil, err
		}
		logger.Debugf("availability zones: %+v", list)
		availabilityZones := make([]common.AvailabilityZone, len(list))
		for i, obj := range list {
			zone, err := obj.GetMap()
			if err != nil {
				return nil, err
			}
			name, err := zone["name"].GetString()
			if err != nil {
				return nil, err
			}
			availabilityZones[i] = maasAvailabilityZone{name}
		}
		e.availabilityZones = availabilityZones
	}
	return e.availabilityZones, nil
}

// InstanceAvailabilityZoneNames returns the availability zone names for each
// of the specified instances.
func (e *maasEnviron) InstanceAvailabilityZoneNames(ids []instance.Id) ([]string, error) {
	instances, err := e.Instances(ids)
	if err != nil && err != environs.ErrPartialInstances {
		return nil, err
	}
	zones := make([]string, len(instances))
	for i, inst := range instances {
		if inst == nil {
			continue
		}
		zones[i] = inst.(*maasInstance).zone()
	}
	return zones, nil
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

type maasPlacement struct {
	nodeName string
	zoneName string
}

func (e *maasEnviron) parsePlacement(placement string) (*maasPlacement, error) {
	pos := strings.IndexRune(placement, '=')
	if pos == -1 {
		// If there's no '=' delimiter, assume it's a node name.
		return &maasPlacement{nodeName: placement}, nil
	}
	switch key, value := placement[:pos], placement[pos+1:]; key {
	case "zone":
		availabilityZone := value
		zones, err := e.AvailabilityZones()
		if err != nil {
			return nil, err
		}
		for _, z := range zones {
			if z.Name() == availabilityZone {
				return &maasPlacement{zoneName: availabilityZone}, nil
			}
		}
		return nil, errors.Errorf("invalid availability zone %q", availabilityZone)
	}
	return nil, errors.Errorf("unknown placement directive: %v", placement)
}

func (env *maasEnviron) PrecheckInstance(series string, cons constraints.Value, placement string) error {
	if placement == "" {
		return nil
	}
	_, err := env.parsePlacement(placement)
	return err
}

const (
	capNetworksManagement = "networks-management"
	capStaticIPAddresses  = "static-ipaddresses"
)

// getCapabilities asks the MAAS server for its capabilities, if
// supported by the server.
func (env *maasEnviron) getCapabilities() (set.Strings, error) {
	caps := make(set.Strings)
	var result gomaasapi.JSONObject
	var err error

	for a := shortAttempt.Start(); a.Next(); {
		client := env.getMAASClient().GetSubObject("version/")
		result, err = client.CallGet("", nil)
		if err != nil {
			if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == 404 {
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
func (environ *maasEnviron) acquireNode(nodeName, zoneName string, cons constraints.Value, includeNetworks, excludeNetworks []string) (gomaasapi.MAASObject, error) {
	acquireParams := convertConstraints(cons)
	addNetworks(acquireParams, includeNetworks, excludeNetworks)
	acquireParams.Add("agent_name", environ.ecfg().maasAgentName())
	if zoneName != "" {
		acquireParams.Add("zone", zoneName)
	}
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

const bridgeConfigTemplate = `cat >> {{.Config}} << EOF

iface {{.PrimaryNIC}} inet manual

auto {{.Bridge}}
iface {{.Bridge}} inet dhcp
    bridge_ports {{.PrimaryNIC}}
EOF
grep -q 'iface {{.PrimaryNIC}} inet dhcp' {{.Config}} && \
sed -i 's/iface {{.PrimaryNIC}} inet dhcp//' {{.Config}}`

// setupJujuNetworking returns a string representing the script to run
// in order to prepare the Juju-specific networking config on a node.
func setupJujuNetworking(primaryIface string) (string, error) {
	parsedTemplate := template.Must(
		template.New("BridgeConfig").Parse(bridgeConfigTemplate),
	)
	var buf bytes.Buffer
	err := parsedTemplate.Execute(&buf, map[string]interface{}{
		"Config":     "/etc/network/interfaces",
		"Bridge":     environs.DefaultBridgeName,
		"PrimaryNIC": primaryIface,
	})
	if err != nil {
		return "", errors.Annotate(err, "bridge config template error")
	}
	return buf.String(), nil
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

// setupNetworks prepares a []network.Info for the given instance. Any
// networks in networksToDisable will be configured as disabled on the
// machine. The interface name discovered as primary is also returned.
func (environ *maasEnviron) setupNetworks(inst instance.Instance, networksToDisable set.Strings) ([]network.Info, string, error) {
	// Get the instance network interfaces first.
	interfaces, primaryIface, err := environ.getInstanceNetworkInterfaces(inst)
	if err != nil {
		return nil, "", errors.Annotatef(err, "getInstanceNetworkInterfaces failed")
	}
	logger.Debugf("node %q has network interfaces %v", inst.Id(), interfaces)
	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, "", errors.Annotatef(err, "getInstanceNetworks failed")
	}
	logger.Debugf("node %q has networks %v", inst.Id(), networks)
	var tempNetworkInfo []network.Info
	for _, netw := range networks {
		disabled := networksToDisable.Contains(netw.Name)
		netCIDR := &net.IPNet{
			IP:   net.ParseIP(netw.IP),
			Mask: net.IPMask(net.ParseIP(netw.Mask)),
		}
		macs, err := environ.getNetworkMACs(netw.Name)
		if err != nil {
			return nil, "", errors.Annotatef(err, "getNetworkMACs failed")
		}
		logger.Debugf("network %q has MACs: %v", netw.Name, macs)
		for _, mac := range macs {
			if ifinfo, ok := interfaces[mac]; ok {
				tempNetworkInfo = append(tempNetworkInfo, network.Info{
					MACAddress:    mac,
					InterfaceName: ifinfo.InterfaceName,
					DeviceIndex:   ifinfo.DeviceIndex,
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
	return networkInfo, primaryIface, nil
}

// DistributeInstances implements the state.InstanceDistributor policy.
func (e *maasEnviron) DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	return common.DistributeInstances(e, candidates, distributionGroup)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// StartInstance is specified in the InstanceBroker interface.
func (environ *maasEnviron) StartInstance(args environs.StartInstanceParams) (
	*environs.StartInstanceResult, error,
) {
	var availabilityZones []string
	var nodeName string
	if args.Placement != "" {
		placement, err := environ.parsePlacement(args.Placement)
		if err != nil {
			return nil, err
		}
		switch {
		case placement.zoneName != "":
			availabilityZones = append(availabilityZones, placement.zoneName)
		default:
			nodeName = placement.nodeName
		}
	}

	// If no placement is specified, then automatically spread across
	// the known zones for optimal spread across the instance distribution
	// group.
	if args.Placement == "" {
		var group []instance.Id
		var err error
		if args.DistributionGroup != nil {
			group, err = args.DistributionGroup()
			if err != nil {
				return nil, errors.Annotate(err, "cannot get distribution group")
			}
		}
		zoneInstances, err := availabilityZoneAllocations(environ, group)
		if errors.IsNotImplemented(err) {
			// Availability zones are an extension, so we may get a
			// not implemented error; ignore these.
		} else if err != nil {
			return nil, errors.Annotate(err, "cannot get availability zone allocations")
		} else if len(zoneInstances) > 0 {
			for _, z := range zoneInstances {
				availabilityZones = append(availabilityZones, z.ZoneName)
			}
		}
	}
	if len(availabilityZones) == 0 {
		availabilityZones = []string{""}
	}

	requestedNetworks := args.MachineConfig.Networks
	includeNetworks := append(args.Constraints.IncludeNetworks(), requestedNetworks...)
	excludeNetworks := args.Constraints.ExcludeNetworks()

	snArgs := selectNodeArgs{
		AvailabilityZones: availabilityZones,
		NodeName:          nodeName,
		IncludeNetworks:   includeNetworks,
		ExcludeNetworks:   excludeNetworks,
	}
	node, err := environ.selectNode(snArgs)
	if err != nil {
		return nil, errors.Errorf("cannot run instances: %v", err)
	}

	inst := &maasInstance{maasObject: node, environ: environ}
	defer func() {
		if err != nil {
			if err := environ.StopInstances(inst.Id()); err != nil {
				logger.Errorf("error releasing failed instance: %v", err)
			}
		}
	}()

	hc, err := inst.hardwareCharacteristics()
	if err != nil {
		return nil, err
	}

	selectedTools, err := args.Tools.Match(tools.Filter{
		Arch: *hc.Arch,
	})
	if err != nil {
		return nil, err
	}
	args.MachineConfig.Tools = selectedTools[0]

	var networkInfo []network.Info
	networkInfo, primaryIface, err := environ.setupNetworks(inst, set.NewStrings(excludeNetworks...))
	if err != nil {
		return nil, err
	}

	hostname, err := inst.hostname()
	if err != nil {
		return nil, err
	}
	// Override the network bridge to use for both LXC and KVM
	// containers on the new instance.
	if args.MachineConfig.AgentEnvironment == nil {
		args.MachineConfig.AgentEnvironment = make(map[string]string)
	}
	args.MachineConfig.AgentEnvironment[agent.LxcBridge] = environs.DefaultBridgeName
	if err := environs.FinishMachineConfig(args.MachineConfig, environ.Config()); err != nil {
		return nil, err
	}
	series := args.MachineConfig.Tools.Version.Series

	cloudcfg, err := environ.newCloudinitConfig(hostname, primaryIface, series)
	if err != nil {
		return nil, err
	}
	userdata, err := environs.ComposeUserData(args.MachineConfig, cloudcfg)
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	if err := environ.startNode(*inst.maasObject, series, userdata); err != nil {
		return nil, err
	}
	logger.Debugf("started instance %q", inst.Id())

	if multiwatcher.AnyJobNeedsState(args.MachineConfig.Jobs...) {
		if err := common.AddStateInstance(environ.Storage(), inst.Id()); err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}

	return &environs.StartInstanceResult{
		Instance:    inst,
		Hardware:    hc,
		NetworkInfo: networkInfo,
	}, nil
}

type selectNodeArgs struct {
	AvailabilityZones []string
	NodeName          string
	Constraints       constraints.Value
	IncludeNetworks   []string
	ExcludeNetworks   []string
}

func (environ *maasEnviron) selectNode(args selectNodeArgs) (*gomaasapi.MAASObject, error) {
	var err error
	var node gomaasapi.MAASObject

	for i, zoneName := range args.AvailabilityZones {
		node, err = environ.acquireNode(
			args.NodeName,
			zoneName,
			args.Constraints,
			args.IncludeNetworks,
			args.ExcludeNetworks,
		)

		if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == http.StatusConflict {
			if i+1 < len(args.AvailabilityZones) {
				logger.Infof("could not acquire a node in zone %q, trying another zone", zoneName)
				continue
			}
		}
		if err != nil {
			return nil, errors.Errorf("cannot run instances: %v", err)
		}
		// Since a return at the end of the function is required
		// just break here.
		break
	}
	return &node, nil
}

// newCloudinitConfig creates a cloudinit.Config structure
// suitable as a base for initialising a MAAS node.
func (environ *maasEnviron) newCloudinitConfig(hostname, primaryIface, series string) (*cloudinit.Config, error) {
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
			bridgeScript, err := setupJujuNetworking(primaryIface)
			if err != nil {
				return nil, errors.Trace(err)
			}
			cloudcfg.AddPackage("bridge-utils")
			cloudcfg.AddScripts(
				"set -xe",
				runCmd,
				"ifdown "+primaryIface,
				bridgeScript,
				"ifup "+environs.DefaultBridgeName,
			)
		}
	}
	return cloudcfg, nil
}

func (environ *maasEnviron) releaseNodes(nodes gomaasapi.MAASObject, ids url.Values, recurse bool) error {
	err := ReleaseNodes(nodes, ids)
	if err == nil {
		return nil
	}
	maasErr, ok := err.(gomaasapi.ServerError)
	if !ok {
		return errors.Annotate(err, "cannot release nodes")
	}

	// StatusCode 409 means a node couldn't be released due to
	// a state conflict. Likely it's already released or disk
	// erasing. We're assuming an error of 409 *only* means it's
	// safe to assume the instance is already released.
	// MaaS also releases (or attempts) all nodes, and raises
	// a single error on failure. So even with an error 409, all
	// nodes have been released.
	if maasErr.StatusCode == 409 {
		logger.Infof("ignoring error while releasing nodes (%v); all nodes released OK", err)
		return nil
	}

	// a status code of 400, 403 or 404 means one of the nodes
	// couldn't be found and none have been released. We have
	// to release all the ones we can individually.
	if maasErr.StatusCode != 400 && maasErr.StatusCode != 403 && maasErr.StatusCode != 404 {
		return errors.Annotate(err, "cannot release nodes")
	}
	if !recurse {
		// this node has already been released and we're golden
		return nil
	}

	var lastErr error
	for _, id := range ids["nodes"] {
		idFilter := url.Values{}
		idFilter.Add("nodes", id)
		err := environ.releaseNodes(nodes, idFilter, false)
		if err != nil {
			lastErr = err
			logger.Errorf("error while releasing node %v (%v)", id, err)
		}
	}
	return errors.Trace(lastErr)

}

// StopInstances is specified in the InstanceBroker interface.
func (environ *maasEnviron) StopInstances(ids ...instance.Id) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(ids) == 0 {
		return nil
	}
	nodes := environ.getMAASClient().GetSubObject("nodes")
	err := environ.releaseNodes(nodes, getSystemIdValues("nodes", ids), true)
	if err != nil {
		// error will already have been wrapped
		return err
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

// AllocateAddress requests an address to be allocated for the
// given instance on the given network.
func (environ *maasEnviron) AllocateAddress(instId instance.Id, netId network.Id, addr network.Address) error {
	subnets, err := environ.Subnets(instId)
	if err != nil {
		return errors.Trace(err)
	}
	var foundSub *network.BasicInfo
	for i, sub := range subnets {
		if sub.ProviderId == netId {
			foundSub = &subnets[i]
			break
		}
	}
	if foundSub == nil {
		return errors.Errorf("could not find network matching %v", netId)
	}
	cidr := foundSub.CIDR
	ipaddresses := environ.getMAASClient().GetSubObject("ipaddresses")
	err = ReserveIPAddress(ipaddresses, cidr, addr)
	if err != nil {
		maasErr, ok := err.(gomaasapi.ServerError)
		if !ok {
			return errors.Trace(err)
		}
		// For an "out of range" IP address, maas raises
		// StaticIPAddressOutOfRange - an error 403
		// If there are no more addresses we get
		// StaticIPAddressExhaustion - an error 503
		// For an address already in use we get
		// StaticIPAddressUnavailable - an error 404
		if maasErr.StatusCode == 404 {
			return environs.ErrIPAddressUnavailable
		} else if maasErr.StatusCode == 503 {
			return environs.ErrIPAddressesExhausted
		}
		// any error other than a 404 or 503 is "unexpected" and should
		// be returned directly.
		return errors.Trace(err)
	}

	return nil
}

// ReleaseAddress releases a specific address previously allocated with
// AllocateAddress.
func (environ *maasEnviron) ReleaseAddress(_ instance.Id, _ network.Id, addr network.Address) error {
	ipaddresses := environ.getMAASClient().GetSubObject("ipaddresses")
	// This can return a 404 error if the address has already been released
	// or is unknown by maas. However this, like any other error, would be
	// unexpected - so we don't treat it specially and just return it to
	// the caller.
	err := ReleaseIPAddress(ipaddresses, addr)
	if err != nil {
		return errors.Annotatef(err, "failed to release IP address %v", addr.Value)
	}
	return nil
}

// Subnets returns basic information about all subnets known
// by the provider for the environment, for a specific instance.
func (environ *maasEnviron) Subnets(instId instance.Id) ([]network.BasicInfo, error) {
	instances, err := environ.acquiredInstances([]instance.Id{instId})
	if err != nil {
		return nil, errors.Annotatef(err, "could not find instance %v", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %v", instId)
	}
	inst := instances[0]
	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "getInstanceNetworks failed")
	}
	logger.Debugf("node %q has networks %v", instId, networks)
	var networkInfo []network.BasicInfo
	for _, netw := range networks {
		netCIDR := &net.IPNet{
			IP:   net.ParseIP(netw.IP),
			Mask: net.IPMask(net.ParseIP(netw.Mask)),
		}
		netInfo := network.BasicInfo{
			CIDR:       netCIDR.String(),
			VLANTag:    netw.VLANTag,
			ProviderId: network.Id(netw.Name),
		}

		// Verify we filled-in everything for all networks
		// and drop incomplete records.
		if netInfo.ProviderId == "" || netInfo.CIDR == "" {
			logger.Warningf("ignoring network  %q: missing information (%#v)", netw.Name, netInfo)
			continue
		}

		networkInfo = append(networkInfo, netInfo)
	}
	logger.Debugf("available networks for instance %v: %#v", inst.Id(), networkInfo)
	return networkInfo, nil
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
// to ifaceInfo for each network interface of the given instance, as
// discovered during the commissioning phase. In addition, it also
// returns the interface name discovered as primary.
func (environ *maasEnviron) getInstanceNetworkInterfaces(inst instance.Instance) (map[string]ifaceInfo, string, error) {
	maasInst := inst.(*maasInstance)
	maasObj := maasInst.maasObject
	result, err := maasObj.CallGet("details", nil)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	// Get the node's lldp / lshw details discovered at commissioning.
	data, err := result.GetBytes()
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	var parsed map[string]interface{}
	if err := bson.Unmarshal(data, &parsed); err != nil {
		return nil, "", errors.Trace(err)
	}
	lshwData, ok := parsed["lshw"]
	if !ok {
		return nil, "", errors.Errorf("no hardware information available for node %q", inst.Id())
	}
	lshwXML, ok := lshwData.([]byte)
	if !ok {
		return nil, "", errors.Errorf("invalid hardware information for node %q", inst.Id())
	}
	// Now we have the lshw XML data, parse it to extract and return NICs.
	return extractInterfaces(inst, lshwXML)
}

type ifaceInfo struct {
	DeviceIndex   int
	InterfaceName string
}

// extractInterfaces parses the XML output of lswh and extracts all
// network interfaces, returing a map MAC address to ifaceInfo, as
// well as the interface name discovered as primary.
func extractInterfaces(inst instance.Instance, lshwXML []byte) (map[string]ifaceInfo, string, error) {
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
		return nil, "", errors.Annotatef(err, "cannot parse lshw XML details for node %q", inst.Id())
	}
	primaryIface := ""
	interfaces := make(map[string]ifaceInfo)
	var processNodes func(nodes []Node) error
	processNodes = func(nodes []Node) error {
		for _, node := range nodes {
			if strings.HasPrefix(node.Id, "network") {
				// If there's a single interface, the ID won't have an
				// index suffix.
				index := 0
				if strings.HasPrefix(node.Id, "network:") {
					// There is an index suffix, parse it.
					var err error
					index, err = strconv.Atoi(strings.TrimPrefix(node.Id, "network:"))
					if err != nil {
						return errors.Annotatef(err, "lshw output for node %q has invalid ID suffix for %q", inst.Id(), node.Id)
					}
				}
				if index == 0 && primaryIface == "" {
					primaryIface = node.LogicalName
					logger.Debugf("node %q primary network interface is %q", inst.Id(), primaryIface)
				}
				interfaces[node.Serial] = ifaceInfo{
					DeviceIndex:   index,
					InterfaceName: node.LogicalName,
				}
			}
			if err := processNodes(node.Children); err != nil {
				return err
			}
		}
		return nil
	}
	err := processNodes(lshw.Nodes)
	return interfaces, primaryIface, err
}
