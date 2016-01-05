// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type interfacesSuite struct {
	providerSuite
}

var _ = gc.Suite(&interfacesSuite{})

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

	const exampleJSON = `
    [
        {
            "name": "eth0", 
            "links": [
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
                    "ip_address": "10.20.19.103", 
                    "id": 436, 
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
                        "space": "public", 
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
        }
    ]`

	vlan0 := maasVLAN{
		ID:          5001,
		Name:        "untagged",
		VID:         0,
		MTU:         1500,
		Fabric:      "managed",
		ResourceURI: "/MAAS/api/1.0/vlans/5001/",
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

	expected := []maasInterface{{
		ID:          91,
		Name:        "eth0",
		Type:        "physical",
		Enabled:     true,
		MACAddress:  "52:54:00:70:9b:fe",
		VLAN:        vlan0,
		EffectveMTU: 1500,
		Links: []maasInterfaceLink{{
			ID: 436,
			Subnet: &maasSubnet{
				ID:          3,
				Name:        "pxe",
				Space:       "default",
				VLAN:        vlan0,
				GatewayIP:   "10.20.19.2",
				DNSServers:  []string{},
				CIDR:        "10.20.19.0/24",
				ResourceURI: "/MAAS/api/1.0/subnets/3/",
			},
			IPAddress: "10.20.19.103",
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
				Space:       "public",
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
	}}

	result, err := parseInterfaces([]byte(exampleJSON))
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
}
