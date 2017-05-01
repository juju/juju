// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/go-oracle-cloud/api"
	"github.com/juju/go-oracle-cloud/common"
	"github.com/juju/go-oracle-cloud/response"
	"github.com/juju/loggo"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	commonProvider "github.com/juju/juju/provider/oracle/common"
)

var logger = loggo.GetLogger("juju.provider.oracle.network")

// NetworkingAPI defines methods needed to interact with the networking features
// of the Oracle API
type NetworkingAPI interface {
	commonProvider.Instancer
	commonProvider.Composer

	// AllIpNetworks fetches all IP networks matching a filter. A nil valued filter
	// will return all IP networks
	AllIpNetworks([]api.Filter) (response.AllIpNetworks, error)

	// AllAcls fetches all ACLs that match a given filter.
	AllAcls([]api.Filter) (response.AllAcls, error)
}

// Environ implements the environs.Networking interface
type Environ struct {
	client NetworkingAPI

	env commonProvider.OracleInstancer
}

var _ environs.Networking = (*Environ)(nil)

// NewEnviron returns a new instance of Environ
func NewEnviron(api NetworkingAPI, env commonProvider.OracleInstancer) *Environ {
	return &Environ{
		client: api,
		env:    env,
	}
}

// Subnets is defined on the environs.Networking interface.
func (e Environ) Subnets(id instance.Id, subnets []network.Id) ([]network.SubnetInfo, error) {
	ret := []network.SubnetInfo{}
	found := make(map[string]bool)
	if id != instance.UnknownId {
		instanceNets, err := e.NetworkInterfaces(id)
		if err != nil {
			return ret, errors.Trace(err)
		}
		if len(subnets) == 0 {
			for _, val := range instanceNets {
				found[string(val.ProviderSubnetId)] = false
			}
		}
		for _, val := range instanceNets {
			if _, ok := found[string(val.ProviderSubnetId)]; !ok {
				continue
			} else {
				found[string(val.ProviderSubnetId)] = true
				subnetInfo := network.SubnetInfo{
					CIDR:            val.CIDR,
					ProviderId:      val.ProviderSubnetId,
					SpaceProviderId: val.ProviderSpaceId,
				}
				ret = append(ret, subnetInfo)
			}
		}
	} else {
		subnets, err := e.getSubnetInfoAsMap()
		if err != nil {
			return ret, errors.Trace(err)
		}
		if len(subnets) == 0 {
			for key, _ := range subnets {
				found[key] = false
			}
		}
		for key, val := range subnets {
			if _, ok := found[key]; !ok {
				continue
			}
			found[key] = true
			ret = append(ret, val)
		}
	}
	missing := []string{}
	for key, ok := range found {
		if !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, errors.Errorf(
			"missing subnets: %s", strings.Join(missing, ","))
	}
	return ret, nil
}

// getSubnetInfoAsMap will return the subnet information
// for the getSubnetInfo as a map rather than a slice
func (e Environ) getSubnetInfoAsMap() (map[string]network.SubnetInfo, error) {
	subnets, err := e.getSubnetInfo()
	if err != nil {
		return nil, err
	}
	ret := make(map[string]network.SubnetInfo, len(subnets))
	for _, val := range subnets {
		ret[string(val.ProviderId)] = val
	}
	return ret, nil
}

// getSubnetInfo returns subnet information for all subnets known to
// the oracle provider
func (e Environ) getSubnetInfo() ([]network.SubnetInfo, error) {
	networks, err := e.client.AllIpNetworks(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]network.SubnetInfo, len(networks.Result))
	idx := 0
	for _, val := range networks.Result {
		var spaceId network.Id
		if val.IpNetworkExchange != nil {
			spaceId = network.Id(*val.IpNetworkExchange)
		}
		subnets[idx] = network.SubnetInfo{
			ProviderId:      network.Id(val.Name),
			CIDR:            val.IpAddressPrefix,
			SpaceProviderId: spaceId,
			AvailabilityZones: []string{
				"default",
			},
		}
		idx++
	}
	return subnets, nil
}

