// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
)

type providerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	testMAASObject *gomaasapi.TestMAASObject
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	restoreTimeouts := envtesting.PatchAttemptStrategies(&shortAttempt)
	TestMAASObject := gomaasapi.NewTestMAAS("1.0")
	s.testMAASObject = TestMAASObject
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddSuiteCleanup(func(*gc.C) {
		restoreFinishBootstrap()
		restoreTimeouts()
	})
	s.PatchValue(&nodeDeploymentTimeout, func(*maasEnviron) time.Duration {
		return coretesting.ShortWait
	})
	s.PatchValue(&resolveHostnames, func(addrs []network.Address) []network.Address {
		return addrs
	})
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&version.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&series.HostSeries, func() string { return coretesting.FakeDefaultSeries })
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.SetFeatureFlags(feature.AddressAllocation)
	s.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.testMAASObject.TestServer.Clear()
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.testMAASObject.Close()
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

const exampleAgentName = "dfb69555-0bc4-4d1f-85f2-4ee390974984"

var maasEnvAttrs = coretesting.Attrs{
	"name":            "test env",
	"type":            "maas",
	"maas-oauth":      "a:b:c",
	"maas-agent-name": exampleAgentName,
}

// makeEnviron creates a functional maasEnviron for a test.
func (suite *providerSuite) makeEnviron() *maasEnviron {
	testAttrs := coretesting.Attrs{}
	for k, v := range maasEnvAttrs {
		testAttrs[k] = v
	}
	testAttrs["maas-server"] = suite.testMAASObject.TestServer.URL
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cfg)
	if err != nil {
		panic(err)
	}
	return env
}

func (suite *providerSuite) setupFakeTools(c *gc.C) {
	storageDir := c.MkDir()
	suite.PatchValue(&envtools.DefaultBaseURL, "file://"+storageDir+"/tools")
	suite.UploadFakeToolsToDirectory(c, storageDir, "released", "released")
}

func (suite *providerSuite) addNode(jsonText string) instance.Id {
	node := suite.testMAASObject.TestServer.NewNode(jsonText)
	resourceURI, _ := node.GetField("resource_uri")
	return instance.Id(resourceURI)
}

func (suite *providerSuite) getInstance(systemId string) *maasInstance {
	input := fmt.Sprintf(`{"system_id": %q}`, systemId)
	node := suite.testMAASObject.TestServer.NewNode(input)
	return &maasInstance{&node}
}

func (suite *providerSuite) getNetwork(name string, id int, vlanTag int) *gomaasapi.MAASObject {
	var vlan string
	if vlanTag == 0 {
		vlan = "null"
	} else {
		vlan = fmt.Sprintf("%d", vlanTag)
	}
	var input string
	input = fmt.Sprintf(`{"name": %q, "ip":"192.168.%d.1", "netmask": "255.255.255.0",`+
		`"vlan_tag": %s, "description": "%s_%d_%d" }`, name, id, vlan, name, id, vlanTag)
	network := suite.testMAASObject.TestServer.NewNetwork(input)
	return &network
}

func createSubnetInfo(subnetID, spaceID, ipRange uint) network.SubnetInfo {
	return network.SubnetInfo{
		CIDR:              fmt.Sprintf("192.168.%d.0/24", ipRange),
		ProviderId:        network.Id(strconv.Itoa(int(subnetID))),
		AllocatableIPLow:  net.ParseIP(fmt.Sprintf("192.168.%d.139", ipRange)).To4(),
		AllocatableIPHigh: net.ParseIP(fmt.Sprintf("192.168.%d.255", ipRange)).To4(),
		SpaceProviderId:   network.Id(fmt.Sprintf("Space %d", spaceID)),
	}
}

func createSubnet(ipRange, spaceAndNICID uint) gomaasapi.CreateSubnet {
	var s gomaasapi.CreateSubnet
	s.DNSServers = []string{"192.168.1.2"}
	s.Name = fmt.Sprintf("maas-eth%d", spaceAndNICID)
	s.Space = fmt.Sprintf("Space %d", spaceAndNICID)
	s.GatewayIP = fmt.Sprintf("192.168.%v.1", ipRange)
	s.CIDR = fmt.Sprintf("192.168.%v.0/24", ipRange)
	return s
}

func (suite *providerSuite) addSubnet(c *gc.C, ipRange, spaceAndNICID uint, systemID string) uint {
	out := bytes.Buffer{}
	err := json.NewEncoder(&out).Encode(createSubnet(ipRange, spaceAndNICID))
	c.Assert(err, jc.ErrorIsNil)
	subnet := suite.testMAASObject.TestServer.NewSubnet(&out)
	c.Assert(err, jc.ErrorIsNil)

	other := gomaasapi.AddressRange{}
	other.Start = fmt.Sprintf("192.168.%d.139", ipRange)
	other.End = fmt.Sprintf("192.168.%d.149", ipRange)
	other.Purpose = []string{"not-the-dynamic-range"}
	suite.testMAASObject.TestServer.AddFixedAddressRange(subnet.ID, other)

	ar := gomaasapi.AddressRange{}
	ar.Start = fmt.Sprintf("192.168.%d.10", ipRange)
	ar.End = fmt.Sprintf("192.168.%d.138", ipRange)
	ar.Purpose = []string{"something", "dynamic-range"}
	suite.testMAASObject.TestServer.AddFixedAddressRange(subnet.ID, ar)
	if systemID != "" {
		var nni gomaasapi.NodeNetworkInterface
		nni.Name = subnet.Name
		nni.Links = append(nni.Links, gomaasapi.NetworkLink{
			ID:     uint(1),
			Mode:   "auto",
			Subnet: subnet,
		})
		suite.testMAASObject.TestServer.SetNodeNetworkLink(systemID, nni)
	}
	return subnet.ID
}
