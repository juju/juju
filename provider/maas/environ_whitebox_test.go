// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type environSuite struct {
	providerSuite
}

const (
	allocatedNode = `{"system_id": "test-allocated"}`
)

var _ = gc.Suite(&environSuite{})

// getTestConfig creates a customized sample MAAS provider configuration.
func getTestConfig(name, server, oauth, secret string) *config.Config {
	ecfg, err := newConfig(map[string]interface{}{
		"name":            name,
		"maas-server":     server,
		"maas-oauth":      oauth,
		"admin-secret":    secret,
		"authorized-keys": "I-am-not-a-real-key",
	})
	if err != nil {
		panic(err)
	}
	return ecfg.Config
}

func (suite *environSuite) setupFakeTools(c *gc.C) {
	suite.PatchValue(&simplestreams.SimplestreamsJujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	suite.PatchValue(&envtools.DefaultBaseURL, "file://"+storageDir+"/tools")
	suite.UploadFakeToolsToDirectory(c, storageDir, "released", "released")
}

func (suite *environSuite) addNode(jsonText string) instance.Id {
	node := suite.testMAASObject.TestServer.NewNode(jsonText)
	resourceURI, _ := node.GetField("resource_uri")
	return instance.Id(resourceURI)
}

func (suite *environSuite) TestInstancesReturnsInstances(c *gc.C) {
	id := suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances([]instance.Id{id})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Id(), gc.Equals, id)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfEmptyParameter(c *gc.C) {
	suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances([]instance.Id{})

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNilParameter(c *gc.C) {
	suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances(nil)

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoneFound(c *gc.C) {
	instances, err := suite.makeEnviron().Instances([]instance.Id{"unknown"})
	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestAllInstances(c *gc.C) {
	id := suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().AllInstances()

	c.Check(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Id(), gc.Equals, id)
}

func (suite *environSuite) TestAllInstancesReturnsEmptySliceIfNoInstance(c *gc.C) {
	instances, err := suite.makeEnviron().AllInstances()

	c.Check(err, jc.ErrorIsNil)
	c.Check(instances, gc.HasLen, 0)
}

func (suite *environSuite) TestInstancesReturnsErrorIfPartialInstances(c *gc.C) {
	known := suite.addNode(allocatedNode)
	suite.addNode(`{"system_id": "test2"}`)
	unknown := instance.Id("unknown systemID")
	instances, err := suite.makeEnviron().Instances([]instance.Id{known, unknown})

	c.Check(err, gc.Equals, environs.ErrPartialInstances)
	c.Assert(instances, gc.HasLen, 2)
	c.Check(instances[0].Id(), gc.Equals, known)
	c.Check(instances[1], gc.IsNil)
}

func (suite *environSuite) TestStorageReturnsStorage(c *gc.C) {
	env := suite.makeEnviron()
	stor := env.Storage()
	c.Check(stor, gc.NotNil)
	// The Storage object is really a maasStorage.
	specificStorage := stor.(*maasStorage)
	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environUnlocked, gc.Equals, env)
}

func decodeUserData(userData string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(userData)
	if err != nil {
		return []byte(""), err
	}
	return utils.Gunzip(data)
}

func (suite *environSuite) TestStartInstanceStartsInstance(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	// Create node 0: it will be used as the bootstrap node.
	suite.testMAASObject.TestServer.NewNode(fmt.Sprintf(
		`{"system_id": "node0", "hostname": "host0", "architecture": "%s/generic", "memory": 1024, "cpu_count": 1}`,
		arch.HostArch()),
	)
	lshwXML, err := suite.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f0": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASObject.TestServer.AddNodeDetails("node0", lshwXML)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	// The bootstrap node has been acquired and started.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Check(found, jc.IsTrue)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// Test the instance id is correctly recorded for the bootstrap node.
	// Check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)
	insts, err := env.AllInstances()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// Create node 1: it will be used as instance number 1.
	suite.testMAASObject.TestServer.NewNode(fmt.Sprintf(
		`{"system_id": "node1", "hostname": "host1", "architecture": "%s/generic", "memory": 1024, "cpu_count": 1}`,
		arch.HostArch()),
	)
	lshwXML, err = suite.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f1": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASObject.TestServer.AddNodeDetails("node1", lshwXML)
	instance, hc := testing.AssertStartInstance(c, env, "1")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instance, gc.NotNil)
	c.Assert(hc, gc.NotNil)
	c.Check(hc.String(), gc.Equals, fmt.Sprintf("arch=%s cpu-cores=1 mem=1024M", arch.HostArch()))

	// The instance number 1 has been acquired and started.
	actions, found = operations["node1"]
	c.Assert(found, jc.IsTrue)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// The value of the "user data" parameter used when starting the node
	// contains the run cmd used to write the machine information onto
	// the node's filesystem.
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node1"]
	c.Assert(found, jc.IsTrue)
	c.Assert(len(nodeRequestValues), gc.Equals, 2)
	userData := nodeRequestValues[1].Get("user_data")
	decodedUserData, err := decodeUserData(userData)
	c.Assert(err, jc.ErrorIsNil)
	info := machineInfo{"host1"}
	cloudcfg, err := cloudinit.New("precise")
	c.Assert(err, jc.ErrorIsNil)
	cloudinitRunCmd, err := info.cloudinitRunCmd(cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	data, err := goyaml.Marshal(cloudinitRunCmd)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(string(decodedUserData), jc.Contains, string(data))

	// Trash the tools and try to start another instance.
	suite.PatchValue(&envtools.DefaultBaseURL, "")
	instance, _, _, err = testing.StartInstance(env, "2")
	c.Check(instance, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

var testNetworkValues = []struct {
	includeNetworks []string
	excludeNetworks []string
	expectedResult  url.Values
}{
	{
		nil,
		nil,
		url.Values{},
	},
	{
		[]string{"included_net_1"},
		nil,
		url.Values{"networks": {"included_net_1"}},
	},
	{
		nil,
		[]string{"excluded_net_1"},
		url.Values{"not_networks": {"excluded_net_1"}},
	},
	{
		[]string{"included_net_1", "included_net_2"},
		[]string{"excluded_net_1", "excluded_net_2"},
		url.Values{
			"networks":     {"included_net_1", "included_net_2"},
			"not_networks": {"excluded_net_1", "excluded_net_2"},
		},
	},
}

func (suite *environSuite) getInstance(systemId string) *maasInstance {
	input := fmt.Sprintf(`{"system_id": %q}`, systemId)
	node := suite.testMAASObject.TestServer.NewNode(input)
	return &maasInstance{&node}
}

func (suite *environSuite) newNetwork(name string, id int, vlanTag int, defaultGateway string) *gomaasapi.MAASObject {
	var vlan string
	if vlanTag == 0 {
		vlan = "null"
	} else {
		vlan = fmt.Sprintf("%d", vlanTag)
	}

	if defaultGateway != "null" {
		// since we use %s below only "null" (if passed) should remain unquoted.
		defaultGateway = fmt.Sprintf("%q", defaultGateway)
	}

	// TODO(dimitern): Use JSON tags on structs, JSON encoder, or at least
	// text/template below and in similar cases.
	input := fmt.Sprintf(`{
		"name": %q,
		"ip":"192.168.%d.2",
		"netmask": "255.255.255.0",
		"vlan_tag": %s,
		"description": "%s_%d_%d",
		"default_gateway": %s
	}`,
		name,
		id,
		vlan,
		name, id, vlanTag,
		defaultGateway,
	)
	network := suite.testMAASObject.TestServer.NewNetwork(input)
	return &network
}

func (suite *environSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	suite.getInstance("test1")

	err := suite.makeEnviron().StopInstances()
	c.Check(err, jc.ErrorIsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	c.Check(operations, gc.DeepEquals, map[string][]string{})
}

func (suite *environSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	suite.getInstance("test1")
	suite.getInstance("test2")
	suite.getInstance("test3")
	// mark test1 and test2 as being allocated, but not test3.
	// The release operation will ignore test3.
	suite.testMAASObject.TestServer.OwnedNodes()["test1"] = true
	suite.testMAASObject.TestServer.OwnedNodes()["test2"] = true

	err := suite.makeEnviron().StopInstances("test1", "test2", "test3")
	c.Check(err, jc.ErrorIsNil)
	operations := suite.testMAASObject.TestServer.NodesOperations()
	c.Check(operations, gc.DeepEquals, []string{"release"})
	c.Assert(suite.testMAASObject.TestServer.OwnedNodes()["test1"], jc.IsFalse)
	c.Assert(suite.testMAASObject.TestServer.OwnedNodes()["test2"], jc.IsFalse)
}

func (suite *environSuite) TestStopInstancesIgnoresConflict(c *gc.C) {
	releaseNodes := func(nodes gomaasapi.MAASObject, ids url.Values) error {
		return gomaasapi.ServerError{StatusCode: 409}
	}
	suite.PatchValue(&ReleaseNodes, releaseNodes)
	env := suite.makeEnviron()
	err := env.StopInstances("test1")
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestStopInstancesIgnoresMissingNodeAndRecurses(c *gc.C) {
	attemptedNodes := [][]string{}
	releaseNodes := func(nodes gomaasapi.MAASObject, ids url.Values) error {
		attemptedNodes = append(attemptedNodes, ids["nodes"])
		return gomaasapi.ServerError{StatusCode: 404}
	}
	suite.PatchValue(&ReleaseNodes, releaseNodes)
	env := suite.makeEnviron()
	err := env.StopInstances("test1", "test2")
	c.Assert(err, jc.ErrorIsNil)

	expectedNodes := [][]string{{"test1", "test2"}, {"test1"}, {"test2"}}
	c.Assert(attemptedNodes, gc.DeepEquals, expectedNodes)
}

func (suite *environSuite) TestStopInstancesReturnsUnexpectedMAASError(c *gc.C) {
	releaseNodes := func(nodes gomaasapi.MAASObject, ids url.Values) error {
		return gomaasapi.ServerError{StatusCode: 405}
	}
	suite.PatchValue(&ReleaseNodes, releaseNodes)
	env := suite.makeEnviron()
	err := env.StopInstances("test1")
	c.Assert(err, gc.NotNil)
	maasErr, ok := errors.Cause(err).(gomaasapi.ServerError)
	c.Assert(ok, jc.IsTrue)
	c.Assert(maasErr.StatusCode, gc.Equals, 405)
}

func (suite *environSuite) TestStopInstancesReturnsUnexpectedError(c *gc.C) {
	releaseNodes := func(nodes gomaasapi.MAASObject, ids url.Values) error {
		return environs.ErrNoInstances
	}
	suite.PatchValue(&ReleaseNodes, releaseNodes)
	env := suite.makeEnviron()
	err := env.StopInstances("test1")
	c.Assert(err, gc.NotNil)
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (suite *environSuite) TestControllerInstances(c *gc.C) {
	env := suite.makeEnviron()
	_, err := env.ControllerInstances()
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)

	tests := [][]instance.Id{{}, {"inst-0"}, {"inst-0", "inst-1"}}
	for _, expected := range tests {
		err := common.SaveState(env.Storage(), &common.BootstrapState{
			StateInstances: expected,
		})
		c.Assert(err, jc.ErrorIsNil)
		controllerInstances, err := env.ControllerInstances()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(controllerInstances, jc.SameContents, expected)
	}
}

func (suite *environSuite) TestControllerInstancesFailsIfNoStateInstances(c *gc.C) {
	env := suite.makeEnviron()
	_, err := env.ControllerInstances()
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *environSuite) TestDestroy(c *gc.C) {
	env := suite.makeEnviron()
	suite.getInstance("test1")
	suite.testMAASObject.TestServer.OwnedNodes()["test1"] = true // simulate acquire
	data := makeRandomBytes(10)
	suite.testMAASObject.TestServer.NewFile("filename", data)
	stor := env.Storage()

	err := env.Destroy()
	c.Check(err, jc.ErrorIsNil)

	// Instances have been stopped.
	operations := suite.testMAASObject.TestServer.NodesOperations()
	c.Check(operations, gc.DeepEquals, []string{"release"})
	c.Check(suite.testMAASObject.TestServer.OwnedNodes()["test1"], jc.IsFalse)
	// Files have been cleaned up.
	listing, err := envstorage.List(stor, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

func (suite *environSuite) TestBootstrapSucceeds(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(fmt.Sprintf(
		`{"system_id": "thenode", "hostname": "host", "architecture": "%s/generic", "memory": 256, "cpu_count": 8}`,
		arch.HostArch()),
	)
	lshwXML, err := suite.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f0": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASObject.TestServer.AddNodeDetails("thenode", lshwXML)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestBootstrapNodeNotDeployed(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(fmt.Sprintf(
		`{"system_id": "thenode", "hostname": "host", "architecture": "%s/generic", "memory": 256, "cpu_count": 8}`,
		arch.HostArch()),
	)
	lshwXML, err := suite.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f0": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASObject.TestServer.AddNodeDetails("thenode", lshwXML)
	// Ensure node will not be reported as deployed by changing its status.
	suite.testMAASObject.TestServer.ChangeNode("thenode", "status", "4")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state.*")
}

func (suite *environSuite) TestBootstrapNodeFailedDeploy(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(fmt.Sprintf(
		`{"system_id": "thenode", "hostname": "host", "architecture": "%s/generic", "memory": 256, "cpu_count": 8}`,
		arch.HostArch()),
	)
	lshwXML, err := suite.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f0": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	suite.testMAASObject.TestServer.AddNodeDetails("thenode", lshwXML)
	// Set the node status to "Failed deployment"
	suite.testMAASObject.TestServer.ChangeNode("thenode", "status", "11")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state. instance \"/api/.*/nodes/thenode/\" failed to deploy")
}

func (suite *environSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	env := suite.makeEnviron()
	// Disable auto-uploading by setting the agent version.
	cfg, err := env.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, jc.ErrorIsNil)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Check(err, gc.ErrorMatches, "Juju cannot bootstrap because no tools are available for your model(.|\n)*")
}

func (suite *environSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, gc.ErrorMatches, ".*409.*")
}

func (suite *environSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := suite.makeEnviron()
	// Add a dummy file to storage so we can use that to check the
	// obtained source later.
	data := makeRandomBytes(10)
	stor := NewStorage(env)
	err := stor.Put("tools/filename", bytes.NewBuffer([]byte(data)), int64(len(data)))
	c.Assert(err, jc.ErrorIsNil)
	sources, err := envtools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (suite *environSuite) TestSupportedArchitectures(c *gc.C) {
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "precise"}`)
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "trusty"}`)
	suite.testMAASObject.TestServer.AddBootImage("uuid-1", `{"architecture": "amd64", "release": "precise"}`)
	suite.testMAASObject.TestServer.AddBootImage("uuid-1", `{"architecture": "ppc64el", "release": "trusty"}`)
	env := suite.makeEnviron()
	a, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.SameContents, []string{"amd64", "ppc64el"})
}

func (suite *environSuite) TestSupportedArchitecturesFallback(c *gc.C) {
	// If we cannot query boot-images (e.g. MAAS server version 1.4),
	// then Juju will fall over to listing all the available nodes.
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "architecture": "amd64/generic"}`)
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node1", "architecture": "armhf"}`)
	env := suite.makeEnviron()
	a, err := env.SupportedArchitectures()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(a, jc.SameContents, []string{"amd64", "armhf"})
}

