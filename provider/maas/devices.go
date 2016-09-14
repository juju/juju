// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"net/url"
	"path"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// TODO(dimitern): The types below should be part of gomaasapi.
// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119310616

type maasZone struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ResourceURI string `json:"resource_uri"`
}

type maasMACAddress struct {
	MACAddress string `json:"mac_address"`
}

type maasDevice struct {
	SystemID      string           `json:"system_id"`
	Parent        string           `json:"parent"`
	Hostname      string           `json:"hostname"`
	IPAddresses   []string         `json:"ip_addresses"`
	Owner         string           `json:"owner"`
	Zone          maasZone         `json:"zone"`
	MACAddressSet []maasMACAddress `json:"macaddress_set"`
	TagNames      []string         `json:"tag_names"`
	ResourceURI   string           `json:"resource_uri"`
}

func parseDevice(jsonBytes []byte) (*maasDevice, error) {
	var device maasDevice
	if err := json.Unmarshal(jsonBytes, &device); err != nil {
		return nil, errors.Annotate(err, "parsing device")
	}
	return &device, nil
}

func getJSONBytes(object json.Marshaler) ([]byte, error) {
	rawBytes, err := object.MarshalJSON()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get JSON bytes")
	}
	return rawBytes, nil
}

func (env *maasEnviron) createDevice(hostInstanceID instance.Id, hostname string, primaryMACAddress string) (*maasDevice, error) {
	devicesAPI := env.getMAASClient().GetSubObject("devices")
	params := make(url.Values)
	params.Add("hostname", hostname)
	params.Add("parent", extractSystemId(hostInstanceID))
	params.Add("mac_addresses", primaryMACAddress)

	result, err := devicesAPI.CallPost("new", params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	deviceJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	device, err := parseDevice(deviceJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("created device: %+v", device)
	return device, nil
}

func parseInterface(jsonBytes []byte) (*maasInterface, error) {
	var iface maasInterface
	if err := json.Unmarshal(jsonBytes, &iface); err != nil {
		return nil, errors.Annotate(err, "parsing interface")
	}
	return &iface, nil
}

func (env *maasEnviron) createDeviceInterface(deviceID instance.Id, name, macAddress, vlanID string) (*maasInterface, error) {
	deviceSystemID := extractSystemId(deviceID)
	uri := path.Join("nodes", deviceSystemID, "interfaces")
	interfacesAPI := env.getMAASClient().GetSubObject(uri)

	params := make(url.Values)
	params.Add("name", name)
	params.Add("mac_address", macAddress)
	params.Add("vlan", vlanID)

	result, err := interfacesAPI.CallPost("create_physical", params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaceJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	iface, err := parseInterface(interfaceJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return iface, nil
}

func (env *maasEnviron) updateDeviceInterface(deviceID instance.Id, interfaceID, name, macAddress, vlanID string) (*maasInterface, error) {
	deviceSystemID := extractSystemId(deviceID)
	uri := path.Join("nodes", deviceSystemID, "interfaces", interfaceID)
	interfacesAPI := env.getMAASClient().GetSubObject(uri)

	params := make(url.Values)
	params.Add("name", name)
	params.Add("mac_address", macAddress)
	params.Add("vlan", vlanID)

	result, err := interfacesAPI.Update(params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaceJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	iface, err := parseInterface(interfaceJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return iface, nil
}

func (env *maasEnviron) linkDeviceInterfaceToSubnet(deviceID instance.Id, interfaceID, subnetID string, mode maasLinkMode) (*maasInterface, error) {
	deviceSystemID := extractSystemId(deviceID)
	uri := path.Join("nodes", deviceSystemID, "interfaces", interfaceID)
	interfacesAPI := env.getMAASClient().GetSubObject(uri)

	params := make(url.Values)
	params.Add("mode", string(mode))
	params.Add("subnet", subnetID)

	result, err := interfacesAPI.CallPost("link_subnet", params)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaceJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	iface, err := parseInterface(interfaceJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return iface, nil
}

func (env *maasEnviron) deviceInterfaces(deviceID instance.Id) ([]maasInterface, error) {
	deviceSystemID := extractSystemId(deviceID)
	uri := path.Join("nodes", deviceSystemID, "interfaces")
	interfacesAPI := env.getMAASClient().GetSubObject(uri)

	result, err := interfacesAPI.CallGet("", nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfacesJSON, err := getJSONBytes(result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	interfaces, err := parseInterfaces(interfacesJSON)
	if err != nil {
		return nil, errors.Trace(err)
	}
	logger.Debugf("device %q interfaces: %+v", deviceSystemID, interfaces)
	return interfaces, nil

}

func (env *maasEnviron) deviceInterfaceInfo(deviceID instance.Id, nameToParentName map[string]string) ([]network.InterfaceInfo, error) {
	interfaces, err := env.deviceInterfaces(deviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	interfaceInfo := make([]network.InterfaceInfo, 0, len(interfaces))
	for _, nic := range interfaces {
		nicInfo := network.InterfaceInfo{
			InterfaceName:       nic.Name,
			InterfaceType:       network.EthernetInterface,
			MACAddress:          nic.MACAddress,
			MTU:                 nic.EffectveMTU,
			VLANTag:             nic.VLAN.VID,
			ProviderId:          network.Id(strconv.Itoa(nic.ID)),
			ProviderVLANId:      network.Id(strconv.Itoa(nic.VLAN.ID)),
			Disabled:            !nic.Enabled,
			NoAutoStart:         !nic.Enabled,
			ParentInterfaceName: nameToParentName[nic.Name],
		}

		if len(nic.Links) == 0 {
			logger.Debugf("device %q interface %q has no links", deviceID, nic.Name)
			interfaceInfo = append(interfaceInfo, nicInfo)
			continue
		}

		for _, link := range nic.Links {
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(string(link.Mode), link.IPAddress)

			if link.IPAddress == "" {
				logger.Debugf("device %q interface %q has no address", deviceID, nic.Name)
				interfaceInfo = append(interfaceInfo, nicInfo)
				continue
			}

			if link.Subnet == nil {
				logger.Debugf("device %q interface %q link %d missing subnet", deviceID, nic.Name, link.ID)
				interfaceInfo = append(interfaceInfo, nicInfo)
				continue
			}

			nicInfo.CIDR = link.Subnet.CIDR
			nicInfo.Address = network.NewAddressOnSpace(link.Subnet.Space, link.IPAddress)
			nicInfo.ProviderSubnetId = network.Id(strconv.Itoa(link.Subnet.ID))
			nicInfo.ProviderAddressId = network.Id(strconv.Itoa(link.ID))
			if link.Subnet.GatewayIP != "" {
				nicInfo.GatewayAddress = network.NewAddressOnSpace(link.Subnet.Space, link.Subnet.GatewayIP)
			}
			if len(link.Subnet.DNSServers) > 0 {
				nicInfo.DNSServers = network.NewAddressesOnSpace(link.Subnet.Space, link.Subnet.DNSServers...)
			}

			interfaceInfo = append(interfaceInfo, nicInfo)
		}
	}
	logger.Debugf("device %q has interface info: %+v", deviceID, interfaceInfo)
	return interfaceInfo, nil
}

func (env *maasEnviron) deviceInterfaceInfo2(deviceID string, nameToParentName map[string]string) ([]network.InterfaceInfo, error) {
	args := gomaasapi.DevicesArgs{SystemIDs: []string{deviceID}}
	devices, err := env.maasController.Devices(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(devices) != 1 {
		return nil, errors.Errorf("unexpected response requesting device %v: %v", deviceID, devices)
	}
	interfaces := devices[0].InterfaceSet()

	interfaceInfo := make([]network.InterfaceInfo, 0, len(interfaces))
	for _, nic := range interfaces {
		vlanId := 0
		vlanVid := 0
		vlan := nic.VLAN()
		if vlan != nil {
			vlanId = vlan.ID()
			vlanVid = vlan.VID()
		}
		nicInfo := network.InterfaceInfo{
			InterfaceName:       nic.Name(),
			InterfaceType:       network.EthernetInterface,
			MACAddress:          nic.MACAddress(),
			MTU:                 nic.EffectiveMTU(),
			VLANTag:             vlanVid,
			ProviderId:          network.Id(strconv.Itoa(nic.ID())),
			ProviderVLANId:      network.Id(strconv.Itoa(vlanId)),
			Disabled:            !nic.Enabled(),
			NoAutoStart:         !nic.Enabled(),
			ParentInterfaceName: nameToParentName[nic.Name()],
		}

		if len(nic.Links()) == 0 {
			logger.Debugf("device %q interface %q has no links", deviceID, nic.Name())
			interfaceInfo = append(interfaceInfo, nicInfo)
			continue
		}

		for _, link := range nic.Links() {
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(link.Mode(), link.IPAddress())

			subnet := link.Subnet()
			if link.IPAddress() == "" || subnet == nil {
				logger.Debugf("device %q interface %q has no address", deviceID, nic.Name())
				interfaceInfo = append(interfaceInfo, nicInfo)
				continue
			}

			nicInfo.CIDR = subnet.CIDR()
			nicInfo.Address = network.NewAddressOnSpace(subnet.Space(), link.IPAddress())
			nicInfo.ProviderSubnetId = network.Id(strconv.Itoa(subnet.ID()))
			nicInfo.ProviderAddressId = network.Id(strconv.Itoa(link.ID()))
			if subnet.Gateway() != "" {
				nicInfo.GatewayAddress = network.NewAddressOnSpace(subnet.Space(), subnet.Gateway())
			}
			if len(subnet.DNSServers()) > 0 {
				nicInfo.DNSServers = network.NewAddressesOnSpace(subnet.Space(), subnet.DNSServers()...)
			}

			interfaceInfo = append(interfaceInfo, nicInfo)
		}
	}
	logger.Debugf("device %q has interface info: %+v", deviceID, interfaceInfo)
	return interfaceInfo, nil
}
