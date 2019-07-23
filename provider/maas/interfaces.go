// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
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
	modeAuto    maasLinkMode = "auto"
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
	typeBridge   maasInterfaceType = "bridge"
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

// NetworkInterfaces implements Environ.NetworkInterfaces.
func (env *maasEnviron) NetworkInterfaces(ctx context.ProviderCallContext, instId instance.Id) ([]network.InterfaceInfo, error) {
	inst, err := env.getInstance(ctx, instId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if env.usingMAAS2() {
		dnsSearchDomains, err := env.Domains(ctx)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return maas2NetworkInterfaces(ctx, inst.(*maas2Instance), subnetsMap, dnsSearchDomains...)
	} else {
		mi := inst.(*maas1Instance)
		return maasObjectNetworkInterfaces(ctx, mi.maasObject, subnetsMap)
	}
}

// maasObjectNetworkInterfaces implements environs.NetworkInterfaces() using the
// new (1.9+) MAAS API, parsing the node details JSON embedded into the given
// maasObject to extract all the relevant InterfaceInfo fields. It returns an
// error satisfying errors.IsNotSupported() if it cannot find the required
// "interface_set" node details field.
func maasObjectNetworkInterfaces(
	_ context.ProviderCallContext, maasObject *gomaasapi.MAASObject, subnetsMap map[string]corenetwork.Id,
) ([]network.InterfaceInfo, error) {
	interfaceSet, ok := maasObject.GetMap()["interface_set"]
	if !ok || interfaceSet.IsNil() {
		// This means we're using an older MAAS API.
		return nil, errors.NotSupportedf("interface_set")
	}

	// TODO(dimitern): Change gomaasapi JSONObject to give access to the raw
	// JSON bytes directly, rather than having to do call MarshalJSON just so
	// the result can be unmarshaled from it.
	//
	// LKK Card: https://canonical.leankit.com/Boards/View/101652562/119311323

	rawBytes, err := interfaceSet.MarshalJSON()
	if err != nil {
		return nil, errors.Annotate(err, "cannot get interface_set JSON bytes")
	}

	interfaces, err := parseInterfaces(rawBytes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	infos := make([]network.InterfaceInfo, 0, len(interfaces))
	for i, iface := range interfaces {
		// The below works for all types except bonds and their members.
		parentName := strings.Join(iface.Parents, "")
		var nicType network.InterfaceType
		switch iface.Type {
		case typePhysical:
			nicType = network.EthernetInterface
			children := strings.Join(iface.Children, "")
			if parentName == "" && len(iface.Children) == 1 && strings.HasPrefix(children, "bond") {
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
		case typeBridge:
			nicType = network.BridgeInterface
		}

		nicInfo := network.InterfaceInfo{
			DeviceIndex:         i,
			MACAddress:          iface.MACAddress,
			ProviderId:          corenetwork.Id(fmt.Sprintf("%v", iface.ID)),
			VLANTag:             iface.VLAN.VID,
			InterfaceName:       iface.Name,
			InterfaceType:       nicType,
			ParentInterfaceName: parentName,
			Disabled:            !iface.Enabled,
			NoAutoStart:         !iface.Enabled,
		}

		if len(iface.Links) == 0 {
			logger.Debugf("interface %q has no links", iface.Name)
			infos = append(infos, nicInfo)
			continue
		}

		for _, link := range iface.Links {
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(string(link.Mode))

			if link.IPAddress == "" && link.Subnet == nil {
				logger.Debugf("interface %q link %d has neither subnet nor address", iface.Name, link.ID)
				infos = append(infos, nicInfo)
			} else {
				// We set it here initially without a space, just so we don't
				// lose it when we have no linked subnet below.
				nicInfo.Address = network.NewAddress(link.IPAddress)
				nicInfo.ProviderAddressId = corenetwork.Id(fmt.Sprintf("%v", link.ID))
			}

			sub := link.Subnet
			if sub == nil {
				logger.Debugf("interface %q link %d missing subnet", iface.Name, link.ID)
				infos = append(infos, nicInfo)
				continue
			}

			nicInfo.CIDR = sub.CIDR
			nicInfo.ProviderSubnetId = corenetwork.Id(fmt.Sprintf("%v", sub.ID))
			nicInfo.ProviderVLANId = corenetwork.Id(fmt.Sprintf("%v", sub.VLAN.ID))

			// Now we know the subnet and space, we can update the address to
			// store the space with it.
			nicInfo.Address = network.NewAddressOnSpace(sub.Space, link.IPAddress)
			spaceId, ok := subnetsMap[sub.CIDR]
			if !ok {
				// The space we found is not recognised.
				// No provider space info is available.
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
			// same name and device index, just different address, subnet, etc.
			infos = append(infos, nicInfo)
		}
	}
	return infos, nil
}

func maas2NetworkInterfaces(
	_ context.ProviderCallContext,
	instance *maas2Instance,
	subnetsMap map[string]corenetwork.Id,
	dnsSearchDomains ...string,
) ([]network.InterfaceInfo, error) {
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
		case typeBridge:
			nicType = network.BridgeInterface
		}

		vlanTag := 0
		if iface.VLAN() != nil {
			vlanTag = iface.VLAN().VID()
		}
		nicInfo := network.InterfaceInfo{
			DeviceIndex:         i,
			MACAddress:          iface.MACAddress(),
			ProviderId:          corenetwork.Id(fmt.Sprintf("%v", iface.ID())),
			VLANTag:             vlanTag,
			InterfaceName:       iface.Name(),
			InterfaceType:       nicType,
			ParentInterfaceName: parentName,
			Disabled:            !iface.Enabled(),
			NoAutoStart:         !iface.Enabled(),
		}

		if len(iface.Links()) == 0 {
			logger.Debugf("interface %q has no links", iface.Name())
			infos = append(infos, nicInfo)
			continue
		}

		for _, link := range iface.Links() {
			nicInfo.ConfigType = maasLinkToInterfaceConfigType(link.Mode())

			if link.IPAddress() == "" && link.Subnet() == nil {
				logger.Debugf("interface %q link %d has neither subnet nor address", iface.Name(), link.ID())
				infos = append(infos, nicInfo)
			} else {
				// We set it here initially without a space, just so we don't
				// lose it when we have no linked subnet below.
				nicInfo.Address = network.NewAddress(link.IPAddress())
				nicInfo.ProviderAddressId = corenetwork.Id(fmt.Sprintf("%v", link.ID()))
			}

			sub := link.Subnet()
			if sub == nil {
				logger.Debugf("interface %q link %d missing subnet", iface.Name(), link.ID())
				infos = append(infos, nicInfo)
				continue
			}

			nicInfo.CIDR = sub.CIDR()
			nicInfo.ProviderSubnetId = corenetwork.Id(fmt.Sprintf("%v", sub.ID()))
			nicInfo.ProviderVLANId = corenetwork.Id(fmt.Sprintf("%v", sub.VLAN().ID()))

			// Now we know the subnet and space, we can update the address to
			// store the space with it.
			nicInfo.Address = network.NewAddressOnSpace(sub.Space(), link.IPAddress())
			spaceId, ok := subnetsMap[sub.CIDR()]
			if !ok {
				// The space we found is not recognised.
				// No provider space info is available.
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
			nicInfo.DNSSearchDomains = dnsSearchDomains
			nicInfo.GatewayAddress = gwAddr
			nicInfo.MTU = sub.VLAN().MTU()

			// Each link we represent as a separate InterfaceInfo, but with the
			// same name and device index, just different address, subnet, etc.
			infos = append(infos, nicInfo)
		}
	}
	return infos, nil
}

func parseInterfaces(jsonBytes []byte) ([]maasInterface, error) {
	var interfaces []maasInterface
	if err := json.Unmarshal(jsonBytes, &interfaces); err != nil {
		return nil, errors.Annotate(err, "parsing interfaces")
	}
	return interfaces, nil
}

func maasLinkToInterfaceConfigType(mode string) network.InterfaceConfigType {
	switch maasLinkMode(mode) {
	case modeUnknown:
		return network.ConfigUnknown
	case modeDHCP:
		return network.ConfigDHCP
	case modeStatic, modeAuto:
		return network.ConfigStatic
	case modeLinkUp:
	default:
	}

	return network.ConfigManual
}
