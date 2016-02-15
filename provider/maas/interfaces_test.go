// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"fmt"
	"text/template"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/network"
)

////////////////////////////////////////////////////////////////////////////////
// New (1.9 and later) environs.NetworkInterfaces() implementation tests follow.

type interfacesSuite struct {
	providerSuite
}

var _ = gc.Suite(&interfacesSuite{})

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

	result, err := parseInterfaces([]byte(exampleInterfaceSetJSON))
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, expected)
}

func (s *interfacesSuite) TestMAASObjectNetworkInterfaces(c *gc.C) {
	nodeJSON := fmt.Sprintf(`{
        "system_id": "foo",
        "interface_set": %s
    }`, exampleInterfaceSetJSON)
	obj := s.testMAASObject.TestServer.NewNode(nodeJSON)

	expected := []network.InterfaceInfo{{
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.20.19.0/24",
		NetworkName:       "juju-private",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		AvailabilityZones: nil,
		VLANTag:           0,
		InterfaceName:     "eth0",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("default", "10.20.19.103"),
		DNSServers:        network.NewAddressesOnSpace("default", "10.20.19.2", "10.20.19.3"),
		DNSSearch:         "",
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("default", "10.20.19.2"),
		ExtraConfig:       nil,
	}, {
		DeviceIndex:       0,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.20.19.0/24",
		NetworkName:       "juju-private",
		ProviderId:        "91",
		ProviderSubnetId:  "3",
		AvailabilityZones: nil,
		VLANTag:           0,
		InterfaceName:     "eth0",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("default", "10.20.19.104"),
		DNSServers:        network.NewAddressesOnSpace("default", "10.20.19.2", "10.20.19.3"),
		DNSSearch:         "",
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("default", "10.20.19.2"),
		ExtraConfig:       nil,
	}, {
		DeviceIndex:       1,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.50.19.0/24",
		NetworkName:       "juju-private",
		ProviderId:        "150",
		ProviderSubnetId:  "5",
		AvailabilityZones: nil,
		VLANTag:           50,
		InterfaceName:     "eth0.50",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("admin", "10.50.19.103"),
		DNSServers:        nil,
		DNSSearch:         "",
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("admin", "10.50.19.2"),
		ExtraConfig:       nil,
	}, {
		DeviceIndex:       2,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.100.19.0/24",
		NetworkName:       "juju-private",
		ProviderId:        "151",
		ProviderSubnetId:  "6",
		AvailabilityZones: nil,
		VLANTag:           100,
		InterfaceName:     "eth0.100",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("public", "10.100.19.103"),
		DNSServers:        nil,
		DNSSearch:         "",
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("public", "10.100.19.2"),
		ExtraConfig:       nil,
	}, {
		DeviceIndex:       3,
		MACAddress:        "52:54:00:70:9b:fe",
		CIDR:              "10.250.19.0/24",
		NetworkName:       "juju-private",
		ProviderId:        "152",
		ProviderSubnetId:  "8",
		AvailabilityZones: nil,
		VLANTag:           250,
		InterfaceName:     "eth0.250",
		Disabled:          false,
		NoAutoStart:       false,
		ConfigType:        "static",
		Address:           network.NewAddressOnSpace("storage", "10.250.19.103"),
		DNSServers:        nil,
		DNSSearch:         "",
		MTU:               1500,
		GatewayAddress:    network.NewAddressOnSpace("storage", "10.250.19.2"),
		ExtraConfig:       nil,
	}}

	infos, err := maasObjectNetworkInterfaces(&obj)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(infos, jc.DeepEquals, expected)
}

////////////////////////////////////////////////////////////////////////////////
// Legacy (pre 1.9) environs.NetworkInterfaces() implementation tests follow.

const lshwXMLTemplate = `
<?xml version="1.0" standalone="yes" ?>
<!-- generated by lshw-B.02.16 -->
<list>
<node id="node1" claimed="true" class="system" handle="DMI:0001">
 <description>Computer</description>
 <product>VirtualBox ()</product>
 <width units="bits">64</width>
  <node id="core" claimed="true" class="bus" handle="DMI:0008">
   <description>Motherboard</description>
    <node id="pci" claimed="true" class="bridge" handle="PCIBUS:0000:00">
     <description>Host bridge</description>{{$list := .}}{{range $mac, $ifi := $list}}
      <node id="network{{if gt (len $list) 1}}:{{$ifi.DeviceIndex}}{{end}}"{{if $ifi.Disabled}} disabled="true"{{end}} claimed="true" class="network" handle="PCI:0000:00:03.0">
       <description>Ethernet interface</description>
       <product>82540EM Gigabit Ethernet Controller</product>
       <logicalname>{{$ifi.InterfaceName}}</logicalname>
       <serial>{{$mac}}</serial>
      </node>{{end}}
    </node>
  </node>
</node>
</list>
`

