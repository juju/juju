// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
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
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/gomaasapi"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/tools"
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
	ReleaseNodes             = releaseNodes
	ReserveIPAddress         = reserveIPAddress
	ReserveIPAddressOnDevice = reserveIPAddressOnDevice
	ReleaseIPAddress         = releaseIPAddress
	DeploymentStatusCall     = deploymentStatusCall
)

func releaseNodes(nodes gomaasapi.MAASObject, ids url.Values) error {
	_, err := nodes.CallPost("release", ids)
	return err
}

func reserveIPAddress(ipaddresses gomaasapi.MAASObject, cidr string, addr network.Address) error {
	params := url.Values{}
	params.Add("network", cidr)
	params.Add("requested_address", addr.Value)
	_, err := ipaddresses.CallPost("reserve", params)
	return err
}

func reserveIPAddressOnDevice(devices gomaasapi.MAASObject, deviceId string, addr network.Address) error {
	device := devices.GetSubObject(deviceId)
	params := url.Values{}
	if addr.Value != "" {
		params.Add("requested_address", addr.Value)
	}
	_, err := device.CallPost("claim_sticky_ip_address", params)
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

	// The following are initialized from the discovered MAAS API capabilities.
	supportsDevices   bool
	supportsStaticIPs bool
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

	// Since we need to switch behavior based on the available API capabilities,
	// get them as soon as possible and cache them.
	capabilities, err := env.getCapabilities()
	if err != nil {
		logger.Warningf("cannot get MAAS API capabilities: %v", err)
	}
	logger.Debugf("MAAS API capabilities: %v", capabilities.SortedValues())
	env.supportsDevices = capabilities.Contains(capDevices)
	env.supportsStaticIPs = capabilities.Contains(capStaticIPAddresses)
	return env, nil
}

const noDevicesWarning = `
WARNING: Using MAAS version older than 1.8.2: devices API support not detected!

Juju cannot guarantee resources allocated to containers, like DHCP
leases or static IP addresses will be properly cleaned up when the
container, its host, or the environment is destroyed.

Juju recommends upgrading MAAS to version 1.8.2 or later.
`

