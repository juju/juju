// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/json"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type maasLinkMode string

const (
	modeUnknown maasLinkMode = ""
	modeAuto    maasLinkMode = "auto"
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

func parseInterfaces(jsonBytes []byte) ([]maasInterface, error) {
	var interfaces []maasInterface
	if err := json.Unmarshal(jsonBytes, &interfaces); err != nil {
		return nil, errors.Annotate(err, "parsing interfaces")
	}
	return interfaces, nil
}

func maasInterfacesToInterfaceInfo(interfaces []maasInterface) []network.InterfaceInfo {
	infos := make([]network.InterfaceInfo, len(interfaces))
	for i, iface := range interfaces {
		nicInfo := network.InterfaceInfo{
			DeviceIndex:   i,
			MACAddress:    iface.MACAddress,
			ProviderId:    network.Id(fmt.Sprintf("%v", iface.ID)),
			VLANTag:       iface.VLAN.VID,
			InterfaceName: iface.Name,
			Disabled:      !iface.Enabled,
			NoAutoStart:   !iface.Enabled,
			// TODO(dimitern): Drop this in a follow-up - without it
			// provisioning fails as it's validated.
			NetworkName: network.DefaultPrivate,
		}

		if len(iface.Links) < 1 {
			logger.Warningf("interface %q not linked to any subnets", iface.Name)
			infos[i] = nicInfo
			continue
		}

		// TODO(dimitern): For now we ignore all but the first link.
		link := iface.Links[0]
		switch link.Mode {
		case modeLinkUp:
			nicInfo.ConfigType = network.ConfigManual
		case modeDHCP:
			nicInfo.ConfigType = network.ConfigDHCP
		case modeStatic, modeAuto:
			nicInfo.ConfigType = network.ConfigStatic
		default:
			nicInfo.ConfigType = network.ConfigUnknown
		}

		if link.IPAddress != "" {
			ipAddr := network.NewScopedAddress(link.IPAddress, network.ScopeCloudLocal)
			nicInfo.Address = ipAddr
		} else {
			logger.Warningf("interface %q has no address", iface.Name)
		}

		if link.Subnet == nil {
			logger.Warningf("interface %q link %d missing subnet", iface.Name, link.ID)
			infos[i] = nicInfo
			continue
		}

		sub := link.Subnet
		nicInfo.CIDR = sub.CIDR
		if sub.GatewayIP != "" {
			gwAddr := network.NewScopedAddress(sub.GatewayIP, network.ScopeCloudLocal)
			nicInfo.GatewayAddress = gwAddr
		}
		nicInfo.ProviderSubnetId = network.Id(fmt.Sprintf("%v", sub.ID))
		nicInfo.DNSServers = network.NewAddresses(sub.DNSServers...)
		// TODO: DNSSearch (get from get-curtin-config?), MTU, parent/child
		// relationships will be nice..
		infos[i] = nicInfo
	}
	return infos
}

// NetworkInterfaces implements Environ.NetworkInterfaces.
func (environ *maasEnviron) NetworkInterfaces(instId instance.Id) ([]network.InterfaceInfo, error) {
	if !environ.supportsNetworkDeploymentUbuntu {
		return environ.legacyNetworkInterfaces(instId)
	}

	inst, err := environ.getInstance(instId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return inst.(*maasInstance).interfaces()
}
