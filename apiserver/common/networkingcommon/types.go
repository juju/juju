// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"encoding/json"
	"net"
	"regexp"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	providercommon "github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
)

// BackingSubnet defines the methods supported by a Subnet entity
// stored persistently.
//
// TODO(dimitern): Once the state backing is implemented, remove this
// and just use *state.Subnet.
type BackingSubnet interface {
	CIDR() string
	VLANTag() int
	ProviderId() network.Id
	AvailabilityZones() []string
	Status() string
	SpaceName() string
	Life() params.Life
}

// BackingSubnetInfo describes a single subnet to be added in the
// backing store.
//
// TODO(dimitern): Replace state.SubnetInfo with this and remove
// BackingSubnetInfo, once the rest of state backing methods and the
// following pre-reqs are done:
// * subnetDoc.AvailabilityZone becomes subnetDoc.AvailabilityZones,
//   adding an upgrade step to migrate existing non empty zones on
//   subnet docs. Also change state.Subnet.AvailabilityZone to
// * add subnetDoc.SpaceName - no upgrade step needed, as it will only
//   be used for new space-aware subnets.
// * Subnets need a reference count to calculate Status.
// * ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//   and return all subnets, including the AvailabilityZones (for EC2;
//   empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId network.Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AvailabilityZones describes which availability zone(s) this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceName holds the juju network space this subnet is
	// associated with. Can be empty if not supported.
	SpaceName string

	// Status holds the status of the subnet. Normally this will be
	// calculated from the reference count and Life of a subnet.
	Status string

	// Live holds the life of the subnet
	Life params.Life
}

// BackingSpace defines the methods supported by a Space entity stored
// persistently.
type BackingSpace interface {
	// Name returns the space name.
	Name() string

	// Subnets returns the subnets in the space
	Subnets() ([]BackingSubnet, error)

	// ProviderId returns the network ID of the provider
	ProviderId() network.Id

	// Zones returns a list of availability zone(s) that this
	// space is in. It can be empty if the provider does not support
	// availability zones.
	Zones() []string

	// Life returns the lifecycle state of the space
	Life() params.Life
}

// Backing defines the methods needed by the API facade to store and
// retrieve information from the underlying persistency layer (state
// DB).
type NetworkBacking interface {
	environs.EnvironConfigGetter

	// AvailabilityZones returns all cached availability zones (i.e.
	// not from the provider, but in state).
	AvailabilityZones() ([]providercommon.AvailabilityZone, error)

	// SetAvailabilityZones replaces the cached list of availability
	// zones with the given zones.
	SetAvailabilityZones([]providercommon.AvailabilityZone) error

	// AddSpace creates a space
	AddSpace(Name string, ProviderId network.Id, Subnets []string, Public bool) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]BackingSpace, error)

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(BackingSubnetInfo) (BackingSubnet, error)

	// AllSubnets returns all backing subnets.
	AllSubnets() ([]BackingSubnet, error)

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag
}

func BackingSubnetToParamsSubnet(subnet BackingSubnet) params.Subnet {
	cidr := subnet.CIDR()
	vlantag := subnet.VLANTag()
	providerid := subnet.ProviderId()
	zones := subnet.AvailabilityZones()
	status := subnet.Status()
	var spaceTag names.SpaceTag
	if subnet.SpaceName() != "" {
		spaceTag = names.NewSpaceTag(subnet.SpaceName())
	}

	return params.Subnet{
		CIDR:       cidr,
		VLANTag:    vlantag,
		ProviderId: string(providerid),
		Zones:      zones,
		Status:     status,
		SpaceTag:   spaceTag.String(),
		Life:       subnet.Life(),
	}
}

type byMACThenCIDRThenIndexThenName []params.NetworkConfig

func (c byMACThenCIDRThenIndexThenName) Len() int {
	return len(c)
}

func (c byMACThenCIDRThenIndexThenName) Swap(i, j int) {
	orgI, orgJ := c[i], c[j]
	c[j], c[i] = orgI, orgJ
}

