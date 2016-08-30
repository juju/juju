// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

const maas2VersionResponse = `{"version": "unknown", "subversion": "", "capabilities": ["networks-management", "static-ipaddresses", "ipv6-deployment-ubuntu", "devices-management", "storage-deployment-ubuntu", "network-deployment-ubuntu"]}`

type baseProviderSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
	controllerUUID string
}

func (suite *baseProviderSuite) setupFakeTools(c *gc.C) {
	suite.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	toolsDir := filepath.Join(storageDir, "tools")
	suite.PatchValue(&envtools.DefaultBaseURL, utils.MakeFileURL(toolsDir))
	suite.UploadFakeToolsToDirectory(c, storageDir, "released", "released")
}

func (s *baseProviderSuite) SetUpSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpSuite(c)
	restoreTimeouts := envtesting.PatchAttemptStrategies(&shortAttempt)
	restoreFinishBootstrap := envtesting.DisableFinishBootstrap()
	s.AddCleanup(func(*gc.C) {
		restoreFinishBootstrap()
		restoreTimeouts()
	})
}

func (s *baseProviderSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(&series.HostSeries, func() string { return series.LatestLts() })
}

func (s *baseProviderSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func (s *baseProviderSuite) TearDownSuite(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.TearDownSuite(c)
}

type providerSuite struct {
	baseProviderSuite
	testMAASObject *gomaasapi.TestMAASObject
}

func spaceJSON(space gomaasapi.CreateSpace) *bytes.Buffer {
	var out bytes.Buffer
	err := json.NewEncoder(&out).Encode(space)
	if err != nil {
		panic(err)
	}
	return &out
}

func (s *providerSuite) SetUpSuite(c *gc.C) {
	s.baseProviderSuite.SetUpSuite(c)
	s.testMAASObject = gomaasapi.NewTestMAAS("1.0")
}

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	mockCapabilities := func(client *gomaasapi.MAASObject) (set.Strings, error) {
		return set.NewStrings("network-deployment-ubuntu"), nil
	}
	mockGetController := func(maasServer, apiKey string) (gomaasapi.Controller, error) {
		return nil, gomaasapi.NewUnsupportedVersionError("oops")
	}
	s.PatchValue(&GetCapabilities, mockCapabilities)
	s.PatchValue(&GetMAAS2Controller, mockGetController)
	// Creating a space ensures that the spaces endpoint won't 404.
	s.testMAASObject.TestServer.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-0"}))
}

func (s *providerSuite) TearDownTest(c *gc.C) {
	s.baseProviderSuite.TearDownTest(c)
	s.testMAASObject.TestServer.Clear()
}

func (s *providerSuite) TearDownSuite(c *gc.C) {
	s.baseProviderSuite.TearDownSuite(c)
	s.testMAASObject.Close()
}

var maasEnvAttrs = coretesting.Attrs{
	"name": "test-env",
	"type": "maas",
}

// makeEnviron creates a functional maasEnviron for a test.
func (suite *providerSuite) makeEnviron() *maasEnviron {
	cred := cloud.NewCredential(cloud.OAuth1AuthType, map[string]string{
		"maas-oauth": "a:b:c",
	})
	cloud := environs.CloudSpec{
		Type:       "maas",
		Name:       "maas",
		Endpoint:   suite.testMAASObject.TestServer.URL,
		Credential: &cred,
	}
	attrs := coretesting.FakeConfig().Merge(maasEnvAttrs)
	suite.controllerUUID = coretesting.FakeControllerConfig().ControllerUUID()
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(cloud, cfg)
	if err != nil {
		panic(err)
	}
	return env
}

func (suite *providerSuite) addNode(jsonText string) instance.Id {
	node := suite.testMAASObject.TestServer.NewNode(jsonText)
	resourceURI, _ := node.GetField("resource_uri")
	return instance.Id(resourceURI)
}

func (suite *providerSuite) getInstance(systemId string) *maas1Instance {
	input := fmt.Sprintf(`{"system_id": %q}`, systemId)
	node := suite.testMAASObject.TestServer.NewNode(input)
	statusGetter := func(instance.Id) (string, string) {
		return "unknown", "FAKE"
	}
	return &maas1Instance{&node, nil, statusGetter}
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
		CIDR:            fmt.Sprintf("192.168.%d.0/24", ipRange),
		ProviderId:      network.Id(strconv.Itoa(int(subnetID))),
		SpaceProviderId: network.Id(fmt.Sprintf("%d", spaceID)),
	}
}

func createSubnet(ipRange, spaceAndNICID uint) gomaasapi.CreateSubnet {
	space := fmt.Sprintf("space-%d", spaceAndNICID)
	return createSubnetWithSpace(ipRange, spaceAndNICID, space)
}

func createSubnetWithSpace(ipRange, NICID uint, space string) gomaasapi.CreateSubnet {
	var s gomaasapi.CreateSubnet
	s.DNSServers = []string{"192.168.1.2"}
	s.Name = fmt.Sprintf("maas-eth%d", NICID)
	s.Space = space
	s.GatewayIP = fmt.Sprintf("192.168.%v.1", ipRange)
	s.CIDR = fmt.Sprintf("192.168.%v.0/24", ipRange)
	return s
}

func (suite *providerSuite) addSubnet(c *gc.C, ipRange, spaceAndNICID uint, systemID string) uint {
	space := fmt.Sprintf("space-%d", spaceAndNICID)
	return suite.addSubnetWithSpace(c, ipRange, spaceAndNICID, space, systemID)
}

func (suite *providerSuite) addSubnetWithSpace(c *gc.C, ipRange, NICID uint, space string, systemID string) uint {
	out := bytes.Buffer{}
	err := json.NewEncoder(&out).Encode(createSubnetWithSpace(ipRange, NICID, space))
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
