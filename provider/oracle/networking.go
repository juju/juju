// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	oci "github.com/juju/go-oracle-cloud/api"
	ociCommon "github.com/juju/go-oracle-cloud/common"
	ociResponse "github.com/juju/go-oracle-cloud/response"

	"github.com/juju/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	names "gopkg.in/juju/names.v2"
	// "github.com/juju/juju/cloudconfig/cloudinit"
)

var _ environs.NetworkingEnviron = (*oracleEnviron)(nil)

// Only ubuntu for now. There is no CentOS image in the oracle
// compute marketplace
var ubuntuInterfaceTemplate = `
auto %s
iface %s inet dhcp
`

const (
	// defaultNicName is the default network internet card name inside a vm
	defaultNicName = "eth0"
	// nicPrefix si the default network internet card prefix name inside a vm
	nicPrefix = "eth"
	// interfacesConfigDir default path of interfaces.d directory
	interfacesConfigDir = `/etc/network/interfaces.d`
)

// getIPExchangeAndNetworks return all ip networks that are tied with
// the ip exchange networks
func (e *oracleEnviron) getIPExchangesAndNetworks() (map[string][]ociResponse.IpNetwork, error) {
	logger.Infof("Getting ip exchanges and networks")
	ret := map[string][]ociResponse.IpNetwork{}
	exchanges, err := e.client.AllIpNetworkExchanges(nil)
	if err != nil {
		return ret, err
	}
	ipNets, err := e.client.AllIpNetworks(nil)
	if err != nil {
		return ret, err
	}
	for _, val := range exchanges.Result {
		ret[val.Name] = []ociResponse.IpNetwork{}
	}
	for _, val := range ipNets.Result {
		if val.IpNetworkExchange == nil {
			continue
		}
		if _, ok := ret[*val.IpNetworkExchange]; ok {
			ret[*val.IpNetworkExchange] = append(ret[*val.IpNetworkExchange], val)
		}
	}
	return ret, nil
}

// Subnet returns basic information about subnets
// known by the oracle provider for the environment
func (e *oracleEnviron) Subnets(inst instance.Id, subnetIds []network.Id) ([]network.SubnetInfo, error) {
	ret := []network.SubnetInfo{}
	found := make(map[string]bool)
	if inst != instance.UnknownId {
		instanceNets, err := e.NetworkInterfaces(inst)
		if err != nil {
			return ret, errors.Trace(err)
		}
		if len(subnetIds) == 0 {
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
		if len(subnetIds) == 0 {
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
		return nil, errors.Errorf("missing subnets: %s", strings.Join(missing, ","))
	}
	return ret, nil
}

// return all network cards attributes from a oracle instance
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

// DeleteMachineVnicSet will delete the machine virtual nic and all acl
// rules that are bound with it
func (o *oracleEnviron) DeleteMachineVnicSet(machineId string) error {
	if err := o.firewall.removeACLAndRules(machineId); err != nil {
		return errors.Trace(err)
	}
	name := o.client.ComposeName(o.namespace.Value(machineId))
	err := o.client.DeleteVnicSet(name)
	if err != nil {
		if !oci.IsNotFound(err) {
			return errors.Trace(err)
		}
	}
	return nil
}

func (o *oracleEnviron) ensureVnicSet(machineId string, tags []string) (ociResponse.VnicSet, error) {
	acl, err := o.firewall.createDefaultACLAndRules(machineId)
	if err != nil {
		return ociResponse.VnicSet{}, errors.Trace(err)
	}
	name := o.client.ComposeName(o.namespace.Value(machineId))
	details, err := o.client.VnicSetDetails(name)
	if err != nil {
		if !oci.IsNotFound(err) {
			return ociResponse.VnicSet{}, errors.Trace(err)
		}
		logger.Debugf("Creating vnic set %q", name)
		vnicSetParams := oci.VnicSetParams{
			AppliedAcls: []string{
				acl.Name,
			},
			Description: "Juju created vnic set",
			Name:        name,
			Tags:        tags,
		}
		details, err := o.client.CreateVnicSet(vnicSetParams)
		if err != nil {
			return ociResponse.VnicSet{}, errors.Trace(err)
		}
		return details, nil
	}
	return details, nil
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
			InterfaceType: network.EthernetInterface,
		}

		// gsamfira: VEthernet NICs are connected to shared networks
		// I have not found a way to interrogate details about the shared
		// networks available inside the oracle cloud. There is some documentation
		// on the matter here:
		// https://docs.oracle.com/cloud-machine/latest/stcomputecs/ELUAP/GUID-8CBE0F4E-E376-4C93-BB56-884836273168.htm
		// but I have not been able to get any information about the shared networks
		// using the resources described there.
		// We only populate Space information for NICs attached to IPNetworks (user defined)
		if nicObj.GetType() == ociCommon.VNic {
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

// getSubnetInfoAsMap will return the subnet information as a map of subnets
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
		logger.Infof("found network %s with space %s", string(val.ProviderId), string(val.SpaceProviderId))
		providerID := string(val.SpaceProviderId)
		tmp := strings.Split(providerID, `/`)
		name := tmp[len(tmp)-1]
		// Oracle allows us to attach an IP network to a space belonging to
		// another user using the web portal. We recompose the provider ID (which is unique)
		// and compare to the provider ID of the space. If they match, the space belongs to us
		tmpProviderId := e.client.ComposeName(name)
		if tmpProviderId != providerID {
			continue
		}
		if space, ok := exchanges[name]; !ok {
			logger.Infof("creating new space obj for %s and adding %s", name, string(val.ProviderId))
			exchanges[name] = network.SpaceInfo{
				Name:       name,
				ProviderId: val.SpaceProviderId,
				Subnets: []network.SubnetInfo{
					val,
				},
			}
		} else {
			logger.Infof("appending subnet %s to %s", string(val.ProviderId), string(space.Name))
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