func (c byMACThenCIDRThenIndexThenName) Less(i, j int) bool {
	if c[i].MACAddress == c[j].MACAddress {
		// Same MACAddress means related interfaces.
		if c[i].CIDR == "" || c[j].CIDR == "" {
			// Empty CIDRs go at the bottom, otherwise order by InterfaceName.
			return c[i].CIDR != "" || c[i].InterfaceName < c[j].InterfaceName
		}
		if c[i].DeviceIndex == c[j].DeviceIndex {
			if c[i].InterfaceName == c[j].InterfaceName {
				// Sort addresses of the same interface.
				return c[i].CIDR < c[j].CIDR || c[i].Address < c[j].Address
			}
			// Prefer shorter names (e.g. parents) with equal DeviceIndex.
			return c[i].InterfaceName < c[j].InterfaceName
		}
		// When both CIDR and DeviceIndex are non-empty, order by DeviceIndex
		return c[i].DeviceIndex < c[j].DeviceIndex
	}
	// Group by MACAddress.
	return c[i].MACAddress < c[j].MACAddress
}

// SortNetworkConfigsByParents returns the given input sorted, such that any
// child interfaces appear after their parents.
func SortNetworkConfigsByParents(input []params.NetworkConfig) []params.NetworkConfig {
	sortedInputCopy := CopyNetworkConfigs(input)
	sort.Stable(byMACThenCIDRThenIndexThenName(sortedInputCopy))
	return sortedInputCopy
}

type byInterfaceName []params.NetworkConfig

func (c byInterfaceName) Len() int {
	return len(c)
}

func (c byInterfaceName) Swap(i, j int) {
	orgI, orgJ := c[i], c[j]
	c[j], c[i] = orgI, orgJ
}

func (c byInterfaceName) Less(i, j int) bool {
	return c[i].InterfaceName < c[j].InterfaceName
}

// SortNetworkConfigsByInterfaceName returns the given input sorted by
// InterfaceName.
func SortNetworkConfigsByInterfaceName(input []params.NetworkConfig) []params.NetworkConfig {
	sortedInputCopy := CopyNetworkConfigs(input)
	sort.Stable(byInterfaceName(sortedInputCopy))
	return sortedInputCopy
}

