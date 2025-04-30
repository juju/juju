// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

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
func (env *maasEnviron) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]corenetwork.InterfaceInfos, error) {
	switch len(ids) {
	case 0:
		return nil, environs.ErrNoInstances
	case 1: // short-cut
		ifList, err := env.networkInterfacesForInstance(ctx, ids[0])
		if err != nil {
			return nil, err
		}
		return []corenetwork.InterfaceInfos{ifList}, nil
	}

	// Fetch instance information for the IDs we are interested in.
	insts, err := env.Instances(ctx, ids)
	partialInfo := err == environs.ErrPartialInstances
	if err != nil && err != environs.ErrPartialInstances {
		if err == environs.ErrNoInstances {
			return nil, err
		}
		return nil, errors.Trace(err)
	}

	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	infos := make([]corenetwork.InterfaceInfos, len(ids))
	dnsSearchDomains, err := env.Domains(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for idx, inst := range insts {
		if inst == nil {
			continue // unknown instance ID
		}

		ifList, err := maasNetworkInterfaces(ctx, inst.(*maasInstance), subnetsMap, dnsSearchDomains...)
		if err != nil {
			return nil, errors.Annotatef(err, "obtaining network interfaces for instance %v", ids[idx])
		}
		infos[idx] = ifList
	}

	if partialInfo {
		err = environs.ErrPartialInstances
	}
	return infos, err
}

func (env *maasEnviron) networkInterfacesForInstance(ctx context.Context, instId instance.Id) (corenetwork.InterfaceInfos, error) {
	inst, err := env.getInstance(ctx, instId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnetsMap, err := env.subnetToSpaceIds(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	dnsSearchDomains, err := env.Domains(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return maasNetworkInterfaces(ctx, inst.(*maasInstance), subnetsMap, dnsSearchDomains...)
}

func maasNetworkInterfaces(
	ctx context.Context,
	instance *maasInstance,
	subnetsMap map[string]corenetwork.Id,
	dnsSearchDomains ...string,
) (corenetwork.InterfaceInfos, error) {
	interfaces := instance.machine.InterfaceSet()
	infos := make(corenetwork.InterfaceInfos, 0, len(interfaces))
	bonds := set.NewStrings()
	for _, iface := range interfaces {
		if maasInterfaceType(iface.Type()) == typeBond {
			bonds.Add(iface.Name())
		}
	}
	for i, iface := range interfaces {

		// The below works for all types except bonds and their members.
		parentName := strings.Join(iface.Parents(), "")
		var nicType corenetwork.LinkLayerDeviceType
		switch maasInterfaceType(iface.Type()) {
		case typePhysical:
			nicType = corenetwork.EthernetDevice
			children := strings.Join(iface.Children(), "")
			if parentName == "" && len(iface.Children()) == 1 && bonds.Contains(children) {
				parentName = children
			}
		case typeBond:
			parentName = ""
			nicType = corenetwork.BondDevice
		case typeVLAN:
			nicType = corenetwork.VLAN8021QDevice
		case typeBridge:
			nicType = corenetwork.BridgeDevice
		}

		vlanTag := 0
		if iface.VLAN() != nil {
			vlanTag = iface.VLAN().VID()
		}
		nicInfo := corenetwork.InterfaceInfo{
			DeviceIndex:         i,
			MACAddress:          iface.MACAddress(),
			ProviderId:          corenetwork.Id(fmt.Sprintf("%v", iface.ID())),
			VLANTag:             vlanTag,
			InterfaceName:       iface.Name(),
			InterfaceType:       nicType,
			ParentInterfaceName: parentName,
			Disabled:            !iface.Enabled(),
			NoAutoStart:         !iface.Enabled(),
			Origin:              corenetwork.OriginProvider,
		}

		if len(iface.Links()) == 0 {
			logger.Debugf(ctx, "interface %q has no links", iface.Name())
			infos = append(infos, nicInfo)
			continue
		}

		for _, link := range iface.Links() {
			configType := maasLinkToInterfaceConfigType(link.Mode())

			if link.IPAddress() == "" && link.Subnet() == nil {
				logger.Debugf(ctx, "interface %q link %d has neither subnet nor address", iface.Name(), link.ID())
				infos = append(infos, nicInfo)
			} else {
				// We set it here initially without a space, just so we don't
				// lose it when we have no linked subnet below.
				//
				// NOTE(achilleasa): the original code used a last-write-wins
				// policy. Do we need to append link addresses to the list?
				nicInfo.Addresses = corenetwork.ProviderAddresses{
					corenetwork.NewMachineAddress(link.IPAddress(), corenetwork.WithConfigType(configType)).AsProviderAddress(),
				}
				nicInfo.ProviderAddressId = corenetwork.Id(fmt.Sprintf("%v", link.ID()))
			}

			sub := link.Subnet()
			if sub == nil {
				logger.Debugf(ctx, "interface %q link %d missing subnet", iface.Name(), link.ID())
				infos = append(infos, nicInfo)
				continue
			}

			nicInfo.ProviderSubnetId = corenetwork.Id(fmt.Sprintf("%v", sub.ID()))
			nicInfo.ProviderVLANId = corenetwork.Id(fmt.Sprintf("%v", sub.VLAN().ID()))

			// Provider addresses are created with a space name massaged
			// to conform to Juju's space name rules.
			space := corenetwork.ConvertSpaceName(sub.Space(), nil)

			// Now we know the subnet and space, we can update the address to
			// store the space with it.
			nicInfo.Addresses[0] = corenetwork.NewMachineAddress(
				link.IPAddress(), corenetwork.WithCIDR(sub.CIDR()), corenetwork.WithConfigType(configType),
			).AsProviderAddress(corenetwork.WithSpaceName(space))

			spaceId, ok := subnetsMap[sub.CIDR()]
			if !ok {
				// The space we found is not recognised.
				// No provider space info is available.
				logger.Warningf(ctx, "interface %q link %d has unrecognised space %q", iface.Name(), link.ID(), sub.Space())
			} else {
				nicInfo.Addresses[0].ProviderSpaceID = spaceId
				nicInfo.ProviderSpaceId = spaceId
			}

			gwAddr := corenetwork.NewMachineAddress(sub.Gateway()).AsProviderAddress(corenetwork.WithSpaceName(space))
			nicInfo.DNSServers = sub.DNSServers()
			if ok {
				gwAddr.ProviderSpaceID = spaceId
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

func maasLinkToInterfaceConfigType(mode string) corenetwork.AddressConfigType {
	switch maasLinkMode(mode) {
	case modeUnknown:
		return corenetwork.ConfigUnknown
	case modeDHCP:
		return corenetwork.ConfigDHCP
	case modeStatic, modeAuto:
		return corenetwork.ConfigStatic
	case modeLinkUp:
	default:
	}

	return corenetwork.ConfigManual
}
