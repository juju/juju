// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

////////////////////////////////////////////////////////////////////////////////
// TODO(dimitern): The types below should be part of gomaasapi.
// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119310616

type maasLinkMode string

const (
	modeUnknown maasLinkMode = ""
	modeStatic  maasLinkMode = "static"
	modeDHCP    maasLinkMode = "dhcp"
	modeLinkUp  maasLinkMode = "link_up"
)

// InterfaceConfigType translates mode's value to its
// network.InterfaceConfigType equivalent.
func (mode maasLinkMode) InterfaceConfigType() network.InterfaceConfigType {
	switch mode {
	case modeUnknown:
		return network.ConfigUnknown
	case modeDHCP:
		return network.ConfigDHCP
	case modeStatic, modeLinkUp:
		return network.ConfigStatic
	}
	return network.ConfigManual
}

type maasInterfaceLink struct {
	ID        int          `json:"id"`
	Subnet    *maasSubnet  `json:"subnet,omitempty"`
	IPAddress string       `json:"ip_address,omitempty"`
	Mode      maasLinkMode `json:"mode"`
}

// ProviderId returns link.ID as network.Id.
func (link maasInterfaceLink) ProviderId() network.Id {
	return network.Id(fmt.Sprint(link.ID))
}

// Address translates link's IPAddress (if set) to network.Address.
func (link maasInterfaceLink) Address() network.Address {
	var result network.Address

	if link.Subnet == nil {
		return result
	}

	result = network.NewAddressOnSpace(link.Subnet.Space, link.IPAddress)
	result.SpaceProviderId = link.Subnet.SpaceProviderId

	return result
}

// InterfaceInfo translates link to the equivalent network.InterfaceInfo.
func (link maasInterfaceLink) InterfaceInfo() *network.InterfaceInfo {
	result := &network.InterfaceInfo{
		DeviceIndex:       -1, // avoid overriding top-level maasInterface.DeviceIndex value
		VLANTag:           -1, // same as above.
		ProviderAddressId: link.ProviderId(),
		ConfigType:        link.Mode.InterfaceConfigType(),
	}

	if link.Subnet != nil {
		result.Update(link.Subnet.InterfaceInfo())
		result.Address = link.Address()
		result.ProviderSpaceId = link.Subnet.SpaceProviderId
	}

	return result
}

type maasInterfaceType string

const (
	typeUnknown  maasInterfaceType = ""
	typePhysical maasInterfaceType = "physical"
	typeVLAN     maasInterfaceType = "vlan"
	typeBond     maasInterfaceType = "bond"
)

// InterfaceType translates ifaceType to its network.InterfaceType.
func (ifaceType maasInterfaceType) InterfaceType() network.InterfaceType {
	switch ifaceType {
	case typePhysical:
		return network.EthernetInterface
	case typeVLAN:
		return network.VLAN_8021QInterface
	case typeBond:
		return network.BondInterface
	}
	return network.UnknownInterface
}

type maasInterface struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Type    maasInterfaceType `json:"type"`
	Enabled bool              `json:"enabled"`

	MACAddress  string   `json:"mac_address"`
	VLAN        maasVLAN `json:"vlan"`
	EffectveMTU int      `json:"effective_mtu"`

	Links []maasInterfaceLink `json:"links"`

	Parents  []string `json:"parents"`
	Children []string `json:"children"`

	ResourceURI string `json:"resource_uri"`
}

// ProviderId returns iface.ID as network.Id.
func (iface maasInterface) ProviderId() network.Id {
	return network.Id(fmt.Sprint(iface.ID))
}

// ParentInterfaceName returns the name of this interface's parent interface,
// suitable to populate network.InterfaceInfo. MAAS network model semantics for
// parent/child relationships are translated to Juju's equivalents.
func (iface maasInterface) ParentInterfaceName() string {
	switch iface.Type {
	case typeBond:
		// In Juju's network model bonds have no parent, except if a bridge is
		// created on top of the bond.
	case typePhysical:
		// MAAS network model represents interfaces enslaved into a bond as
		// parents of that bond, and the bond itself as the single child of each
		// slave.
		if len(iface.Parents) == 0 && len(iface.Children) == 1 {
			parent := iface.Children[0]
			// A single child can be a bond or a VLAN (with name derived from
			// its parent name).
			if !strings.HasPrefix(parent, iface.Name) {
				return parent
			}
		}
	case typeVLAN:
		// VLAN interfaces have a single parent.
		if len(iface.Parents) == 1 {
			return iface.Parents[0]
		}
	}

	return ""
}

