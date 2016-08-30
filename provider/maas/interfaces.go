// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/utils/set"

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

type maasInterfaceLink struct {
	ID        int          `json:"id"`
	Subnet    *maasSubnet  `json:"subnet,omitempty"`
	IPAddress string       `json:"ip_address,omitempty"`
	Mode      maasLinkMode `json:"mode"`
}

type maasInterfaceType string

const (
	typeUnknown  maasInterfaceType = ""
	typePhysical maasInterfaceType = "physical"
	typeVLAN     maasInterfaceType = "vlan"
	typeBond     maasInterfaceType = "bond"
)

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

type maasVLAN struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	VID         int    `json:"vid"`
	MTU         int    `json:"mtu"`
	Fabric      string `json:"fabric"`
	ResourceURI string `json:"resource_uri"`
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
}

func (m maasInterface) GoString() string {
	return fmt.Sprintf(`"%s(%s)"`, m.Name, m.Type)
}

func parseInterfaces(jsonBytes []byte) ([]maasInterface, error) {
	var interfaces []maasInterface
	if err := json.Unmarshal(jsonBytes, &interfaces); err != nil {
		return nil, errors.Annotate(err, "parsing interfaces")
	}
	return interfaces, nil
}

type byTypeThenName struct {
	// Embedded so we sort a copy of the original.
	interfaces []maasInterface
}

func (b byTypeThenName) Len() int { return len(b.interfaces) }

func (b byTypeThenName) Swap(i, j int) {
	b.interfaces[i], b.interfaces[j] = b.interfaces[j], b.interfaces[i]
}

func (b byTypeThenName) Less(i, j int) bool {
	first, second := b.interfaces[i], b.interfaces[j]
	switch first.Type {
	case second.Type:
		// Same types sort by name, but eth0.50 comes before eth0.100.
		if first.Type == typeVLAN && first.Parents[0] == second.Parents[0] {
			return first.VLAN.VID < second.VLAN.VID
		}
		return first.Name < second.Name
	case typeBond:
		// Bonds come on top, before physical and VLAN..
		return second.Type == typePhysical || second.Type == typeVLAN
	case typePhysical:
		// Physical come before VLANs, but after bonds.
		return second.Type == typeVLAN || second.Type != typeBond
	}

	// VLANs always come last.
	return first.Type != typeVLAN
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

	pxeMACAddress, hostname, err := maasObjectPXEMACAddressAndHostname(maasObject)
	if err != nil {
		logger.Warningf("missing pxe_mac.mac_address and/or hostname: %v", err)
	}

	rawBytes, err := interfaceSet.MarshalJSON()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get interface_set JSON bytes")
	}

	interfaces, err := parseInterfaces(rawBytes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// In order to correctly match parent-to-child relationships, sort the list
	// by type (bond, physical, vlan) then name, so parents appear before their
	// children. Using embedded slice to preserve the original interfaces order.
	ordered := &byTypeThenName{interfaces: interfaces}
	sort.Sort(ordered)

	bonds := set.NewStrings()
	infos := make([]network.InterfaceInfo, 0, len(ordered.interfaces))
	var bootInterface maasInterface
	for i, iface := range ordered.interfaces {

		var nicType network.InterfaceType
		parentName := ""
		switch iface.Type {
		case typeBond:
			nicType = network.BondInterface
			bonds.Add(iface.Name)
		case typePhysical:
			nicType = network.EthernetInterface
			// Single child can mean either iface is a bond slave, or it has a
			// VLAN child. iface.Parents is not as useful, because MAAS models
			// bonds as children to their slaves.
			if len(iface.Children) == 1 && bonds.Contains(iface.Children[0]) {
				parentName = iface.Children[0]
			}
		case typeVLAN:
			nicType = network.VLAN_8021QInterface
			if len(iface.Parents) == 1 {
				parentName = iface.Parents[0]
			} else {
				// Shouldn't happen, but log it for easier debugging.
				logger.Debugf("VLAN interface %q - expected one parent, got: %v", iface.Name, iface.Parents)
			}
		}

		matchesPXEMAC := pxeMACAddress == iface.MACAddress && pxeMACAddress != ""
		isTopLevel := parentName == ""
		if bootInterface.Name == "" && isTopLevel && matchesPXEMAC {
			// Only top-level physical interfaces can be used for PXE booting,
			// and once we find it, we should stick with it.
			logger.Infof("boot interface is %q", iface.Name)
			bootInterface = iface
		}

		nicInfo := network.InterfaceInfo{
			DeviceIndex:         i,
			MACAddress:          iface.MACAddress,
			ProviderId:          network.Id(fmt.Sprintf("%v", iface.ID)),
			VLANTag:             iface.VLAN.VID,
			InterfaceName:       iface.Name,
			ParentInterfaceName: parentName,
			InterfaceType:       nicType,
			Disabled:            !iface.Enabled,
			NoAutoStart:         !iface.Enabled,
		}

		for _, link := range iface.Links {
			switch link.Mode {
			case modeUnknown:
				nicInfo.ConfigType = network.ConfigUnknown
			case modeDHCP:
				nicInfo.ConfigType = network.ConfigDHCP
			case modeStatic, modeLinkUp:
				nicInfo.ConfigType = network.ConfigStatic
			default:
				nicInfo.ConfigType = network.ConfigManual
			}

			if link.IPAddress == "" {
				logger.Debugf("interface %q has no address", iface.Name)
			} else {
				// We set it here initially without a space, just so we don't
				// lose it when we have no linked subnet below.
				nicInfo.Address = network.NewAddress(link.IPAddress)
				nicInfo.ProviderAddressId = network.Id(fmt.Sprintf("%v", link.ID))
			}

			if link.Subnet == nil {
				logger.Debugf("interface %q link %d missing subnet", iface.Name, link.ID)
				infos = append(infos, nicInfo)
				continue
			}

			sub := link.Subnet
			nicInfo.CIDR = sub.CIDR
			nicInfo.ProviderSubnetId = network.Id(fmt.Sprintf("%v", sub.ID))
			nicInfo.ProviderVLANId = network.Id(fmt.Sprintf("%v", sub.VLAN.ID))

			// Now we know the subnet and space, we can update the address to
			// store the space with it.
			nicInfo.Address = network.NewAddressOnSpace(sub.Space, link.IPAddress)
			spaceId, ok := subnetsMap[string(sub.CIDR)]
			if !ok {
				// The space we found is not recognised, no
				// provider id available.
				logger.Warningf("interface %q link %d has unrecognised space %q", iface.Name, link.ID, sub.Space)
			} else {
				nicInfo.Address.SpaceProviderId = spaceId
				nicInfo.ProviderSpaceId = spaceId
			}

			gwAddr := network.NewAddressOnSpace(sub.Space, sub.GatewayIP)
			nicInfo.DNSServers = network.NewAddressesOnSpace(sub.Space, sub.DNSServers...)
			if ok {
				gwAddr.SpaceProviderId = spaceId
				for i := range nicInfo.DNSServers {
					nicInfo.DNSServers[i].SpaceProviderId = spaceId
				}
			}
			nicInfo.GatewayAddress = gwAddr
			nicInfo.MTU = sub.VLAN.MTU

			// Each link we represent as a separate InterfaceInfo, but with the
			// same name and device index, just different addres, subnet, etc.
			infos = append(infos, nicInfo)
		}
	}

	// hostname when available will be put on top of the list.
	if hostname != "" {
		firstInfo := infos[0]
		spaceName := string(firstInfo.Address.SpaceName)
		firstInfo.Address = network.NewAddressOnSpace(spaceName, hostname)
		firstInfo.ProviderAddressId = "" // avoid duplicating the link ID.
		infos = append([]network.InterfaceInfo{firstInfo}, infos...)
	}

	if bootInterface.Name == "" {
		// Could not find the boot interface, no need to reorder results.
		return infos, nil
	}

	bootInterfaceAddresses := make(map[network.Address]int)
	for _, link := range bootInterface.Links {
		if link.IPAddress != "" {
			addr := network.NewAddress(link.IPAddress)
			bootInterfaceAddresses[addr] = len(bootInterfaceAddresses)
		}
	}
	if hostname != "" {
		// Hostname is the "most preferred" address.
		bootInterfaceAddresses[infos[0].Address] = -1
	}

	// Reorder the result to put bootInterfaceAddresses, in order, at the top.
	results := &byPreferedAddresses{
		addressesOrder: bootInterfaceAddresses,
		infos:          infos,
	}
	sort.Sort(results)

	return results.infos, nil
}