func (suite *environSuite) generateHWTemplate(netMacs map[string]ifaceInfo) (string, error) {
	tmpl, err := template.New("test").Parse(lshwXMLTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, netMacs)
	if err != nil {
		return "", err
	}
	return string(buf.Bytes()), nil
}

func (suite *environSuite) TestGetNetworkMACs(c *gc.C) {
	env := suite.makeEnviron()

	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node_1"}`)
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node_2"}`)
	suite.testMAASObject.TestServer.NewNetwork(
		`{"name": "net_1","ip":"0.1.2.0","netmask":"255.255.255.0"}`,
	)
	suite.testMAASObject.TestServer.NewNetwork(
		`{"name": "net_2","ip":"0.2.2.0","netmask":"255.255.255.0"}`,
	)
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node_2", "net_2", "aa:bb:cc:dd:ee:22")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node_1", "net_1", "aa:bb:cc:dd:ee:11")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node_2", "net_1", "aa:bb:cc:dd:ee:21")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node_1", "net_2", "aa:bb:cc:dd:ee:12")

	networks, err := env.getNetworkMACs("net_1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(networks, jc.SameContents, []string{"aa:bb:cc:dd:ee:11", "aa:bb:cc:dd:ee:21"})

	networks, err = env.getNetworkMACs("net_2")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(networks, jc.SameContents, []string{"aa:bb:cc:dd:ee:12", "aa:bb:cc:dd:ee:22"})

	networks, err = env.getNetworkMACs("net_3")
	c.Check(networks, gc.HasLen, 0)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestGetInstanceNetworks(c *gc.C) {
	suite.newNetwork("test_network", 123, 321, "null")
	testInstance := suite.getInstance("instance_for_network")
	suite.testMAASObject.TestServer.ConnectNodeToNetwork("instance_for_network", "test_network")
	networks, err := suite.makeEnviron().getInstanceNetworks(testInstance)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(networks, gc.DeepEquals, []networkDetails{{
		Name:           "test_network",
		IP:             "192.168.123.2",
		Mask:           "255.255.255.0",
		VLANTag:        321,
		Description:    "test_network_123_321",
		DefaultGateway: "", // "null" and "" are treated as N/A.
	},
	})
}

// A typical lshw XML dump with lots of things left out.
const lshwXMLTestExtractInterfaces = `
<?xml version="1.0" standalone="yes" ?>
<!-- generated by lshw-B.02.16 -->
<list>
<node id="machine" claimed="true" class="system" handle="DMI:0001">
 <description>Notebook</description>
 <product>MyMachine</product>
 <version>1.0</version>
 <width units="bits">64</width>
  <node id="core" claimed="true" class="bus" handle="DMI:0002">
   <description>Motherboard</description>
    <node id="cpu" claimed="true" class="processor" handle="DMI:0004">
     <description>CPU</description>
      <node id="pci:2" claimed="true" class="bridge" handle="PCIBUS:0000:03">
        <node id="network:0" claimed="true" disabled="true" class="network" handle="PCI:0000:03:00.0">
         <logicalname>wlan0</logicalname>
         <serial>aa:bb:cc:dd:ee:ff</serial>
        </node>
        <node id="network:1" claimed="true" class="network" handle="PCI:0000:04:00.0">
         <logicalname>eth0</logicalname>
         <serial>aa:bb:cc:dd:ee:f1</serial>
        </node>
      </node>
    </node>
  </node>
  <node id="network:2" claimed="true" class="network" handle="">
   <logicalname>vnet1</logicalname>
   <serial>aa:bb:cc:dd:ee:f2</serial>
  </node>
</node>
</list>
`

// An lshw XML dump with implicit network interface indexes.
const lshwXMLTestExtractInterfacesImplicitIndexes = `
<?xml version="1.0" standalone="yes" ?>
<!-- generated by lshw-B.02.16 -->
<list>
<node id="machine" claimed="true" class="system" handle="DMI:0001">
 <description>Notebook</description>
 <product>MyMachine</product>
 <version>1.0</version>
 <width units="bits">64</width>
  <node id="core" claimed="true" class="bus" handle="DMI:0002">
   <description>Motherboard</description>
    <node id="cpu" claimed="true" class="processor" handle="DMI:0004">
     <description>CPU</description>
      <node id="pci:2" claimed="true" class="bridge" handle="PCIBUS:0000:03">
        <node id="network" claimed="true" disabled="true" class="network" handle="PCI:0000:03:00.0">
         <logicalname>wlan0</logicalname>
         <serial>aa:bb:cc:dd:ee:ff</serial>
        </node>
        <node id="network" claimed="true" class="network" handle="PCI:0000:04:00.0">
         <logicalname>eth0</logicalname>
         <serial>aa:bb:cc:dd:ee:f1</serial>
        </node>
      </node>
    </node>
  </node>
  <node id="network" claimed="true" class="network" handle="">
   <logicalname>vnet1</logicalname>
   <serial>aa:bb:cc:dd:ee:f2</serial>
  </node>
</node>
</list>
`

func (suite *environSuite) TestExtractInterfaces(c *gc.C) {
	rawData := []string{
		lshwXMLTestExtractInterfaces,
		lshwXMLTestExtractInterfacesImplicitIndexes,
	}
	for _, data := range rawData {
		inst := suite.getInstance("testInstance")
		interfaces, err := extractInterfaces(inst, []byte(data))
		c.Assert(err, jc.ErrorIsNil)
		c.Check(interfaces, jc.DeepEquals, map[string]ifaceInfo{
			"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
			"aa:bb:cc:dd:ee:f1": {1, "eth0", false},
			"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
		})
	}
}

func (suite *environSuite) TestGetInstanceNetworkInterfaces(c *gc.C) {
	inst := suite.getInstance("testInstance")
	templateInterfaces := map[string]ifaceInfo{
		"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
		"aa:bb:cc:dd:ee:f1": {1, "eth0", true},
		"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
	}
	lshwXML, err := suite.generateHWTemplate(templateInterfaces)
	c.Assert(err, jc.ErrorIsNil)

	suite.testMAASObject.TestServer.AddNodeDetails("testInstance", lshwXML)
	env := suite.makeEnviron()
	interfaces, err := env.getInstanceNetworkInterfaces(inst)
	c.Assert(err, jc.ErrorIsNil)
	// Both wlan0 and eth0 are disabled in lshw output.
	c.Check(interfaces, jc.DeepEquals, templateInterfaces)
}

func (suite *environSuite) TestSetupNetworks(c *gc.C) {
	testInstance := suite.getInstance("node1")
	templateInterfaces := map[string]ifaceInfo{
		"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
		"aa:bb:cc:dd:ee:f1": {1, "eth0", true},
		"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
	}
	lshwXML, err := suite.generateHWTemplate(templateInterfaces)
	c.Assert(err, jc.ErrorIsNil)

	suite.testMAASObject.TestServer.AddNodeDetails("node1", lshwXML)
	suite.newNetwork("LAN", 2, 42, "null")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "LAN", "aa:bb:cc:dd:ee:f1")
	suite.newNetwork("Virt", 3, 0, "0.1.2.3") // primary + gateway
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "Virt", "aa:bb:cc:dd:ee:f2")
	suite.newNetwork("WLAN", 1, 0, "") // "" same as "null" for gateway
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "WLAN", "aa:bb:cc:dd:ee:ff")
	networkInfo, err := suite.makeEnviron().setupNetworks(testInstance)
	c.Assert(err, jc.ErrorIsNil)

	// Note: order of networks is based on lshwXML
	// Unfortunately, because network.InterfaceInfo is unhashable
	// (contains a map) we can't use jc.SameContents here.
	c.Check(networkInfo, gc.HasLen, 3)
	for _, info := range networkInfo {
		switch info.DeviceIndex {
		case 0:
			c.Check(info, jc.DeepEquals, network.InterfaceInfo{
				MACAddress:    "aa:bb:cc:dd:ee:ff",
				CIDR:          "192.168.1.2/24",
				NetworkName:   "WLAN",
				ProviderId:    "WLAN",
				VLANTag:       0,
				DeviceIndex:   0,
				InterfaceName: "wlan0",
				Disabled:      true, // from networksToDisable("WLAN")
			})
		case 1:
			c.Check(info, jc.DeepEquals, network.InterfaceInfo{
				DeviceIndex:   1,
				MACAddress:    "aa:bb:cc:dd:ee:f1",
				CIDR:          "192.168.2.2/24",
				NetworkName:   "LAN",
				ProviderId:    "LAN",
				VLANTag:       42,
				InterfaceName: "eth0",
				Disabled:      true, // from networksToDisable("WLAN")
			})
		case 2:
			c.Check(info, jc.DeepEquals, network.InterfaceInfo{
				MACAddress:     "aa:bb:cc:dd:ee:f2",
				CIDR:           "192.168.3.2/24",
				NetworkName:    "Virt",
				ProviderId:     "Virt",
				VLANTag:        0,
				DeviceIndex:    2,
				InterfaceName:  "vnet1",
				Disabled:       false,
				GatewayAddress: network.NewAddress("0.1.2.3"), // from newNetwork("Virt", 3, 0, "0.1.2.3")
			})
		}
	}
}

