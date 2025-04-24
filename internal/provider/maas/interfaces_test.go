// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"github.com/juju/gomaasapi/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type interfacesSuite struct {
	maasSuite
}

var _ = gc.Suite(&interfacesSuite{})

func newAddressOnSpaceWithId(
	space string, id network.Id, address string, options ...func(network.AddressMutator),
) network.ProviderAddress {
	newAddress := network.NewMachineAddress(address, options...).AsProviderAddress(network.WithSpaceName(space))
	newAddress.ProviderSpaceID = id
	return newAddress
}

const exampleInterfaceSetJSON = `
[
	{
		"name": "eth0",
		"links": [
			{
				"subnet": {
					"dns_servers": ["10.20.19.2", "10.20.19.3"],
					"name": "pxe",
					"space": "default",
					"vlan": {
						"name": "untagged",
						"vid": 0,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5001,
						"resource_uri": "/MAAS/api/1.0/vlans/5001/"
					},
					"gateway_ip": "10.20.19.2",
					"cidr": "10.20.19.0/24",
					"id": 3,
					"resource_uri": "/MAAS/api/1.0/subnets/3/"
				},
				"ip_address": "10.20.19.103",
				"id": 436,
				"mode": "static"
			},
			{
				"subnet": {
					"dns_servers": ["10.20.19.2", "10.20.19.3"],
					"name": "pxe",
					"space": "default",
					"vlan": {
						"name": "untagged",
						"vid": 0,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5001,
						"resource_uri": "/MAAS/api/1.0/vlans/5001/"
					},
					"gateway_ip": "10.20.19.2",
					"cidr": "10.20.19.0/24",
					"id": 3,
					"resource_uri": "/MAAS/api/1.0/subnets/3/"
				},
				"ip_address": "10.20.19.104",
				"id": 437,
				"mode": "static"
			}
		],
		"tags": [],
		"vlan": {
			"name": "untagged",
			"vid": 0,
			"mtu": 1500,
			"fabric": "managed",
			"id": 5001,
			"resource_uri": "/MAAS/api/1.0/vlans/5001/"
		},
		"enabled": true,
		"id": 91,
		"discovered": [
			{
				"subnet": {
					"dns_servers": [],
					"name": "pxe",
					"space": "default",
					"vlan": {
						"name": "untagged",
						"vid": 0,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5001,
						"resource_uri": "/MAAS/api/1.0/vlans/5001/"
					},
					"gateway_ip": "10.20.19.2",
					"cidr": "10.20.19.0/24",
					"id": 3,
					"resource_uri": "/MAAS/api/1.0/subnets/3/"
				},
				"ip_address": "10.20.19.20"
			}
		],
		"mac_address": "52:54:00:70:9b:fe",
		"parents": [],
		"effective_mtu": 1500,
		"params": {},
		"type": "physical",
		"children": [
			"eth0.100",
			"eth0.250",
			"eth0.50"
		],
		"resource_uri": "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/91/"
	},
	{
		"name": "eth0.50",
		"links": [
			{
				"subnet": {
					"dns_servers": [],
					"name": "admin",
					"space": "admin",
					"vlan": {
						"name": "admin",
						"vid": 50,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5004,
						"resource_uri": "/MAAS/api/1.0/vlans/5004/"
					},
					"gateway_ip": "10.50.19.2",
					"cidr": "10.50.19.0/24",
					"id": 5,
					"resource_uri": "/MAAS/api/1.0/subnets/5/"
				},
				"ip_address": "10.50.19.103",
				"id": 517,
				"mode": "static"
			}
		],
		"tags": [],
		"vlan": {
			"name": "admin",
			"vid": 50,
			"mtu": 1500,
			"fabric": "managed",
			"id": 5004,
			"resource_uri": "/MAAS/api/1.0/vlans/5004/"
		},
		"enabled": true,
		"id": 150,
		"discovered": null,
		"mac_address": "52:54:00:70:9b:fe",
		"parents": [
			"eth0"
		],
		"effective_mtu": 1500,
		"params": {},
		"type": "vlan",
		"children": [],
		"resource_uri": "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/150/"
	},
	{
		"name": "eth0.100",
		"links": [
			{
				"subnet": {
					"dns_servers": [],
					"name": "public",
					"space": "Public",
					"vlan": {
						"name": "public",
						"vid": 100,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5005,
						"resource_uri": "/MAAS/api/1.0/vlans/5005/"
					},
					"gateway_ip": "10.100.19.2",
					"cidr": "10.100.19.0/24",
					"id": 6,
					"resource_uri": "/MAAS/api/1.0/subnets/6/"
				},
				"ip_address": "10.100.19.103",
				"id": 519,
				"mode": "static"
			}
		],
		"tags": [],
		"vlan": {
			"name": "public",
			"vid": 100,
			"mtu": 1500,
			"fabric": "managed",
			"id": 5005,
			"resource_uri": "/MAAS/api/1.0/vlans/5005/"
		},
		"enabled": true,
		"id": 151,
		"discovered": null,
		"mac_address": "52:54:00:70:9b:fe",
		"parents": [
			"eth0"
		],
		"effective_mtu": 1500,
		"params": {},
		"type": "vlan",
		"children": [],
		"resource_uri": "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/151/"
	},
	{
		"name": "eth0.250",
		"links": [
			{
				"subnet": {
					"dns_servers": [],
					"name": "storage",
					"space": "storage",
					"vlan": {
						"name": "storage",
						"vid": 250,
						"mtu": 1500,
						"fabric": "managed",
						"id": 5008,
						"resource_uri": "/MAAS/api/1.0/vlans/5008/"
					},
					"gateway_ip": "10.250.19.2",
					"cidr": "10.250.19.0/24",
					"id": 8,
					"resource_uri": "/MAAS/api/1.0/subnets/8/"
				},
				"ip_address": "10.250.19.103",
				"id": 523,
				"mode": "static"
			}
		],
		"tags": [],
		"vlan": {
			"name": "storage",
			"vid": 250,
			"mtu": 1500,
			"fabric": "managed",
			"id": 5008,
			"resource_uri": "/MAAS/api/1.0/vlans/5008/"
		},
		"enabled": true,
		"id": 152,
		"discovered": null,
		"mac_address": "52:54:00:70:9b:fe",
		"parents": [
			"eth0"
		],
		"effective_mtu": 1500,
		"params": {},
		"type": "vlan",
		"children": [],
		"resource_uri": "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/152/"
	},
        {
                "name": "ens3",
                "parents": [],
                "children": [
                    "br-ens3"
                ],
                "vlan": {
                    "name": "untagged",
                    "fabric": "fabric-0",
                    "id": 5001,
                    "dhcp_on": true,
                    "vid": 0,
                    "primary_rack": "ed4rre",
                    "resource_uri": "/MAAS/api/2.0/vlans/5001/",
                    "mtu": 1500,
                    "secondary_rack": null,
                    "external_dhcp": null
                },
                "resource_uri": "/MAAS/api/2.0/nodes/hreab3/interfaces/10/",
                "effective_mtu": 1500,
                "mac_address": "52:54:00:08:24:2d",
                "discovered": [
                    {
                        "subnet": {
                            "name": "192.168.20.0/24",
                            "rdns_mode": 2,
                            "allow_proxy": true,
                            "gateway_ip": "192.168.20.2",
                            "active_discovery": true,
                            "vlan": {
                                "name": "untagged",
                                "fabric": "fabric-0",
                                "id": 5001,
                                "dhcp_on": true,
                                "vid": 0,
                                "primary_rack": "ed4rre",
                                "resource_uri": "/MAAS/api/2.0/vlans/5001/",
                                "mtu": 1500,
                                "secondary_rack": null,
                                "external_dhcp": null
                            },
                            "id": 4,
                            "resource_uri": "/MAAS/api/2.0/subnets/4/",
                            "space": "space-0",
                            "cidr": "192.168.20.0/24",
                            "dns_servers": []
                        },
                        "ip_address": "192.168.20.192"
                    }
                ],
                "links": [],
                "type": "physical",
                "id": 10,
                "tags": [],
                "enabled": true,
                "params": {}
        },
        {
                "name": "br-ens3",
                "parents": [
                    "ens3"
                ],
                "children": [],
                "vlan": {
                    "name": "untagged",
                    "fabric": "fabric-0",
                    "id": 5001,
                    "dhcp_on": true,
                    "vid": 0,
                    "primary_rack": "ed4rre",
                    "resource_uri": "/MAAS/api/2.0/vlans/5001/",
                    "mtu": 1500,
                    "secondary_rack": null,
                    "external_dhcp": null
                },
                "resource_uri": "/MAAS/api/2.0/nodes/hreab3/interfaces/30/",
                "effective_mtu": 1500,
                "mac_address": "52:54:00:08:24:2d",
                "discovered": [
                    {
                        "subnet": {
                            "name": "192.168.20.0/24",
                            "rdns_mode": 2,
                            "allow_proxy": true,
                            "gateway_ip": "192.168.20.2",
                            "active_discovery": true,
                            "vlan": {
                                "name": "untagged",
                                "fabric": "fabric-0",
                                "id": 5001,
                                "dhcp_on": true,
                                "vid": 0,
                                "primary_rack": "ed4rre",
                                "resource_uri": "/MAAS/api/2.0/vlans/5001/",
                                "mtu": 1500,
                                "secondary_rack": null,
                                "external_dhcp": null
                            },
                            "id": 4,
                            "resource_uri": "/MAAS/api/2.0/subnets/4/",
                            "space": "space-0",
                            "cidr": "192.168.20.0/24",
                            "dns_servers": []
                        },
                        "ip_address": "192.168.20.192"
                    }
                ],
                "links": [
                    {
                        "mode": "dhcp",
                        "id": 1931,
                        "ip_address": "192.168.20.192",
                        "subnet": {
                            "name": "192.168.20.0/24",
                            "rdns_mode": 2,
                            "allow_proxy": true,
                            "gateway_ip": "192.168.20.2",
                            "active_discovery": true,
                            "vlan": {
                                "name": "untagged",
                                "fabric": "fabric-0",
                                "id": 5001,
                                "dhcp_on": true,
                                "vid": 0,
                                "primary_rack": "ed4rre",
                                "resource_uri": "/MAAS/api/2.0/vlans/5001/",
                                "mtu": 1500,
                                "secondary_rack": null,
                                "external_dhcp": null
                            },
                            "id": 4,
                            "resource_uri": "/MAAS/api/2.0/subnets/4/",
                            "space": "space-0",
                            "cidr": "192.168.20.0/24",
                            "dns_servers": []
                        }
                    }
                ],
                "type": "bridge",
                "id": 30,
                "tags": [],
                "enabled": true,
                "params": {
                    "bridge_stp": false,
                    "bridge_fd": 15
                }
        }
]`

