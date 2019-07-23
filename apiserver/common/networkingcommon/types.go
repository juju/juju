// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon

import (
	"net"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	corenetwork "github.com/juju/juju/core/network"
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
	ProviderId() corenetwork.Id
	ProviderNetworkId() corenetwork.Id
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
// * Subnets need a reference count to calculate Status.
// * ensure EC2 and MAAS providers accept empty IDs as Subnets() args
//   and return all subnets, including the AvailabilityZones (for EC2;
//   empty for MAAS as zones are orthogonal to networks).
type BackingSubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId corenetwork.Id

	// ProviderNetworkId is the id of the network containing this
	// subnet from the provider's perspective. It can be empty if the
	// provider doesn't support distinct networks.
	ProviderNetworkId corenetwork.Id

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
	ProviderId() corenetwork.Id
}

// NetworkBacking defines the methods needed by the API facade to store and
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
	AddSpace(Name string, ProviderId corenetwork.Id, Subnets []string, Public bool) error

	// AllSpaces returns all known Juju network spaces.
	AllSpaces() ([]BackingSpace, error)

	// AddSubnet creates a backing subnet for an existing subnet.
	AddSubnet(BackingSubnetInfo) (BackingSubnet, error)

	// AllSubnets returns all backing subnets.
	AllSubnets() ([]BackingSubnet, error)

	// ModelTag returns the tag of the model this state is associated to.
	ModelTag() names.ModelTag

	// ReloadSpaces loads spaces from backing environ
	ReloadSpaces(environ environs.BootstrapEnviron) error
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

