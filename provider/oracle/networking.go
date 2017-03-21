// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	oci "github.com/juju/go-oracle-cloud/api"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	names "gopkg.in/juju/names.v2"
)

var _ environs.NetworkingEnviron = (*oracleEnviron)(nil)

// Only ubuntu for now. There is no CentOS image in the oracle
// compute marketplace
var ubuntuInterfaceTemplate = `
auto %s
iface %s inet dhcp

`

func (e *oracleEnviron) ensureVnicSet(machineId string, tags []string, acls []string) (ociResponse.VnicSet, error) {
	name := e.client.ComposeName(e.namespace.Value(machineId))
	details, err := e.client.VnicSetDetails(name)
	if err != nil {
		if !oci.IsNotFound(err) {
			return nil, err
		}
		vnicSetParams := oci.VnicSetParams{
			AppliedAcls: acls,
			Description: "Juju created vnic set",
			Name:        name,
			Tags:        tags,
		}
		details, err := e.client.CreateVnicSet(vnicSetParams)
		if err != nil {
			return nil, err
		}
		return details, nil
	}
	return details, nil
}

// Subnet returns basic information about subnets
// known by the oracle provider for the environment
func (e *oracleEnviron) Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	return nil, nil
}

func (e *oracleEnviron) getNicAttributes(instance ociResponse.Instance) map[string]ociResponse.Network {
	if instance.Attributes.Network == nil {
		return map[string]ociResponse.Network{}
	}
	ret := make(map[string]ociResponse.Network, len(instance.Attributes.Network))
	for name, obj := range instance.Attributes.Network {
		tmp := strings.TrimPrefix(name, `nimbula_vcable-`)
		ret[tmp] = obj
	}
	return ret
}

// NetworkInterfaces requests information about the
// network interfaces on the given instance
func (e *oracleEnviron) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	id := string(instId)
	instance, err := e.client.InstanceDetails(id)
	if err != nil {
		return nil, err
	}

	n := len(instance.Networking)
	if n == 0 {
		return []network.InterfaceInfo{}, nil
	}
	subnetInfo, err := e.getSubnetInfoAsMap()
	if err != nil {
		return nil, errors.Trace(err)
	}
	nicAttributes := e.getNicAttributes(instance)

	interfaces := make([]network.InterfaceInfo, 0, n)
	idx := 0
	for nicName, nicObj := range instance.Networking {
		// gsamfira: While the API may hold any alphanumeric NIC name
		// of up to 4 characters, inside an ubuntu instance, the NIC will always
		// be named eth0 (where 0 is the index of the NIC).
		// NICs inside the instance will be ordered in the same way the API
		// returns them; i.e. first element returned by the API will be eth0,
		// second element will be eth1 and so on. Sorting is done alphanumerically
		// it makes sense to use the name that will be seen by the juju agent
		// instead of the name that shows up in the provider.
		// TODO (gsamfira): Check NIC naming in CentOS and Windows.
		name := fmt.Sprintf("eth%s", strconv.Itoa(idx))
		deviceIndex := idx

		idx += 1

		deviceAttributes, ok := nicAttributes[nicName]
		if !ok {
			return nil, errors.Errorf("failed to get NIC attributes for %q", nicName)
		}
		mac, ip, err := getMacAndIP(deviceAttributes.Address)
		if err != nil {
			return nil, err
		}
		addr := network.NewScopedAddress(ip, network.ScopeCloudLocal)
		// nicSubnetDetails := subnetInfo[deviceAttributes.Ipnetwork]
		ni := network.InterfaceInfo{
			InterfaceName: name,
			DeviceIndex:   deviceIndex,
			ProviderId:    network.Id(deviceAttributes.Id),
			MACAddress:    mac,
			Address:       addr,
		}

		// gsamfira: VEthernet NICs are connected to shared networks
		// I have not found a way to interrogate details about the shared
		// networks available inside the oracle cloud. There is some documentation
		// on the matter here:
		// https://docs.oracle.com/cloud-machine/latest/stcomputecs/ELUAP/GUID-8CBE0F4E-E376-4C93-BB56-884836273168.htm
		// but I have not been able to get any information about the shared networks
		// using the resources described there.
		// We only populate Space information for NICs attached to IPNetworks (user defined)
		if nicObj.GetType() == ociResponse.VNic {
			nicSubnetDetails := subnetInfo[deviceAttributes.Ipnetwork]
			ni.ProviderSpaceId = nicSubnetDetails.SpaceProviderId
			ni.ProviderSubnetId = nicSubnetDetails.ProviderId
			ni.CIDR = nicSubnetDetails.CIDR
		}
		interfaces = append(interfaces, ni)
	}

	return interfaces, nil
}