// InterfaceInfos translates iface to the equivalent slice of
// network.InterfaceInfo, with one entry per link, setting each entry's
// DeviceIndex to the given value. MAAS API does not support device indices.
func (iface maasInterface) InterfaceInfos(deviceIndex int) []network.InterfaceInfo {
	commonInfo := &network.InterfaceInfo{
		DeviceIndex:         deviceIndex,
		ProviderId:          iface.ProviderId(),
		InterfaceType:       iface.Type.InterfaceType(),
		Disabled:            !iface.Enabled,
		NoAutoStart:         !iface.Enabled,
		MACAddress:          iface.MACAddress,
		InterfaceName:       iface.Name,
		ParentInterfaceName: iface.ParentInterfaceName(),
	}
	commonInfo.Update(iface.VLAN.InterfaceInfo())
	commonInfo.MTU = iface.EffectveMTU

	results := make([]network.InterfaceInfo, len(iface.Links))
	for i, link := range iface.Links {
		linkInfo := link.InterfaceInfo()
		linkInfo.Update(commonInfo)
		results[i] = *linkInfo
	}

	return results
}

type maasInterfaces []maasInterface

// UpdateSpaceProviderIds uses the provided map to update interfaces to have
// SpaceProviderId set on all links with subnets. This is necessary for MAAS
// 1.9, as the API does not return the space ID a subnet is part of but the
// space name.
func (interfaces maasInterfaces) UpdateSpaceProviderIds(subnetCIDRToSpaceProviderId map[string]network.Id) {
	for i, iface := range interfaces {
		for j, link := range iface.Links {
			if link.Subnet == nil {
				continue
			}

			link.Subnet.SpaceProviderId, _ = subnetCIDRToSpaceProviderId[link.Subnet.CIDR]
			iface.Links[j] = link
		}
		interfaces[i] = iface
	}
}

// FirstTopLevelInterfaceByMACAddress returns the first interface with the given
// macAddress, for which ParentInterfaceName() returns empty string. Returns nil
// if no such interface exists.
func (interfaces maasInterfaces) FirstTopLevelInterfaceByMACAddress(macAddress string) *maasInterface {
	for _, iface := range interfaces {
		if iface.MACAddress == macAddress && iface.ParentInterfaceName() == "" {
			return &iface
		}
	}

	return nil
}

type maasVLAN struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	VID         int    `json:"vid"`
	MTU         int    `json:"mtu"`
	Fabric      string `json:"fabric"`
	ResourceURI string `json:"resource_uri"`
}

// ProviderId returns vlan.ID as network.Id.
func (vlan maasVLAN) ProviderId() network.Id {
	return network.Id(fmt.Sprint(vlan.ID))
}

// InterfaceInfo translates vlan to the equivalent network.InterfaceInfo.
func (vlan maasVLAN) InterfaceInfo() *network.InterfaceInfo {
	return &network.InterfaceInfo{
		DeviceIndex:    -1, // avoid overriding top-level maasInterface.DeviceIndex.
		VLANTag:        vlan.VID,
		ProviderVLANId: vlan.ProviderId(),
		MTU:            vlan.MTU,
	}
}

type maasSubnet struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Space       string   `json:"space"`
	VLAN        maasVLAN `json:"vlan"`
	GatewayIP   string   `json:"gateway_ip"`
	DNSServers  []string `json:"dns_servers"`
	CIDR        string   `json:"cidr"`
	ResourceURI string   `json:"resource_uri"`

	SpaceProviderId network.Id `json:"-"`
}

// ProviderId returns subnet.ID as network.Id.
func (subnet maasSubnet) ProviderId() network.Id {
	return network.Id(fmt.Sprint(subnet.ID))
}

// DNSServerAddresses returns subnet.DNSServers transformed to
// []network.Address.
func (subnet maasSubnet) DNSServerAddresses() []network.Address {
	results := make([]network.Address, len(subnet.DNSServers))
	for i, dnsServer := range subnet.DNSServers {
		address := network.NewAddressOnSpace(subnet.Space, dnsServer)
		address.SpaceProviderId = subnet.SpaceProviderId
		results[i] = address
	}

	return results
}

// GatewayAddress returns subnet.GatewayIP as network.Address.
func (subnet maasSubnet) GatewayAddress() network.Address {
	result := network.NewAddressOnSpace(subnet.Space, subnet.GatewayIP)
	result.SpaceProviderId = subnet.SpaceProviderId

	return result
}

// InterfaceInfo translates subnet to the equivalent network.InterfaceInfo.
func (subnet maasSubnet) InterfaceInfo() *network.InterfaceInfo {
	result := &network.InterfaceInfo{
		DeviceIndex:      -1, // avoid overriding top-level maasInterface.DeviceIndex.
		CIDR:             subnet.CIDR,
		ProviderSubnetId: subnet.ProviderId(),
		ProviderSpaceId:  subnet.SpaceProviderId,
		DNSServers:       subnet.DNSServerAddresses(),
		GatewayAddress:   subnet.GatewayAddress(),
	}
	result.Update(subnet.VLAN.InterfaceInfo())

	return result
}

