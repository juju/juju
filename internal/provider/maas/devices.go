// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi/v2"

	corenetwork "github.com/juju/juju/core/network"
)

func (env *maasEnviron) deviceInterfaceInfo(
	ctx context.Context,
	device gomaasapi.Device,
	nameToParentName map[string]string,
	subnetToStaticRoutes map[string][]gomaasapi.StaticRoute,
) (corenetwork.InterfaceInfos, error) {
	deviceID := device.SystemID()
	interfaces := device.InterfaceSet()

	interfaceInfo := make(corenetwork.InterfaceInfos, 0, len(interfaces))
	for idx, nic := range interfaces {
		vlanId := 0
		vlanVid := 0
		vlan := nic.VLAN()
		if vlan != nil {
			vlanId = vlan.ID()
			vlanVid = vlan.VID()
		}
		nicInfo := corenetwork.InterfaceInfo{
			DeviceIndex:         idx,
			InterfaceName:       nic.Name(),
			InterfaceType:       corenetwork.EthernetDevice,
			MACAddress:          nic.MACAddress(),
			MTU:                 nic.EffectiveMTU(),
			VLANTag:             vlanVid,
			ProviderId:          corenetwork.Id(strconv.Itoa(nic.ID())),
			ProviderVLANId:      corenetwork.Id(strconv.Itoa(vlanId)),
			Disabled:            !nic.Enabled(),
			NoAutoStart:         !nic.Enabled(),
			ParentInterfaceName: nameToParentName[nic.Name()],
			Origin:              corenetwork.OriginProvider,
		}
		for _, link := range nic.Links() {
			subnet := link.Subnet()
			if subnet == nil {
				continue
			}
			routes := subnetToStaticRoutes[subnet.CIDR()]
			for _, route := range routes {
				nicInfo.Routes = append(nicInfo.Routes, corenetwork.Route{
					DestinationCIDR: route.Destination().CIDR(),
					GatewayIP:       route.GatewayIP(),
					Metric:          route.Metric(),
				})
			}
		}

		if len(nic.Links()) == 0 {
			logger.Debugf(ctx, "device %q interface %q has no links", deviceID, nic.Name())
			interfaceInfo = append(interfaceInfo, nicInfo)
			continue
		}

		for _, link := range nic.Links() {
			configType := maasLinkToInterfaceConfigType(link.Mode())

			subnet := link.Subnet()
			if link.IPAddress() == "" || subnet == nil {
				logger.Debugf(ctx, "device %q interface %q has no address", deviceID, nic.Name())
				interfaceInfo = append(interfaceInfo, nicInfo)
				continue
			}

			// NOTE(achilleasa): the original code used a last-write-wins
			// policy. Do we need to append link addresses to the list?
			nicInfo.Addresses = corenetwork.ProviderAddresses{
				corenetwork.NewMachineAddress(
					link.IPAddress(),
					corenetwork.WithCIDR(subnet.CIDR()),
					corenetwork.WithConfigType(configType),
				).AsProviderAddress(corenetwork.WithSpaceName(subnet.Space())),
			}
			nicInfo.ProviderSubnetId = corenetwork.Id(strconv.Itoa(subnet.ID()))
			nicInfo.ProviderAddressId = corenetwork.Id(strconv.Itoa(link.ID()))
			if subnet.Gateway() != "" {
				nicInfo.GatewayAddress = corenetwork.NewMachineAddress(
					subnet.Gateway(),
				).AsProviderAddress(corenetwork.WithSpaceName(subnet.Space()))
			}
			if len(subnet.DNSServers()) > 0 {
				nicInfo.DNSServers = subnet.DNSServers()
			}

			interfaceInfo = append(interfaceInfo, nicInfo)
		}
	}
	logger.Debugf(ctx, "device %q has interface info: %+v", deviceID, interfaceInfo)
	return interfaceInfo, nil
}

type deviceCreatorParams struct {
	Name                 string
	Subnet               gomaasapi.Subnet // may be nil
	PrimaryMACAddress    string
	PrimaryNICName       string
	DesiredInterfaceInfo corenetwork.InterfaceInfos
	CIDRToMAASSubnet     map[string]gomaasapi.Subnet
	CIDRToStaticRoutes   map[string][]gomaasapi.StaticRoute
	Machine              gomaasapi.Machine
}