// NetworkConfigsToIndentedJSON returns the given input as an indented JSON
// string.
func NetworkConfigsToIndentedJSON(input []params.NetworkConfig) (string, error) {
	jsonBytes, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

// CopyNetworkConfigs returns a copy of the given input
func CopyNetworkConfigs(input []params.NetworkConfig) []params.NetworkConfig {
	return append([]params.NetworkConfig(nil), input...)
}

// NetworkConfigFromInterfaceInfo converts a slice of network.InterfaceInfo into
// the equivalent params.NetworkConfig slice.
func NetworkConfigFromInterfaceInfo(interfaceInfos []network.InterfaceInfo) []params.NetworkConfig {
	result := make([]params.NetworkConfig, len(interfaceInfos))
	for i, v := range interfaceInfos {
		var dnsServers []string
		for _, nameserver := range v.DNSServers {
			dnsServers = append(dnsServers, nameserver.Value)
		}
		result[i] = params.NetworkConfig{
			DeviceIndex:         v.DeviceIndex,
			MACAddress:          v.MACAddress,
			CIDR:                v.CIDR,
			MTU:                 v.MTU,
			ProviderId:          string(v.ProviderId),
			ProviderSubnetId:    string(v.ProviderSubnetId),
			ProviderSpaceId:     string(v.ProviderSpaceId),
			ProviderVLANId:      string(v.ProviderVLANId),
			ProviderAddressId:   string(v.ProviderAddressId),
			VLANTag:             v.VLANTag,
			InterfaceName:       v.InterfaceName,
			ParentInterfaceName: v.ParentInterfaceName,
			InterfaceType:       string(v.InterfaceType),
			Disabled:            v.Disabled,
			NoAutoStart:         v.NoAutoStart,
			ConfigType:          string(v.ConfigType),
			Address:             v.Address.Value,
			DNSServers:          dnsServers,
			DNSSearchDomains:    v.DNSSearchDomains,
			GatewayAddress:      v.GatewayAddress.Value,
		}
	}
	return result
}

// NetworkConfigsToStateArgs splits the given networkConfig into a slice of
// state.LinkLayerDeviceArgs and a slice of state.LinkLayerDeviceAddress. The
// input is expected to come from MergeProviderAndObservedNetworkConfigs and to
// be sorted.
func NetworkConfigsToStateArgs(networkConfig []params.NetworkConfig) (
	[]state.LinkLayerDeviceArgs,
	[]state.LinkLayerDeviceAddress,
) {
	var devicesArgs []state.LinkLayerDeviceArgs
	var devicesAddrs []state.LinkLayerDeviceAddress

	logger.Tracef("transforming network config to state args: %+v", networkConfig)
	seenDeviceNames := set.NewStrings()
	for _, netConfig := range networkConfig {
		logger.Tracef("transforming device %q", netConfig.InterfaceName)
		if !seenDeviceNames.Contains(netConfig.InterfaceName) {
			// First time we see this, add it to devicesArgs.
			seenDeviceNames.Add(netConfig.InterfaceName)
			var mtu uint
			if netConfig.MTU >= 0 {
				mtu = uint(netConfig.MTU)
			}
			args := state.LinkLayerDeviceArgs{
				Name:        netConfig.InterfaceName,
				MTU:         mtu,
				ProviderID:  network.Id(netConfig.ProviderId),
				Type:        state.LinkLayerDeviceType(netConfig.InterfaceType),
				MACAddress:  netConfig.MACAddress,
				IsAutoStart: !netConfig.NoAutoStart,
				IsUp:        !netConfig.Disabled,
				ParentName:  netConfig.ParentInterfaceName,
			}
			logger.Tracef("state device args for device: %+v", args)
			devicesArgs = append(devicesArgs, args)
		}

		if netConfig.CIDR == "" || netConfig.Address == "" {
			logger.Tracef(
				"skipping empty CIDR %q and/or Address %q of %q",
				netConfig.CIDR, netConfig.Address, netConfig.InterfaceName,
			)
			continue
		}
		_, ipNet, err := net.ParseCIDR(netConfig.CIDR)
		if err != nil {
			logger.Warningf("FIXME: ignoring unexpected CIDR format %q: %v", netConfig.CIDR, err)
			continue
		}
		ipAddr := net.ParseIP(netConfig.Address)
		if ipAddr == nil {
			logger.Warningf("FIXME: ignoring unexpected Address format %q", netConfig.Address)
			continue
		}
		ipNet.IP = ipAddr
		cidrAddress := ipNet.String()

		var derivedConfigMethod state.AddressConfigMethod
		switch method := state.AddressConfigMethod(netConfig.ConfigType); method {
		case state.StaticAddress, state.DynamicAddress,
			state.LoopbackAddress, state.ManualAddress:
			derivedConfigMethod = method
		case "dhcp": // awkward special case
			derivedConfigMethod = state.DynamicAddress
		default:
			derivedConfigMethod = state.StaticAddress
		}

		addr := state.LinkLayerDeviceAddress{
			DeviceName:       netConfig.InterfaceName,
			ProviderID:       network.Id(netConfig.ProviderAddressId),
			ConfigMethod:     derivedConfigMethod,
			CIDRAddress:      cidrAddress,
			DNSServers:       netConfig.DNSServers,
			DNSSearchDomains: netConfig.DNSSearchDomains,
			GatewayAddress:   netConfig.GatewayAddress,
		}
		logger.Tracef("state address args for device: %+v", addr)
		devicesAddrs = append(devicesAddrs, addr)
	}
	logger.Tracef("seen devices: %+v", seenDeviceNames.SortedValues())
	logger.Tracef("network config transformed to state args:\n%+v\n%+v", devicesArgs, devicesAddrs)
	return devicesArgs, devicesAddrs
}

// NetworkingEnvironFromModelConfig constructs and returns
// environs.NetworkingEnviron using the given configGetter. Returns an error
// satisfying errors.IsNotSupported() if the model config does not support
// networking features.
func NetworkingEnvironFromModelConfig(configGetter environs.EnvironConfigGetter) (environs.NetworkingEnviron, error) {
	modelConfig, err := configGetter.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get model config")
	}
	if modelConfig.Type() == "dummy" {
		return nil, errors.NotSupportedf("dummy provider network config")
	}
	env, err := environs.GetEnviron(configGetter, environs.New)
	if err != nil {
		return nil, errors.Annotate(err, "failed to construct a model from config")
	}
	netEnviron, supported := environs.SupportsNetworking(env)
	if !supported {
		// " not supported" will be appended to the message below.
		return nil, errors.NotSupportedf("model %q networking", modelConfig.Name())
	}
	return netEnviron, nil
}

var vlanInterfaceNameRegex = regexp.MustCompile(`^.+\.[0-9]{1,4}[^0-9]?$`)