func (s *interfacesSuite) TestParseInterfacesNoJSON(c *gc.C) {
	result, err := parseInterfaces(nil)
	c.Check(err, gc.ErrorMatches, "parsing interfaces: unexpected end of JSON input")
	c.Check(result, gc.IsNil)
}

func (s *interfacesSuite) TestParseInterfacesBadJSON(c *gc.C) {
	result, err := parseInterfaces([]byte("$bad"))
	c.Check(err, gc.ErrorMatches, `parsing interfaces: invalid character '\$' .*`)
	c.Check(result, gc.IsNil)
}

func (s *interfacesSuite) TestParseInterfacesExampleJSON(c *gc.C) {

	vlan0 := maasVLAN{
		ID:          5001,
		Name:        "untagged",
		VID:         0,
		MTU:         1500,
		Fabric:      "managed",
		ResourceURI: "/MAAS/api/1.0/vlans/5001/",
	}

	vlan20 := maasVLAN{
		ID:          5001,
		Name:        "untagged",
		VID:         0,
		MTU:         1500,
		Fabric:      "fabric-0",
		ResourceURI: "/MAAS/api/2.0/vlans/5001/",
	}

	vlan50 := maasVLAN{
		ID:          5004,
		Name:        "admin",
		VID:         50,
		MTU:         1500,
		Fabric:      "managed",
		ResourceURI: "/MAAS/api/1.0/vlans/5004/",
	}

	vlan100 := maasVLAN{
		ID:          5005,
		Name:        "public",
		VID:         100,
		MTU:         1500,
		Fabric:      "managed",
		ResourceURI: "/MAAS/api/1.0/vlans/5005/",
	}

	vlan250 := maasVLAN{
		ID:          5008,
		Name:        "storage",
		VID:         250,
		MTU:         1500,
		Fabric:      "managed",
		ResourceURI: "/MAAS/api/1.0/vlans/5008/",
	}

	subnetPXE := maasSubnet{
		ID:          3,
		Name:        "pxe",
		Space:       "default",
		VLAN:        vlan0,
		GatewayIP:   "10.20.19.2",
		DNSServers:  []string{"10.20.19.2", "10.20.19.3"},
		CIDR:        "10.20.19.0/24",
		ResourceURI: "/MAAS/api/1.0/subnets/3/",
	}

	expected := []maasInterface{{
		ID:          91,
		Name:        "eth0",
		Type:        "physical",
		Enabled:     true,
		MACAddress:  "52:54:00:70:9b:fe",
		VLAN:        vlan0,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID:        436,
			Subnet:    &subnetPXE,
			IPAddress: "10.20.19.103",
			Mode:      "static",
		}, {
			ID:        437,
			Subnet:    &subnetPXE,
			IPAddress: "10.20.19.104",
			Mode:      "static",
		}},
		Parents:     []string{},
		Children:    []string{"eth0.100", "eth0.250", "eth0.50"},
		ResourceURI: "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/91/",
	}, {
		ID:          150,
		Name:        "eth0.50",
		Type:        "vlan",
		Enabled:     true,
		MACAddress:  "52:54:00:70:9b:fe",
		VLAN:        vlan50,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID: 517,
			Subnet: &maasSubnet{
				ID:          5,
				Name:        "admin",
				Space:       "admin",
				VLAN:        vlan50,
				GatewayIP:   "10.50.19.2",
				DNSServers:  []string{},
				CIDR:        "10.50.19.0/24",
				ResourceURI: "/MAAS/api/1.0/subnets/5/",
			},
			IPAddress: "10.50.19.103",
			Mode:      "static",
		}},
		Parents:     []string{"eth0"},
		Children:    []string{},
		ResourceURI: "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/150/",
	}, {
		ID:          151,
		Name:        "eth0.100",
		Type:        "vlan",
		Enabled:     true,
		MACAddress:  "52:54:00:70:9b:fe",
		VLAN:        vlan100,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID: 519,
			Subnet: &maasSubnet{
				ID:          6,
				Name:        "public",
				Space:       "Public",
				VLAN:        vlan100,
				GatewayIP:   "10.100.19.2",
				DNSServers:  []string{},
				CIDR:        "10.100.19.0/24",
				ResourceURI: "/MAAS/api/1.0/subnets/6/",
			},
			IPAddress: "10.100.19.103",
			Mode:      "static",
		}},
		Parents:     []string{"eth0"},
		Children:    []string{},
		ResourceURI: "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/151/",
	}, {
		ID:          152,
		Name:        "eth0.250",
		Type:        "vlan",
		Enabled:     true,
		MACAddress:  "52:54:00:70:9b:fe",
		VLAN:        vlan250,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID: 523,
			Subnet: &maasSubnet{
				ID:          8,
				Name:        "storage",
				Space:       "storage",
				VLAN:        vlan250,
				GatewayIP:   "10.250.19.2",
				DNSServers:  []string{},
				CIDR:        "10.250.19.0/24",
				ResourceURI: "/MAAS/api/1.0/subnets/8/",
			},
			IPAddress: "10.250.19.103",
			Mode:      "static",
		}},
		Parents:     []string{"eth0"},
		Children:    []string{},
		ResourceURI: "/MAAS/api/1.0/nodes/node-18489434-9eb0-11e5-bdef-00163e40c3b6/interfaces/152/",
	}, {
		ID:          10,
		Name:        "ens3",
		Type:        "physical",
		Enabled:     true,
		MACAddress:  "52:54:00:08:24:2d",
		VLAN:        vlan20,
		EffectveMTU: 1500,
		Links:       []maasInterfaceLink{},
		Parents:     []string{},
		Children:    []string{"br-ens3"},
		ResourceURI: "/MAAS/api/2.0/nodes/hreab3/interfaces/10/",
	}, {
		ID:          30,
		Name:        "br-ens3",
		Type:        "bridge",
		Enabled:     true,
		MACAddress:  "52:54:00:08:24:2d",
		VLAN:        vlan20,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID: 1931,
			Subnet: &maasSubnet{
				ID:          4,
				Name:        "192.168.20.0/24",
				Space:       "space-0",
				VLAN:        vlan20,
				GatewayIP:   "192.168.20.2",
				DNSServers:  []string{},
				CIDR:        "192.168.20.0/24",
				ResourceURI: "/MAAS/api/2.0/subnets/4/",
			},
			IPAddress: "192.168.20.192",
			Mode:      "dhcp",
		}},
		Parents:     []string{"ens3"},
		Children:    []string{},
		ResourceURI: "/MAAS/api/2.0/nodes/hreab3/interfaces/30/",
	}}

	result, err := parseInterfaces([]byte(exampleInterfaceSetJSON))
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
}