func (suite *environSuite) TestConstraintsValidator(c *gc.C) {
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "trusty"}`)
	env := suite.makeEnviron()
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 instance-type=foo")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type"})
}

func (suite *environSuite) TestConstraintsValidatorVocab(c *gc.C) {
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "trusty"}`)
	suite.testMAASObject.TestServer.AddBootImage("uuid-1", `{"architecture": "armhf", "release": "precise"}`)
	env := suite.makeEnviron()
	validator, err := env.ConstraintsValidator()
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: \\[amd64 armhf\\]")
}

func (suite *environSuite) TestSupportsNetworking(c *gc.C) {
	env := suite.makeEnviron()
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)

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

func (suite *environSuite) TestSupportsAddressAllocation(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsAddressAllocation("")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
}

func (suite *environSuite) TestSupportsSpacesDefaultFalse(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsFalse)
}

func (suite *environSuite) TestSupportsSpaceDiscoveryDefaultFalse(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaceDiscovery()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsFalse)
}

func (suite *environSuite) TestSupportsSpaces(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
}

func (suite *environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaceDiscovery()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
}

func (suite *environSuite) createSubnets(c *gc.C, duplicates bool) instance.Instance {
	testInstance := suite.getInstance("node1")
	testServer := suite.testMAASObject.TestServer
	templateInterfaces := map[string]ifaceInfo{
		"aa:bb:cc:dd:ee:ff": {0, "wlan0", true},
		"aa:bb:cc:dd:ee:f1": {1, "eth0", false},
		"aa:bb:cc:dd:ee:f2": {2, "vnet1", false},
	}
	if duplicates {
		templateInterfaces["aa:bb:cc:dd:ee:f3"] = ifaceInfo{3, "eth1", true}
		templateInterfaces["aa:bb:cc:dd:ee:f4"] = ifaceInfo{4, "vnet2", false}
	}
	lshwXML, err := suite.generateHWTemplate(templateInterfaces)
	c.Assert(err, jc.ErrorIsNil)

	testServer.AddNodeDetails("node1", lshwXML)
	// resulting CIDR 192.168.2.1/24
	suite.newNetwork("LAN", 2, 42, "192.168.2.1") // primary + gateway
	testServer.ConnectNodeToNetworkWithMACAddress("node1", "LAN", "aa:bb:cc:dd:ee:f1")
	// resulting CIDR 192.168.3.1/24
	suite.newNetwork("Virt", 3, 0, "")
	testServer.ConnectNodeToNetworkWithMACAddress("node1", "Virt", "aa:bb:cc:dd:ee:f2")
	// resulting CIDR 192.168.1.1/24
	suite.newNetwork("WLAN", 1, 0, "")
	testServer.ConnectNodeToNetworkWithMACAddress("node1", "WLAN", "aa:bb:cc:dd:ee:ff")
	if duplicates {
		testServer.ConnectNodeToNetworkWithMACAddress("node1", "LAN", "aa:bb:cc:dd:ee:f3")
		testServer.ConnectNodeToNetworkWithMACAddress("node1", "Virt", "aa:bb:cc:dd:ee:f4")
	}

	// needed for getNodeGroups to work
	testServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "precise"}`)
	testServer.AddBootImage("uuid-1", `{"architecture": "amd64", "release": "precise"}`)

	jsonText1 := `{
		"ip_range_high":        "192.168.2.255",
		"ip_range_low":         "192.168.2.128",
		"broadcast_ip":         "192.168.2.255",
		"static_ip_range_low":  "192.168.2.0",
		"name":                 "eth0",
		"ip":                   "192.168.2.1",
		"subnet_mask":          "255.255.255.0",
		"management":           2,
		"static_ip_range_high": "192.168.2.127",
		"interface":            "eth0"
	}`
	jsonText2 := `{
		"ip_range_high":        "172.16.0.128",
		"ip_range_low":         "172.16.0.2",
		"broadcast_ip":         "172.16.0.255",
		"static_ip_range_low":  "172.16.0.129",
		"name":                 "eth1",
		"ip":                   "172.16.0.2",
		"subnet_mask":          "255.255.255.0",
		"management":           2,
		"static_ip_range_high": "172.16.0.255",
		"interface":            "eth1"
	}`
	jsonText3 := `{
		"ip_range_high":        "192.168.1.128",
		"ip_range_low":         "192.168.1.2",
		"broadcast_ip":         "192.168.1.255",
		"static_ip_range_low":  "192.168.1.129",
		"name":                 "eth2",
		"ip":                   "192.168.1.2",
		"subnet_mask":          "255.255.255.0",
		"management":           2,
		"static_ip_range_high": "192.168.1.255",
		"interface":            "eth2"
	}`
	jsonText4 := `{
		"ip_range_high":        "172.16.8.128",
		"ip_range_low":         "172.16.8.2",
		"broadcast_ip":         "172.16.8.255",
		"static_ip_range_low":  "172.16.0.129",
		"name":                 "eth3",
		"ip":                   "172.16.8.2",
		"subnet_mask":          "255.255.255.0",
		"management":           2,
		"static_ip_range_high": "172.16.8.255",
		"interface":            "eth3"
	}`
	testServer.NewNodegroupInterface("uuid-0", jsonText1)
	testServer.NewNodegroupInterface("uuid-0", jsonText2)
	testServer.NewNodegroupInterface("uuid-1", jsonText3)
	testServer.NewNodegroupInterface("uuid-1", jsonText4)
	return testInstance
}

func (suite *environSuite) TestSubnetsWithInstanceIdAndSubnetIdsWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	testInstance := suite.createSubnets(c, false)
	subnetsInfo, err := suite.makeEnviron().Subnets(testInstance.Id(), []network.Id{"LAN", "Virt", "WLAN"})
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := []network.SubnetInfo{{
		CIDR:              "192.168.2.2/24",
		ProviderId:        "LAN",
		VLANTag:           42,
		AllocatableIPLow:  net.ParseIP("192.168.2.0"),
		AllocatableIPHigh: net.ParseIP("192.168.2.127"),
	}, {
		CIDR:              "192.168.3.2/24",
		ProviderId:        "Virt",
		AllocatableIPLow:  nil,
		AllocatableIPHigh: nil,
		VLANTag:           0,
	}, {
		CIDR:              "192.168.1.2/24",
		ProviderId:        "WLAN",
		VLANTag:           0,
		AllocatableIPLow:  net.ParseIP("192.168.1.129"),
		AllocatableIPHigh: net.ParseIP("192.168.1.255"),
	}}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsWithInstanceIdNoSubnetIdsWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()
	_, err := env.Subnets(testInstance.Id(), []network.Id{})
	c.Assert(err, gc.ErrorMatches, "subnet IDs must not be empty")

	_, err = env.Subnets(testInstance.Id(), nil)
	c.Assert(err, gc.ErrorMatches, "subnet IDs must not be empty")
}

func (suite *environSuite) TestSubnetsNoInstanceIdWithSubnetIdsWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	suite.createSubnets(c, false)
	_, err := suite.makeEnviron().Subnets(instance.UnknownId, []network.Id{"LAN", "Virt", "WLAN"})
	c.Assert(err, gc.ErrorMatches, "instance ID is required")
}

func (suite *environSuite) TestSubnetsNoInstaceIdNoSubnetIdsWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	suite.createSubnets(c, false)
	env := suite.makeEnviron()
	_, err := env.Subnets(instance.UnknownId, nil)
	c.Assert(err, gc.ErrorMatches, "instance ID is required")
}

func (suite *environSuite) TestSubnetsInvalidInstaceIdAnySubnetIdsWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	suite.createSubnets(c, false)
	env := suite.makeEnviron()
	_, err := env.Subnets("invalid", []network.Id{"anything"})
	c.Assert(err, gc.ErrorMatches, `instance "invalid" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	_, err = env.Subnets("invalid", nil)
	c.Assert(err, gc.ErrorMatches, `instance "invalid" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (suite *environSuite) TestSubnetsWithInstanceIdAndSubnetIdsWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	var subnetIDs []network.Id
	var uintIDs []uint
	for _, i := range []uint{1, 2, 3} {
		id := suite.addSubnet(c, i, i, "node1")
		subnetIDs = append(subnetIDs, network.Id(fmt.Sprintf("%v", id)))
		uintIDs = append(uintIDs, id)
		suite.addSubnet(c, i+5, i, "node2")
		suite.addSubnet(c, i+10, i, "") // not linked to a node
	}
	testInstance := suite.getInstance("node1")
	env := suite.makeEnviron()

	subnetsInfo, err := env.Subnets(testInstance.Id(), subnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []network.SubnetInfo{
		createSubnetInfo(uintIDs[0], 1, 1),
		createSubnetInfo(uintIDs[1], 2, 2),
		createSubnetInfo(uintIDs[2], 3, 3),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(testInstance.Id(), subnetIDs[1:])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo[1:])
}

func (suite *environSuite) TestSubnetsWithInstaceIdNoSubnetIdsWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node1")
	suite.addSubnet(c, 3, 2, "")      // not linked to a node
	suite.addSubnet(c, 4, 2, "node2") // linked to another node
	testInstance := suite.getInstance("node1")
	env := suite.makeEnviron()

	subnetsInfo, err := env.Subnets(testInstance.Id(), []network.Id{})
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []network.SubnetInfo{
		createSubnetInfo(id1, 1, 1),
		createSubnetInfo(id2, 2, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(testInstance.Id(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsInvalidInstaceIdAnySubnetIdsWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	suite.addSubnet(c, 1, 1, "node1")
	suite.addSubnet(c, 2, 2, "node2")

	_, err := suite.makeEnviron().Subnets("invalid", []network.Id{"anything"})
	c.Assert(err, gc.ErrorMatches, `instance "invalid" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
func (suite *environSuite) TestSubnetsNoInstanceIdWithSubnetIdsWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node2")
	subnetIDs := []network.Id{
		network.Id(fmt.Sprintf("%v", id1)),
		network.Id(fmt.Sprintf("%v", id2)),
	}

	subnetsInfo, err := suite.makeEnviron().Subnets(instance.UnknownId, subnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []network.SubnetInfo{
		createSubnetInfo(id1, 1, 1),
		createSubnetInfo(id2, 2, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsNoInstanceIdNoSubnetIdsWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node2")
	env := suite.makeEnviron()

	subnetsInfo, err := suite.makeEnviron().Subnets(instance.UnknownId, []network.Id{})
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []network.SubnetInfo{
		createSubnetInfo(id1, 1, 1),
		createSubnetInfo(id2, 2, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsMissingSubnetWhenSpacesNotSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": []}`)
	testInstance := suite.createSubnets(c, false)
	_, err := suite.makeEnviron().Subnets(testInstance.Id(), []network.Id{"WLAN", "Missing"})
	c.Assert(err, gc.ErrorMatches, "failed to find the following subnets: Missing")
}

func (suite *environSuite) TestSubnetsMissingSubnetWhenSpacesAreSupported(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	testInstance := suite.getInstance("node1")
	suite.addSubnet(c, 1, 1, "node1")
	_, err := suite.makeEnviron().Subnets(testInstance.Id(), []network.Id{"1", "2"})
	c.Assert(err, gc.ErrorMatches, "failed to find the following subnets: 2")
}

func (suite *environSuite) TestSubnetsNoDuplicates(c *gc.C) {
	testInstance := suite.createSubnets(c, true)

	subnetsInfo, err := suite.makeEnviron().Subnets(testInstance.Id(), []network.Id{"LAN", "Virt", "WLAN"})
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := []network.SubnetInfo{{
		CIDR:              "192.168.2.2/24",
		ProviderId:        "LAN",
		VLANTag:           42,
		AllocatableIPLow:  net.ParseIP("192.168.2.0"),
		AllocatableIPHigh: net.ParseIP("192.168.2.127"),
	}, {
		CIDR:              "192.168.3.2/24",
		ProviderId:        "Virt",
		AllocatableIPLow:  nil,
		AllocatableIPHigh: nil,
		VLANTag:           0,
	}, {
		CIDR:              "192.168.1.2/24",
		ProviderId:        "WLAN",
		VLANTag:           0,
		AllocatableIPLow:  net.ParseIP("192.168.1.129"),
		AllocatableIPHigh: net.ParseIP("192.168.1.255"),
	}}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSpaces(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	for _, i := range []uint{1, 2, 3} {
		suite.addSubnet(c, i, i, "node1")
		suite.addSubnet(c, i+5, i, "node1")
	}

	spaces, err := suite.makeEnviron().Spaces()
	c.Assert(err, jc.ErrorIsNil)
	expectedSpaces := []network.SpaceInfo{{
		ProviderId: "Space 1",
		Subnets: []network.SubnetInfo{
			createSubnetInfo(1, 1, 1),
			createSubnetInfo(2, 1, 6),
		},
	}, {
		ProviderId: "Space 2",
		Subnets: []network.SubnetInfo{
			createSubnetInfo(3, 2, 2),
			createSubnetInfo(4, 2, 7),
		},
	}, {
		ProviderId: "Space 3",
		Subnets: []network.SubnetInfo{
			createSubnetInfo(5, 3, 3),
			createSubnetInfo(6, 3, 8),
		},
	}}
	c.Assert(spaces, jc.DeepEquals, expectedSpaces)
}

func (suite *environSuite) TestSpacesNeedsSupportsSpaces(c *gc.C) {
	_, err := suite.makeEnviron().Spaces()
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (suite *environSuite) assertSpaces(c *gc.C, numberOfSubnets int, filters []network.Id) {
	server := suite.testMAASObject.TestServer
	server.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	testInstance := suite.createSubnets(c, false)
	systemID := "node1"
	for i := 1; i <= numberOfSubnets; i++ {
		// Put most, but not all, of the subnets on node1.
		if i == 2 {
			systemID = "node2"
		} else {
			systemID = "node1"
		}
		suite.addSubnet(c, uint(i), uint(i), systemID)
	}

	subnets, err := suite.makeEnviron().Subnets(testInstance.Id(), filters)
	c.Assert(err, jc.ErrorIsNil)
	expectedSubnets := []network.SubnetInfo{
		createSubnetInfo(1, 1, 1),
		createSubnetInfo(3, 3, 3),
	}
	c.Assert(subnets, jc.DeepEquals, expectedSubnets)

}

func (suite *environSuite) TestSubnetsWithSpacesAllSubnets(c *gc.C) {
	suite.assertSpaces(c, 3, []network.Id{})
}

func (suite *environSuite) TestSubnetsWithSpacesFilteredIds(c *gc.C) {
	suite.assertSpaces(c, 4, []network.Id{"1", "3"})
}

func (suite *environSuite) TestSubnetsWithSpacesMissingSubnet(c *gc.C) {
	server := suite.testMAASObject.TestServer
	server.SetVersionJSON(`{"capabilities": ["network-deployment-ubuntu"]}`)
	testInstance := suite.createSubnets(c, false)
	for _, i := range []uint{1, 2} {
		suite.addSubnet(c, i, i, "node1")
	}

	_, err := suite.makeEnviron().Subnets(testInstance.Id(), []network.Id{"1", "3", "6"})
	errorRe := regexp.MustCompile("failed to find the following subnets: (\\d), (\\d)$")
	errorText := err.Error()
	c.Assert(errorRe.MatchString(errorText), jc.IsTrue)
	matches := errorRe.FindStringSubmatch(errorText)
	c.Assert(matches, gc.HasLen, 3)
	c.Assert(matches[1:], jc.SameContents, []string{"3", "6"})
}

func (suite *environSuite) TestAllocateAddress(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)

	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()

	// note that the default test server always succeeds if we provide a
	// valid instance id and net id
	err := env.AllocateAddress(testInstance.Id(), "LAN", &network.Address{Value: "192.168.2.1"}, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestAllocateAddressDevices(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses", "devices-management"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()

	// Work around the lack of support for devices PUT and POST without hostname
	// set in gomaasapi's testservices
	newParams := func(macAddress string, instId instance.Id, hostnameSuffix string) url.Values {
		c.Check(macAddress, gc.Equals, "aa:bb:cc:dd:ee:f0") // passed to AllocateAddress() below
		c.Check(instId, gc.Equals, testInstance.Id())
		c.Check(hostnameSuffix, gc.Equals, "juju-machine-0-kvm-5") // passed to AllocateAddress() below
		params := make(url.Values)
		params.Add("mac_addresses", macAddress)
		params.Add("hostname", "auto-generated.maas")
		params.Add("parent", extractSystemId(instId))
		return params
	}
	suite.PatchValue(&NewDeviceParams, newParams)
	updateHostname := func(client *gomaasapi.MAASObject, deviceID, deviceHostname, hostnameSuffix string) (string, error) {
		c.Check(client, gc.NotNil)
		c.Check(deviceID, gc.Matches, `node-[0-9a-f-]+`)
		c.Check(deviceHostname, gc.Equals, "auto-generated.maas")  // "generated" above in NewDeviceParams()
		c.Check(hostnameSuffix, gc.Equals, "juju-machine-0-kvm-5") // passed to AllocateAddress() below
		return "auto-generated-juju-lxc.maas", nil
	}
	suite.PatchValue(&UpdateDeviceHostname, updateHostname)

	// note that the default test server always succeeds if we provide a
	// valid instance id and net id
	err := env.AllocateAddress(
		testInstance.Id(),
		"LAN",
		&network.Address{Value: "192.168.2.1"},
		"aa:bb:cc:dd:ee:f0",
		"juju-machine-0-kvm-5",
	)
	c.Assert(err, jc.ErrorIsNil)

	devicesArray := suite.getDeviceArray(c)
	c.Assert(devicesArray, gc.HasLen, 1)

	device, err := devicesArray[0].GetMap()
	c.Assert(err, jc.ErrorIsNil)

	hostname, err := device["hostname"].GetString()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hostname, gc.Equals, "auto-generated.maas")

	parent, err := device["parent"].GetString()
	c.Assert(err, jc.ErrorIsNil)
	trimmedId := strings.TrimRight(string(testInstance.Id()), "/")
	split := strings.Split(trimmedId, "/")
	maasId := split[len(split)-1]
	c.Assert(parent, gc.Equals, maasId)

	addressesArray, err := device["ip_addresses"].GetArray()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addressesArray, gc.HasLen, 1)
	address, err := addressesArray[0].GetString()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(address, gc.Equals, "192.168.2.1")

	macArray, err := device["macaddress_set"].GetArray()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(macArray, gc.HasLen, 1)
	macMap, err := macArray[0].GetMap()
	c.Assert(err, jc.ErrorIsNil)
	mac, err := macMap["mac_address"].GetString()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mac, gc.Equals, "aa:bb:cc:dd:ee:f0")
}

func (suite *environSuite) TestTransformDeviceHostname(c *gc.C) {
	for i, test := range []struct {
		deviceHostname string
		hostnameSuffix string

		expectedOutput string
		expectedError  string
	}{{
		deviceHostname: "shiny-town.maas",
		hostnameSuffix: "juju-machine-1-lxc-2",
		expectedOutput: "shiny-town-juju-machine-1-lxc-2.maas",
	}, {
		deviceHostname: "foo.subdomain.example.com",
		hostnameSuffix: "suffix",
		expectedOutput: "foo-suffix.subdomain.example.com",
	}, {
		deviceHostname: "bad-food.example.com",
		hostnameSuffix: "suffix.example.org",
		expectedOutput: "bad-food-suffix.example.org.example.com",
	}, {
		deviceHostname: "strangers-and.freaks",
		hostnameSuffix: "just-this",
		expectedOutput: "strangers-and-just-this.freaks",
	}, {
		deviceHostname: "no-dot-hostname",
		hostnameSuffix: "anything",
		expectedError:  `unexpected device "dev-id" hostname "no-dot-hostname"`,
	}, {
		deviceHostname: "anything",
		hostnameSuffix: "",
		expectedError:  "hostname suffix cannot be empty",
	}} {
		c.Logf(
			"test #%d: %q + %q -> %q (err: %s)",
			i, test.deviceHostname, test.hostnameSuffix,
			test.expectedOutput, test.expectedError,
		)
		output, err := transformDeviceHostname("dev-id", test.deviceHostname, test.hostnameSuffix)
		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
			c.Check(output, gc.Equals, "")
			continue
		}
		c.Check(err, jc.ErrorIsNil)
		c.Check(output, gc.Equals, test.expectedOutput)
	}
}

func (suite *environSuite) patchDeviceCreation() {
	// Work around the lack of support for devices PUT and POST without hostname
	// set in gomaasapi's testservices
	newParams := func(macAddress string, instId instance.Id, _ string) url.Values {
		params := make(url.Values)
		params.Add("mac_addresses", macAddress)
		params.Add("hostname", "auto-generated.maas")
		params.Add("parent", extractSystemId(instId))
		return params
	}
	suite.PatchValue(&NewDeviceParams, newParams)
	updateHostname := func(_ *gomaasapi.MAASObject, _, _, _ string) (string, error) {
		return "auto-generated-juju-lxc.maas", nil
	}
	suite.PatchValue(&UpdateDeviceHostname, updateHostname)
}

func (suite *environSuite) TestAllocateAddressDevicesFailures(c *gc.C) {
	suite.SetFeatureFlags()
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["devices-management"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()
	suite.patchDeviceCreation()

	responses := []string{
		"claim_sticky_ip_address failed",
		"GetMap of the response failed",
		"no ip_addresses in response",
		"unexpected ip_addresses in response",
		"IP in ip_addresses not a string",
	}
	reserveIP := func(_ gomaasapi.MAASObject, deviceID, macAddress string, addr network.Address) (network.Address, error) {
		c.Check(deviceID, gc.Matches, "node-[a-f0-9]+")
		c.Check(macAddress, gc.Matches, "aa:bb:cc:dd:ee:f0")
		c.Check(addr, jc.DeepEquals, network.Address{})
		nextError := responses[0]
		return network.Address{}, errors.New(nextError)
	}
	suite.PatchValue(&ReserveIPAddressOnDevice, reserveIP)

	for len(responses) > 0 {
		addr := &network.Address{}
		err := env.AllocateAddress(
			testInstance.Id(), network.AnySubnet, addr,
			"aa:bb:cc:dd:ee:f0", "juju-lxc",
		)
		c.Check(err, gc.ErrorMatches, responses[0])
		responses = responses[1:]
	}
}

func (suite *environSuite) getDeviceArray(c *gc.C) []gomaasapi.JSONObject {
	devicesURL := "/api/1.0/devices/?op=list"
	resp, err := http.Get(suite.testMAASObject.TestServer.Server.URL + devicesURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)

	defer resp.Body.Close()
	content, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	result, err := gomaasapi.Parse(gomaasapi.Client{}, content)
	c.Assert(err, jc.ErrorIsNil)

	devicesArray, err := result.GetArray()
	c.Assert(err, jc.ErrorIsNil)
	return devicesArray
}

func (suite *environSuite) TestReleaseAddressDeletesDevice(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses", "devices-management"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()
	suite.patchDeviceCreation()

	addr := network.NewAddress("192.168.2.1")
	err := env.AllocateAddress(testInstance.Id(), "LAN", &addr, "foo", "juju-lxc")
	c.Assert(err, jc.ErrorIsNil)

	devicesArray := suite.getDeviceArray(c)
	c.Assert(devicesArray, gc.HasLen, 1)

	// Since we're mocking out updateDeviceHostname, no need to check if the
	// hostname was updated (only manually tested for now until we change
	// gomaasapi).

	err = env.ReleaseAddress(testInstance.Id(), "LAN", addr, "foo", "juju-lxc")
	c.Assert(err, jc.ErrorIsNil)

	devicesArray = suite.getDeviceArray(c)
	c.Assert(devicesArray, gc.HasLen, 0)
}

func (suite *environSuite) TestAllocateAddressInvalidInstance(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
	env := suite.makeEnviron()
	addr := network.Address{Value: "192.168.2.1"}
	instId := instance.Id("foo")
	err := env.AllocateAddress(instId, "bar", &addr, "foo", "juju-lxc")
	expected := fmt.Sprintf("failed to allocate address %q for instance %q.*", addr, instId)
	c.Assert(err, gc.ErrorMatches, expected)
}

func (suite *environSuite) TestAllocateAddressMissingSubnet(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()
	err := env.AllocateAddress(testInstance.Id(), "bar", &network.Address{Value: "192.168.2.1"}, "foo", "bar")
	c.Assert(errors.Cause(err), gc.ErrorMatches, "failed to find the following subnets: bar")
}

func (suite *environSuite) TestAllocateAddressIPAddressUnavailable(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()

	reserveIPAddress := func(ipaddresses gomaasapi.MAASObject, cidr string, addr network.Address) error {
		return gomaasapi.ServerError{StatusCode: 404}
	}
	suite.PatchValue(&ReserveIPAddress, reserveIPAddress)

	ipAddress := network.Address{Value: "192.168.2.1"}
	err := env.AllocateAddress(testInstance.Id(), "LAN", &ipAddress, "foo", "bar")
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrIPAddressUnavailable)
	expected := fmt.Sprintf("failed to allocate address %q for instance %q.*", ipAddress, testInstance.Id())
	c.Assert(err, gc.ErrorMatches, expected)
}

func (s *environSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	placement := "zone=zone1"
	err := env.PrecheckInstance(coretesting.FakeDefaultSeries, constraints.Value{}, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestReleaseAddress(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()

	err := env.AllocateAddress(testInstance.Id(), "LAN", &network.Address{Value: "192.168.2.1"}, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)

	ipAddress := network.Address{Value: "192.168.2.1"}
	macAddress := "foobar"
	hostname := "myhostname"
	err = env.ReleaseAddress(testInstance.Id(), "bar", ipAddress, macAddress, hostname)
	c.Assert(err, jc.ErrorIsNil)

	// by releasing again we can test that the first release worked, *and*
	// the error handling of ReleaseError
	err = env.ReleaseAddress(testInstance.Id(), "bar", ipAddress, macAddress, hostname)
	expected := fmt.Sprintf("(?s).*failed to release IP address %q from instance %q.*", ipAddress, testInstance.Id())
	c.Assert(err, gc.ErrorMatches, expected)
}

func (suite *environSuite) TestReleaseAddressRetry(c *gc.C) {
	suite.testMAASObject.TestServer.SetVersionJSON(`{"capabilities": ["networks-management","static-ipaddresses"]}`)
	// Patch short attempt params.
	suite.PatchValue(&shortAttempt, utils.AttemptStrategy{
		Min: 5,
	})
	// Patch IP address release call to MAAS.
	retries := 0
	enoughRetries := 10
	suite.PatchValue(&ReleaseIPAddress, func(ipaddresses gomaasapi.MAASObject, addr network.Address) error {
		retries++
		if retries < enoughRetries {
			return errors.New("ouch")
		}
		return nil
	})

	testInstance := suite.createSubnets(c, false)
	env := suite.makeEnviron()

	err := env.AllocateAddress(testInstance.Id(), "LAN", &network.Address{Value: "192.168.2.1"}, "foo", "bar")
	c.Assert(err, jc.ErrorIsNil)

	// ReleaseAddress must fail with 5 retries.
	ipAddress := network.Address{Value: "192.168.2.1"}
	macAddress := "foobar"
	hostname := "myhostname"
	err = env.ReleaseAddress(testInstance.Id(), "bar", ipAddress, macAddress, hostname)
	expected := fmt.Sprintf("(?s).*failed to release IP address %q from instance %q: ouch", ipAddress, testInstance.Id())
	c.Assert(err, gc.ErrorMatches, expected)
	c.Assert(retries, gc.Equals, 5)

	// Now let it succeed after 3 retries.
	retries = 0
	enoughRetries = 3
	err = env.ReleaseAddress(testInstance.Id(), "bar", ipAddress, macAddress, hostname)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retries, gc.Equals, 3)
}

func (s *environSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	placement := "zone=zone2"
	err := env.PrecheckInstance(coretesting.FakeDefaultSeries, constraints.Value{}, placement)
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "zone2"`)
}

func (s *environSuite) TestPrecheckInstanceAvailZonesUnsupported(c *gc.C) {
	env := s.makeEnviron()
	placement := "zone=test-unknown"
	err := env.PrecheckInstance(coretesting.FakeDefaultSeries, constraints.Value{}, placement)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *environSuite) TestPrecheckInvalidPlacement(c *gc.C) {
	env := s.makeEnviron()
	err := env.PrecheckInstance(coretesting.FakeDefaultSeries, constraints.Value{}, "notzone=anything")
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: notzone=anything")
}

func (s *environSuite) TestPrecheckNodePlacement(c *gc.C) {
	env := s.makeEnviron()
	err := env.PrecheckInstance(coretesting.FakeDefaultSeries, constraints.Value{}, "assumed_node_name")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestStartInstanceAvailZone(c *gc.C) {
	// Add a node for the started instance.
	s.newNode(c, "thenode1", "host1", map[string]interface{}{"zone": "test-available"})
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	inst, err := s.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst.(*maasInstance).zone(), gc.Equals, "test-available")
}

func (s *environSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	_, err := s.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.ErrorMatches, `invalid availability zone "test-unknown"`)
}

func (s *environSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instance.Instance, error) {
	env := s.bootstrap(c)
	params := environs.StartInstanceParams{Placement: "zone=" + zone}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (s *environSuite) TestStartInstanceUnmetConstraints(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", nil)
	params := environs.StartInstanceParams{Constraints: constraints.MustParse("mem=8G")}
	_, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, gc.ErrorMatches, "cannot run instances:.* 409.*")
}

func (s *environSuite) TestStartInstanceConstraints(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", nil)
	s.newNode(c, "thenode2", "host2", map[string]interface{}{"memory": 8192})
	params := environs.StartInstanceParams{Constraints: constraints.MustParse("mem=8G")}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*result.Hardware.Mem, gc.Equals, uint64(8192))
}

var nodeStorageAttrs = []map[string]interface{}{
	{
		"name":       "sdb",
		"id":         1,
		"id_path":    "/dev/disk/by-id/id_for_sda",
		"path":       "/dev/sdb",
		"model":      "Samsung_SSD_850_EVO_250GB",
		"block_size": 4096,
		"serial":     "S21NNSAFC38075L",
		"size":       uint64(250059350016),
	},
	{
		"name":       "sda",
		"id":         2,
		"path":       "/dev/sda",
		"model":      "Samsung_SSD_850_EVO_250GB",
		"block_size": 4096,
		"serial":     "XXXX",
		"size":       uint64(250059350016),
	},
	{
		"name":       "sdc",
		"id":         3,
		"path":       "/dev/sdc",
		"model":      "Samsung_SSD_850_EVO_250GB",
		"block_size": 4096,
		"serial":     "YYYYYYY",
		"size":       uint64(250059350016),
	},
}

var storageConstraintAttrs = map[string]interface{}{
	"1": "1",
	"2": "root",
	"3": "3",
}

func (s *environSuite) TestStartInstanceStorage(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", map[string]interface{}{
		"memory":                  8192,
		"physicalblockdevice_set": nodeStorageAttrs,
		"constraint_map":          storageConstraintAttrs,
	})
	params := environs.StartInstanceParams{Volumes: []storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000},
		{Tag: names.NewVolumeTag("3"), Size: 2000000},
	}}
	result, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Volumes, jc.DeepEquals, []storage.Volume{
		{
			names.NewVolumeTag("1"),
			storage.VolumeInfo{
				Size:       238475,
				VolumeId:   "volume-1",
				HardwareId: "id_for_sda",
			},
		},
		{
			names.NewVolumeTag("3"),
			storage.VolumeInfo{
				Size:       238475,
				VolumeId:   "volume-3",
				HardwareId: "",
			},
		},
	})
	c.Assert(result.VolumeAttachments, jc.DeepEquals, []storage.VolumeAttachment{
		{
			names.NewVolumeTag("1"),
			names.NewMachineTag("1"),
			storage.VolumeAttachmentInfo{
				DeviceName: "",
				ReadOnly:   false,
			},
		},
		{
			names.NewVolumeTag("3"),
			names.NewMachineTag("1"),
			storage.VolumeAttachmentInfo{
				DeviceName: "sdc",
				ReadOnly:   false,
			},
		},
	})
}

func (s *environSuite) TestStartInstanceUnsupportedStorage(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", map[string]interface{}{
		"memory": 8192,
	})
	params := environs.StartInstanceParams{Volumes: []storage.VolumeParams{
		{Tag: names.NewVolumeTag("1"), Size: 2000000},
		{Tag: names.NewVolumeTag("3"), Size: 2000000},
	}}
	_, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, gc.ErrorMatches, "the version of MAAS being used does not support Juju storage")
	operations := s.testMAASObject.TestServer.NodesOperations()
	c.Check(operations, gc.DeepEquals, []string{"acquire", "acquire", "release"})
	c.Assert(s.testMAASObject.TestServer.OwnedNodes()["node0"], jc.IsTrue)
	c.Assert(s.testMAASObject.TestServer.OwnedNodes()["thenode1"], jc.IsFalse)
}

func (s *environSuite) TestGetAvailabilityZones(c *gc.C) {
	env := s.makeEnviron()

	zones, err := env.AvailabilityZones()
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
	c.Assert(zones, gc.IsNil)

	s.testMAASObject.TestServer.AddZone("whatever", "andever")
	zones, err = env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
	c.Assert(zones[0].Available(), jc.IsTrue)

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	s.testMAASObject.TestServer.AddZone("somewhere", "outthere")
	zones, err = env.AvailabilityZones()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

type mockAvailabilityZoneAllocations struct {
	group  []instance.Id // input param
	result []common.AvailabilityZoneInstances
	err    error
}

func (m *mockAvailabilityZoneAllocations) AvailabilityZoneAllocations(
	e common.ZonedEnviron, group []instance.Id,
) ([]common.AvailabilityZoneInstances, error) {
	m.group = group
	return m.result, m.err
}

func (s *environSuite) newNode(c *gc.C, nodename, hostname string, attrs map[string]interface{}) {
	allAttrs := map[string]interface{}{
		"system_id":    nodename,
		"hostname":     hostname,
		"architecture": fmt.Sprintf("%s/generic", arch.HostArch()),
		"memory":       1024,
		"cpu_count":    1,
	}
	for k, v := range attrs {
		allAttrs[k] = v
	}
	data, err := json.Marshal(allAttrs)
	c.Assert(err, jc.ErrorIsNil)
	s.testMAASObject.TestServer.NewNode(string(data))
	lshwXML, err := s.generateHWTemplate(map[string]ifaceInfo{"aa:bb:cc:dd:ee:f0": {0, "eth0", false}})
	c.Assert(err, jc.ErrorIsNil)
	s.testMAASObject.TestServer.AddNodeDetails(nodename, lshwXML)
}

func (s *environSuite) bootstrap(c *gc.C) environs.Environ {
	s.newNode(c, "node0", "bootstrap-host", nil)
	s.setupFakeTools(c)
	env := s.makeEnviron()
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		Placement: "bootstrap-host",
	})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func (s *environSuite) TestStartInstanceDistributionParams(c *gc.C) {
	env := s.bootstrap(c)
	var mock mockAvailabilityZoneAllocations
	s.PatchValue(&availabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// no distribution group specified
	s.newNode(c, "node1", "host1", nil)
	testing.AssertStartInstance(c, env, "1")
	c.Assert(mock.group, gc.HasLen, 0)

	// distribution group specified: ensure it's passed through to AvailabilityZone.
	s.newNode(c, "node2", "host2", nil)
	expectedInstances := []instance.Id{"i-0", "i-1"}
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return expectedInstances, nil
		},
	}
	_, err := testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mock.group, gc.DeepEquals, expectedInstances)
}

func (s *environSuite) TestStartInstanceDistributionErrors(c *gc.C) {
	env := s.bootstrap(c)
	mock := mockAvailabilityZoneAllocations{
		err: errors.New("AvailabilityZoneAllocations failed"),
	}
	s.PatchValue(&availabilityZoneAllocations, mock.AvailabilityZoneAllocations)
	_, _, _, err := testing.StartInstance(env, "1")
	c.Assert(err, gc.ErrorMatches, "cannot get availability zone allocations: AvailabilityZoneAllocations failed")

	mock.err = nil
	dgErr := errors.New("DistributionGroup failed")
	params := environs.StartInstanceParams{
		DistributionGroup: func() ([]instance.Id, error) {
			return nil, dgErr
		},
	}
	_, err = testing.StartInstanceWithParams(env, "1", params, nil)
	c.Assert(err, gc.ErrorMatches, "cannot get distribution group: DistributionGroup failed")
}

func (s *environSuite) TestStartInstanceDistribution(c *gc.C) {
	env := s.bootstrap(c)
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	s.newNode(c, "node1", "host1", map[string]interface{}{"zone": "test-available"})
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(inst.(*maasInstance).zone(), gc.Equals, "test-available")
}

func (s *environSuite) TestStartInstanceDistributionAZNotImplemented(c *gc.C) {
	env := s.bootstrap(c)

	mock := mockAvailabilityZoneAllocations{err: errors.NotImplementedf("availability zones")}
	s.PatchValue(&availabilityZoneAllocations, mock.AvailabilityZoneAllocations)

	// Instance will be created without an availability zone specified.
	s.newNode(c, "node1", "host1", nil)
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(inst.(*maasInstance).zone(), gc.Equals, "")
}

func (s *environSuite) TestStartInstanceDistributionFailover(c *gc.C) {
	mock := mockAvailabilityZoneAllocations{
		result: []common.AvailabilityZoneInstances{{
			ZoneName: "zone1",
		}, {
			ZoneName: "zonelord",
		}, {
			ZoneName: "zone2",
		}},
	}
	s.PatchValue(&availabilityZoneAllocations, mock.AvailabilityZoneAllocations)
	s.testMAASObject.TestServer.AddZone("zone1", "description")
	s.testMAASObject.TestServer.AddZone("zone2", "description")
	s.newNode(c, "node2", "host2", map[string]interface{}{"zone": "zone2"})

	env := s.bootstrap(c)
	inst, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(inst.(*maasInstance).zone(), gc.Equals, "zone2")
	c.Assert(s.testMAASObject.TestServer.NodesOperations(), gc.DeepEquals, []string{
		// one acquire for the bootstrap, three for StartInstance (with zone failover)
		"acquire", "acquire", "acquire", "acquire",
	})
	c.Assert(s.testMAASObject.TestServer.NodesOperationRequestValues(), gc.DeepEquals, []url.Values{{
		"name":       []string{"bootstrap-host"},
		"agent_name": []string{exampleAgentName},
	}, {
		"zone":       []string{"zone1"},
		"agent_name": []string{exampleAgentName},
	}, {
		"zone":       []string{"zonelord"},
		"agent_name": []string{exampleAgentName},
	}, {
		"zone":       []string{"zone2"},
		"agent_name": []string{exampleAgentName},
	}})
}

func (s *environSuite) TestStartInstanceDistributionOneAssigned(c *gc.C) {
	mock := mockAvailabilityZoneAllocations{
		result: []common.AvailabilityZoneInstances{{
			ZoneName: "zone1",
		}, {
			ZoneName: "zone2",
		}},
	}
	s.PatchValue(&availabilityZoneAllocations, mock.AvailabilityZoneAllocations)
	s.testMAASObject.TestServer.AddZone("zone1", "description")
	s.testMAASObject.TestServer.AddZone("zone2", "description")
	s.newNode(c, "node1", "host1", map[string]interface{}{"zone": "zone1"})
	s.newNode(c, "node2", "host2", map[string]interface{}{"zone": "zone2"})

	env := s.bootstrap(c)
	testing.AssertStartInstance(c, env, "1")
	c.Assert(s.testMAASObject.TestServer.NodesOperations(), gc.DeepEquals, []string{
		// one acquire for the bootstrap, one for StartInstance.
		"acquire", "acquire",
	})
}