// maasObjectPXEMACAddressAndHostname extract the values of pxe_mac.mac_address
// and hostname fields from the given maasObject, or returns an error.
func maasObjectPXEMACAddressAndHostname(maasObject *gomaasapi.MAASObject) (pxeMACAddress, hostname string, _ error) {
	// Get the PXE MAC address so we can put the top-level physical NIC with
	// that MAC on top of the list, as it matches the preferred "private"
	// address of the node.
	maasObjectMap := maasObject.GetMap()
	pxeMAC, ok := maasObjectMap["pxe_mac"]
	if !ok || pxeMAC.IsNil() {
		return "", "", errors.Errorf("missing or null pxe_mac field")
	}
	pxeMACMap, err := pxeMAC.GetMap()
	if err != nil {
		return "", "", errors.Annotatef(err, "getting pxe_mac map failed")
	}
	pxeMACAddressField, ok := pxeMACMap["mac_address"]
	if !ok || pxeMACAddressField.IsNil() {
		return "", "", errors.Errorf("missing or null pxe_mac.mac_address field")
	}
	pxeMACAddress, err = pxeMACAddressField.GetString()
	if err != nil {
		return "", "", errors.Annotatef(err, "getting pxe_mac.mac_address failed")
	}

	hostnameField, ok := maasObjectMap["hostname"]
	if !ok || hostnameField.IsNil() {
		return "", "", errors.Errorf("missing or null hostname field")
	}
	hostname, err = hostnameField.GetString()
	if err != nil {
		return "", "", errors.Annotatef(err, "getting hostname failed")
	}

	return pxeMACAddress, hostname, nil
}

type byPreferedAddresses struct {
	addressesOrder map[network.Address]int
	infos          []network.InterfaceInfo
}

func (b byPreferedAddresses) Len() int { return len(b.infos) }

func (b byPreferedAddresses) Swap(i, j int) {
	b.infos[i], b.infos[j] = b.infos[j], b.infos[i]
}

func (b byPreferedAddresses) Less(i, j int) bool {
	first, second := b.infos[i], b.infos[j]
	firstOrder, firstPreferred := b.addressesOrder[first.Address]
	secondOrder, secondPreferred := b.addressesOrder[second.Address]

	switch {
	case firstPreferred && secondPreferred:
		// Both are preferred, use given order.
		return firstOrder < secondOrder
	case firstPreferred:
		return true
	case secondPreferred:
		return false
	case first.VLANTag != second.VLANTag:
		// Sort VLAN NICs' addresses after those from untagged VLANs, e.g.
		// addresses from "eth0.50" after "eth0", and "eth1.100" after
		// "eth1.20".
		return first.VLANTag < second.VLANTag
	}

	// Neither is preferred, order by Value.
	return first.Address.Value < second.Address.Value
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