// NetworkConfigFromInterfaceInfo converts a slice of network.InterfaceInfo into
// the equivalent params.NetworkConfig slice.
func NetworkConfigFromInterfaceInfo(interfaceInfos []network.InterfaceInfo) []params.NetworkConfig {
	result := make([]params.NetworkConfig, len(interfaceInfos))
	for i, v := range interfaceInfos {
		var dnsServers []string
		for _, nameserver := range v.DNSServers {
			dnsServers = append(dnsServers, nameserver.Value)
		}
		routes := make([]params.NetworkRoute, len(v.Routes))
		for j, route := range v.Routes {
			routes[j] = params.NetworkRoute{
				DestinationCIDR: route.DestinationCIDR,
				GatewayIP:       route.GatewayIP,
				Metric:          route.Metric,
			}
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
			Routes:              routes,
			IsDefaultGateway:    v.IsDefaultGateway,
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
				ProviderID:  corenetwork.Id(netConfig.ProviderId),
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
			ProviderID:       corenetwork.Id(netConfig.ProviderAddressId),
			ConfigMethod:     derivedConfigMethod,
			CIDRAddress:      cidrAddress,
			DNSServers:       netConfig.DNSServers,
			DNSSearchDomains: netConfig.DNSSearchDomains,
			GatewayAddress:   netConfig.GatewayAddress,
			IsDefaultGateway: netConfig.IsDefaultGateway,
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
	cloudSpec, err := configGetter.CloudSpec()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get cloudspec")
	}
	if cloudSpec.Type == cloud.CloudTypeCAAS {
		return nil, errors.NotSupportedf("CAAS model %q networking", modelConfig.Name())
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

// NetworkConfigSource defines the necessary calls to obtain the network
// configuration of a machine.
type NetworkConfigSource interface {
	// SysClassNetPath returns the Linux kernel userspace SYSFS path used by
	// this source. DefaultNetworkConfigSource() uses network.SysClassNetPath.
	SysClassNetPath() string

	// Interfaces returns information about all network interfaces on the
	// machine as []net.Interface.
	Interfaces() ([]net.Interface, error)

	// InterfaceAddresses returns information about all addresses assigned to
	// the network interface with the given name.
	InterfaceAddresses(name string) ([]net.Addr, error)
}

// MergeProviderAndObservedNetworkConfigs returns the effective network configs,
// using observedConfigs as a base and selectively updating it using the
// matching providerConfigs for each interface.
func MergeProviderAndObservedNetworkConfigs(
	providerConfigs, observedConfigs []params.NetworkConfig,
) []params.NetworkConfig {

	providerConfigByName := networkConfigsByName(providerConfigs)
	logger.Tracef("known provider config by name: %+v", providerConfigByName)

	providerConfigByAddress := networkConfigsByAddress(providerConfigs)
	logger.Tracef("known provider config by address: %+v", providerConfigByAddress)

	var results []params.NetworkConfig
	for _, observed := range observedConfigs {

		name, ipAddress := observed.InterfaceName, observed.Address
		finalConfig := observed

		providerConfig, known := providerConfigByName[name]
		if known {
			finalConfig = mergeObservedAndProviderInterfaceConfig(finalConfig, providerConfig)
			logger.Debugf("updated observed interface config for %q with: %+v", name, providerConfig)
		}

		providerConfig, known = providerConfigByAddress[ipAddress]
		if known {
			finalConfig = mergeObservedAndProviderAddressConfig(finalConfig, providerConfig)
			logger.Debugf("updated observed address config for %q with: %+v", name, providerConfig)
		}

		results = append(results, finalConfig)
		logger.Debugf("merged config for %q: %+v", name, finalConfig)
	}

	return results
}

func networkConfigsByName(input []params.NetworkConfig) map[string]params.NetworkConfig {
	configsByName := make(map[string]params.NetworkConfig, len(input))
	for _, config := range input {
		configsByName[config.InterfaceName] = config
	}
	return configsByName
}

func networkConfigsByAddress(input []params.NetworkConfig) map[string]params.NetworkConfig {
	configsByAddress := make(map[string]params.NetworkConfig, len(input))
	for _, config := range input {
		configsByAddress[config.Address] = config
	}
	return configsByAddress
}

func mergeObservedAndProviderInterfaceConfig(observedConfig, providerConfig params.NetworkConfig) params.NetworkConfig {
	logger.Debugf("mergeObservedAndProviderInterfaceConfig %+v %+v", observedConfig, providerConfig)
	finalConfig := observedConfig

	// The following fields cannot be observed and are only known by the
	// provider.
	finalConfig.ProviderId = providerConfig.ProviderId
	finalConfig.ProviderVLANId = providerConfig.ProviderVLANId
	finalConfig.ProviderSubnetId = providerConfig.ProviderSubnetId

	// The following few fields are only updated if their observed values are
	// empty.

	if observedConfig.InterfaceType == "" {
		finalConfig.InterfaceType = providerConfig.InterfaceType
	}

	if observedConfig.VLANTag == 0 {
		finalConfig.VLANTag = providerConfig.VLANTag
	}

	if observedConfig.ParentInterfaceName == "" {
		finalConfig.ParentInterfaceName = providerConfig.ParentInterfaceName
	}
	logger.Debugf("mergeObservedAndProviderInterfaceConfig %+v", finalConfig)

	return finalConfig
}

func mergeObservedAndProviderAddressConfig(observedConfig, providerConfig params.NetworkConfig) params.NetworkConfig {
	finalConfig := observedConfig

	// The following fields cannot be observed and are only known by the
	// provider.
	finalConfig.ProviderAddressId = providerConfig.ProviderAddressId
	finalConfig.ProviderSubnetId = providerConfig.ProviderSubnetId
	finalConfig.ProviderSpaceId = providerConfig.ProviderSpaceId

	// The following few fields are only updated if their observed values are
	// empty.

	if observedConfig.ProviderVLANId == "" {
		finalConfig.ProviderVLANId = providerConfig.ProviderVLANId
	}

	if observedConfig.VLANTag == 0 {
		finalConfig.VLANTag = providerConfig.VLANTag
	}

	if observedConfig.ConfigType == "" {
		finalConfig.ConfigType = providerConfig.ConfigType
	}

	if observedConfig.CIDR == "" {
		finalConfig.CIDR = providerConfig.CIDR
	}

	if observedConfig.GatewayAddress == "" {
		finalConfig.GatewayAddress = providerConfig.GatewayAddress
	}

	if len(observedConfig.DNSServers) == 0 {
		finalConfig.DNSServers = providerConfig.DNSServers
	}

	if len(observedConfig.DNSSearchDomains) == 0 {
		finalConfig.DNSSearchDomains = providerConfig.DNSSearchDomains
	}

	return finalConfig
}

func networkToParamsNetworkInfo(info network.NetworkInfo) params.NetworkInfo {
	addresses := make([]params.InterfaceAddress, len(info.Addresses))
	for i, addr := range info.Addresses {
		addresses[i] = params.InterfaceAddress{
			Address: addr.Address,
			CIDR:    addr.CIDR,
		}
	}
	return params.NetworkInfo{
		MACAddress:    info.MACAddress,
		InterfaceName: info.InterfaceName,
		Addresses:     addresses,
	}
}

func MachineNetworkInfoResultToNetworkInfoResult(inResult state.MachineNetworkInfoResult) params.NetworkInfoResult {
	if inResult.Error != nil {
		return params.NetworkInfoResult{Error: common.ServerError(inResult.Error)}
	}
	infos := make([]params.NetworkInfo, len(inResult.NetworkInfos))
	for i, info := range inResult.NetworkInfos {
		infos[i] = networkToParamsNetworkInfo(info)
	}
	return params.NetworkInfoResult{
		Info: infos,
	}
}

func FanConfigToFanConfigResult(config network.FanConfig) params.FanConfigResult {
	result := params.FanConfigResult{make([]params.FanConfigEntry, len(config))}
	for i, entry := range config {
		result.Fans[i] = params.FanConfigEntry{entry.Underlay.String(), entry.Overlay.String()}
	}
	return result
}

func FanConfigResultToFanConfig(config params.FanConfigResult) (network.FanConfig, error) {
	rv := make(network.FanConfig, len(config.Fans))
	for i, entry := range config.Fans {
		_, ipnet, err := net.ParseCIDR(entry.Underlay)
		if err != nil {
			return nil, err
		}
		rv[i].Underlay = ipnet
		_, ipnet, err = net.ParseCIDR(entry.Overlay)
		if err != nil {
			return nil, err
		}
		rv[i].Overlay = ipnet
	}
	return rv, nil
}
