// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"net"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	names "gopkg.in/juju/names.v2"
)

// These methods here belong of the environ.Networking interface
// their are provided here to tell the juju
// if the oracle provider supports different network options
// Netowrking interface defines methods that environmnets with
// networking capabilities must implement.
// Together these implements also the NetworkingEnviron interface

// Subnet returns basic information about subnets
// known by the oracle provider for the environmnet
func (e oracleEnviron) Subnets(
	inst instance.Id,
	subnetIds []network.Id,
) ([]network.SubnetInfo, error) {

	return nil, nil
}

// NetworkInterfaces requests information about the
// network interfaces on the given instance
func (e oracleEnviron) NetworkInterfaces(
	instId instance.Id,
) ([]network.InterfaceInfo, error) {

	id := string(instId)

	instance, err := e.client.InstanceDetails(e.client.ComposeName(id))
	if err != nil {
		return nil, err
	}

	n := len(instance.Networking)
	if n == 0 {
		return nil, errors.Errorf(
			"Cannot get the network interfaces of the instance %s", id,
		)
	}

	interfaces := make([]network.InterfaceInfo, 0, n)
	//TODO(sgiulitti) find all the info and parse it
	for nicName := range instance.Networking {
		ni := network.InterfaceInfo{InterfaceName: nicName}

		// for every attribute we should check if
		// we are dealing with the nicName attributes
		for key, value := range instance.Attributes.Network {
			// if we are dealing with the correct one
			// we should extract and populate the InterfaceInfo struct
			if strings.Contains(key, nicName) {
				ni.DeviceIndex, err = strconv.Atoi(value.Vethernet_id)
				if err != nil {
					return nil, errors.Trace(err)
				}

				// get the mac and IP
				mac, address, err := getMacAndIP(value.Address)
				if err != nil {
					return nil, errors.Trace(err)
				}

				ni.MACAddress = mac
				ni.Address = network.NewAddress(address)

				break
			}
		}

		interfaces = append(interfaces, ni)
	}

	return interfaces, nil
}

// getMacAndIp picks and returns the correct mac and ip from the given slice
// if the slice does not contain a valid mac and ip it will return an error
func getMacAndIP(address []string) (mac string, ip string, err error) {

	if address == nil {
		return "", "", errors.New(
			"Empty address slice given",
		)
	}

	if len(address) != 2 {
		return "", "", errors.Errorf(
			"more than one mac and one ip in address %#v",
			address,
		)
	}

	for _, val := range address {

		valIp := net.ParseIP(val)
		if valIp != nil {
			ip = val
			continue
		}

		_, err = net.ParseMAC(val)
		if err != nil {
			return "", "", errors.Errorf(
				"The address is not an mac neighter an ip %s", val,
			)
		}

		mac = val
	}

	return mac, ip, nil
}

// SupportsSpaces returns whether the current oracle environment supports
// spaces. The returned error satisfies errors.IsNotSupported(),
// unless a general API failure occurs.
func (e oracleEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

// SupportsSpaceDiscovery returns whether the current environment
// supports discovering spaces from the oracle provider. The returned error
// satisfies errors.IsNotSupported(), unless a general API failure occurs.
func (e oracleEnviron) SupportsSpaceDiscovery() (bool, error) {
	return false, errors.NotSupportedf("space discovery")
}

// Spaces returns a slice of network.SpaceInfo with info, including
// details of all associated subnets, about all spaces known to the
// oracle provider that have subnets available.
func (e oracleEnviron) Spaces() ([]network.SpaceInfo, error) {
	return nil, nil
}

// AllocateContainerAddresses allocates a static address for each of the
// container NICs in preparedInfo, hosted by the hostInstanceID. Returns the
// network config including all allocated addresses on success.
func (e oracleEnviron) AllocateContainerAddresses(
	hostInstanceID instance.Id,
	containerTag names.MachineTag,
	preparedInfo []network.InterfaceInfo,
) ([]network.InterfaceInfo, error) {
	return nil, errors.NotSupportedf("containers")
}

// ReleaseContainerAddresses releases the previously allocated
// addresses matching the interface details passed in.
func (e oracleEnviron) ReleaseContainerAddresses(interfaces []network.ProviderInterfaceInfo) error {
	return errors.NotSupportedf("container")
}