func (env *maasEnviron) createAndPopulateDevice(ctx context.Context, params deviceCreatorParams) (gomaasapi.Device, error) {
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
			_ = device.Delete()
		}
	}()
	interfaces := device.InterfaceSet()
	if len(interfaces) != 1 {
		// Shouldn't be possible as machine.CreateDevice always
		// returns a device with one interface.
		names := make([]string, len(interfaces))
		for i, iface := range interfaces {
			names[i] = iface.Name()
		}
		err = errors.Errorf("unexpected number of interfaces "+
			"in response from creating device: %v", names)
		return nil, err
	}
	primaryNIC := interfaces[0]
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

		subnet, knownSubnet := params.CIDRToMAASSubnet[nic.PrimaryAddress().CIDR]
		if !knownSubnet {
			if primaryNICVLAN == nil {
				// There is no primary NIC VLAN, so we can't fallback to the
				// primaryNIC VLAN. Instead we'll emit a warning that no
				// subnet is found, nor a primary NIC VLAN is available.
				logger.Warningf(ctx, "NIC %v has no subnet and no primary NIC VLAN", nic.InterfaceName)
				continue
			}

			logger.Warningf(ctx, "NIC %v has no subnet - setting to manual and using 'primaryNIC' VLAN %d", nic.InterfaceName, primaryNICVLAN.ID())
			createArgs.VLAN = primaryNICVLAN
		} else {
			createArgs.VLAN = subnet.VLAN()
			logger.Infof(ctx, "linking NIC %v to subnet %v - using static IP", nic.InterfaceName, subnet.CIDR())
		}

		createdNIC, err := device.CreateInterface(createArgs)
		if err != nil {
			return nil, errors.Annotate(err, "creating device interface")
		}
		logger.Debugf(ctx, "created device interface: %+v", createdNIC)
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
		}
		logger.Debugf(ctx, "linked device interface to subnet: %+v", createdNIC)
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
func (env *maasEnviron) lookupStaticRoutes(ctx context.Context) (map[string][]gomaasapi.StaticRoute, error) {
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
				logger.Debugf(ctx, "static-routes not supported: %v", err)
				handled = true
				staticRoutes = nil
			} else {
				logger.Warningf(ctx, "looking up static routes generated IsUnexpectedError, but didn't match: %q %#v", msg, err)
			}
		} else {
			logger.Warningf(ctx, "not IsUnexpectedError: %#v", err)
		}
		if !handled {
			logger.Warningf(ctx, "error looking up static-routes: %v", err)
			return nil, errors.Annotate(err, "unable to look up static-routes")
		}
	}
	for _, route := range staticRoutes {
		source := route.Source()
		sourceCIDR := source.CIDR()
		subnetToStaticRoutes[sourceCIDR] = append(subnetToStaticRoutes[sourceCIDR], route)
	}
	logger.Debugf(ctx, "found static routes: %# v", subnetToStaticRoutes)
	return subnetToStaticRoutes, nil
}

func (env *maasEnviron) prepareDeviceDetails(ctx context.Context, name string, machine gomaasapi.Machine, preparedInfo corenetwork.InterfaceInfos) (deviceCreatorParams, error) {
	var zeroParams deviceCreatorParams

	subnetCIDRToSubnet, err := env.lookupSubnets()
	if err != nil {
		return zeroParams, errors.Trace(err)
	}
	subnetToStaticRoutes, err := env.lookupStaticRoutes(ctx)
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

	var primaryNICInfo corenetwork.InterfaceInfo
	for _, nic := range preparedInfo {
		if nic.InterfaceName == params.PrimaryNICName {
			primaryNICInfo = nic
			break
		}
	}
	if primaryNICInfo.InterfaceName == "" {
		return zeroParams, errors.Errorf("cannot find primary interface for container")
	}
	logger.Debugf(ctx, "primary device NIC prepared info: %+v", primaryNICInfo)

	primaryNICSubnetCIDR := primaryNICInfo.PrimaryAddress().CIDR
	subnet, hasSubnet := subnetCIDRToSubnet[primaryNICSubnetCIDR]
	if hasSubnet {
		params.Subnet = subnet
	} else {
		logger.Debugf(ctx, "primary device NIC %q has no linked subnet - leaving unconfigured", primaryNICInfo.InterfaceName)
	}
	params.PrimaryMACAddress = primaryNICInfo.MACAddress
	return params, nil
}

func validateExistingDevice(ctx context.Context, netInfo corenetwork.InterfaceInfos, device gomaasapi.Device) (bool, error) {
	// Compare the desired device characteristics with the actual device
	interfaces := device.InterfaceSet()
	if len(interfaces) < len(netInfo) {
		logger.Debugf(ctx, "existing device doesn't have enough interfaces, wanted %d, found %d", len(netInfo), len(interfaces))
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
			logger.Debugf(ctx, "existing device doesn't have device for MAC Address %q, found: %s", desired.MACAddress, found)
			// No such network interface
			return false, nil
		}
		// TODO: we should have a way to know what space we are targeting, rather than a desired subnet CIDR
		foundCIDR := false
		for _, link := range actual.Links() {
			subnet := link.Subnet()
			if subnet != nil {
				cidr := subnet.CIDR()
				if cidr == desired.PrimaryAddress().CIDR {
					foundCIDR = true
				}
			}
		}
		if !foundCIDR {
			logger.Debugf(ctx, "could not find Subnet link for CIDR: %q", desired.PrimaryAddress().CIDR)
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
func (env *maasEnviron) checkForExistingDevice(ctx context.Context, params deviceCreatorParams) (gomaasapi.Device, error) {
	devicesArgs := gomaasapi.DevicesArgs{
		Hostname: []string{params.Name},
	}
	maybeDevices, err := params.Machine.Devices(devicesArgs)
	if err != nil {
		logger.Warningf(ctx, "error while trying to lookup %q: %v", params.Name, err)
		// not considered fatal, since we'll attempt to create the device if we didn't find it
		return nil, nil
	}
	if len(maybeDevices) == 0 {
		logger.Debugf(ctx, "no existing MAAS devices for container %q, creating", params.Name)
		return nil, nil
	}
	if len(maybeDevices) > 1 {
		logger.Warningf(ctx, "found more than 1 MAAS devices (%d) for container %q", len(maybeDevices),
			params.Name)
		return nil, errors.Errorf("found more than 1 MAAS device (%d) for container %q",
			len(maybeDevices), params.Name)
	}
	device := maybeDevices[0]
	// Now validate that this device has the right interfaces
	matches, err := validateExistingDevice(ctx, params.DesiredInterfaceInfo, device)
	if err != nil {
		return nil, err
	}
	if matches {
		logger.Debugf(ctx, "found MAAS device for container %q using existing device", params.Name)
		return device, nil
	}
	logger.Debugf(ctx, "found existing MAAS device for container %q but interfaces did not match, removing device", params.Name)
	// We found a device, but it doesn't match what we need. remove it and we'll create again.
	_ = device.Delete()
	return nil, nil
}
