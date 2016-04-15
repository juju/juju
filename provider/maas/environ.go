// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cloudconfig/providerinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/status"
	"github.com/juju/juju/tools"
)

const (
	// The string from the api indicating the dynamic range of a subnet.
	dynamicRange = "dynamic-range"
	// The version strings indicating the MAAS API version.
	apiVersion1 = "1.0"
	apiVersion2 = "2.0"
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
	ReserveIPAddressOnDevice = reserveIPAddressOnDevice
	NewDeviceParams          = newDeviceParams
	UpdateDeviceHostname     = updateDeviceHostname
	ReleaseIPAddress         = releaseIPAddress
	DeploymentStatusCall     = deploymentStatusCall
	GetCapabilities          = getCapabilities
	GetMAAS2Controller       = getMAAS2Controller
)

func getMAAS2Controller(maasServer, apiKey string) (gomaasapi.Controller, error) {
	return gomaasapi.NewController(gomaasapi.ControllerArgs{maasServer, apiKey})
}

func subnetToSpaceIds(spaces gomaasapi.MAASObject) (map[string]network.Id, error) {
	spacesJson, err := spaces.CallGet("", nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spacesArray, err := spacesJson.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsMap := make(map[string]network.Id)
	for _, spaceJson := range spacesArray {
		spaceMap, err := spaceJson.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerIdRaw, err := spaceMap["id"].GetFloat64()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerId := network.Id(fmt.Sprintf("%.0f", providerIdRaw))
		subnetsArray, err := spaceMap["subnets"].GetArray()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, subnetJson := range subnetsArray {
			subnetMap, err := subnetJson.GetMap()
			if err != nil {
				return nil, errors.Trace(err)
			}
			subnet, err := subnetMap["cidr"].GetString()
			if err != nil {
				return nil, errors.Trace(err)
			}
			subnetsMap[subnet] = providerId
		}
	}
	return subnetsMap, nil
}

func releaseNodes(nodes gomaasapi.MAASObject, ids url.Values) error {
	_, err := nodes.CallPost("release", ids)
	return err
}

func reserveIPAddressOnDevice(devices gomaasapi.MAASObject, deviceID, macAddress string, addr network.Address) (network.Address, error) {
	device := devices.GetSubObject(deviceID)
	params := url.Values{}
	if addr.Value != "" {
		params.Add("requested_address", addr.Value)
	}
	if macAddress != "" {
		params.Add("mac_address", macAddress)
	}
	resp, err := device.CallPost("claim_sticky_ip_address", params)
	if err != nil {
		return network.Address{}, errors.Annotatef(
			err, "failed to reserve sticky IP address for device %q",
			deviceID,
		)
	}
	respMap, err := resp.GetMap()
	if err != nil {
		return network.Address{}, errors.Annotate(err, "failed to parse response")
	}
	addresses, err := respMap["ip_addresses"].GetArray()
	if err != nil {
		return network.Address{}, errors.Annotatef(err, "failed to parse IP addresses")
	}
	if len(addresses) == 0 {
		return network.Address{}, errors.Errorf(
			"expected to find a sticky IP address for device %q: MAAS API response contains no IP addresses",
			deviceID,
		)
	}
	var firstAddress network.Address
	for _, address := range addresses {
		value, err := address.GetString()
		if err != nil {
			return network.Address{}, errors.Annotatef(err,
				"failed to parse reserved IP address for device %q",
				deviceID,
			)
		}
		if ip := net.ParseIP(value); ip == nil {
			return network.Address{}, errors.Annotatef(err,
				"failed to parse reserved IP address %q for device %q",
				value, deviceID,
			)
		}
		if firstAddress.Value == "" {
			// We only need the first address, but we're logging all we got.
			firstAddress = network.NewAddress(value)
		}
		logger.Debugf("reserved address %q for device %q and MAC %q", value, deviceID, macAddress)
	}
	return firstAddress, nil
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

	ecfgUnlocked       *maasModelConfig
	maasClientUnlocked *gomaasapi.MAASObject
	storageUnlocked    storage.Storage

	// maasController provides access to the MAAS 2.0 API.
	maasController gomaasapi.Controller

	availabilityZonesMutex sync.Mutex
	availabilityZones      []common.AvailabilityZone

	// apiVersion tells us if we are using the MAAS 1.0 or 2.0 api.
	apiVersion string
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

func (env *maasEnviron) usingMAAS2() bool {
	if !featureflag.Enabled(feature.MAAS2) {
		return false
	}
	return env.apiVersion == apiVersion2
}

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

// ControllerInstances is specified in the Environ interface.
func (env *maasEnviron) ControllerInstances() ([]instance.Id, error) {
	return common.ProviderStateInstances(env, env.Storage())
}

// ecfg returns the environment's maasModelConfig, and protects it with a
// mutex.
func (env *maasEnviron) ecfg() *maasModelConfig {
	env.ecfgMutex.Lock()
	cfg := *env.ecfgUnlocked
	env.ecfgMutex.Unlock()
	return &cfg
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
		return errors.Trace(err)
	}

	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	env.ecfgUnlocked = ecfg

	// We need to know the version of the server we're on. We support 1.9
	// and 2.0. MAAS 1.9 uses the 1.0 api version and 2.0 uses the 2.0 api
	// version.
	apiVersion := apiVersion2
	maas2Enabled := featureflag.Enabled(feature.MAAS2)
	controller, err := GetMAAS2Controller(ecfg.maasServer(), ecfg.maasOAuth())
	switch {
	case !maas2Enabled && err == nil:
		return errors.NewNotSupported(nil, "MAAS 2 is not supported unless the 'maas2' feature flag is set")
	case !maas2Enabled || gomaasapi.IsUnsupportedVersionError(err):
		apiVersion = apiVersion1
		authClient, err := gomaasapi.NewAuthenticatedClient(ecfg.maasServer(), ecfg.maasOAuth(), apiVersion1)
		if err != nil {
			return errors.Trace(err)
		}
		env.maasClientUnlocked = gomaasapi.NewMAAS(*authClient)
		caps, err := GetCapabilities(env.maasClientUnlocked)
		if err != nil {
			return errors.Trace(err)
		}
		if !caps.Contains(capNetworkDeploymentUbuntu) {
			return errors.NotSupportedf("MAAS 1.9 or more recent is required")
		}
	case err != nil:
		return errors.Trace(err)
	default:
		env.maasController = controller
	}
	env.apiVersion = apiVersion
	return nil
}

// SupportedArchitectures is specified on the EnvironCapability interface.
func (env *maasEnviron) SupportedArchitectures() ([]string, error) {
	env.archMutex.Lock()
	defer env.archMutex.Unlock()
	if env.supportedArchitectures != nil {
		return env.supportedArchitectures, nil
	}

	fetchArchitectures := env.allArchitecturesWithFallback
	if env.usingMAAS2() {
		fetchArchitectures = env.allArchitectures2
	}
	architectures, err := fetchArchitectures()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env.supportedArchitectures = architectures
	return env.supportedArchitectures, nil
}

// SupportsSpaces is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery is specified on environs.Networking.
func (env *maasEnviron) SupportsSpaceDiscovery() (bool, error) {
	return true, nil
}

// SupportsAddressAllocation is specified on environs.Networking.
func (env *maasEnviron) SupportsAddressAllocation(_ network.Id) (bool, error) {
	return true, nil
}

// allArchitectures2 uses the MAAS2 controller to get architectures from boot
// resources.
func (env *maasEnviron) allArchitectures2() ([]string, error) {
	resources, err := env.maasController.BootResources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	architectures := set.NewStrings()
	for _, resource := range resources {
		architectures.Add(strings.Split(resource.Architecture(), "/")[0])
	}
	return architectures.SortedValues(), nil
}

// allArchitectureWithFallback queries MAAS for all of the boot-images
// across all registered nodegroups and collapses them down to unique
// architectures.
func (env *maasEnviron) allArchitecturesWithFallback() ([]string, error) {
	architectures, err := env.allArchitectures()
	if err != nil || len(architectures) == 0 {
		logger.Debugf("error querying boot-images: %v", err)
		logger.Debugf("falling back to listing nodes")
		architectures, err := env.nodeArchitectures()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return architectures, nil
	} else {
		return architectures, nil
	}
}

func (env *maasEnviron) allArchitectures() ([]string, error) {
	nodegroups, err := env.getNodegroups()
	if err != nil {
		return nil, err
	}
	architectures := set.NewStrings()
	for _, nodegroup := range nodegroups {
		bootImages, err := env.nodegroupBootImages(nodegroup)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get boot images for nodegroup %v", nodegroup)
		}
		for _, image := range bootImages {
			architectures.Add(image.architecture)
		}
	}
	return architectures.SortedValues(), nil
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
	// This is fine - nodeArchitectures is only used in MAAS 1 cases.
	allInstances, err := env.instances1(filter)
	if err != nil {
		return nil, err
	}
	architectures := make(set.Strings)
	for _, inst := range allInstances {
		inst := inst.(*maas1Instance)
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
		var availabilityZones []common.AvailabilityZone
		var err error
		if e.usingMAAS2() {
			availabilityZones, err = e.availabilityZones2()
			if err != nil {
				return nil, errors.Trace(err)
			}
		} else {
			availabilityZones, err = e.availabilityZones1()
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		e.availabilityZones = availabilityZones
	}
	return e.availabilityZones, nil
}

func (e *maasEnviron) availabilityZones1() ([]common.AvailabilityZone, error) {
	zonesObject := e.getMAASClient().GetSubObject("zones")
	result, err := zonesObject.CallGet("", nil)
	if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusNotFound {
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
	return availabilityZones, nil
}

func (e *maasEnviron) availabilityZones2() ([]common.AvailabilityZone, error) {
	zones, err := e.maasController.Zones()
	if err != nil {
		return nil, errors.Trace(err)
	}
	availabilityZones := make([]common.AvailabilityZone, len(zones))
	for i, zone := range zones {
		availabilityZones[i] = maasAvailabilityZone{zone.Name()}
	}
	return availabilityZones, nil

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
		z, err := inst.(maasInstance).zone()
		if err != nil {
			logger.Errorf("could not get availability zone %v", err)
			continue
		}
		zones[i] = z
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
	capNetworkDeploymentUbuntu = "network-deployment-ubuntu"
)

// getCapabilities asks the MAAS server for its capabilities, if
// supported by the server.
func getCapabilities(client *gomaasapi.MAASObject) (set.Strings, error) {
	caps := make(set.Strings)
	var result gomaasapi.JSONObject
	var err error

	for a := shortAttempt.Start(); a.Next(); {
		version := client.GetSubObject("version/")
		result, err = version.CallGet("", nil)
		if err != nil {
			if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == 404 {
				return caps, errors.NotSupportedf("MAAS version 1.9 or more recent is required")
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

var dashSuffix = regexp.MustCompile("^(.*)-\\d+$")

func spaceNamesToSpaceInfo(spaces []string, spaceMap map[string]network.SpaceInfo) ([]network.SpaceInfo, error) {
	spaceInfos := []network.SpaceInfo{}
	for _, name := range spaces {
		info, ok := spaceMap[name]
		if !ok {
			matches := dashSuffix.FindAllStringSubmatch(name, 1)
			if matches == nil {
				return nil, errors.Errorf("unrecognised space in constraint %q", name)
			}
			// A -number was added to the space name when we
			// converted to a juju name, we found
			info, ok = spaceMap[matches[0][1]]
			if !ok {
				return nil, errors.Errorf("unrecognised space in constraint %q", name)
			}
		}
		spaceInfos = append(spaceInfos, info)
	}
	return spaceInfos, nil
}

func (environ *maasEnviron) spaceNamesToSpaceInfo(positiveSpaces, negativeSpaces []string) ([]network.SpaceInfo, []network.SpaceInfo, error) {
	spaces, err := environ.Spaces()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	spaceMap := make(map[string]network.SpaceInfo)
	empty := set.Strings{}
	for _, space := range spaces {
		jujuName := network.ConvertSpaceName(space.Name, empty)
		spaceMap[jujuName] = space
	}
	positiveSpaceIds, err := spaceNamesToSpaceInfo(positiveSpaces, spaceMap)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	negativeSpaceIds, err := spaceNamesToSpaceInfo(negativeSpaces, spaceMap)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return positiveSpaceIds, negativeSpaceIds, nil
}

// acquireNode2 allocates a machine from MAAS2.
func (environ *maasEnviron) acquireNode2(
	nodeName, zoneName string,
	cons constraints.Value,
	interfaces []interfaceBinding,
	volumes []volumeInfo,
) (maasInstance, error) {
	acquireParams := convertConstraints2(cons)
	positiveSpaceNames, negativeSpaceNames := convertSpacesFromConstraints(cons.Spaces)
	positiveSpaces, negativeSpaces, err := environ.spaceNamesToSpaceInfo(positiveSpaceNames, negativeSpaceNames)
	// If spaces aren't supported the constraints should be empty anyway.
	if err != nil && !errors.IsNotSupported(err) {
		return nil, errors.Trace(err)
	}
	err = addInterfaces2(&acquireParams, interfaces, positiveSpaces, negativeSpaces)
	if err != nil {
		return nil, errors.Trace(err)
	}
	addStorage2(&acquireParams, volumes)
	acquireParams.AgentName = environ.ecfg().maasAgentName()
	if zoneName != "" {
		acquireParams.Zone = zoneName
	}
	if nodeName != "" {
		acquireParams.Hostname = nodeName
	} else if cons.Arch == nil {
		logger.Warningf(
			"no architecture was specified, acquiring an arbitrary node",
		)
	}
	// Currently not using the constraints match returned here.
	machine, _, err := environ.maasController.AllocateMachine(acquireParams)

	if err != nil {
		return nil, errors.Trace(err)
	}
	return &maas2Instance{machine}, nil
}

// acquireNode allocates a node from the MAAS.
func (environ *maasEnviron) acquireNode(
	nodeName, zoneName string,
	cons constraints.Value,
	interfaces []interfaceBinding,
	volumes []volumeInfo,
) (gomaasapi.MAASObject, error) {

	acquireParams := convertConstraints(cons)
	positiveSpaceNames, negativeSpaceNames := convertSpacesFromConstraints(cons.Spaces)
	positiveSpaces, negativeSpaces, err := environ.spaceNamesToSpaceInfo(positiveSpaceNames, negativeSpaceNames)
	// If spaces aren't supported the constraints should be empty anyway.
	if err != nil && !errors.IsNotSupported(err) {
		return gomaasapi.MAASObject{}, errors.Trace(err)
	}
	err = addInterfaces(acquireParams, interfaces, positiveSpaces, negativeSpaces)
	if err != nil {
		return gomaasapi.MAASObject{}, errors.Trace(err)
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
	for a := shortAttempt.Start(); a.Next(); {
		client := environ.getMAASClient().GetSubObject("nodes/")
		logger.Tracef("calling acquire with params: %+v", acquireParams)
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
func (environ *maasEnviron) startNode(node gomaasapi.MAASObject, series string, userdata []byte) (*gomaasapi.MAASObject, error) {
	params := url.Values{
		"distro_series": {series},
		"user_data":     {string(userdata)},
	}
	// Initialize err to a non-nil value as a sentinel for the following
	// loop.
	err := fmt.Errorf("(no error)")
	var result gomaasapi.JSONObject
	for a := shortAttempt.Start(); a.Next() && err != nil; {
		result, err = node.CallPost("start", params)
		if err == nil {
			break
		}
	}

	if err == nil {
		var startedNode gomaasapi.MAASObject
		startedNode, err = result.GetMAASObject()
		if err != nil {
			logger.Errorf("cannot process API response after successfully starting node: %v", err)
			return nil, err
		}
		return &startedNode, nil
	}
	return nil, err
}

func (environ *maasEnviron) startNode2(node maas2Instance, series string, userdata []byte) (*maas2Instance, error) {
	err := node.machine.Start(gomaasapi.StartArgs{DistroSeries: series, UserData: string(userdata)})
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Machine.Start updates the machine in-place when it succeeds.
	return &maas2Instance{node.machine}, nil

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
		// TODO (mfoord): this branch is for old versions of MAAS and
		// can be removed, but this means fixing tests.
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

	var interfaceBindings []interfaceBinding
	if len(args.EndpointBindings) != 0 {
		for endpoint, spaceProviderID := range args.EndpointBindings {
			interfaceBindings = append(interfaceBindings, interfaceBinding{
				Name:            endpoint,
				SpaceProviderId: string(spaceProviderID),
			})
		}
	}
	snArgs := selectNodeArgs{
		Constraints:       args.Constraints,
		AvailabilityZones: availabilityZones,
		NodeName:          nodeName,
		Interfaces:        interfaceBindings,
		Volumes:           volumes,
	}
	var inst maasInstance
	if !environ.usingMAAS2() {
		selectedNode, err := environ.selectNode(snArgs)
		if err != nil {
			return nil, errors.Errorf("cannot run instances: %v", err)
		}

		inst = &maas1Instance{
			maasObject:   selectedNode,
			environ:      environ,
			statusGetter: environ.deploymentStatusOne,
		}
	} else {
		inst, err = environ.selectNode2(snArgs)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot run instances")
		}
	}
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
		return nil, errors.Trace(err)
	}
	args.InstanceConfig.Tools = selectedTools[0]

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
		return nil, errors.Trace(err)
	}
	series := args.InstanceConfig.Tools.Version.Series

	cloudcfg, err := environ.newCloudinitConfig(hostname, series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	userdata, err := providerinit.ComposeUserData(args.InstanceConfig, cloudcfg, MAASRenderer{})
	if err != nil {
		return nil, errors.Annotatef(err, "could not compose userdata for bootstrap node")
	}
	logger.Debugf("maas user data; %d bytes", len(userdata))

	subnetsMap, err := environ.subnetToSpaceIds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var interfaces []network.InterfaceInfo
	if !environ.usingMAAS2() {
		inst1 := inst.(*maas1Instance)
		startedNode, err := environ.startNode(*inst1.maasObject, series, userdata)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Once the instance has started the response should contain the
		// assigned IP addresses, even when NICs are set to "auto" instead of
		// "static". So instead of selectedNode, which only contains the
		// acquire-time details (no IP addresses for NICs set to "auto" vs
		// "static"), we use the up-to-date startedNode response to get the
		// interfaces.
		interfaces, err = maasObjectNetworkInterfaces(startedNode, subnetsMap)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		// TODO (mfoord): handling of interfaces to be added in a
		// follow-up.
		_, err := environ.startNode2(*inst.(*maas2Instance), series, userdata)
		if err != nil {
			return nil, errors.Trace(err)
		}
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
	// TODO (mfoord): inst.volumes not implemented for MAAS 2.
	resultVolumes, resultAttachments, err := inst.volumes(
		names.NewMachineTag(args.InstanceConfig.MachineId),
		requestedVolumes,
	)
	if err != nil {
		return nil, err
	}
	if len(resultVolumes) != len(requestedVolumes) {
		err = errors.Errorf("requested %v storage volumes. %v returned.", len(requestedVolumes), len(resultVolumes))
		return nil, err
	}

	return &environs.StartInstanceResult{
		Instance:          inst,
		Hardware:          hc,
		NetworkInfo:       interfaces,
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
	if environ.usingMAAS2() {
		return environ.waitForNodeDeployment2(id)
	}
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

func (environ *maasEnviron) waitForNodeDeployment2(id instance.Id) error {
	longAttempt := utils.AttemptStrategy{
		Delay: 10 * time.Second,
		Total: nodeDeploymentTimeout(environ),
	}

	for a := longAttempt.Start(); a.Next(); {
		machine, err := environ.getInstance(id)
		if err != nil {
			return errors.Trace(err)
		}
		stat := machine.Status()
		if stat.Status == status.StatusRunning {
			return nil
		}
		if stat.Status == status.StatusProvisioningError {
			return errors.Errorf("instance %q failed to deploy", id)

		}
	}
	return errors.Errorf("instance %q is started but not deployed", id)
}

func (environ *maasEnviron) deploymentStatusOne(id instance.Id) (string, string) {
	results, err := environ.deploymentStatus(id)
	if err != nil {
		return "", ""
	}
	systemId := extractSystemId(id)
	substatus := environ.getDeploymentSubstatus(systemId)
	return results[systemId], substatus
}

func (environ *maasEnviron) getDeploymentSubstatus(systemId string) string {
	nodesAPI := environ.getMAASClient().GetSubObject("nodes")
	result, err := nodesAPI.CallGet("list", nil)
	if err != nil {
		return ""
	}
	slices, err := result.GetArray()
	if err != nil {
		return ""
	}
	for _, slice := range slices {
		resultMap, err := slice.GetMap()
		if err != nil {
			continue
		}
		sysId, err := resultMap["system_id"].GetString()
		if err != nil {
			continue
		}
		if sysId == systemId {
			message, err := resultMap["substatus_message"].GetString()
			if err != nil {
				logger.Warningf("could not get string for substatus_message: %v", resultMap["substatus_message"])
				return ""
			}
			return message
		}
	}

	return ""
}

// deploymentStatus returns the deployment state of MAAS instances with
// the specified Juju instance ids.
// Note: the result is a map of MAAS systemId to state.
func (environ *maasEnviron) deploymentStatus(ids ...instance.Id) (map[string]string, error) {
	nodesAPI := environ.getMAASClient().GetSubObject("nodes")
	result, err := DeploymentStatusCall(nodesAPI, ids...)
	if err != nil {
		if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusBadRequest {
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

		if err, ok := errors.Cause(err).(gomaasapi.ServerError); ok && err.StatusCode == http.StatusConflict {
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

func (environ *maasEnviron) selectNode2(args selectNodeArgs) (maasInstance, error) {
	var err error
	var inst maasInstance

	for i, zoneName := range args.AvailabilityZones {
		inst, err = environ.acquireNode2(
			args.NodeName,
			zoneName,
			args.Constraints,
			args.Interfaces,
			args.Volumes,
		)

		if gomaasapi.IsNoMatchError(err) {
			if i+1 < len(args.AvailabilityZones) {
				logger.Infof("could not acquire a node in zone %q, trying another zone", zoneName)
				continue
			}
		}
		if err != nil {
			return nil, errors.Annotatef(err, "cannot run instance")
		}
		// Since a return at the end of the function is required
		// just break here.
		break
	}
	return inst, nil
}

// setupJujuNetworking returns a string representing the script to run
// in order to prepare the Juju-specific networking config on a node.
func setupJujuNetworking() string {
	// For ubuntu series < xenial we prefer python2 over python3
	// as we don't want to invalidate lots of testing against
	// known cloud-image contents. A summary of Ubuntu releases
	// and python inclusion in the default install of Ubuntu
	// Server is as follows:
	//
	// 12.04 precise:  python 2 (2.7.3)
	// 14.04 trusty:   python 2 (2.7.5) and python3 (3.4.0)
	// 14.10 utopic:   python 2 (2.7.8) and python3 (3.4.2)
	// 15.04 vivid:    python 2 (2.7.9) and python3 (3.4.3)
	// 15.10 wily:     python 2 (2.7.9) and python3 (3.4.3)
	// 16.04 xenial:   python 3 only (3.5.1)
	//
	// going forward:  python 3 only

	return fmt.Sprintf(`
trap 'rm -f %[1]q' EXIT

if [ -x /usr/bin/python2 ]; then
    juju_networking_preferred_python_binary=/usr/bin/python2
elif [ -x /usr/bin/python3 ]; then
    juju_networking_preferred_python_binary=/usr/bin/python3
elif [ -x /usr/bin/python ]; then
    juju_networking_preferred_python_binary=/usr/bin/python
fi

if [ ! -z "${juju_networking_preferred_python_binary:-}" ]; then
    if [ -f %[1]q ]; then
# We are sharing this code between master, maas-spaces2 and 1.25.
# For the moment we want master and 1.25 to not bridge all interfaces.
# This setting allows us to easily switch the behaviour when merging
# the code between those various branches.
        juju_bridge_all_interfaces=1
        if [ $juju_bridge_all_interfaces -eq 1 ]; then
            $juju_networking_preferred_python_binary %[1]q --bridge-prefix=%[2]q --one-time-backup --activate %[4]q
        else
            juju_ipv4_interface_to_bridge=$(ip -4 route list exact default | head -n1 | cut -d' ' -f5)
            $juju_networking_preferred_python_binary %[1]q --bridge-name=%[3]q --interface-to-bridge="${juju_ipv4_interface_to_bridge:-unknown}" --one-time-backup --activate %[4]q
        fi
    fi
else
    echo "error: no Python installation found; cannot run Juju's bridge script"
fi`,
		bridgeScriptPath,
		instancecfg.DefaultBridgePrefix,
		instancecfg.DefaultBridgeName,
		"/etc/network/interfaces")
}

func renderEtcNetworkInterfacesScript() string {
	return setupJujuNetworking()
}

// newCloudinitConfig creates a cloudinit.Config structure suitable as a base
// for initialising a MAAS node.
func (environ *maasEnviron) newCloudinitConfig(hostname, forSeries string) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(forSeries)
	if err != nil {
		return nil, err
	}

	info := machineInfo{hostname}
	runCmd, err := info.cloudinitRunCmd(cloudcfg)
	if err != nil {
		return nil, errors.Trace(err)
	}

	operatingSystem, err := series.GetOSFromSeries(forSeries)
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
			cloudcfg.AddPackage("bridge-utils")
			cloudcfg.AddBootTextFile(bridgeScriptPath, bridgeScriptPython, 0755)
			cloudcfg.AddScripts(setupJujuNetworking())
		}
	}
	return cloudcfg, nil
}

func (environ *maasEnviron) releaseNodes1(nodes gomaasapi.MAASObject, ids url.Values, recurse bool) error {
	err := ReleaseNodes(nodes, ids)
	if err == nil {
		return nil
	}
	maasErr, ok := errors.Cause(err).(gomaasapi.ServerError)
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
		err := environ.releaseNodes1(nodes, idFilter, false)
		if err != nil {
			lastErr = err
			logger.Errorf("error while releasing node %v (%v)", id, err)
		}
	}
	return errors.Trace(lastErr)

}

func (environ *maasEnviron) releaseNodes2(ids []instance.Id, recurse bool) error {
	args := gomaasapi.ReleaseMachinesArgs{
		SystemIDs: instanceIdsToSystemIDs(ids),
		Comment:   "Released by Juju MAAS provider",
	}
	err := environ.maasController.ReleaseMachines(args)

	switch {
	case err == nil:
		return nil
	case gomaasapi.IsCannotCompleteError(err):
		// CannotCompleteError means a node couldn't be released due to
		// a state conflict. Likely it's already released or disk
		// erasing. We're assuming this error *only* means it's
		// safe to assume the instance is already released.
		// MaaS also releases (or attempts) all nodes, and raises
		// a single error on failure. So even with an error 409, all
		// nodes have been released.
		logger.Infof("ignoring error while releasing nodes (%v); all nodes released OK", err)
		return nil
	case gomaasapi.IsBadRequestError(err), gomaasapi.IsPermissionError(err):
		// a status code of 400 or 403 means one of the nodes
		// couldn't be found and none have been released. We have to
		// release all the ones we can individually.
		if !recurse {
			// this node has already been released and we're golden
			return nil
		}
		return environ.releaseNodesIndividually(ids)

	default:
		return errors.Annotatef(err, "cannot release nodes")
	}
}

func (environ *maasEnviron) releaseNodesIndividually(ids []instance.Id) error {
	var lastErr error
	for _, id := range ids {
		err := environ.releaseNodes2([]instance.Id{id}, false)
		if err != nil {
			lastErr = err
			logger.Errorf("error while releasing node %v (%v)", id, err)
		}
	}
	return errors.Trace(lastErr)
}

func instanceIdsToSystemIDs(ids []instance.Id) []string {
	systemIDs := make([]string, len(ids))
	for index, id := range ids {
		systemIDs[index] = string(id)
	}
	return systemIDs
}

// StopInstances is specified in the InstanceBroker interface.
func (environ *maasEnviron) StopInstances(ids ...instance.Id) error {
	// Shortcut to exit quickly if 'instances' is an empty slice or nil.
	if len(ids) == 0 {
		return nil
	}

	if environ.usingMAAS2() {
		err := environ.releaseNodes2(ids, true)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		nodes := environ.getMAASClient().GetSubObject("nodes")
		err := environ.releaseNodes1(nodes, getSystemIdValues("nodes", ids), true)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return common.RemoveStateInstances(environ.Storage(), ids...)

}

// acquireInstances calls the MAAS API to list acquired nodes.
//
// The "ids" slice is a filter for specific instance IDs.
// Due to how this works in the HTTP API, an empty "ids"
// matches all instances (not none as you might expect).
func (environ *maasEnviron) acquiredInstances(ids []instance.Id) ([]instance.Instance, error) {
	if !environ.usingMAAS2() {
		filter := getSystemIdValues("id", ids)
		filter.Add("agent_name", environ.ecfg().maasAgentName())
		return environ.instances1(filter)
	}
	args := gomaasapi.MachinesArgs{
		AgentName: environ.ecfg().maasAgentName(),
		SystemIDs: instanceIdsToSystemIDs(ids),
	}
	return environ.instances2(args)
}

// instances calls the MAAS API to list nodes matching the given filter.
func (environ *maasEnviron) instances1(filter url.Values) ([]instance.Instance, error) {
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
		instances[index] = &maas1Instance{
			maasObject:   &node,
			environ:      environ,
			statusGetter: environ.deploymentStatusOne,
		}
	}
	return instances, nil
}

func (environ *maasEnviron) instances2(args gomaasapi.MachinesArgs) ([]instance.Instance, error) {
	machines, err := environ.maasController.Machines(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	instances := make([]instance.Instance, len(machines))
	for index, machine := range machines {
		instances[index] = &maas2Instance{machine}
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
		return nil, errors.Trace(err)
	}
	if len(instances) == 0 {
		return nil, environs.ErrNoInstances
	}

	idMap := make(map[instance.Id]instance.Instance)
	for _, instance := range instances {
		idMap[instance.Id()] = instance
	}

	missing := false
	result := make([]instance.Instance, len(ids))
	for index, id := range ids {
		val, ok := idMap[id]
		if !ok {
			missing = true
			continue
		}
		result[index] = val
	}

	if missing {
		return result, environs.ErrPartialInstances
	}
	return result, nil
}

// transformDeviceHostname transforms deviceHostname to include hostnameSuffix
// after the first "." in deviceHostname. Returns errors if deviceHostname does
// not contain any "." or hostnameSuffix is empty.
func transformDeviceHostname(deviceID, deviceHostname, hostnameSuffix string) (string, error) {
	if hostnameSuffix == "" {
		return "", errors.New("hostname suffix cannot be empty")
	}
	parts := strings.SplitN(deviceHostname, ".", 2)
	if len(parts) != 2 {
		return "", errors.Errorf("unexpected device %q hostname %q", deviceID, deviceHostname)
	}
	return fmt.Sprintf("%s-%s.%s", parts[0], hostnameSuffix, parts[1]), nil
}

// updateDeviceHostname updates the hostname of a MAAS device to be unique and
// to contain the given hostnameSuffix.
func updateDeviceHostname(client *gomaasapi.MAASObject, deviceID, deviceHostname, hostnameSuffix string) (string, error) {

	newHostname, err := transformDeviceHostname(deviceID, deviceHostname, hostnameSuffix)
	if err != nil {
		return "", errors.Trace(err)
	}

	deviceObj := client.GetSubObject("devices").GetSubObject(deviceID)
	params := make(url.Values)
	params.Add("hostname", newHostname)
	if _, err := deviceObj.Update(params); err != nil {
		return "", errors.Annotatef(err, "updating device %q hostname to %q", deviceID, newHostname)
	}
	return newHostname, nil
}

// newDeviceParams prepares the params to call "devices new" API. Declared
// separately so it can be mocked out in the test to work around the gomaasapi's
// testservice limitation.
func newDeviceParams(macAddress string, instId instance.Id, _ string) url.Values {
	params := make(url.Values)
	params.Add("mac_addresses", macAddress)
	// We create the device without a hostname, to allow MAAS to create a unique
	// hostname first.
	params.Add("parent", extractSystemId(instId))

	return params
}

// newDevice creates a new MAAS device with parent instance instId, using the
// given macAddress and hostnameSuffix, returning the ID of the new device.
func (environ *maasEnviron) newDevice(macAddress string, instId instance.Id, hostnameSuffix string) (string, error) {
	client := environ.getMAASClient()
	devices := client.GetSubObject("devices")
	// Make the params in a separate function to make it easier to work
	// around the limitation of gomaasapi's testservice expecting all 3
	// arguments (parent, mac_addresses, and hostname) to be filled in.
	params := NewDeviceParams(macAddress, instId, hostnameSuffix)
	logger.Tracef(
		"creating a new MAAS device for MAC %q, parent %q", macAddress, instId,
	)
	result, err := devices.CallPost("new", params)
	if err != nil {
		return "", errors.Trace(err)
	}

	resultMap, err := result.GetMap()
	if err != nil {
		return "", errors.Trace(err)
	}

	deviceID, err := resultMap["system_id"].GetString()
	if err != nil {
		return "", errors.Trace(err)
	}
	deviceHostname, err := resultMap["hostname"].GetString()
	if err != nil {
		return deviceID, errors.Trace(err)
	}

	logger.Tracef("created device %q with MAC %q and hostname %q", deviceID, macAddress, deviceHostname)

	newHostname, err := UpdateDeviceHostname(client, deviceID, deviceHostname, hostnameSuffix)
	if err != nil {
		return deviceID, errors.Trace(err)
	}
	logger.Tracef("updated device %q hostname to %q", deviceID, newHostname)

	return deviceID, nil
}

// fetchFullDevice fetches an existing device ID associated with the given
// macAddress, or returns an error if there is no device.
func (environ *maasEnviron) fetchFullDevice(macAddress string) (map[string]gomaasapi.JSONObject, error) {
	if macAddress == "" {
		return nil, errors.Errorf("given MAC address is empty")
	}

	client := environ.getMAASClient()
	devices := client.GetSubObject("devices")
	params := url.Values{}
	params.Add("mac_address", macAddress)

	result, err := devices.CallGet("list", params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resultArray, err := result.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(resultArray) == 0 {
		return nil, errors.NotFoundf("no device for MAC address %q", macAddress)
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

func (environ *maasEnviron) fetchDevice(macAddress string) (string, error) {
	deviceMap, err := environ.fetchFullDevice(macAddress)
	if err != nil {
		return "", errors.Trace(err)
	}

	deviceID, err := deviceMap["system_id"].GetString()
	if err != nil {
		return "", errors.Trace(err)
	}
	return deviceID, nil
}

// createOrFetchDevice returns a device Id associated with a MAC address. If
// there is not already one it will create one.
func (environ *maasEnviron) createOrFetchDevice(macAddress string, instId instance.Id, hostname string) (string, error) {
	device, err := environ.fetchDevice(macAddress)
	if err == nil {
		return device, nil
	}
	if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	return environ.newDevice(macAddress, instId, hostname)
}

// AllocateAddress requests an address to be allocated for the
// given instance on the given network.
func (environ *maasEnviron) AllocateAddress(instId instance.Id, subnetId network.Id, addr *network.Address, macAddress, hostname string) (err error) {
	logger.Tracef(
		"AllocateAddress for instId %q, subnet %q, addr %q, MAC %q, hostname %q",
		instId, subnetId, addr, macAddress, hostname,
	)
	if addr == nil {
		return errors.NewNotValid(nil, "invalid address: cannot be nil")
	}

	if !environs.AddressAllocationEnabled() {
		logger.Tracef("creating device for container %q with MAC %q", hostname, macAddress)
		deviceID, err := environ.createOrFetchDevice(macAddress, instId, hostname)
		if err != nil {
			return errors.Annotatef(
				err,
				"failed creating MAAS device for container %q with MAC address %q",
				hostname, macAddress,
			)
		}
		logger.Infof(
			"created device %q for container %q with MAC address %q on parent node %q",
			deviceID, hostname, macAddress, instId,
		)
		devices := environ.getMAASClient().GetSubObject("devices")
		newAddr, err := ReserveIPAddressOnDevice(devices, deviceID, macAddress, network.Address{})
		if err != nil {
			return errors.Trace(err)
		}
		logger.Infof(
			"reserved sticky IP address %q for device %q with MAC address %q representing container %q",
			newAddr, deviceID, macAddress, hostname,
		)
		*addr = newAddr
		return nil
	}
	defer errors.DeferredAnnotatef(&err, "failed to allocate address %q for instance %q", addr, instId)

	client := environ.getMAASClient()
	deviceID, err := environ.createOrFetchDevice(macAddress, instId, hostname)
	if err != nil {
		return errors.Trace(err)
	}

	devices := client.GetSubObject("devices")
	newAddr, err := ReserveIPAddressOnDevice(devices, deviceID, macAddress, *addr)
	if err == nil {
		logger.Infof(
			"allocated address %q for instance %q on device %q (asked for address %q)",
			addr, instId, deviceID, newAddr,
		)
		return nil
	}

	maasErr, ok := errors.Cause(err).(gomaasapi.ServerError)
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
		logger.Tracef("getting device ID for container %q with MAC %q", macAddress, hostname)
		deviceID, err := environ.fetchDevice(macAddress)
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
		"releasing address: %q, MAC address: %q, hostname: %q",
		addr, macAddress, hostname,
	)
	// Addresses originally allocated without a device will have macAddress
	// set to "". We shouldn't look for a device for these addresses.
	if macAddress != "" {
		device, err := environ.fetchFullDevice(macAddress)
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

// subnetsFromNode fetches all the subnets for a specific node.
func (environ *maasEnviron) subnetsFromNode(nodeId string) ([]gomaasapi.JSONObject, error) {
	client := environ.getMAASClient().GetSubObject("nodes").GetSubObject(nodeId)
	json, err := client.CallGet("", nil)
	if err != nil {
		if maasErr, ok := errors.Cause(err).(gomaasapi.ServerError); ok && maasErr.StatusCode == http.StatusNotFound {
			return nil, errors.NotFoundf("intance %q", nodeId)
		}
		return nil, errors.Trace(err)
	}
	nodeMap, err := json.GetMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfacesArray, err := nodeMap["interface_set"].GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var subnets []gomaasapi.JSONObject
	for _, iface := range interfacesArray {
		ifaceMap, err := iface.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		linksArray, err := ifaceMap["links"].GetArray()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, link := range linksArray {
			linkMap, err := link.GetMap()
			if err != nil {
				return nil, errors.Trace(err)
			}
			subnet, ok := linkMap["subnet"]
			if !ok {
				return nil, errors.New("subnet not found")
			}
			subnets = append(subnets, subnet)
		}
	}
	return subnets, nil
}

// Deduce the allocatable portion of the subnet by subtracting the dynamic
// range from the full subnet range.
func (environ *maasEnviron) allocatableRangeForSubnet(cidr string, subnetId string) (net.IP, net.IP, error) {
	// Initialize the low and high bounds of the allocatable range to the
	// whole CIDR. Reduce the scope of this when we find the dynamic range.
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// Skip IPv6 subnets until we can handle them correctly.
	if ip.To4() == nil && ip.To16() != nil {
		logger.Debugf("ignoring static IP range for IPv6 subnet %q", cidr)
		return nil, nil, nil
	}

	// TODO(mfoord): needs updating to work with IPv6 as well.
	lowBound, err := network.IPv4ToDecimal(ip)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// Don't include the zero address in the allocatable bounds.
	lowBound = lowBound + 1
	ones, bits := ipnet.Mask.Size()
	zeros := bits - ones
	numIPs := uint32(1) << uint32(zeros)
	highBound := lowBound + numIPs - 2

	client := environ.getMAASClient().GetSubObject("subnets").GetSubObject(subnetId)

	json, err := client.CallGet("reserved_ip_ranges", nil)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	jsonRanges, err := json.GetArray()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	for _, jsonRange := range jsonRanges {
		rangeMap, err := jsonRange.GetMap()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		purposeArray, err := rangeMap["purpose"].GetArray()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		found := false
		for _, jsonPurpose := range purposeArray {
			purpose, err := jsonPurpose.GetString()
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			if purpose == dynamicRange {
				found = true
				break
			}
		}
		if !found {
			// This is not the range we're looking for
			continue
		}

		start, err := rangeMap["start"].GetString()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		end, err := rangeMap["end"].GetString()
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		dynamicLow, err := network.IPv4ToDecimal(net.ParseIP(start))
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		dynamicHigh, err := network.IPv4ToDecimal(net.ParseIP(end))
		if err != nil {
			return nil, nil, errors.Trace(err)
		}

		// We pick the larger of the two portions of the subnet around
		// the dynamic range. Either ending one below the start of the
		// dynamic range or starting one after the end.
		above := highBound - dynamicHigh
		below := dynamicLow - lowBound
		if above > below {
			lowBound = dynamicHigh + 1
		} else {
			highBound = dynamicLow - 1
		}
		break
	}
	return network.DecimalToIPv4(lowBound), network.DecimalToIPv4(highBound), nil
}

// subnetsWithSpaces uses the MAAS 1.9+ API to fetch subnet information
// including space id.
func (environ *maasEnviron) subnetsWithSpaces(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	var nodeId string
	if instId != instance.UnknownId {
		inst, err := environ.getInstance(instId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		nodeId, err = environ.nodeIdFromInstance(inst)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	subnets, err := environ.filteredSubnets(nodeId, subnetIds)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if instId != instance.UnknownId {
		logger.Debugf("instance %q has subnets %v", instId, subnets)
	} else {
		logger.Debugf("found subnets %v", subnets)
	}

	return subnets, nil
}

// subnetFromJson populates a network.SubnetInfo from a gomaasapi.JSONObject
// representing a single subnet. This can come from either the subnets api
// endpoint or the node endpoint.
func (environ *maasEnviron) subnetFromJson(subnet gomaasapi.JSONObject, spaceId network.Id) (network.SubnetInfo, error) {
	var subnetInfo network.SubnetInfo
	fields, err := subnet.GetMap()
	if err != nil {
		return subnetInfo, errors.Trace(err)
	}
	subnetIdFloat, err := fields["id"].GetFloat64()
	if err != nil {
		return subnetInfo, errors.Annotatef(err, "cannot get subnet Id")
	}
	subnetId := strconv.Itoa(int(subnetIdFloat))
	cidr, err := fields["cidr"].GetString()
	if err != nil {
		return subnetInfo, errors.Annotatef(err, "cannot get cidr")
	}
	vid := 0
	vidField, ok := fields["vid"]
	if ok && !vidField.IsNil() {
		// vid is optional, so assume it's 0 when missing or nil.
		vidFloat, err := vidField.GetFloat64()
		if err != nil {
			return subnetInfo, errors.Errorf("cannot get vlan tag: %v", err)
		}
		vid = int(vidFloat)
	}
	allocatableLow, allocatableHigh, err := environ.allocatableRangeForSubnet(cidr, subnetId)
	if err != nil {
		return subnetInfo, errors.Trace(err)
	}

	subnetInfo = network.SubnetInfo{
		ProviderId:        network.Id(subnetId),
		VLANTag:           vid,
		CIDR:              cidr,
		SpaceProviderId:   spaceId,
		AllocatableIPLow:  allocatableLow,
		AllocatableIPHigh: allocatableHigh,
	}
	return subnetInfo, nil
}

// filteredSubnets fetches subnets, filtering optionally by nodeId and/or a
// slice of subnetIds. If subnetIds is empty then all subnets for that node are
// fetched. If nodeId is empty, all subnets are returned (filtering by subnetIds
// first, if set).
func (environ *maasEnviron) filteredSubnets(nodeId string, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	var jsonNets []gomaasapi.JSONObject
	var err error
	if nodeId != "" {
		jsonNets, err = environ.subnetsFromNode(nodeId)
		if err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		jsonNets, err = environ.fetchAllSubnets()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	subnetIdSet := make(map[string]bool)
	for _, netId := range subnetIds {
		subnetIdSet[string(netId)] = false
	}

	subnetsMap, err := environ.subnetToSpaceIds()
	if err != nil {
		return nil, errors.Trace(err)
	}

	subnets := []network.SubnetInfo{}
	for _, jsonNet := range jsonNets {
		fields, err := jsonNet.GetMap()
		if err != nil {
			return nil, err
		}
		subnetIdFloat, err := fields["id"].GetFloat64()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get subnet Id: %v")
		}
		subnetId := strconv.Itoa(int(subnetIdFloat))
		// If we're filtering by subnet id check if this subnet is one
		// we're looking for.
		if len(subnetIds) != 0 {
			_, ok := subnetIdSet[subnetId]
			if !ok {
				// This id is not what we're looking for.
				continue
			}
			subnetIdSet[subnetId] = true
		}
		cidr, err := fields["cidr"].GetString()
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get subnet Id")
		}
		spaceId, ok := subnetsMap[cidr]
		if !ok {
			logger.Warningf("unrecognised subnet: %q, setting empty space id", cidr)
			spaceId = network.UnknownId
		}

		subnetInfo, err := environ.subnetFromJson(jsonNet, spaceId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnets = append(subnets, subnetInfo)
		logger.Tracef("found subnet with info %#v", subnetInfo)
	}
	return subnets, checkNotFound(subnetIdSet)
}

func (environ *maasEnviron) getInstance(instId instance.Id) (instance.Instance, error) {
	instances, err := environ.acquiredInstances([]instance.Id{instId})
	// TODO (mfoord): the error returned from gomaasapi for MAAS 2 will be
	// different.
	if err != nil {
		if maasErr, ok := errors.Cause(err).(gomaasapi.ServerError); ok && maasErr.StatusCode == http.StatusNotFound {
			return nil, errors.NotFoundf("instance %q", instId)
		}
		return nil, errors.Annotatef(err, "getting instance %q", instId)
	}
	if len(instances) == 0 {
		return nil, errors.NotFoundf("instance %q", instId)
	}
	inst := instances[0]
	return inst, nil
}

// fetchAllSubnets calls the MAAS subnets API to get all subnets and returns the
// JSON response or an error. If capNetworkDeploymentUbuntu is not available, an
// error satisfying errors.IsNotSupported will be returned.
func (environ *maasEnviron) fetchAllSubnets() ([]gomaasapi.JSONObject, error) {
	client := environ.getMAASClient().GetSubObject("subnets")

	json, err := client.CallGet("", nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return json.GetArray()
}

// subnetToSpaceIds fetches the spaces from MAAS and builds a map of subnets to
// space ids.
func (environ *maasEnviron) subnetToSpaceIds() (map[string]network.Id, error) {
	subnetsMap := make(map[string]network.Id)
	spaces, err := environ.Spaces()
	if err != nil {
		return subnetsMap, errors.Trace(err)
	}
	for _, space := range spaces {
		for _, subnet := range space.Subnets {
			subnetsMap[subnet.CIDR] = space.ProviderId
		}
	}
	return subnetsMap, nil
}

// Spaces returns all the spaces, that have subnets, known to the provider.
// Space name is not filled in as the provider doesn't know the juju name for
// the space.
func (environ *maasEnviron) Spaces() ([]network.SpaceInfo, error) {
	if environ.usingMAAS2() {
		return environ.spaces2()
	} else {
		return environ.spaces1()
	}
}

func (environ *maasEnviron) spaces1() ([]network.SpaceInfo, error) {
	spacesClient := environ.getMAASClient().GetSubObject("spaces")
	spacesJson, err := spacesClient.CallGet("", nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	spacesArray, err := spacesJson.GetArray()
	if err != nil {
		return nil, errors.Trace(err)
	}
	spaces := []network.SpaceInfo{}
	for _, spaceJson := range spacesArray {
		spaceMap, err := spaceJson.GetMap()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerIdRaw, err := spaceMap["id"].GetFloat64()
		if err != nil {
			return nil, errors.Trace(err)
		}
		providerId := network.Id(fmt.Sprintf("%.0f", providerIdRaw))
		name, err := spaceMap["name"].GetString()
		if err != nil {
			return nil, errors.Trace(err)
		}

		space := network.SpaceInfo{Name: name, ProviderId: providerId}
		subnetsArray, err := spaceMap["subnets"].GetArray()
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, subnetJson := range subnetsArray {
			subnet, err := environ.subnetFromJson(subnetJson, providerId)
			if err != nil {
				return nil, errors.Trace(err)
			}
			space.Subnets = append(space.Subnets, subnet)
		}
		// Skip spaces with no subnets.
		if len(space.Subnets) > 0 {
			spaces = append(spaces, space)
		}
	}
	return spaces, nil
}

func (environ *maasEnviron) spaces2() ([]network.SpaceInfo, error) {
	spaces, err := environ.maasController.Spaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []network.SpaceInfo
	for _, space := range spaces {
		if len(space.Subnets()) == 0 {
			continue
		}
		outSpace := network.SpaceInfo{
			Name:       space.Name(),
			ProviderId: network.Id(strconv.Itoa(space.ID())),
			Subnets:    make([]network.SubnetInfo, len(space.Subnets())),
		}
		for i, subnet := range space.Subnets() {
			subnetInfo := network.SubnetInfo{
				ProviderId:      network.Id(strconv.Itoa(subnet.ID())),
				VLANTag:         subnet.VLAN().VID(),
				CIDR:            subnet.CIDR(),
				SpaceProviderId: network.Id(strconv.Itoa(space.ID())),
				// TODO (babbageclunk): not setting
				// AllocatableIPLow/High - these aren't exposed in
				// gomaasapi just yet.
			}
			outSpace.Subnets[i] = subnetInfo
		}
		result = append(result, outSpace)
	}
	return result, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance. subnetIds must not be
// empty. Implements NetworkingEnviron.Subnets.
func (environ *maasEnviron) Subnets(instId instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	return environ.subnetsWithSpaces(instId, subnetIds)
}

func checkNotFound(subnetIdSet map[string]bool) error {
	notFound := []string{}
	for subnetId, found := range subnetIdSet {
		if !found {
			notFound = append(notFound, string(subnetId))
		}
	}
	if len(notFound) != 0 {
		return errors.Errorf("failed to find the following subnets: %v", strings.Join(notFound, ", "))
	}
	return nil
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
	if environ.ecfg().maasAgentName() == "" {
		logger.Warningf("No MAAS agent name specified.\n\n" +
			"The environment is either not running or from a very early Juju version.\n" +
			"It is not safe to release all MAAS instances without an agent name.\n" +
			"If the environment is still running, please manually decomission the MAAS machines.")
		return errors.New("unsafe destruction")
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

func (environ *maasEnviron) nodeIdFromInstance(inst instance.Instance) (string, error) {
	maasInst := inst.(*maas1Instance)
	maasObj := maasInst.maasObject
	nodeId, err := maasObj.GetField("system_id")
	if err != nil {
		return "", err
	}
	return nodeId, err
}

func (env *maasEnviron) AllocateContainerAddresses(hostInstanceID instance.Id, preparedInfo []network.InterfaceInfo) ([]network.InterfaceInfo, error) {
	if len(preparedInfo) == 0 {
		return nil, errors.Errorf("no prepared info to allocate")
	}
	logger.Debugf("using prepared container info: %+v", preparedInfo)

	subnetCIDRToVLANID := make(map[string]string)
	subnetsAPI := env.getMAASClient().GetSubObject("subnets")
	result, err := subnetsAPI.CallGet("", nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get subnets")
	}
	subnetsJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get subnets JSON")
	}
	var subnets []maasSubnet
	if err := json.Unmarshal(subnetsJSON, &subnets); err != nil {
		return nil, errors.Annotate(err, "cannot parse subnets JSON")
	}
	for _, subnet := range subnets {
		subnetCIDRToVLANID[subnet.CIDR] = strconv.Itoa(subnet.VLAN.ID)
	}

	var primaryNICInfo network.InterfaceInfo
	for _, nic := range preparedInfo {
		if nic.InterfaceName == "eth0" {
			primaryNICInfo = nic
			break
		}
	}
	if primaryNICInfo.InterfaceName == "" {
		return nil, errors.Errorf("cannot find primary interface for container")
	}
	logger.Debugf("primary device NIC prepared info: %+v", primaryNICInfo)

	primaryMACAddress := primaryNICInfo.MACAddress
	containerDevice, err := env.createDevice(hostInstanceID, primaryMACAddress)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create device for container")
	}
	deviceID := instance.Id(containerDevice.ResourceURI)
	logger.Debugf("created device %q with primary MAC address %q", deviceID, primaryMACAddress)

	interfaces, err := env.deviceInterfaces(deviceID)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	if len(interfaces) != 1 {
		return nil, errors.Errorf("expected 1 device interface, got %d", len(interfaces))
	}

	primaryNICName := interfaces[0].Name
	primaryNICID := strconv.Itoa(interfaces[0].ID)
	primaryNICSubnetCIDR := primaryNICInfo.CIDR
	primaryNICVLANID := subnetCIDRToVLANID[primaryNICSubnetCIDR]
	updatedPrimaryNIC, err := env.updateDeviceInterface(deviceID, primaryNICID, primaryNICName, primaryMACAddress, primaryNICVLANID)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot update device interface %q", interfaces[0].Name)
	}
	logger.Debugf("device %q primary interface %q updated: %+v", containerDevice.SystemID, primaryNICName, updatedPrimaryNIC)

	deviceNICIDs := make([]string, len(preparedInfo))
	nameToParentName := make(map[string]string)
	for i, nic := range preparedInfo {
		maasNICID := ""
		nameToParentName[nic.InterfaceName] = nic.ParentInterfaceName
		if nic.InterfaceName != primaryNICName {
			nicVLANID := subnetCIDRToVLANID[nic.CIDR]
			createdNIC, err := env.createDeviceInterface(deviceID, nic.InterfaceName, nic.MACAddress, nicVLANID)
			if err != nil {
				return nil, errors.Annotate(err, "creating device interface")
			}
			maasNICID = strconv.Itoa(createdNIC.ID)
			logger.Debugf("created device interface: %+v", createdNIC)
		} else {
			maasNICID = primaryNICID
		}
		deviceNICIDs[i] = maasNICID
		subnetID := string(nic.ProviderSubnetId)

		linkedInterface, err := env.linkDeviceInterfaceToSubnet(deviceID, maasNICID, subnetID, modeStatic)
		if err != nil {
			return nil, errors.Annotate(err, "cannot link device interface to subnet")
		}
		logger.Debugf("linked device interface to subnet: %+v", linkedInterface)
	}
	finalInterfaces, err := env.deviceInterfaceInfo(deviceID, nameToParentName)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get device interfaces")
	}
	logger.Debugf("allocated device interfaces: %+v", finalInterfaces)
	return finalInterfaces, nil
}