// NetworkInterfaces is defined on the environs.Networking interface.
func (e Environ) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	providerId, err := e.env.ProviderID(instId)
	if err != nil {
		return nil, err
	}

	instance, err := e.client.InstanceDetails(providerId)
	if err != nil {
		return nil, err
	}

	if len(instance.Networking) == 0 {
		return []network.InterfaceInfo{}, nil
	}
	subnetInfo, err := e.getSubnetInfoAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	nicAttributes := e.getNicAttributes(instance)

	interfaces := make([]network.InterfaceInfo, 0, len(instance.Networking))
	idx := 0
	for nicName, nicObj := range instance.Networking {
		// gsamfira: While the API may hold any alphanumeric NIC name
		// of up to 4 characters, inside an ubuntu instance,
		// the NIC will always be named eth0 (where 0 is the index of the NIC).
		// NICs inside the instance will be ordered in the same way the API
		// returns them; i.e. first element returned by the API will be eth0,
		// second element will be eth1 and so on. Sorting is done
		// alphanumerically it makes sense to use the name that will be
		// seen by the juju agent instead of the name that shows up
		// in the provider.
		// TODO (gsamfira): Check NIC naming in CentOS and Windows.
		name := fmt.Sprintf("eth%s", strconv.Itoa(idx))
		deviceIndex := idx

		idx += 1

		deviceAttributes, ok := nicAttributes[nicName]
		if !ok {
			return nil, errors.Errorf(
				"failed to get NIC attributes for %q", nicName)
		}
		mac, ip, err := GetMacAndIP(deviceAttributes.Address)
		if err != nil {
			return nil, err
		}
		addr := network.NewScopedAddress(ip, network.ScopeCloudLocal)
		nic := network.InterfaceInfo{
			InterfaceName: name,
			DeviceIndex:   deviceIndex,
			ProviderId:    network.Id(deviceAttributes.Id),
			MACAddress:    mac,
			Address:       addr,
			InterfaceType: network.EthernetInterface,
		}

		// gsamfira: VEthernet NICs are connected to shared networks
		// I have not found a way to interrogate details about the shared
		// networks available inside the oracle cloud. There is some
		// documentation on the matter here:
		//
		// https://docs.oracle.com/cloud-machine/latest/stcomputecs/ELUAP/GUID-8CBE0F4E-E376-4C93-BB56-884836273168.htm
		//
		// but I have not been able to get any information about the
		// shared networks using the resources described there.
		// We only populate Space information for NICs attached to
		// IPNetworks (user defined)
		if nicObj.GetType() == common.VNic {
			nicSubnetDetails := subnetInfo[deviceAttributes.Ipnetwork]
			nic.ProviderSpaceId = nicSubnetDetails.SpaceProviderId
			nic.ProviderSubnetId = nicSubnetDetails.ProviderId
			nic.CIDR = nicSubnetDetails.CIDR
		}
		interfaces = append(interfaces, nic)
	}

	return interfaces, nil
}

// getNicAttributes returns all network cards attributes from a oracle instance
func (e Environ) getNicAttributes(instance response.Instance) map[string]response.Network {
	if instance.Attributes.Network == nil {
		return map[string]response.Network{}
	}
	n := len(instance.Attributes.Network)
	ret := make(map[string]response.Network, n)
	for name, obj := range instance.Attributes.Network {
		tmp := strings.TrimPrefix(name, `nimbula_vcable-`)
		ret[tmp] = obj
	}
	return ret
}

// canAccessNetworkAPI checks whether or not we have access to the necessary
// API endpoints needed for spaces support
func (e *Environ) canAccessNetworkAPI() (bool, error) {
	_, err := e.client.AllAcls(nil)
	if err != nil {
		if api.IsMethodNotAllowed(err) {
			return false, nil
		}
		return false, errors.Trace(err)
	}
	return true, nil
}

// SupportsSpaces is defined on the environs.Networking interface.
func (e Environ) SupportsSpaces() (bool, error) {
	logger.Infof("checking for spaces support")
	access, err := e.canAccessNetworkAPI()
	if err != nil {
		return false, errors.Trace(err)
	}
	if access {
		return true, nil
	}
	return false, nil
}

// SupportsSpaceDiscovery is defined on the environs.Networking interface.
func (e Environ) SupportsSpaceDiscovery() (bool, error) {
	access, err := e.canAccessNetworkAPI()
	if err != nil {
		return false, errors.Trace(err)
	}
	if access {
		return true, nil
	}
	return false, nil
}

// SupportsContainerAddresses is defined on the environs.Networking interface.
func (e Environ) SupportsContainerAddresses() (bool, error) {
	return false, errors.NotSupportedf("container address allocation")
}

// AllocateContainerAddresses is defined on the environs.Networking interface.
func (e Environ) AllocateContainerAddresses(
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo,
) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("containers")
}

// ReleaseContainerAddresses is defined on the environs.Networking interface.
func (e Environ) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container")
}

// Spaces is defined on the environs.Networking interface.
func (e Environ) Spaces() ([]network.SpaceInfo, error) {
	networks, err := e.getSubnetInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	exchanges := map[string]network.SpaceInfo{}
	for _, val := range networks {
		if val.SpaceProviderId == "" {
			continue
		}
		logger.Infof("found network %s with space %s",
			string(val.ProviderId), string(val.SpaceProviderId))
		providerID := string(val.SpaceProviderId)
		tmp := strings.Split(providerID, `/`)
		name := tmp[len(tmp)-1]
		// Oracle allows us to attach an IP network to a space belonging to
		// another user using the web portal. We recompose the provider ID
		// (which is unique) and compare to the provider ID of the space.
		// If they match, the space belongs to us
		tmpProviderId := e.client.ComposeName(name)
		if tmpProviderId != providerID {
			continue
		}
		if space, ok := exchanges[name]; !ok {
			logger.Infof("creating new space obj for %s and adding %s",
				name, string(val.ProviderId))
			exchanges[name] = network.SpaceInfo{
				Name:       name,
				ProviderId: val.SpaceProviderId,
				Subnets: []network.SubnetInfo{
					val,
				},
			}
		} else {
			logger.Infof("appending subnet %s to %s",
				string(val.ProviderId), string(space.Name))
			space.Subnets = append(space.Subnets, val)
			exchanges[name] = space
		}

	}
	var ret []network.SpaceInfo
	for _, val := range exchanges {
		ret = append(ret, val)
	}

	logger.Infof("returning spaces: %v", ret)
	return ret, nil
}

// ProviderSpaceInfo is defined on the environs.NetworkingEnviron interface.
func (Environ) ProviderSpaceInfo(providerSpaceId string) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// IsSpaceRoutable is defined on the environs.NetworkingEnviron interface.
func (Environ) IsSpaceRoutable(targetSpace *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}