// Bootstrap is specified in the Environ interface.
func (env *maasEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	if !environs.AddressAllocationEnabled() {
		// When address allocation is not enabled, we should use the
		// default bridge for both LXC and KVM containers. The bridge
		// is created as part of the userdata for every node during
		// StartInstance.
		logger.Infof(
			"address allocation feature disabled; using %q bridge for all containers",
			instancecfg.DefaultBridgeName,
		)
		args.ContainerBridgeName = instancecfg.DefaultBridgeName

		if !env.supportsDevices {
			// Inform the user container resources might leak.
			ctx.Infof("WARNING: %s", noDevicesWarning)
		}
	} else {
		logger.Debugf(
			"address allocation feature enabled; using static IPs for containers: %q",
			instancecfg.DefaultBridgeName,
		)
	}

	result, series, finalizer, err := common.BootstrapInstance(ctx, env, args)
	if err != nil {
		return nil, err
	}

	// We want to destroy the started instance if it doesn't transition to Deployed.
	defer func() {
		if err != nil {
			if err := env.StopInstances(result.Instance.Id()); err != nil {
				logger.Errorf("error releasing bootstrap instance: %v", err)
			}
		}
	}()
	// Wait for bootstrap instance to change to deployed state.
	if err := env.waitForNodeDeployment(result.Instance.Id()); err != nil {
		return nil, errors.Annotate(err, "bootstrap instance started but did not change to Deployed state")
	}

	bsResult := &environs.BootstrapResult{
		Arch:     *result.Hardware.Arch,
		Series:   series,
		Finalize: finalizer,
	}
	return bsResult, nil
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
	if err != nil || len(bootImages) == 0 {
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

// SupportsSpaces is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaces() (bool, error) {
	caps, err := env.getCapabilities()
	if err != nil {
		return false, errors.Annotatef(err, "getCapabilities failed")
	}

	return caps.Contains(capNetworkDeploymentUbuntu), nil
}

// SupportsAddressAllocation is specified on environs.Networking.
func (env *maasEnviron) SupportsAddressAllocation(_ network.Id) (bool, error) {
	if !environs.AddressAllocationEnabled() {
		if !env.supportsDevices {
			return false, errors.NotSupportedf("address allocation")
		}
		// We can use devices for DHCP-allocated container IPs.
		return true, nil
	}

	return env.supportsStaticIPs, nil
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

func (env *maasEnviron) getNodegroupInterfaces(nodegroups []string) map[string][]net.IP {
	nodegroupsObject := env.getMAASClient().GetSubObject("nodegroups")

	nodegroupsInterfacesMap := make(map[string][]net.IP)
	for _, uuid := range nodegroups {
		interfacesObject := nodegroupsObject.GetSubObject(uuid).GetSubObject("interfaces")
		interfacesResult, err := interfacesObject.CallGet("list", nil)
		if err != nil {
			logger.Debugf("cannot list interfaces for nodegroup %v: %v", uuid, err)
			continue
		}
		interfaces, err := interfacesResult.GetArray()
		if err != nil {
			logger.Debugf("cannot get interfaces for nodegroup %v: %v", uuid, err)
			continue
		}
		for _, interfaceResult := range interfaces {
			nic, err := interfaceResult.GetMap()
			if err != nil {
				logger.Debugf("cannot get interface %v for nodegroup %v: %v", nic, uuid, err)
				continue
			}
			ip, err := nic["ip"].GetString()
			if err != nil {
				logger.Debugf("cannot get interface IP %v for nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_low, err := nic["static_ip_range_low"].GetString()
			if err != nil {
				logger.Debugf("cannot get static IP range lower bound for interface %v on nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_high, err := nic["static_ip_range_high"].GetString()
			if err != nil {
				logger.Infof("cannot get static IP range higher bound for interface %v on nodegroup %v: %v", nic, uuid, err)
				continue
			}
			static_low_ip := net.ParseIP(static_low)
			static_high_ip := net.ParseIP(static_high)
			if static_low_ip == nil || static_high_ip == nil {
				logger.Debugf("invalid IP in static range for interface %v on nodegroup %v: %q %q", nic, uuid, static_low_ip, static_high_ip)
				continue
			}
			nodegroupsInterfacesMap[ip] = []net.IP{static_low_ip, static_high_ip}
		}
	}
	return nodegroupsInterfacesMap
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
	capNetworksManagement      = "networks-management"
	capStaticIPAddresses       = "static-ipaddresses"
	capDevices                 = "devices-management"
	capNetworkDeploymentUbuntu = "network-deployment-ubuntu"
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
		} else {
			break
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

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode(
	nodeName, zoneName string,
	cons constraints.Value,
	interfaces []interfaceBinding,
	volumes []volumeInfo,
) (gomaasapi.MAASObject, error) {

	acquireParams := convertConstraints(cons)
	if err := addInterfaces(acquireParams, interfaces); err != nil {
		return gomaasapi.MAASObject{}, err
	}
	addStorage(acquireParams, volumes)
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
	params := url.Values{
		"distro_series": {series},
		"user_data":     {string(userdata)},
	}
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	err := fmt.Errorf("(no error)")
	for a := shortAttempt.Start(); a.Next() && err != nil; {
		_, err = node.CallPost("start", params)
	}
	return err
}

// setupNetworks prepares a []network.InterfaceInfo for the given
// instance. Any networks in networksToDisable will be configured as
// disabled on the machine. Any disabled network interfaces (as
// discovered from the lshw output for the node) will stay disabled.
// The interface name discovered as primary is also returned.
func (environ *maasEnviron) setupNetworks(inst instance.Instance, networksToDisable set.Strings) ([]network.InterfaceInfo, string, error) {
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
	var tempInterfaceInfo []network.InterfaceInfo
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
				tempInterfaceInfo = append(tempInterfaceInfo, network.InterfaceInfo{
					MACAddress:    mac,
					InterfaceName: ifinfo.InterfaceName,
					DeviceIndex:   ifinfo.DeviceIndex,
					CIDR:          netCIDR.String(),
					VLANTag:       netw.VLANTag,
					ProviderId:    network.Id(netw.Name),
					NetworkName:   netw.Name,
					Disabled:      disabled || ifinfo.Disabled,
				})
			}
		}
	}
	// Verify we filled-in everything for all networks/interfaces
	// and drop incomplete records.
	var interfaceInfo []network.InterfaceInfo
	for _, info := range tempInterfaceInfo {
		if info.ProviderId == "" || info.NetworkName == "" || info.CIDR == "" {
			logger.Infof("ignoring interface %q: missing subnet info", info.InterfaceName)
			continue
		}
		if info.MACAddress == "" || info.InterfaceName == "" {
			logger.Infof("ignoring subnet %q: missing interface info", info.ProviderId)
			continue
		}
		interfaceInfo = append(interfaceInfo, info)
	}
	logger.Debugf("node %q network information: %#v", inst.Id(), interfaceInfo)
	return interfaceInfo, primaryIface, nil
}

// DistributeInstances implements the state.InstanceDistributor policy.
func (e *maasEnviron) DistributeInstances(candidates, distributionGroup []instance.Id) ([]instance.Id, error) {
	return common.DistributeInstances(e, candidates, distributionGroup)
}

var availabilityZoneAllocations = common.AvailabilityZoneAllocations

// MaintainInstance is specified in the InstanceBroker interface.
func (*maasEnviron) MaintainInstance(args environs.StartInstanceParams) error {
	return nil
}

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

	// Storage.
	volumes, err := buildMAASVolumeParameters(args.Volumes, args.Constraints)
	if err != nil {
		return nil, errors.Annotate(err, "invalid volume parameters")
	}

	snArgs := selectNodeArgs{
		Constraints:       args.Constraints,
		AvailabilityZones: availabilityZones,
		NodeName:          nodeName,
		// TODO(dimitern): Once we have interface bindings for services in state
		// and in StartInstanceParams, pass them here.
		Interfaces: nil,
		Volumes:    volumes,
	}
	node, err := environ.selectNode(snArgs)
	if err != nil {
		return nil, errors.Errorf("cannot run instances: %v", err)
	}

	inst := &maasInstance{node}
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
	args.InstanceConfig.Tools = selectedTools[0]

	var networkInfo []network.InterfaceInfo
	networkInfo, primaryIface, err := environ.setupNetworks(inst, nil)
	if err != nil {
		return nil, err
	}

	hostname, err := inst.hostname()
	if err != nil {
		return nil, err
	}
	// Override the network bridge to use for both LXC and KVM
	// containers on the new instance, if address allocation feature
	// flag is not enabled.
	if !environs.AddressAllocationEnabled() {
		if args.InstanceConfig.AgentEnvironment == nil {
			args.InstanceConfig.AgentEnvironment = make(map[string]string)
		}
		args.InstanceConfig.AgentEnvironment[agent.LxcBridge] = instancecfg.DefaultBridgeName
	}
	if err := instancecfg.FinishInstanceConfig(args.InstanceConfig, environ.Config()); err != nil {
		return nil, err
	}
	series := args.InstanceConfig.Tools.Version.Series

	cloudcfg, err := environ.newCloudinitConfig(hostname, primaryIface, series)
	if err != nil {
		return nil, err
	}
	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, MAASRenderer{})
	if err != nil {
		msg := fmt.Errorf("could not compose userdata for bootstrap node: %v", err)
		return nil, msg
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	if err := environ.startNode(*inst.maasObject, series, userdata); err != nil {
		return nil, err
	}
	logger.Debugf("started instance %q", inst.Id())

	if multiwatcher.AnyJobNeedsState(args.InstanceConfig.Jobs...) {
		if err := common.AddStateInstance(environ.Storage(), inst.Id()); err != nil {
			logger.Errorf("could not record instance in provider-state: %v", err)
		}
	}

	requestedVolumes := make([]names.VolumeTag, len(args.Volumes))
	for i, v := range args.Volumes {
		requestedVolumes[i] = v.Tag
	}
	resultVolumes, resultAttachments, err := inst.volumes(
		names.NewMachineTag(args.InstanceConfig.MachineId),
		requestedVolumes,
	)
	if err != nil {
		return nil, err
	}
	if len(resultVolumes) != len(requestedVolumes) {
		err = errors.New("the version of MAAS being used does not support Juju storage")
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance:          inst,
		Hardware:          hc,
		NetworkInfo:       networkInfo,
		Volumes:           resultVolumes,
		VolumeAttachments: resultAttachments,
	}, nil
}

// Override for testing.
var nodeDeploymentTimeout = func(environ *maasEnviron) time.Duration {
	sshTimeouts := environ.Config().BootstrapSSHOpts()
	return sshTimeouts.Timeout
}

func (environ *maasEnviron) waitForNodeDeployment(id instance.Id) error {
	systemId := extractSystemId(id)
	longAttempt := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Total: nodeDeploymentTimeout(environ),
	}

	for a := longAttempt.Start(); a.Next(); {
		statusValues, err := environ.deploymentStatus(id)
		if errors.IsNotImplemented(err) {
			return nil
		}
		if err != nil {
			return errors.Trace(err)
		}
		if statusValues[systemId] == "Deployed" {
			return nil
		}
		if statusValues[systemId] == "Failed deployment" {
			return errors.Errorf("instance %q failed to deploy", id)
		}
	}
	return errors.Errorf("instance %q is started but not deployed", id)
}

// deploymentStatus returns the deployment state of MAAS instances with
// the specified Juju instance ids.
// Note: the result is a map of MAAS systemId to state.
func (environ *maasEnviron) deploymentStatus(ids ...instance.Id) (map[string]string, error) {
	nodesAPI := environ.getMAASClient().GetSubObject("nodes")
	result, err := DeploymentStatusCall(nodesAPI, ids...)
	if err != nil {
		if err, ok := err.(gomaasapi.ServerError); ok && err.StatusCode == http.StatusBadRequest {
			return nil, errors.NewNotImplemented(err, "deployment status")
		}
		return nil, errors.Trace(err)
	}
	resultMap, err := result.GetMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	statusValues := make(map[string]string)
	for systemId, jsonValue := range resultMap {
		status, err := jsonValue.GetString()
		if err != nil {
			return nil, errors.Trace(err)
		}
		statusValues[systemId] = status
	}
	return statusValues, nil
}

func deploymentStatusCall(nodes gomaasapi.MAASObject, ids ...instance.Id) (gomaasapi.JSONObject, error) {
	filter := getSystemIdValues("nodes", ids)
	return nodes.CallGet("deployment_status", filter)
}

type selectNodeArgs struct {
	AvailabilityZones []string
	NodeName          string
	Constraints       constraints.Value
	Interfaces        []interfaceBinding
	Volumes           []volumeInfo
}

func (environ *maasEnviron) selectNode(args selectNodeArgs) (*gomaasapi.MAASObject, error) {
	var err error
	var node gomaasapi.MAASObject

	for i, zoneName := range args.AvailabilityZones {
		node, err = environ.acquireNode(
			args.NodeName,
			zoneName,
			args.Constraints,
			args.Interfaces,
			args.Volumes,
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

const modifyEtcNetworkInterfaces = `isDHCP() {
    grep -q "iface ${PRIMARY_IFACE} inet dhcp" {{.Config}}
    return $?
}

isStatic() {
    grep -q "iface ${PRIMARY_IFACE} inet static" {{.Config}}
    return $?
}

unAuto() {
    # Remove the line auto starting the primary interface. \s*$ matches
    # whitespace and the end of the line to avoid mangling aliases.
    grep -q "auto ${PRIMARY_IFACE}\s*$" {{.Config}} && \
    sed -i "s/auto ${PRIMARY_IFACE}\s*$//" {{.Config}}
}

# Change the config to make $PRIMARY_IFACE manual instead of DHCP,
# then create the bridge and enslave $PRIMARY_IFACE into it.
if isDHCP; then
    sed -i "s/iface ${PRIMARY_IFACE} inet dhcp//" {{.Config}}
    cat >> {{.Config}} << EOF

# Primary interface (defining the default route)
iface ${PRIMARY_IFACE} inet manual

# Bridge to use for LXC/KVM containers
auto {{.Bridge}}
iface {{.Bridge}} inet dhcp
    bridge_ports ${PRIMARY_IFACE}
EOF
    # Make the primary interface not auto-starting.
    unAuto
elif isStatic
then
    sed -i "s/iface ${PRIMARY_IFACE} inet static/iface {{.Bridge}} inet static\n    bridge_ports ${PRIMARY_IFACE}/" {{.Config}}
    sed -i "s/auto ${PRIMARY_IFACE}\s*$/auto {{.Bridge}}/" {{.Config}}
    cat >> {{.Config}} << EOF

# Primary interface (defining the default route)
iface ${PRIMARY_IFACE} inet manual
EOF
fi`

const bridgeConfigTemplate = `
# In case we already created the bridge, don't do it again.
grep -q "iface {{.Bridge}} inet dhcp" {{.Config}} && exit 0

# Discover primary interface at run-time using the default route (if set)
PRIMARY_IFACE=$(ip route list exact 0/0 | egrep -o 'dev [^ ]+' | cut -b5-)

# If $PRIMARY_IFACE is empty, there's nothing to do.
[ -z "$PRIMARY_IFACE" ] && exit 0

# Bring down the primary interface while /e/n/i still matches the live config.
# Will bring it back up within a bridge after updating /e/n/i.
ifdown -v ${PRIMARY_IFACE}

# Log the contents of /etc/network/interfaces prior to modifying
echo "Contents of /etc/network/interfaces before changes"
cat /etc/network/interfaces
{{.Script}}
# Log the contents of /etc/network/interfaces after modifying
echo "Contents of /etc/network/interfaces after changes"
cat /etc/network/interfaces

ifup -v {{.Bridge}}
`

// setupJujuNetworking returns a string representing the script to run
// in order to prepare the Juju-specific networking config on a node.
func setupJujuNetworking() (string, error) {
	modifyConfigScript, err := renderEtcNetworkInterfacesScript("/etc/network/interfaces", instancecfg.DefaultBridgeName)
	if err != nil {
		return "", err
	}
	parsedTemplate := template.Must(
		template.New("BridgeConfig").Parse(bridgeConfigTemplate),
	)
	var buf bytes.Buffer
	err = parsedTemplate.Execute(&buf, map[string]interface{}{
		"Config": "/etc/network/interfaces",
		"Bridge": instancecfg.DefaultBridgeName,
		"Script": modifyConfigScript,
	})
	if err != nil {
		return "", errors.Annotate(err, "bridge config template error")
	}
	return buf.String(), nil
}

func renderEtcNetworkInterfacesScript(config, bridge string) (string, error) {
	parsedTemplate := template.Must(
		template.New("ModifyConfigScript").Parse(modifyEtcNetworkInterfaces),
	)
	var buf bytes.Buffer
	err := parsedTemplate.Execute(&buf, map[string]interface{}{
		"Config": config,
		"Bridge": bridge,
	})
	if err != nil {
		return "", errors.Annotate(err, "modify /etc/network/interfaces script template error")
	}
	return buf.String(), nil
}

// newCloudinitConfig creates a cloudinit.Config structure
// suitable as a base for initialising a MAAS node.
func (environ *maasEnviron) newCloudinitConfig(hostname, primaryIface, ser string) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(ser)
	if err != nil {
		return nil, err
	}

	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd(cloudcfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	operatingSystem, err := series.GetOSFromSeries(ser)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch operatingSystem {
	case os.Windows:
		cloudcfg.AddScripts(runCmd)
	case os.Ubuntu:
		cloudcfg.SetSystemUpdate(true)
		cloudcfg.AddScripts("set -xe", runCmd)
		// Only create the default bridge if we're not using static
		// address allocation for containers.
		if !environs.AddressAllocationEnabled() {
			// Address allocated feature flag might be disabled, but
			// DisableNetworkManagement can still disable the bridge
			// creation.
			if on, set := environ.Config().DisableNetworkManagement(); on && set {
				logger.Infof(
					"network management disabled - not using %q bridge for containers",
					instancecfg.DefaultBridgeName,
				)
				break
			}
			bridgeScript, err := setupJujuNetworking()
			if err != nil {
				return nil, errors.Trace(err)
			}
			cloudcfg.AddPackage("bridge-utils")
			cloudcfg.AddRunCmd(bridgeScript)
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
		instances[index] = &maasInstance{&node}
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

// newDevice creates a new MAAS device for a MAC address, returning the Id of
// the new device.
func (environ *maasEnviron) newDevice(macAddress string, instId instance.Id, hostname string) (string, error) {
	client := environ.getMAASClient()
	devices := client.GetSubObject("devices")
	params := url.Values{}
	params.Add("mac_addresses", macAddress)
	params.Add("hostname", hostname)
	params.Add("parent", extractSystemId(instId))
	logger.Tracef("creating a new MAAS device for MAC %q, hostname %q, parent %q", macAddress, hostname, string(instId))
	result, err := devices.CallPost("new", params)
	if err != nil {
		return "", errors.Trace(err)
	}

	resultMap, err := result.GetMap()
	if err != nil {
		return "", errors.Trace(err)
	}

	device, err := resultMap["system_id"].GetString()
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Tracef("created device %q", device)
	return device, nil
}

// fetchFullDevice fetches an existing device Id associated with a MAC address
// and/or hostname, or returns an error if there is no device.
func (environ *maasEnviron) fetchFullDevice(macAddress, hostname string) (map[string]gomaasapi.JSONObject, error) {
	client := environ.getMAASClient()
	devices := client.GetSubObject("devices")
	params := url.Values{}
	if macAddress != "" {
		params.Add("mac_address", macAddress)
	}
	if hostname != "" {
		params.Add("hostname", hostname)
	}
	result, err := devices.CallGet("list", params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	resultArray, err := result.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(resultArray) == 0 {
		return nil, errors.NotFoundf("no device for MAC %q and/or hostname %q", macAddress, hostname)
	}
	if len(resultArray) != 1 {
		return nil, errors.Errorf("unexpected response, expected 1 device got %d", len(resultArray))
	}
	resultMap, err := resultArray[0].GetMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Tracef("device found as %+v", resultMap)
	return resultMap, nil
}

func (environ *maasEnviron) fetchDevice(macAddress, hostname string) (string, error) {
	deviceMap, err := environ.fetchFullDevice(macAddress, hostname)
	if err != nil {
		return "", errors.Trace(err)
	}

	deviceId, err := deviceMap["system_id"].GetString()
	if err != nil {
		return "", errors.Trace(err)
	}
	return deviceId, nil
}

// createOrFetchDevice returns a device Id associated with a MAC address. If
// there is not already one it will create one.
func (environ *maasEnviron) createOrFetchDevice(macAddress string, instId instance.Id, hostname string) (string, error) {
	device, err := environ.fetchDevice(macAddress, hostname)
	if err == nil {
		return device, nil
	}
	if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	device, err = environ.newDevice(macAddress, instId, hostname)
	if err != nil {
		return "", errors.Trace(err)
	}
	return device, nil
}

// AllocateAddress requests an address to be allocated for the
// given instance on the given network.
func (environ *maasEnviron) AllocateAddress(instId instance.Id, subnetId network.Id, addr network.Address, macAddress, hostname string) (err error) {
	logger.Tracef(
		"AllocateAddress for instId %q, subnet %q, addr %q, MAC %q, hostname %q",
		instId, subnetId, addr, macAddress, hostname,
	)

	if !environs.AddressAllocationEnabled() {
		if !environ.supportsDevices {
			logger.Warningf(
				"resources used by container %q with MAC address %q can leak: devices API not supported",
				hostname, macAddress,
			)
			return errors.NotSupportedf("address allocation")
		}
		logger.Tracef("creating device for container %q with MAC %q", hostname, macAddress)
		deviceID, err := environ.createOrFetchDevice(macAddress, instId, hostname)
		if err != nil {
			return errors.Annotatef(
				err,
				"creating MAAS device for container %q with MAC address %q",
				hostname, macAddress,
			)
		}
		logger.Infof(
			"created device %q for container %q with MAC address %q on parent node %q",
			deviceID, hostname, macAddress, instId,
		)
		devices := environ.getMAASClient().GetSubObject("devices")
		if err := reserveIPAddressOnDevice(devices, deviceID, network.Address{}); err != nil {
			return errors.Annotatef(err, "reserving a sticky IP address for device %q", deviceID)
		}
		logger.Infof("reserved sticky IP address for device %q representing container %q", deviceID, hostname)

		return nil
	}
	defer errors.DeferredAnnotatef(&err, "failed to allocate address %q for instance %q", addr, instId)

	client := environ.getMAASClient()
	var maasErr gomaasapi.ServerError
	if environ.supportsDevices {
		device, err := environ.createOrFetchDevice(macAddress, instId, hostname)
		if err != nil {
			return err
		}

		devices := client.GetSubObject("devices")
		err = ReserveIPAddressOnDevice(devices, device, addr)
		if err == nil {
			logger.Infof("allocated address %q for instance %q on device %q", addr, instId, device)
			return nil
		}

		var ok bool
		maasErr, ok = err.(gomaasapi.ServerError)
		if !ok {
			return errors.Trace(err)
		}
	} else {

		var subnets []network.SubnetInfo

		subnets, err = environ.Subnets(instId, []network.Id{subnetId})
		logger.Tracef("Subnets(%q, %q, %q) returned: %v (%v)", instId, subnetId, addr, subnets, err)
		if err != nil {
			return errors.Trace(err)
		}
		if len(subnets) != 1 {
			return errors.Errorf("could not find subnet matching %q", subnetId)
		}
		foundSub := subnets[0]
		logger.Tracef("found subnet %#v", foundSub)

		cidr := foundSub.CIDR
		ipaddresses := client.GetSubObject("ipaddresses")
		err = ReserveIPAddress(ipaddresses, cidr, addr)
		if err == nil {
			logger.Infof("allocated address %q for instance %q on subnet %q", addr, instId, cidr)
			return nil
		}

		var ok bool
		maasErr, ok = err.(gomaasapi.ServerError)
		if !ok {
			return errors.Trace(err)
		}
	}
	// For an "out of range" IP address, maas raises
	// StaticIPAddressOutOfRange - an error 403
	// If there are no more addresses we get
	// StaticIPAddressExhaustion - an error 503
	// For an address already in use we get
	// StaticIPAddressUnavailable - an error 404
	if maasErr.StatusCode == 404 {
		logger.Tracef("address %q not available for allocation", addr)
		return environs.ErrIPAddressUnavailable
	} else if maasErr.StatusCode == 503 {
		logger.Tracef("no more addresses available on the subnet")
		return environs.ErrIPAddressesExhausted
	}
	// any error other than a 404 or 503 is "unexpected" and should
	// be returned directly.
	return errors.Trace(err)
}

// ReleaseAddress releases a specific address previously allocated with
// AllocateAddress.
func (environ *maasEnviron) ReleaseAddress(instId instance.Id, _ network.Id, addr network.Address, macAddress, hostname string) (err error) {
	if !environs.AddressAllocationEnabled() {
		if !environ.supportsDevices {
			logger.Warningf(
				"resources used by container %q with MAC address %q can leak: devices API not supported",
				hostname, macAddress,
			)
			return errors.NotSupportedf("address allocation")
		}
		logger.Tracef("getting device ID for container %q with MAC %q", macAddress, hostname)
		deviceID, err := environ.fetchDevice(macAddress, hostname)
		if err != nil {
			return errors.Annotatef(
				err,
				"getting MAAS device for container %q with MAC address %q",
				hostname, macAddress,
			)
		}
		logger.Tracef("deleting device %q for container %q", deviceID, hostname)
		apiDevice := environ.getMAASClient().GetSubObject("devices").GetSubObject(deviceID)
		if err := apiDevice.Delete(); err != nil {
			return errors.Annotatef(
				err,
				"deleting MAAS device %q for container %q with MAC address %q",
				deviceID, instId, macAddress,
			)
		}
		logger.Debugf("deleted device %q for container %q with MAC address %q", deviceID, instId, macAddress)
		return nil
	}

	defer errors.DeferredAnnotatef(&err, "failed to release IP address %q from instance %q", addr, instId)

	logger.Infof(
		"releasing address: %q, MAC address: %q, hostname: %q, supports devices: %v",
		addr, macAddress, hostname, environ.supportsDevices,
	)
	// Addresses originally allocated without a device will have macAddress
	// set to "". We shouldn't look for a device for these addresses.
	if environ.supportsDevices && macAddress != "" {
		device, err := environ.fetchFullDevice(macAddress, hostname)
		if err == nil {
			addresses, err := device["ip_addresses"].GetArray()
			if err != nil {
				return err
			}
			systemId, err := device["system_id"].GetString()
			if err != nil {
				return err
			}

			if len(addresses) == 1 {
				// With our current usage of devices they will always
				// have exactly one IP address, but in theory that
				// could change and this code will continue to work.
				// The device is only destroyed when we come to release
				// the last address. Race conditions aside.
				deviceAPI := environ.getMAASClient().GetSubObject("devices").GetSubObject(systemId)
				err = deviceAPI.Delete()
				return err
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		// No device for this IP address, release the address normally.
	}

	ipaddresses := environ.getMAASClient().GetSubObject("ipaddresses")
	retries := 0
	for a := shortAttempt.Start(); a.Next(); {
		retries++
		// This can return a 404 error if the address has already been released
		// or is unknown by maas. However this, like any other error, would be
		// unexpected - so we don't treat it specially and just return it to
		// the caller.
		err = ReleaseIPAddress(ipaddresses, addr)
		if err == nil {
			break
		}
		logger.Infof("failed to release address %q from instance %q, will retry", addr, instId)
	}
	if err != nil {
		logger.Warningf("failed to release address %q from instance %q after %d attempts: %v", addr, instId, retries, err)
	}
	return err
}

// NetworkInterfaces implements Environ.NetworkInterfaces.
func (environ *maasEnviron) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	instances, err := environ.acquiredInstances([]instance.Id{instId})
	if err != nil {
		return nil, errors.Annotatef(err, "could not find instance %q", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %q", instId)
	}
	inst := instances[0]
	interfaces, _, err := environ.getInstanceNetworkInterfaces(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get instance %q network interfaces", instId)
	}

	networks, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to get instance %q subnets", instId)
	}

	macToNetworkMap := make(map[string]networkDetails)
	for _, network := range networks {
		macs, err := environ.listConnectedMacs(network)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, mac := range macs {
			macToNetworkMap[mac] = network
		}
	}

	result := []network.InterfaceInfo{}
	for serial, iface := range interfaces {
		deviceIndex := iface.DeviceIndex
		interfaceName := iface.InterfaceName
		disabled := iface.Disabled

		ifaceInfo := network.InterfaceInfo{
			DeviceIndex:   deviceIndex,
			InterfaceName: interfaceName,
			Disabled:      disabled,
			NoAutoStart:   disabled,
			MACAddress:    serial,
			ConfigType:    network.ConfigDHCP,
		}
		details, ok := macToNetworkMap[serial]
		if ok {
			ifaceInfo.VLANTag = details.VLANTag
			ifaceInfo.ProviderSubnetId = network.Id(details.Name)
			mask := net.IPMask(net.ParseIP(details.Mask))
			cidr := net.IPNet{net.ParseIP(details.IP), mask}
			ifaceInfo.CIDR = cidr.String()
			ifaceInfo.Address = network.NewAddress(cidr.IP.String())
		} else {
			logger.Debugf("no subnet information for MAC address %q, instance %q", serial, instId)
		}
		result = append(result, ifaceInfo)
	}
	return result, nil
}

// listConnectedMacs calls the MAAS list_connected_macs API to fetch all the
// the MAC addresses attached to a specific network.
func (environ *maasEnviron) listConnectedMacs(network networkDetails) ([]string, error) {
	client := environ.getMAASClient().GetSubObject("networks").GetSubObject(network.Name)
	json, err := client.CallGet("list_connected_macs", nil)
	if err != nil {
		return nil, err
	}

	macs, err := json.GetArray()
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, macObj := range macs {
		macMap, err := macObj.GetMap()
		if err != nil {
			return nil, err
		}
		mac, err := macMap["mac_address"].GetString()
		if err != nil {
			return nil, err
		}

		result = append(result, mac)
	}
	return result, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance. subnetIds must not be
// empty. Implements NetworkingEnviron.Subnets.
func (environ *maasEnviron) Subnets(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	// At some point in the future an empty netIds may mean "fetch all subnets"
	// but until that functionality is needed it's an error.
	if len(subnetIds) == 0 {
		return nil, errors.Errorf("subnetIds must not be empty")
	}
	instances, err := environ.acquiredInstances([]instance.Id{instId})
	if err != nil {
		return nil, errors.Annotatef(err, "could not find instance %q", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %v", instId)
	}
	inst := instances[0]
	// The MAAS API get networks call returns named subnets, not physical networks,
	// so we save the data from this call into a variable called subnets.
	// http://maas.ubuntu.com/docs/api.html#networks
	subnets, err := environ.getInstanceNetworks(inst)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get instance %q subnets", instId)
	}
	logger.Debugf("instance %q has subnets %v", instId, subnets)

	nodegroups, err := environ.getNodegroups()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get instance %q node groups", instId)
	}
	nodegroupInterfaces := environ.getNodegroupInterfaces(nodegroups)

	subnetIdSet := make(map[network.Id]bool)
	for _, netId := range subnetIds {
		subnetIdSet[netId] = false
	}
	processedIds := make(map[network.Id]bool)

	var networkInfo []network.SubnetInfo
	for _, subnet := range subnets {
		_, ok := subnetIdSet[network.Id(subnet.Name)]
		if !ok {
			// This id is not what we're looking for.
			continue
		}
		if _, ok := processedIds[network.Id(subnet.Name)]; ok {
			// Don't add the same subnet twice.
			continue
		}
		// mark that we've found this subnet
		processedIds[network.Id(subnet.Name)] = true
		subnetIdSet[network.Id(subnet.Name)] = true
		netCIDR := &net.IPNet{
			IP:   net.ParseIP(subnet.IP),
			Mask: net.IPMask(net.ParseIP(subnet.Mask)),
		}
		var allocatableHigh, allocatableLow net.IP
		for ip, bounds := range nodegroupInterfaces {
			contained := netCIDR.Contains(net.ParseIP(ip))
			if contained {
				allocatableLow = bounds[0]
				allocatableHigh = bounds[1]
				break
			}
		}
		subnetInfo := network.SubnetInfo{
			CIDR:              netCIDR.String(),
			VLANTag:           subnet.VLANTag,
			ProviderId:        network.Id(subnet.Name),
			AllocatableIPLow:  allocatableLow,
			AllocatableIPHigh: allocatableHigh,
		}

		// Verify we filled-in everything for all networks
		// and drop incomplete records.
		if subnetInfo.ProviderId == "" || subnetInfo.CIDR == "" {
			logger.Infof("ignoring subnet  %q: missing information (%#v)", subnet.Name, subnetInfo)
			continue
		}

		logger.Tracef("found subnet with info %#v", subnetInfo)
		networkInfo = append(networkInfo, subnetInfo)
	}
	logger.Debugf("available subnets for instance %v: %#v", inst.Id(), networkInfo)

	notFound := []network.Id{}
	for subnetId, found := range subnetIdSet {
		if !found {
			notFound = append(notFound, subnetId)
		}
	}
	if len(notFound) != 0 {
		return nil, errors.Errorf("failed to find the following subnets: %v", notFound)
	}

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
	if !environ.supportsDevices {
		// Warn the user that container resources can leak.
		logger.Warningf(noDevicesWarning)
	}

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
	Disabled      bool
}

// extractInterfaces parses the XML output of lswh and extracts all
// network interfaces, returing a map MAC address to ifaceInfo, as
// well as the interface name discovered as primary.
func extractInterfaces(inst instance.Instance, lshwXML []byte) (map[string]ifaceInfo, string, error) {
	type Node struct {
		Id          string `xml:"id,attr"`
		Disabled    bool   `xml:"disabled,attr,omitempty"`
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
	var baseIndex int
	processNodes = func(nodes []Node) error {
		for _, node := range nodes {
			if strings.HasPrefix(node.Id, "network") {
				index := baseIndex
				if strings.HasPrefix(node.Id, "network:") {
					// There is an index suffix, parse it.
					var err error
					index, err = strconv.Atoi(strings.TrimPrefix(node.Id, "network:"))
					if err != nil {
						return errors.Annotatef(err, "lshw output for node %q has invalid ID suffix for %q", inst.Id(), node.Id)
					}
				} else {
					baseIndex++
				}

				if primaryIface == "" && !node.Disabled {
					primaryIface = node.LogicalName
					logger.Debugf("node %q primary network interface is %q", inst.Id(), primaryIface)
				}
				interfaces[node.Serial] = ifaceInfo{
					DeviceIndex:   index,
					InterfaceName: node.LogicalName,
					Disabled:      node.Disabled,
				}
				if node.Disabled {
					logger.Debugf("node %q skipping disabled network interface %q", inst.Id(), node.LogicalName)
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
