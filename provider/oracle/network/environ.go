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
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
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
func (e Environ) Subnets(
	ctx context.ProviderCallContext, id instance.Id, subnets []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	ret := []corenetwork.SubnetInfo{}
	found := make(map[string]bool)
	if id != instance.UnknownId {
		instanceNets, err := e.NetworkInterfaces(ctx, id)
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
				subnetInfo := corenetwork.SubnetInfo{
					CIDR:            val.CIDR,
					ProviderId:      val.ProviderSubnetId,
					ProviderSpaceId: val.ProviderSpaceId,
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
			for key := range subnets {
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
func (e Environ) getSubnetInfoAsMap() (map[string]corenetwork.SubnetInfo, error) {
	subnets, err := e.getSubnetInfo()
	if err != nil {
		return nil, err
	}
	ret := make(map[string]corenetwork.SubnetInfo, len(subnets))
	for _, val := range subnets {
		ret[string(val.ProviderId)] = val
	}
	return ret, nil
}

// getSubnetInfo returns subnet information for all subnets known to
// the oracle provider
func (e Environ) getSubnetInfo() ([]corenetwork.SubnetInfo, error) {
	networks, err := e.client.AllIpNetworks(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]corenetwork.SubnetInfo, len(networks.Result))
	idx := 0
	for _, val := range networks.Result {
		var spaceId corenetwork.Id
		if val.IpNetworkExchange != nil {
			spaceId = corenetwork.Id(*val.IpNetworkExchange)
		}
		subnets[idx] = corenetwork.SubnetInfo{
			ProviderId:      corenetwork.Id(val.Name),
			CIDR:            val.IpAddressPrefix,
			ProviderSpaceId: spaceId,
			AvailabilityZones: []string{
				"default",
			},
		}
		idx++
	}
	return subnets, nil
}

// NetworkInterfaces is defined on the environs.Networking interface.
func (e Environ) NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error) {
	instance, err := e.env.Details(instId)
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
			ProviderId:    corenetwork.Id(deviceAttributes.Id),
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
			nic.ProviderSpaceId = nicSubnetDetails.ProviderSpaceId
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
func (e Environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
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
func (e Environ) SupportsSpaceDiscovery(ctx context.ProviderCallContext) (bool, error) {
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
func (e Environ) SupportsContainerAddresses(ctx context.ProviderCallContext) (bool, error) {
	return false, errors.NotSupportedf("container address allocation")
}

// AllocateContainerAddresses is defined on the environs.Networking interface.
func (e Environ) AllocateContainerAddresses(
	ctx context.ProviderCallContext,
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo,
) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("containers")
}

// ReleaseContainerAddresses is defined on the environs.Networking interface.
func (e Environ) ReleaseContainerAddresses(ctx context.ProviderCallContext, interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container")
}

// Spaces is defined on the environs.Networking interface.
func (e Environ) Spaces(ctx context.ProviderCallContext) ([]corenetwork.SpaceInfo, error) {
	networks, err := e.getSubnetInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	exchanges := map[string]corenetwork.SpaceInfo{}
	for _, val := range networks {
		if val.ProviderSpaceId == "" {
			continue
		}
		logger.Infof("found network %s with space %s", string(val.ProviderId), string(val.ProviderSpaceId))
		providerID := string(val.ProviderSpaceId)
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
			exchanges[name] = corenetwork.SpaceInfo{
				Name:       name,
				ProviderId: val.ProviderSpaceId,
				Subnets: []corenetwork.SubnetInfo{
					val,
				},
			}
		} else {
			logger.Infof("appending subnet %s to %s", string(val.ProviderId), space.Name)
			space.Subnets = append(space.Subnets, val)
			exchanges[name] = space
		}

	}
	var ret []corenetwork.SpaceInfo
	for _, val := range exchanges {
		ret = append(ret, val)
	}

	logger.Infof("returning spaces: %v", ret)
	return ret, nil
}

// ProviderSpaceInfo is defined on the environs.NetworkingEnviron interface.
func (Environ) ProviderSpaceInfo(
	ctx context.ProviderCallContext, space *corenetwork.SpaceInfo,
) (*environs.ProviderSpaceInfo, error) {
	return nil, errors.NotSupportedf("provider space info")
}

// AreSpacesRoutable is defined on the environs.NetworkingEnviron interface.
func (Environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SSHAddresses is defined on the environs.SSHAddresses interface.
func (*Environ) SSHAddresses(ctx context.ProviderCallContext, addresses []network.Address) ([]network.Address, error) {
	return addresses, nil
}

// SuperSubnets implements environs.SuperSubnets
func (*Environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	return nil, errors.NotSupportedf("super subnets")
}