func (s *interfacesSuite) TestMAASNetworkInterfaces(c *gc.C) {
	vlan0 := fakeVLAN{
		id:  5001,
		vid: 0,
		mtu: 1500,
	}

	vlan50 := fakeVLAN{
		id:  5004,
		vid: 50,
		mtu: 1500,
	}

	vlan100 := fakeVLAN{
		id:  5005,
		vid: 100,
		mtu: 1500,
	}

	vlan250 := fakeVLAN{
		id:  5008,
		vid: 250,
		mtu: 1500,
	}

	subnetPXE := fakeSubnet{
		id:         3,
		space:      "Default",
		vlan:       vlan0,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}

	exampleInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan0,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
				&fakeLink{
					id:        437,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.104",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
		&fakeInterface{
			id:         150,
			name:       "eth0.50",
			type_:      "vlan",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan50,
			links: []gomaasapi.Link{
				&fakeLink{
					id: 517,
					subnet: &fakeSubnet{
						id:         5,
						space:      "admin",
						vlan:       vlan50,
						gateway:    "10.50.19.2",
						cidr:       "10.50.19.0/24",
						dnsServers: []string{},
					},
					ipAddress: "10.50.19.103",
					mode:      "static",
				},
			},
			parents:  []string{"eth0"},
			children: []string{},
		},
		&fakeInterface{
			id:         151,
			name:       "eth0.100",
			type_:      "vlan",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan100,
			links: []gomaasapi.Link{
				&fakeLink{
					id: 519,
					subnet: &fakeSubnet{
						id:         6,
						space:      "Public",
						vlan:       vlan100,
						gateway:    "10.100.19.2",
						cidr:       "10.100.19.0/24",
						dnsServers: []string{},
					},
					ipAddress: "10.100.19.103",
					mode:      "static",
				},
			},
			parents:  []string{"eth0"},
			children: []string{},
		},
		&fakeInterface{
			id:         152,
			name:       "eth0.250",
			type_:      "vlan",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       vlan250,
			links: []gomaasapi.Link{
				&fakeLink{
					id: 523,
					subnet: &fakeSubnet{
						id:         8,
						space:      "storage",
						vlan:       vlan250,
						gateway:    "10.250.19.2",
						cidr:       "10.250.19.0/24",
						dnsServers: []string{},
					},
					ipAddress: "10.250.19.103",
					mode:      "static",
				},
			},
			parents:  []string{"eth0"},
			children: []string{},
		},
		&fakeInterface{
			id:         92,
			name:       "eth1",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:ff",
			vlan:       vlan0,
			links: []gomaasapi.Link{
				&fakeLink{
					id:        436,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.103",
					mode:      "static",
				},
				&fakeLink{
					id:        437,
					subnet:    &subnetPXE,
					ipAddress: "10.20.19.104",
					mode:      "static",
				},
			},
			parents:  []string{},
			children: []string{"irregular_bond0"},
		},
		&fakeInterface{
			id:         101,
			name:       "irregular_bond0",
			type_:      "bond",
			enabled:    true,
			macAddress: "52:54:00:70:9b:ff",
			vlan:       vlan0,
			links:      []gomaasapi.Link{},
			parents:    []string{"eth1"},
			children:   []string{},
		},
	}

	subnetsMap := make(map[string]network.Id)
	subnetsMap["10.250.19.0/24"] = "3"
	subnetsMap["192.168.1.0/24"] = "0"

	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "436",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.103", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers:       []string{"10.20.19.2", "10.20.19.3"},
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "437",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.104", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers:       []string{"10.20.19.2", "10.20.19.3"},
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         1,
		MACAddress:          "52:54:00:70:9b:fe",
		ProviderId:          "150",
		ProviderSubnetId:    "5",
		VLANTag:             50,
		ProviderVLANId:      "5004",
		ProviderAddressId:   "517",
		InterfaceName:       "eth0.50",
		ParentInterfaceName: "eth0",
		InterfaceType:       "802.1q",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.50.19.103", network.WithCIDR("10.50.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("admin")),
		},
		DNSServers:       nil,
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.50.19.2").AsProviderAddress(network.WithSpaceName("admin")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         2,
		MACAddress:          "52:54:00:70:9b:fe",
		ProviderId:          "151",
		ProviderSubnetId:    "6",
		VLANTag:             100,
		ProviderVLANId:      "5005",
		ProviderAddressId:   "519",
		InterfaceName:       "eth0.100",
		ParentInterfaceName: "eth0",
		InterfaceType:       "802.1q",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.100.19.103", network.WithCIDR("10.100.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("public")),
		},
		DNSServers:       nil,
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.100.19.2").AsProviderAddress(network.WithSpaceName("public")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         3,
		MACAddress:          "52:54:00:70:9b:fe",
		ProviderId:          "152",
		ProviderSubnetId:    "8",
		VLANTag:             250,
		ProviderVLANId:      "5008",
		ProviderAddressId:   "523",
		ProviderSpaceId:     "3",
		InterfaceName:       "eth0.250",
		ParentInterfaceName: "eth0",
		InterfaceType:       "802.1q",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{newAddressOnSpaceWithId(
			"storage", "3", "10.250.19.103", network.WithCIDR("10.250.19.0/24"), network.WithConfigType(network.ConfigStatic),
		)},
		DNSServers:       nil,
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   newAddressOnSpaceWithId("storage", "3", "10.250.19.2"),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         4,
		MACAddress:          "52:54:00:70:9b:ff",
		ProviderId:          "92",
		ProviderSubnetId:    "3",
		VLANTag:             0,
		ProviderVLANId:      "5001",
		ProviderAddressId:   "436",
		InterfaceName:       "eth1",
		ParentInterfaceName: "irregular_bond0",
		InterfaceType:       "ethernet",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.103", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers:       []string{"10.20.19.2", "10.20.19.3"},
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:         4,
		MACAddress:          "52:54:00:70:9b:ff",
		ProviderId:          "92",
		ProviderSubnetId:    "3",
		VLANTag:             0,
		ProviderVLANId:      "5001",
		ProviderAddressId:   "437",
		InterfaceName:       "eth1",
		ParentInterfaceName: "irregular_bond0",
		InterfaceType:       "ethernet",
		Disabled:            false,
		NoAutoStart:         false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.104", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers:       []string{"10.20.19.2", "10.20.19.3"},
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}, {
		DeviceIndex:       5,
		MACAddress:        "52:54:00:70:9b:ff",
		ProviderId:        "101",
		ProviderSubnetId:  "",
		VLANTag:           0,
		ProviderVLANId:    "",
		ProviderAddressId: "",
		InterfaceName:     "irregular_bond0",
		InterfaceType:     "bond",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses:         nil,
		DNSServers:        nil,
		DNSSearchDomains:  nil,
		MTU:               0,
		Origin:            network.OriginProvider,
	}}
	machine := &fakeMachine{interfaceSet: exampleInterfaces}
	instance := &maasInstance{machine: machine}

	infos, err := maasNetworkInterfaces(s.callCtx, instance, subnetsMap)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(infos, jc.DeepEquals, expected)
}