var (
	netInterfaces  = net.Interfaces
	interfaceAddrs = (*net.Interface).Addrs
)

// GetObservedNetworkConfig discovers what network interfaces exist on the
// machine, and returns that as a sorted slice of params.NetworkConfig to later
// update the state network config we have about the machine.
func GetObservedNetworkConfig() ([]params.NetworkConfig, error) {
	logger.Tracef("discovering observed machine network config...")

	interfaces, err := netInterfaces()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get network interfaces")
	}

	var observedConfig []params.NetworkConfig
	for _, nic := range interfaces {
		isUp := nic.Flags&net.FlagUp > 0

		derivedType := network.EthernetInterface
		derivedConfigType := ""
		if nic.Flags&net.FlagLoopback > 0 {
			derivedType = network.LoopbackInterface
			derivedConfigType = string(network.ConfigLoopback)
		} else if vlanInterfaceNameRegex.MatchString(nic.Name) {
			derivedType = network.VLAN_8021QInterface
		}

		nicConfig := params.NetworkConfig{
			DeviceIndex:   nic.Index,
			MACAddress:    nic.HardwareAddr.String(),
			ConfigType:    derivedConfigType,
			MTU:           nic.MTU,
			InterfaceName: nic.Name,
			InterfaceType: string(derivedType),
			NoAutoStart:   !isUp,
			Disabled:      !isUp,
		}

		addrs, err := interfaceAddrs(&nic)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot get interface %q addresses", nic.Name)
		}

		if len(addrs) == 0 {
			observedConfig = append(observedConfig, nicConfig)
			logger.Infof("no addresses observed on interface %q", nic.Name)
			continue
		}

		for _, addr := range addrs {
			cidrAddress := addr.String()
			if cidrAddress == "" {
				continue
			}
			ip, ipNet, err := net.ParseCIDR(cidrAddress)
			if err != nil {
				logger.Warningf("cannot parse interface %q address %q as CIDR: %v", nic.Name, cidrAddress, err)
				if ip := net.ParseIP(cidrAddress); ip == nil {
					return nil, errors.Errorf("cannot parse interface %q IP address %q", nic.Name, cidrAddress)
				} else {
					ipNet = &net.IPNet{}
				}
				ipNet.IP = ip
				ipNet.Mask = net.IPv4Mask(255, 255, 255, 0)
				logger.Infof("assuming interface %q has observed address %q", nic.Name, ipNet.String())
			}
			if ip.To4() == nil {
				logger.Debugf("skipping observed IPv6 address %q on %q: not fully supported yet", ip, nic.Name)
				continue
			}

			nicConfigCopy := nicConfig
			nicConfigCopy.CIDR = ipNet.String()
			nicConfigCopy.Address = ip.String()

			// TODO(dimitern): Add DNS servers, search domains, and gateway
			// later.

			observedConfig = append(observedConfig, nicConfigCopy)
		}
	}
	sortedConfig := SortNetworkConfigsByParents(observedConfig)

	logger.Tracef("about to update network config with observed: %+v", sortedConfig)
	return sortedConfig, nil
}