func parseInterfaces(jsonBytes []byte) (maasInterfaces, error) {
	var interfaces maasInterfaces
	if err := json.Unmarshal(jsonBytes, &interfaces); err != nil {
		return nil, errors.Annotate(err, "parsing interfaces")
	}
	return interfaces, nil
}

// maasObjectNetworkInterfaces implements environs.NetworkInterfaces() using the
// MAAS 1.9 API, parsing the node details JSON embedded into the given
// maasObject to extract all the relevant InterfaceInfo fields. It returns an
// error satisfying errors.IsNotSupported() if it cannot find the required
// "interface_set" node details field. If the "pxe_mac"."mac_address" is also
// present, it will be used to determine the boot interface addresses and put
// them at the head of the results, as "most preferred" addresses. Finally, when
// the "hostname" is present, it will be put at index 0.
func maasObjectNetworkInterfaces(maasObject *gomaasapi.MAASObject, subnetsMap map[string]network.Id) ([]network.InterfaceInfo, error) {
	interfaceSet, ok := maasObject.GetMap()["interface_set"]
	if !ok || interfaceSet.IsNil() {
		return nil, errors.NotSupportedf("interface_set")
	}

	pxeMACAddress, err := extractMAASObjectPXEMACAddress(maasObject)
	if err != nil {
		logger.Warningf("missing pxe_mac.mac_address: %v", err)
	}

	hostname, err := extractMAASObjectHostname(maasObject)
	if err != nil {
		logger.Warningf("missing hostname: %v", err)
	}

	rawBytes, err := interfaceSet.MarshalJSON()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get interface_set JSON bytes")
	}

	interfaces, err := parseInterfaces(rawBytes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	interfaces.UpdateSpaceProviderIds(subnetsMap)
	bootInterface := interfaces.FirstTopLevelInterfaceByMACAddress(pxeMACAddress)

	var infos []network.InterfaceInfo
	nextDeviceIndex := 0
	if bootInterface != nil {
		// Add the boot interface addresses on top as preferred IP addresses.
		infos = bootInterface.InterfaceInfos(nextDeviceIndex)
		nextDeviceIndex++
	}

	for _, iface := range interfaces {
		if bootInterface != nil && iface.Name == bootInterface.Name {
			// Boot interface's infos already added before the loop.
			continue
		}
		infos = append(infos, iface.InterfaceInfos(nextDeviceIndex)...)
		nextDeviceIndex++
	}

	// hostname, if available, is considered the "most preferred" address of the
	// machine. MAAS creates hostnames that resolve to the boot interface's IP
	// addresses (usually one, but can be more with aliases). Since we already
	// placed the preferred addresses on top, here we can copy the first one and
	// transform it to a network.HostName.
	if hostname != "" {
		firstInfo := infos[0]
		spaceName := string(firstInfo.Address.SpaceName)
		spaceID := firstInfo.Address.SpaceProviderId
		firstInfo.Address = network.NewAddressOnSpace(spaceName, hostname)
		firstInfo.Address.SpaceProviderId = spaceID
		firstInfo.ProviderAddressId = "" // avoid duplicating the link ID.
		infos = append([]network.InterfaceInfo{firstInfo}, infos...)
	}

	return infos, nil
}

// extractMAASObjectPXEMACAddress extract the value of "pxe_mac"."mac_address"
// field from the given maasObject, or returns an error.
func extractMAASObjectPXEMACAddress(maasObject *gomaasapi.MAASObject) (string, error) {
	objectMap := maasObject.GetMap()
	pxeMAC, ok := objectMap["pxe_mac"]
	if !ok || pxeMAC.IsNil() {
		return "", errors.Errorf("missing or null pxe_mac field")
	}
	pxeMACMap, err := pxeMAC.GetMap()
	if err != nil {
		return "", errors.Annotatef(err, "getting pxe_mac map failed")
	}
	pxeMACAddressField, ok := pxeMACMap["mac_address"]
	if !ok || pxeMACAddressField.IsNil() {
		return "", errors.Errorf("missing or null pxe_mac.mac_address field")
	}
	pxeMACAddress, err := pxeMACAddressField.GetString()
	if err != nil {
		return "", errors.Annotatef(err, "getting pxe_mac.mac_address failed")
	}

	return pxeMACAddress, nil
}

// extractMAASObjectHostname extract the value of "hostname" field from the
// given maasObject, or returns an error.
func extractMAASObjectHostname(maasObject *gomaasapi.MAASObject) (string, error) {
	objectMap := maasObject.GetMap()
	hostnameField, ok := objectMap["hostname"]
	if !ok || hostnameField.IsNil() {
		return "", errors.Errorf("missing or null hostname field")
	}
	hostname, err := hostnameField.GetString()
	if err != nil {
		return "", errors.Annotatef(err, "getting hostname failed")
	}

	return hostname, nil
}