// The same test, but now "Virt" network does not have matched MAC address
func (suite *environSuite) TestSetupNetworksPartialMatch(c *gc.C) {
	testInstance := suite.getInstance("node1")
	templateInterfaces := map[string]ifaceInfo{
		"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
		"aa:bb:cc:dd:ee:f1": {1, "eth0", false},
		"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
	}
	lshwXML, err := suite.generateHWTemplate(templateInterfaces)
	c.Assert(err, jc.ErrorIsNil)

	suite.testMAASObject.TestServer.AddNodeDetails("node1", lshwXML)
	suite.newNetwork("LAN", 2, 42, "192.168.2.1")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "LAN", "aa:bb:cc:dd:ee:f1")
	suite.newNetwork("Virt", 3, 0, "")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "Virt", "aa:bb:cc:dd:ee:f3")
	networkInfo, err := suite.makeEnviron().setupNetworks(testInstance)
	c.Assert(err, jc.ErrorIsNil)

	// Note: order of networks is based on lshwXML
	c.Check(networkInfo, jc.DeepEquals, []network.InterfaceInfo{{
		MACAddress:     "aa:bb:cc:dd:ee:f1",
		CIDR:           "192.168.2.2/24",
		NetworkName:    "LAN",
		ProviderId:     "LAN",
		VLANTag:        42,
		DeviceIndex:    1,
		InterfaceName:  "eth0",
		Disabled:       false,
		GatewayAddress: network.NewAddress("192.168.2.1"),
	}})
}