func (s *interfacesSuite) TestMAASInterfacesNilVLAN(c *gc.C) {
	vlan0 := fakeVLAN{
		id:  5001,
		vid: 0,
		mtu: 1500,
	}

	subnetPXE := fakeSubnet{
		id:         3,
		space:      "default",
		vlan:       vlan0,
		gateway:    "10.20.19.2",
		cidr:       "10.20.19.0/24",
		dnsServers: []string{"10.20.19.2", "10.20.19.3"},
	}

	exampleInterfaces := []gomaasapi.Interface{
		&fakeInterface{
			id:         91,
			name:       "eth0",
			type_:      "physical",
			enabled:    true,
			macAddress: "52:54:00:70:9b:fe",
			vlan:       nil,
			links: []gomaasapi.Link{&fakeLink{
				id:        436,
				subnet:    &subnetPXE,
				ipAddress: "10.20.19.103",
				mode:      "static",
			}},
			parents:  []string{},
			children: []string{"eth0.100", "eth0.250", "eth0.50"},
		},
	}
	machine := &fakeMachine{interfaceSet: exampleInterfaces}
	instance := &maasInstance{machine: machine}

	expected := network.InterfaceInfos{{
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		VLANTag:           0,
		ProviderVLANId:    "5001",
		ProviderAddressId: "436",
		InterfaceName:     "eth0",
		InterfaceType:     "ethernet",
		Disabled:          false,
		NoAutoStart:       false,
		Addresses: network.ProviderAddresses{
			network.NewMachineAddress(
				"10.20.19.103", network.WithCIDR("10.20.19.0/24"), network.WithConfigType(network.ConfigStatic),
			).AsProviderAddress(network.WithSpaceName("default")),
		},
		DNSServers:       []string{"10.20.19.2", "10.20.19.3"},
		DNSSearchDomains: nil,
		MTU:              1500,
		GatewayAddress:   network.NewMachineAddress("10.20.19.2").AsProviderAddress(network.WithSpaceName("default")),
		Origin:           network.OriginProvider,
	}}

	infos, err := maasNetworkInterfaces(s.callCtx, instance, map[string]network.Id{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(infos, jc.DeepEquals, expected)
}