// MergeProviderAndObservedNetworkConfigs returns the effective, sorted, network
// configs after merging providerConfig with observedConfig.
func MergeProviderAndObservedNetworkConfigs(providerConfigs, observedConfigs []params.NetworkConfig) ([]params.NetworkConfig, error) {
	providerConfigsByName := make(map[string][]params.NetworkConfig)
	sortedProviderConfigs := SortNetworkConfigsByParents(providerConfigs)
	for _, config := range sortedProviderConfigs {
		name := config.InterfaceName
		providerConfigsByName[name] = append(providerConfigsByName[name], config)
	}

	jsonProviderConfig, err := NetworkConfigsToIndentedJSON(sortedProviderConfigs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot serialize provider config %#v as JSON", sortedProviderConfigs)
	}
	logger.Debugf("provider network config of machine:\n%s", jsonProviderConfig)

	sortedObservedConfigs := SortNetworkConfigsByParents(observedConfigs)
	jsonObservedConfig, err := NetworkConfigsToIndentedJSON(sortedObservedConfigs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot serialize observed config %#v as JSON", sortedObservedConfigs)
	}
	logger.Debugf("observed network config of machine:\n%s", jsonObservedConfig)

	var mergedConfigs []params.NetworkConfig
	for _, config := range sortedObservedConfigs {
		name := config.InterfaceName
		logger.Tracef("merging observed config for device %q: %+v", name, config)
		if strings.HasPrefix(name, instancecfg.DefaultBridgePrefix) {
			logger.Tracef("found potential juju bridge %q in observed config", name)
			unprefixedName := strings.TrimPrefix(name, instancecfg.DefaultBridgePrefix)
			underlyingConfigs, underlyingKnownByProvider := providerConfigsByName[unprefixedName]
			logger.Tracef("device %q underlying %q has provider config: %+v", name, unprefixedName, underlyingConfigs)
			if underlyingKnownByProvider {
				// This config is for a bridge created by Juju and not known by
				// the provider. The bridge is configured to adopt the address
				// allocated to the underlying interface, which is known by the
				// provider. However, since the same underlying interface can
				// have multiple addresses, we need to match the adopted
				// bridgeConfig to the correct address.

				var underlyingConfig params.NetworkConfig
				for i, underlying := range underlyingConfigs {
					if underlying.Address == config.Address {
						logger.Tracef("replacing undelying config %+v", underlying)
						// Remove what we found before changing it below.
						underlyingConfig = underlying
						underlyingConfigs = append(underlyingConfigs[:i], underlyingConfigs[i+1:]...)
						break
					}
				}
				logger.Tracef("underlying provider config after update: %+v", underlyingConfigs)

				bridgeConfig := config
				bridgeConfig.InterfaceType = string(network.BridgeInterface)
				bridgeConfig.ConfigType = underlyingConfig.ConfigType
				bridgeConfig.VLANTag = underlyingConfig.VLANTag
				bridgeConfig.ProviderId = "" // Juju-created bridges never have a ProviderID
				bridgeConfig.ProviderSpaceId = underlyingConfig.ProviderSpaceId
				bridgeConfig.ProviderVLANId = underlyingConfig.ProviderVLANId
				bridgeConfig.ProviderSubnetId = underlyingConfig.ProviderSubnetId
				bridgeConfig.ProviderAddressId = underlyingConfig.ProviderAddressId
				if underlyingParent := underlyingConfig.ParentInterfaceName; underlyingParent != "" {
					bridgeConfig.ParentInterfaceName = instancecfg.DefaultBridgePrefix + underlyingParent
				}

				underlyingConfig.ConfigType = string(network.ConfigManual)
				underlyingConfig.ParentInterfaceName = name
				underlyingConfig.ProviderAddressId = ""
				underlyingConfig.CIDR = ""
				underlyingConfig.Address = ""

				underlyingConfigs = append(underlyingConfigs, underlyingConfig)
				providerConfigsByName[unprefixedName] = underlyingConfigs
				logger.Tracef("updated provider network config by name: %+v", providerConfigsByName)

				mergedConfigs = append(mergedConfigs, bridgeConfig)
				continue
			}
		}

		knownProviderConfigs, knownByProvider := providerConfigsByName[name]
		if !knownByProvider {
			// Not known by the provider and not a Juju-created bridge, so just
			// use the observed config for it.
			logger.Tracef("device %q not known to provider - adding only observed config: %+v", name, config)
			mergedConfigs = append(mergedConfigs, config)
			continue
		}
		logger.Tracef("device %q has known provider network config: %+v", name, knownProviderConfigs)

		for _, providerConfig := range knownProviderConfigs {
			if providerConfig.Address == config.Address {
				logger.Tracef(
					"device %q has observed address %q, index %d, and MTU %q; overriding index %d and MTU %d from provider config",
					name, config.Address, config.DeviceIndex, config.MTU, providerConfig.DeviceIndex, providerConfig.MTU,
				)
				// Prefer observed device indices and MTU values as more up-to-date.
				providerConfig.DeviceIndex = config.DeviceIndex
				providerConfig.MTU = config.MTU

				mergedConfigs = append(mergedConfigs, providerConfig)
				break
			}
		}
	}

	sortedMergedConfigs := SortNetworkConfigsByParents(mergedConfigs)

	jsonMergedConfig, err := NetworkConfigsToIndentedJSON(sortedMergedConfigs)
	if err != nil {
		errors.Annotatef(err, "cannot serialize merged config %#v as JSON", sortedMergedConfigs)
	}
	logger.Debugf("combined machine network config:\n%s", jsonMergedConfig)

	return mergedConfigs, nil
}