func maas2NetworkInterfaces(instance *maas2Instance, subnetsMap map[string]network.Id) ([]network.InterfaceInfo, error) {
	interfaces := instance.machine.InterfaceSet()
	infos := make([]network.InterfaceInfo, 0, len(interfaces))
	for i, iface := range interfaces {

		// The below works for all types except bonds and their members.
		parentName := strings.Join(iface.Parents(), "")
		var nicType network.InterfaceType
		switch maasInterfaceType(iface.Type()) {
		case typePhysical:
			nicType = network.EthernetInterface
			children := strings.Join(iface.Children(), "")
			if parentName == "" && len(iface.Children()) == 1 && strings.HasPrefix(children, "bond") {
				// FIXME: Verify the bond exists, regardless of its name.
				// This is a bond member, set the parent correctly (from
				// Juju's perspective) - to the bond itself.
				parentName = children
			}
		case typeBond:
			parentName = ""
			nicType = network.BondInterface
		case typeVLAN:
			nicType = network.VLAN_8021QInterface
		}

		vlanTag := 0
		if iface.VLAN() != nil {
			vlanTag = iface.VLAN().VID()
		}
		nicInfo := network.InterfaceInfo{
			DeviceIndex:         i,
			MACAddress:          iface.MACAddress(),
			ProviderId:          network.Id(fmt.Sprintf("%v", iface.ID())),
			VLANTag:             vlanTag,
			InterfaceName:       iface.Name(),
			InterfaceType:       nicType,
			ParentInterfaceName: parentName,
			Disabled:            !iface.Enabled(),
			NoAutoStart:         !iface.Enabled(),
		}

		for _, link := range iface.Links() {
			switch maasLinkMode(link.Mode()) {
			case modeUnknown:
				nicInfo.ConfigType = network.ConfigUnknown
			case modeDHCP:
				nicInfo.ConfigType = network.ConfigDHCP
			case modeStatic, modeLinkUp:
				nicInfo.ConfigType = network.ConfigStatic
			default:
				nicInfo.ConfigType = network.ConfigManual
			}

			if link.IPAddress() == "" {
				logger.Debugf("interface %q has no address", iface.Name())
			} else {
				// We set it here initially without a space, just so we don't
				// lose it when we have no linked subnet below.
				nicInfo.Address = network.NewAddress(link.IPAddress())
				nicInfo.ProviderAddressId = network.Id(fmt.Sprintf("%v", link.ID()))
			}

			if link.Subnet() == nil {
				logger.Debugf("interface %q link %d missing subnet", iface.Name(), link.ID())
				infos = append(infos, nicInfo)
				continue
			}

			sub := link.Subnet()
			nicInfo.CIDR = sub.CIDR()
			nicInfo.ProviderSubnetId = network.Id(fmt.Sprintf("%v", sub.ID()))
			nicInfo.ProviderVLANId = network.Id(fmt.Sprintf("%v", sub.VLAN().ID()))

			// Now we know the subnet and space, we can update the address to
			// store the space with it.
			nicInfo.Address = network.NewAddressOnSpace(sub.Space(), link.IPAddress())
			spaceId, ok := subnetsMap[string(sub.CIDR())]
			if !ok {
				// The space we found is not recognised, no
				// provider id available.
				logger.Warningf("interface %q link %d has unrecognised space %q", iface.Name(), link.ID(), sub.Space())
			} else {
				nicInfo.Address.SpaceProviderId = spaceId
				nicInfo.ProviderSpaceId = spaceId
			}

			gwAddr := network.NewAddressOnSpace(sub.Space(), sub.Gateway())
			nicInfo.DNSServers = network.NewAddressesOnSpace(sub.Space(), sub.DNSServers()...)
			if ok {
				gwAddr.SpaceProviderId = spaceId
				for i := range nicInfo.DNSServers {
					nicInfo.DNSServers[i].SpaceProviderId = spaceId
				}
			}
			nicInfo.GatewayAddress = gwAddr
			nicInfo.MTU = sub.VLAN().MTU()

			// Each link we represent as a separate InterfaceInfo, but with the
			// same name and device index, just different addres, subnet, etc.
			infos = append(infos, nicInfo)
		}
	}
	return infos, nil
}

// NetworkInterfaces implements Environ.NetworkInterfaces.
func (environ *maasEnviron) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	inst, err := environ.getInstance(instId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsMap, err := environ.subnetToSpaceIds()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if environ.usingMAAS2() {
		return maas2NetworkInterfaces(inst.(*maas2Instance), subnetsMap)
	} else {
		mi := inst.(*maas1Instance)
		return maasObjectNetworkInterfaces(mi.maasObject, subnetsMap)
	}
}
