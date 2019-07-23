// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
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
			ProviderId:          corenetwork.Id(strconv.Itoa(nic.ID)),
			ProviderVLANId:      corenetwork.Id(strconv.Itoa(nic.VLAN.ID)),
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
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(string(link.Mode))

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
			nicInfo.ProviderSubnetId = corenetwork.Id(strconv.Itoa(link.Subnet.ID))
			nicInfo.ProviderAddressId = corenetwork.Id(strconv.Itoa(link.ID))
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

func (env *maasEnviron) deviceInterfaceInfo2(device gomaasapi.Device, nameToParentName map[string]string, subnetToStaticRoutes map[string][]gomaasapi.StaticRoute) ([]network.InterfaceInfo, error) {
	deviceID := device.SystemID()
	interfaces := device.InterfaceSet()

	interfaceInfo := make([]network.InterfaceInfo, 0, len(interfaces))
	for idx, nic := range interfaces {
		vlanId := 0
		vlanVid := 0
		vlan := nic.VLAN()
		if vlan != nil {
			vlanId = vlan.ID()
			vlanVid = vlan.VID()
		}
		nicInfo := network.InterfaceInfo{
			DeviceIndex:         idx,
			InterfaceName:       nic.Name(),
			InterfaceType:       network.EthernetInterface,
			MACAddress:          nic.MACAddress(),
			MTU:                 nic.EffectiveMTU(),
			VLANTag:             vlanVid,
			ProviderId:          corenetwork.Id(strconv.Itoa(nic.ID())),
			ProviderVLANId:      corenetwork.Id(strconv.Itoa(vlanId)),
			Disabled:            !nic.Enabled(),
			NoAutoStart:         !nic.Enabled(),
			ParentInterfaceName: nameToParentName[nic.Name()],
		}
		for _, link := range nic.Links() {
			subnet := link.Subnet()
			if subnet == nil {
				continue
			}
			routes := subnetToStaticRoutes[subnet.CIDR()]
			for _, route := range routes {
				nicInfo.Routes = append(nicInfo.Routes, network.Route{
					DestinationCIDR: route.Destination().CIDR(),
					GatewayIP:       route.GatewayIP(),
					Metric:          route.Metric(),
				})
			}
		}

		if len(nic.Links()) == 0 {
			logger.Debugf("device %q interface %q has no links", deviceID, nic.Name())
			interfaceInfo = append(interfaceInfo, nicInfo)
			continue
		}

		for _, link := range nic.Links() {
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(link.Mode())

			subnet := link.Subnet()
			if link.IPAddress() == "" || subnet == nil {
				logger.Debugf("device %q interface %q has no address", deviceID, nic.Name())
				interfaceInfo = append(interfaceInfo, nicInfo)
				continue
			}

			nicInfo.CIDR = subnet.CIDR()
			nicInfo.Address = network.NewAddressOnSpace(subnet.Space(), link.IPAddress())
			nicInfo.ProviderSubnetId = corenetwork.Id(strconv.Itoa(subnet.ID()))
			nicInfo.ProviderAddressId = corenetwork.Id(strconv.Itoa(link.ID()))
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

type deviceCreatorParams struct {
	Name                 string
	Subnet               gomaasapi.Subnet // may be nil
	PrimaryMACAddress    string
	PrimaryNICName       string
	DesiredInterfaceInfo []network.InterfaceInfo
	CIDRToMAASSubnet     map[string]gomaasapi.Subnet
	CIDRToStaticRoutes   map[string][]gomaasapi.StaticRoute
	Machine              gomaasapi.Machine
}

func (env *maasEnviron) createAndPopulateDevice(params deviceCreatorParams) (gomaasapi.Device, error) {
	createDeviceArgs := gomaasapi.CreateMachineDeviceArgs{
		Hostname:      params.Name,
		MACAddress:    params.PrimaryMACAddress,
		Subnet:        params.Subnet, // can be nil
		InterfaceName: params.PrimaryNICName,
	}
	device, err := params.Machine.CreateDevice(createDeviceArgs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			device.Delete()
		}
	}()
	interface_set := device.InterfaceSet()
	if len(interface_set) != 1 {
		// Shouldn't be possible as machine.CreateDevice always
		// returns a device with one interface.
		names := make([]string, len(interface_set))
		for i, iface := range interface_set {
			names[i] = iface.Name()
		}
		err = errors.Errorf("unexpected number of interfaces "+
			"in response from creating device: %v", names)
		return nil, err
	}
	primaryNIC := interface_set[0]
	primaryNICVLAN := primaryNIC.VLAN()

	interfaceCreated := false
	// Populate the rest of the desired interfaces on this device
	for _, nic := range params.DesiredInterfaceInfo {
		if nic.InterfaceName == params.PrimaryNICName {
			// already handled in CreateDevice
			continue
		}
		// We have to register an extra interface for this container
		// (aka 'device'), and then link that device to the desired
		// subnet so that it can acquire an IP address from MAAS.
		createArgs := gomaasapi.CreateInterfaceArgs{
			Name:       nic.InterfaceName,
			MTU:        nic.MTU,
			MACAddress: nic.MACAddress,
		}

		subnet, knownSubnet := params.CIDRToMAASSubnet[nic.CIDR]
		if !knownSubnet {
			logger.Warningf("NIC %v has no subnet - setting to manual and using 'primaryNIC' VLAN %d", nic.InterfaceName, primaryNICVLAN.ID())
			createArgs.VLAN = primaryNICVLAN
		} else {
			createArgs.VLAN = subnet.VLAN()
			logger.Infof("linking NIC %v to subnet %v - using static IP", nic.InterfaceName, subnet.CIDR())
		}

		createdNIC, err := device.CreateInterface(createArgs)
		if err != nil {
			return nil, errors.Annotate(err, "creating device interface")
		}
		logger.Debugf("created device interface: %+v", createdNIC)
		interfaceCreated = true

		if !knownSubnet {
			// If we didn't request an explicit subnet, then we
			// don't need to link the device to that subnet
			continue
		}

		linkArgs := gomaasapi.LinkSubnetArgs{
			Mode:   gomaasapi.LinkModeStatic,
			Subnet: subnet,
		}

		if err := createdNIC.LinkSubnet(linkArgs); err != nil {
			return nil, errors.Annotatef(err, "linking NIC %v to subnet %v", nic.InterfaceName, subnet.CIDR())
		} else {
			logger.Debugf("linked device interface to subnet: %+v", createdNIC)
		}
	}
	// If we have created any secondary interfaces we need to reload device from maas
	// so that the changes are reflected in structure.
	if interfaceCreated {
		deviceID := device.SystemID()
		args := gomaasapi.DevicesArgs{SystemIDs: []string{deviceID}}
		devices, err := env.maasController.Devices(args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(devices) != 1 {
			err = errors.Errorf("unexpected response requesting device %v: %v", deviceID, devices)
			return nil, err
		}
		device = devices[0]
	}
	return device, nil
}

func (env *maasEnviron) lookupSubnets() (map[string]gomaasapi.Subnet, error) {
	subnetCIDRToSubnet := make(map[string]gomaasapi.Subnet)
	spaces, err := env.maasController.Spaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, space := range spaces {
		for _, subnet := range space.Subnets() {
			subnetCIDRToSubnet[subnet.CIDR()] = subnet
		}
	}
	return subnetCIDRToSubnet, nil
}
func (env *maasEnviron) lookupStaticRoutes() (map[string][]gomaasapi.StaticRoute, error) {
	// map from the source subnet (what subnet is the device in), to what
	// static routes should be used.
	subnetToStaticRoutes := make(map[string][]gomaasapi.StaticRoute)
	staticRoutes, err := env.maasController.StaticRoutes()
	if err != nil {
		// MAAS 2.0 does not support static-routes, and will return a 404. MAAS
		// does not report support for static-routes in its capabilities, nor
		// does it have a different API version between 2.1 and 2.0. So we make
		// the attempt, and treat a 404 as not having any configured static
		// routes.
		// gomaaasapi wraps a ServerError in an UnexpectedError, so we need to
		// dig to make sure we have the right cause:
		handled := false
		if gomaasapi.IsUnexpectedError(err) {
			msg := err.Error()
			if strings.Contains(msg, "404") &&
				strings.Contains(msg, "Unknown API endpoint:") &&
				strings.Contains(msg, "/static-routes/") {
				logger.Debugf("static-routes not supported: %v", err)
				handled = true
				staticRoutes = nil
			} else {
				logger.Warningf("looking up static routes generated IsUnexpectedError, but didn't match: %q %#v", msg, err)
			}
		} else {
			logger.Warningf("not IsUnexpectedError: %#v", err)
		}
		if !handled {
			logger.Warningf("error looking up static-routes: %v", err)
			return nil, errors.Annotate(err, "unable to look up static-routes")
		}
	}
	for _, route := range staticRoutes {
		source := route.Source()
		sourceCIDR := source.CIDR()
		subnetToStaticRoutes[sourceCIDR] = append(subnetToStaticRoutes[sourceCIDR], route)
	}
	logger.Debugf("found static routes: %# v", subnetToStaticRoutes)
	return subnetToStaticRoutes, nil
}

func (env *maasEnviron) prepareDeviceDetails(name string, machine gomaasapi.Machine, preparedInfo []network.InterfaceInfo) (deviceCreatorParams, error) {
	var zeroParams deviceCreatorParams

	subnetCIDRToSubnet, err := env.lookupSubnets()
	if err != nil {
		return zeroParams, errors.Trace(err)
	}
	subnetToStaticRoutes, err := env.lookupStaticRoutes()
	if err != nil {
		return zeroParams, errors.Trace(err)
	}
	params := deviceCreatorParams{
		// Containers always use 'eth0' as their primary NIC
		// XXX(jam) 2017-04-13: Except we *don't* do that for KVM containers running Xenial
		Name:                 name,
		Machine:              machine,
		PrimaryNICName:       "eth0",
		DesiredInterfaceInfo: preparedInfo,
		CIDRToMAASSubnet:     subnetCIDRToSubnet,
		CIDRToStaticRoutes:   subnetToStaticRoutes,
	}

	var primaryNICInfo network.InterfaceInfo
	for _, nic := range preparedInfo {
		if nic.InterfaceName == params.PrimaryNICName {
			primaryNICInfo = nic
			break
		}
	}
	if primaryNICInfo.InterfaceName == "" {
		return zeroParams, errors.Errorf("cannot find primary interface for container")
	}
	logger.Debugf("primary device NIC prepared info: %+v", primaryNICInfo)

	primaryNICSubnetCIDR := primaryNICInfo.CIDR
	subnet, hasSubnet := subnetCIDRToSubnet[primaryNICSubnetCIDR]
	if hasSubnet {
		params.Subnet = subnet
	} else {
		logger.Debugf("primary device NIC %q has no linked subnet - leaving unconfigured", primaryNICInfo.InterfaceName)
	}
	params.PrimaryMACAddress = primaryNICInfo.MACAddress
	return params, nil
}

func validateExistingDevice(netInfo []network.InterfaceInfo, device gomaasapi.Device) (bool, error) {
	// Compare the desired device characteristics with the actual device
	interfaces := device.InterfaceSet()
	if len(interfaces) < len(netInfo) {
		logger.Debugf("existing device doesn't have enough interfaces, wanted %d, found %d", len(netInfo), len(interfaces))
		return false, nil
	}
	actualByMAC := make(map[string]gomaasapi.Interface, len(interfaces))
	for _, iface := range interfaces {
		actualByMAC[iface.MACAddress()] = iface
	}
	for _, desired := range netInfo {
		actual, ok := actualByMAC[desired.MACAddress]
		if !ok {
			foundMACs := make([]string, 0, len(actualByMAC))
			for _, iface := range interfaces {
				foundMACs = append(foundMACs, fmt.Sprintf("%s: %s", iface.Name(), iface.MACAddress()))
			}
			found := strings.Join(foundMACs, ", ")
			logger.Debugf("existing device doesn't have device for MAC Address %q, found: %s", desired.MACAddress, found)
			// No such network interface
			return false, nil
		}
		// TODO: we should have a way to know what space we are targeting, rather than a desired subnet CIDR
		foundCIDR := false
		for _, link := range actual.Links() {
			subnet := link.Subnet()
			if subnet != nil {
				cidr := subnet.CIDR()
				if cidr == desired.CIDR {
					foundCIDR = true
				}
			}
		}
		if !foundCIDR {
			logger.Debugf("could not find Subnet link for CIDR: %q", desired.CIDR)
			return false, nil
		}
	}
	return true, nil
}

// checkForExistingDevice checks to see if we've already registered a device
// with this name, and if its information is appropriately populated. If we
// have, then we just return the existing interface info. If we find it, but
// it doesn't match, then we ask MAAS to remove it, which should cause the
// calling code to create it again.
func (env *maasEnviron) checkForExistingDevice(params deviceCreatorParams) (gomaasapi.Device, error) {
	devicesArgs := gomaasapi.DevicesArgs{
		Hostname: []string{params.Name},
	}
	maybeDevices, err := params.Machine.Devices(devicesArgs)
	if err != nil {
		logger.Warningf("error while trying to lookup %q: %v", params.Name, err)
		// not considered fatal, since we'll attempt to create the device if we didn't find it
		return nil, nil
	}
	if len(maybeDevices) == 0 {
		logger.Debugf("no existing MAAS devices for container %q, creating", params.Name)
		return nil, nil
	}
	if len(maybeDevices) > 1 {
		logger.Warningf("found more than 1 MAAS devices (%d) for container %q", len(maybeDevices),
			params.Name)
		return nil, errors.Errorf("found more than 1 MAAS device (%d) for container %q",
			len(maybeDevices), params.Name)
	}
	device := maybeDevices[0]
	// Now validate that this device has the right interfaces
	matches, err := validateExistingDevice(params.DesiredInterfaceInfo, device)
	if err != nil {
		return nil, err
	}
	if matches {
		logger.Debugf("found MAAS device for container %q using existing device", params.Name)
		return device, nil
	}
	logger.Debugf("found existing MAAS device for container %q but interfaces did not match, removing device", params.Name)
	// We found a device, but it doesn't match what we need. remove it and we'll create again.
	device.Delete()
	return nil, nil
}