// The same test, but now no networks have matched MAC
func (suite *environSuite) TestSetupNetworksNoMatch(c *gc.C) {
	testInstance := suite.getInstance("node1")
	templateInterfaces := map[string]ifaceInfo{
		"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
		"aa:bb:cc:dd:ee:f1": {1, "eth0", false},
		"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
	}
	lshwXML, err := suite.generateHWTemplate(templateInterfaces)
	c.Assert(err, jc.ErrorIsNil)

	suite.testMAASObject.TestServer.AddNodeDetails("node1", lshwXML)
	suite.newNetwork("Virt", 3, 0, "")
	suite.testMAASObject.TestServer.ConnectNodeToNetworkWithMACAddress("node1", "Virt", "aa:bb:cc:dd:ee:f3")
	networkInfo, err := suite.makeEnviron().setupNetworks(testInstance)
	c.Assert(err, jc.ErrorIsNil)

	// Note: order of networks is based on lshwXML
	c.Check(networkInfo, gc.HasLen, 0)
}

func (suite *environSuite) TestNetworkInterfacesLegacy(c *gc.C) {
	testInstance := suite.createSubnets(c, false)

	netInfo, err := suite.makeEnviron().NetworkInterfaces(testInstance.Id())
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := []network.InterfaceInfo{{
		DeviceIndex:      0,
		MACAddress:       "aa:bb:cc:dd:ee:ff",
		CIDR:             "192.168.1.2/24",
		ProviderSubnetId: "WLAN",
		VLANTag:          0,
		InterfaceName:    "wlan0",
		Disabled:         true,
		NoAutoStart:      true,
		ConfigType:       network.ConfigDHCP,
		ExtraConfig:      nil,
		GatewayAddress:   network.Address{},
		Address:          network.NewScopedAddress("192.168.1.2", network.ScopeCloudLocal),
	}, {
		DeviceIndex:      1,
		MACAddress:       "aa:bb:cc:dd:ee:f1",
		CIDR:             "192.168.2.2/24",
		ProviderSubnetId: "LAN",
		VLANTag:          42,
		InterfaceName:    "eth0",
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       network.ConfigDHCP,
		ExtraConfig:      nil,
		GatewayAddress:   network.NewScopedAddress("192.168.2.1", network.ScopeCloudLocal),
		Address:          network.NewScopedAddress("192.168.2.2", network.ScopeCloudLocal),
	}, {
		DeviceIndex:      2,
		MACAddress:       "aa:bb:cc:dd:ee:f2",
		CIDR:             "192.168.3.2/24",
		ProviderSubnetId: "Virt",
		VLANTag:          0,
		InterfaceName:    "vnet1",
		Disabled:         false,
		NoAutoStart:      false,
		ConfigType:       network.ConfigDHCP,
		ExtraConfig:      nil,
		GatewayAddress:   network.Address{},
		Address:          network.NewScopedAddress("192.168.3.2", network.ScopeCloudLocal),
	}}
	network.SortInterfaceInfo(netInfo)
	c.Assert(netInfo, jc.DeepEquals, expectedInfo)
}