// getMacAndIp picks and returns the correct mac and ip from the given slice
// if the slice does not contain a valid mac and ip it will return an error
func getMacAndIP(address []string) (mac string, ip string, err error) {
	if address == nil {
		err = errors.New("Empty address slice given")
		return
	}
	for _, val := range address {
		valIp := net.ParseIP(val)
		if valIp != nil {
			ip = val
			continue
		}
		_, err = net.ParseMAC(val)
		if err != nil {
			err = errors.Errorf("The address is not an mac neighter an ip %s", val)
			break
		}
		mac = val
	}
	return
}

// SupportsSpaces returns whether the current oracle environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e *oracleEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery returns whether the current environment
// supports discovering spaces from the oracle provider. The returned error
// satisfies errors.IsNotSupported(), unless a general API failure occurs.
func (e *oracleEnviron) SupportsSpaceDiscovery() (bool, error) {
	return true, nil
}

func (e *oracleEnviron) getSubnetInfo() ([]network.SubnetInfo, error) {
	networks, err := e.client.AllIpNetworks(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets := make([]network.SubnetInfo, len(networks.Result))
	idx := 0
	for _, val := range networks.Result {
		subnets[idx] = network.SubnetInfo{
			ProviderId:      network.Id(val.Name),
			CIDR:            val.IpAddressPrefix,
			SpaceProviderId: network.Id(*val.IpNetworkExchange),
		}
		idx++
	}
	return subnets, nil
}

func (e *oracleEnviron) getSubnetInfoAsMap() (map[string]network.SubnetInfo, error) {
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

// Spaces returns a slice of network.SpaceInfo with info, including
// details of all associated subnets, about all spaces known to the
// oracle provider that have subnets available.
func (e *oracleEnviron) Spaces() ([]network.SpaceInfo, error) {
	networks, err := e.getSubnetInfo()
	if err != nil {
		return nil, errors.Trace(err)
	}
	exchanges := map[string]network.SpaceInfo{}
	for _, val := range networks {
		if val.SpaceProviderId == "" {
			continue
		}
		if space, ok := exchanges[string(val.SpaceProviderId)]; !ok {
			exchanges[string(val.SpaceProviderId)] = network.SpaceInfo{
				Name:       string(val.SpaceProviderId),
				ProviderId: val.SpaceProviderId,
				Subnets: []network.SubnetInfo{
					val,
				},
			}
		} else {
			space.Subnets = append(space.Subnets, val)
		}

	}
	var ret []network.SpaceInfo
	for _, val := range exchanges {
		ret = append(ret, val)
	}
	return ret, nil
}

// SupportsContainerAddresses returns true if the current environment is
// able to allocate addresses for containers. If returning false, we also
// return an IsNotSupported error.
func (e *oracleEnviron) SupportsContainerAddresses() (bool, error) {
	return false, errors.NotSupportedf("container address allocation")
}

// AllocateContainerAddresses allocates a static address for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addresses on success.
func (e *oracleEnviron) AllocateContainerAddresses(
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo,
) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("containers")
}

// ReleaseContainerAddresses releases the previously allocated
// addresses matching the interface details passed in.
func (e *oracleEnviron) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container")
}
