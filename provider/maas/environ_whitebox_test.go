// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gomaasapi"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/arch"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	envstorage "github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/testing"
	jujuversion "github.com/juju/juju/juju/version"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
)

// ensure we conform to the right interfaces
var (
	_ environs.NetworkingEnviron = (*maasEnviron)(nil)
	// Should maasEnviron implement this? It needs ConfigDefaults
	// _ config.ConfigSchemaSource  = (*maasEnviron)(nil)
)

type environSuite struct {
	providerSuite
}

const (
	allocatedNode = `{"system_id": "test-allocated"}`
)

var _ = gc.Suite(&environSuite{})

// ifaceInfo describes an interface to be created on the test server.
type ifaceInfo struct {
	DeviceIndex   int
	InterfaceName string
	Disabled      bool
}

func (suite *environSuite) addNode(jsonText string) instance.Id {
	node := suite.testMAASObject.TestServer.NewNode(jsonText)
	resourceURI, _ := node.GetField("resource_uri")
	return instance.Id(resourceURI)
}

func (suite *environSuite) TestInstancesReturnsInstances(c *gc.C) {
	id := suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances(suite.callCtx, []instance.Id{id})

	c.Check(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Id(), gc.Equals, id)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfEmptyParameter(c *gc.C) {
	suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances(suite.callCtx, []instance.Id{})

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNilParameter(c *gc.C) {
	suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances(suite.callCtx, nil)

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoneFound(c *gc.C) {
	instances, err := suite.makeEnviron().Instances(suite.callCtx, []instance.Id{"unknown"})
	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestAllRunningInstances(c *gc.C) {
	id := suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().AllRunningInstances(suite.callCtx)

	c.Check(err, jc.ErrorIsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Id(), gc.Equals, id)
}

func (suite *environSuite) TestAllRunningInstancesReturnsEmptySliceIfNoInstance(c *gc.C) {
	instances, err := suite.makeEnviron().AllRunningInstances(suite.callCtx)

	c.Check(err, jc.ErrorIsNil)
	c.Check(instances, gc.HasLen, 0)
}

func (suite *environSuite) TestInstancesReturnsErrorIfPartialInstances(c *gc.C) {
	known := suite.addNode(allocatedNode)
	suite.addNode(`{"system_id": "test2"}`)
	unknown := instance.Id("unknown systemID")
	instances, err := suite.makeEnviron().Instances(suite.callCtx, []instance.Id{known, unknown})

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
	specificStorage := stor.(*maas1Storage)
	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environ, gc.Equals, env)
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
	suite.newNode(c, "node0", "host0", nil)
	suite.addSubnet(c, 9, 9, "node0")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
	c.Assert(err, jc.ErrorIsNil)
	// The bootstrap node has been acquired and started.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Check(found, jc.IsTrue)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// Test the instance id is correctly recorded for the bootstrap node.
	// Check that ControllerInstances returns the id of the bootstrap machine.
	instanceIds, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instanceIds, gc.HasLen, 1)
	insts, err := env.AllRunningInstances(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, instanceIds[0])

	// Create node 1: it will be used as instance number 1.
	suite.newNode(c, "node1", "host1", nil)
	suite.addSubnet(c, 8, 8, "node1")
	params := environs.StartInstanceParams{ControllerUUID: suite.controllerUUID}
	err = testing.FillInStartInstanceParams(env, "node1", false, &params)
	c.Assert(err, jc.ErrorIsNil)
	result, err := testing.StartInstanceWithParams(env, suite.callCtx, "1", params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.DisplayName, gc.Equals, "host1")
	hc := result.Hardware
	c.Assert(hc, gc.NotNil)
	c.Check(hc.String(), gc.Equals, fmt.Sprintf("arch=%s cores=1 mem=1024M availability-zone=test_zone", arch.HostArch()))

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
	instance, _, _, err := testing.StartInstance(env, suite.callCtx, suite.controllerUUID, "2")
	c.Check(instance, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (suite *environSuite) getInstance(systemId string) *maas1Instance {
	input := fmt.Sprintf(`{"system_id": %q}`, systemId)
	node := suite.testMAASObject.TestServer.NewNode(input)
	statusGetter := func(context.ProviderCallContext, instance.Id) (string, string) {
		return "unknown", "FAKE"
	}

	return &maas1Instance{&node, nil, statusGetter}
}

func (suite *environSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	suite.getInstance("test1")

	err := suite.makeEnviron().StopInstances(suite.callCtx)
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

	err := suite.makeEnviron().StopInstances(suite.callCtx, "test1", "test2", "test3")
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
	err := env.StopInstances(suite.callCtx, "test1")
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
	err := env.StopInstances(suite.callCtx, "test1", "test2")
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
	err := env.StopInstances(suite.callCtx, "test1")
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
	err := env.StopInstances(suite.callCtx, "test1")
	c.Assert(err, gc.NotNil)
	c.Assert(errors.Cause(err), gc.Equals, environs.ErrNoInstances)
}

func (suite *environSuite) TestControllerInstances(c *gc.C) {
	env := suite.makeEnviron()
	_, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)

	tests := [][]instance.Id{{}, {"inst-0"}, {"inst-0", "inst-1"}}
	for _, expected := range tests {
		err := common.SaveState(env.Storage(), &common.BootstrapState{
			StateInstances: expected,
		})
		c.Assert(err, jc.ErrorIsNil)
		controllerInstances, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(controllerInstances, jc.SameContents, expected)
	}
}

func (suite *environSuite) TestControllerInstancesFailsIfNoStateInstances(c *gc.C) {
	env := suite.makeEnviron()
	_, err := env.ControllerInstances(suite.callCtx, suite.controllerUUID)
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *environSuite) TestDestroy(c *gc.C) {
	env := suite.makeEnviron()
	suite.getInstance("test1")
	suite.testMAASObject.TestServer.OwnedNodes()["test1"] = true // simulate acquire
	data := makeRandomBytes(10)
	suite.testMAASObject.TestServer.NewFile("filename", data)
	stor := env.Storage()

	err := env.Destroy(suite.callCtx)
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
	suite.newNode(c, "thenode", "host", nil)
	suite.addSubnet(c, 9, 9, "thenode")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (suite *environSuite) TestBootstrapNodeNotDeployed(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.newNode(c, "thenode", "host", nil)
	suite.addSubnet(c, 9, 9, "thenode")
	// Ensure node will not be reported as deployed by changing its status.
	suite.testMAASObject.TestServer.ChangeNode("thenode", "status", "4")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state.*")
}

func (suite *environSuite) TestBootstrapNodeFailedDeploy(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.newNode(c, "thenode", "host", nil)
	suite.addSubnet(c, 9, 9, "thenode")
	// Set the node status to "Failed deployment"
	suite.testMAASObject.TestServer.ChangeNode("thenode", "status", "11")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
	c.Assert(err, gc.ErrorMatches, "bootstrap instance started but did not change to Deployed state. instance \"/api/.*/nodes/thenode/\" failed to deploy")
}

func (suite *environSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	env := suite.makeEnviron()
	vers := version.MustParse("1.2.3")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      testing.AdminSecret,
			CAPrivateKey:     coretesting.CAKey,
			// Disable auto-uploading by setting the agent version
			// to something that's not the current version.
			AgentVersion: &vers,
		})
	c.Check(err, gc.ErrorMatches, "Juju cannot bootstrap because no agent binaries are available for your model(.|\n)*")
}

func (suite *environSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		suite.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
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
	err := stor.Put("tools/filename", bytes.NewBuffer(data), int64(len(data)))
	c.Assert(err, jc.ErrorIsNil)
	sources, err := envtools.GetMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sources, gc.HasLen, 0)
}

func (suite *environSuite) TestConstraintsValidator(c *gc.C) {
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "trusty"}`)
	env := suite.makeEnviron()
	validator, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=amd64 cpu-power=10 instance-type=foo virt-type=kvm")
	unsupported, err := validator.Validate(cons)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-power", "instance-type", "virt-type"})
}

func (suite *environSuite) TestConstraintsValidatorVocab(c *gc.C) {
	suite.testMAASObject.TestServer.AddBootImage("uuid-0", `{"architecture": "amd64", "release": "trusty"}`)
	suite.testMAASObject.TestServer.AddBootImage("uuid-1", `{"architecture": "armhf", "release": "precise"}`)
	env := suite.makeEnviron()
	validator, err := env.ConstraintsValidator(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse("arch=ppc64el")
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch=ppc64el\nvalid values are: \\[amd64 armhf\\]")
}

func (suite *environSuite) TestSupportsNetworking(c *gc.C) {
	env := suite.makeEnviron()
	_, supported := environs.SupportsNetworking(env)
	c.Assert(supported, jc.IsTrue)
}

func (suite *environSuite) TestSupportsSpaces(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaces(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
	c.Check(environs.SupportsSpaces(suite.callCtx, env), jc.IsTrue)
}

func (suite *environSuite) TestSupportsSpaceDiscovery(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsSpaceDiscovery(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(supported, jc.IsTrue)
}

func (suite *environSuite) TestSupportsContainerAddresses(c *gc.C) {
	env := suite.makeEnviron()
	supported, err := env.SupportsContainerAddresses(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(supported, jc.IsTrue)
	c.Check(environs.SupportsContainerAddresses(suite.callCtx, env), jc.IsTrue)
}

func (suite *environSuite) TestSubnetsWithInstanceIdAndSubnetIds(c *gc.C) {
	server := suite.testMAASObject.TestServer
	var subnetIDs []corenetwork.Id
	var uintIDs []uint
	for _, i := range []uint{1, 2, 3} {
		server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: fmt.Sprintf("space-%d", i)}))
		id := suite.addSubnet(c, i, i, "node1")
		subnetIDs = append(subnetIDs, corenetwork.Id(fmt.Sprintf("%v", id)))
		uintIDs = append(uintIDs, id)
		suite.addSubnet(c, i+5, i, "node2")
		suite.addSubnet(c, i+10, i, "") // not linked to a node
	}
	testInstance := suite.getInstance("node1")
	env := suite.makeEnviron()

	subnetsInfo, err := env.Subnets(suite.callCtx, testInstance.Id(), subnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []corenetwork.SubnetInfo{
		createSubnetInfo(uintIDs[0], 2, 1),
		createSubnetInfo(uintIDs[1], 3, 2),
		createSubnetInfo(uintIDs[2], 4, 3),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(suite.callCtx, testInstance.Id(), subnetIDs[1:])
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo[1:])
}

func (suite *environSuite) createTwoSpaces() {
	server := suite.testMAASObject.TestServer
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-1"}))
	server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-2"}))
}

func (suite *environSuite) TestSubnetsWithInstanceIdNoSubnetIds(c *gc.C) {
	suite.createTwoSpaces()
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node1")
	suite.addSubnet(c, 3, 2, "")      // not linked to a node
	suite.addSubnet(c, 4, 2, "node2") // linked to another node
	testInstance := suite.getInstance("node1")
	env := suite.makeEnviron()

	subnetsInfo, err := env.Subnets(suite.callCtx, testInstance.Id(), []corenetwork.Id{})
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []corenetwork.SubnetInfo{
		createSubnetInfo(id1, 2, 1),
		createSubnetInfo(id2, 3, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(suite.callCtx, testInstance.Id(), nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsInvalidInstaceIdAnySubnetIds(c *gc.C) {
	suite.createTwoSpaces()
	suite.addSubnet(c, 1, 1, "node1")
	suite.addSubnet(c, 2, 2, "node2")

	_, err := suite.makeEnviron().Subnets(suite.callCtx, "invalid", []corenetwork.Id{"anything"})
	c.Assert(err, gc.ErrorMatches, `instance "invalid" not found`)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (suite *environSuite) TestSubnetsNoInstanceIdWithSubnetIds(c *gc.C) {
	suite.createTwoSpaces()
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node2")
	subnetIDs := []corenetwork.Id{
		corenetwork.Id(fmt.Sprintf("%v", id1)),
		corenetwork.Id(fmt.Sprintf("%v", id2)),
	}

	subnetsInfo, err := suite.makeEnviron().Subnets(suite.callCtx, instance.UnknownId, subnetIDs)
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []corenetwork.SubnetInfo{
		createSubnetInfo(id1, 2, 1),
		createSubnetInfo(id2, 3, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSubnetsNoInstanceIdNoSubnetIds(c *gc.C) {
	suite.createTwoSpaces()
	id1 := suite.addSubnet(c, 1, 1, "node1")
	id2 := suite.addSubnet(c, 2, 2, "node2")
	env := suite.makeEnviron()

	subnetsInfo, err := suite.makeEnviron().Subnets(suite.callCtx, instance.UnknownId, []corenetwork.Id{})
	c.Assert(err, jc.ErrorIsNil)
	expectedInfo := []corenetwork.SubnetInfo{
		createSubnetInfo(id1, 2, 1),
		createSubnetInfo(id2, 3, 2),
	}
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)

	subnetsInfo, err = env.Subnets(suite.callCtx, instance.UnknownId, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnetsInfo, jc.DeepEquals, expectedInfo)
}

func (suite *environSuite) TestSpaces(c *gc.C) {
	suite.createTwoSpaces()
	suite.testMAASObject.TestServer.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: "space-3"}))
	for _, i := range []uint{1, 2, 3} {
		suite.addSubnet(c, i, i, "node1")
		suite.addSubnet(c, i+5, i, "node1")
	}

	spaces, err := suite.makeEnviron().Spaces(suite.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	expectedSpaces := []corenetwork.SpaceInfo{{
		Name:       "space-1",
		ProviderId: "2",
		Subnets: []corenetwork.SubnetInfo{
			createSubnetInfo(1, 2, 1),
			createSubnetInfo(2, 2, 6),
		},
	}, {
		Name:       "space-2",
		ProviderId: "3",
		Subnets: []corenetwork.SubnetInfo{
			createSubnetInfo(3, 3, 2),
			createSubnetInfo(4, 3, 7),
		},
	}, {
		Name:       "space-3",
		ProviderId: "4",
		Subnets: []corenetwork.SubnetInfo{
			createSubnetInfo(5, 4, 3),
			createSubnetInfo(6, 4, 8),
		},
	}}
	c.Assert(spaces, jc.DeepEquals, expectedSpaces)
}

func (suite *environSuite) assertSpaces(c *gc.C, numberOfSubnets int, filters []corenetwork.Id) {
	server := suite.testMAASObject.TestServer
	testInstance := suite.getInstance("node1")
	systemID := "node1"
	for i := 1; i <= numberOfSubnets; i++ {
		server.NewSpace(spaceJSON(gomaasapi.CreateSpace{Name: fmt.Sprintf("space-%d", i)}))
		// Put most, but not all, of the subnets on node1.
		if i == 2 {
			systemID = "node2"
		} else {
			systemID = "node1"
		}
		suite.addSubnet(c, uint(i), uint(i), systemID)
	}

	subnets, err := suite.makeEnviron().Subnets(suite.callCtx, testInstance.Id(), filters)
	c.Assert(err, jc.ErrorIsNil)
	expectedSubnets := []corenetwork.SubnetInfo{
		createSubnetInfo(1, 2, 1),
		createSubnetInfo(3, 4, 3),
	}
	c.Assert(subnets, jc.DeepEquals, expectedSubnets)

}

func (suite *environSuite) TestSubnetsAllSubnets(c *gc.C) {
	suite.assertSpaces(c, 3, []corenetwork.Id{})
}

func (suite *environSuite) TestSubnetsFilteredIds(c *gc.C) {
	suite.assertSpaces(c, 4, []corenetwork.Id{"1", "3"})
}

func (suite *environSuite) TestSubnetsMissingSubnet(c *gc.C) {
	testInstance := suite.getInstance("node1")
	for _, i := range []uint{1, 2} {
		suite.addSubnet(c, i, i, "node1")
	}

	_, err := suite.makeEnviron().Subnets(suite.callCtx, testInstance.Id(), []corenetwork.Id{"1", "3", "6"})
	errorRe := regexp.MustCompile("failed to find the following subnets: (\\d), (\\d)$")
	errorText := err.Error()
	c.Assert(errorRe.MatchString(errorText), jc.IsTrue)
	matches := errorRe.FindStringSubmatch(errorText)
	c.Assert(matches, gc.HasLen, 3)
	c.Assert(matches[1:], jc.SameContents, []string{"3", "6"})
}

func (s *environSuite) TestPrecheckInstanceAvailZone(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.SupportedLTS(), Placement: "zone=zone1"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestPrecheckInstanceAvailZoneUnknown(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.SupportedLTS(), Placement: "zone=zone2"})
	c.Assert(err, gc.ErrorMatches, `availability zone "zone2" not valid`)
}

func (s *environSuite) TestPrecheckInstanceAvailZonesUnsupported(c *gc.C) {
	env := s.makeEnviron()
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.SupportedLTS(), Placement: "zone=test-unknown"})
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *environSuite) TestPrecheckInvalidPlacement(c *gc.C) {
	env := s.makeEnviron()
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.SupportedLTS(), Placement: "notzone=anything"})
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: notzone=anything")
}

func (s *environSuite) TestPrecheckNodePlacement(c *gc.C) {
	env := s.makeEnviron()
	err := env.PrecheckInstance(s.callCtx, environs.PrecheckInstanceParams{Series: jujuversion.SupportedLTS(), Placement: "assumed_node_name"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestDeriveAvailabilityZones(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	zones, err := env.DeriveAvailabilityZones(s.callCtx, environs.StartInstanceParams{Placement: "zone=zone1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.DeepEquals, []string{"zone1"})
}

func (s *environSuite) TestDeriveAvailabilityZonesUnknown(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	zones, err := env.DeriveAvailabilityZones(s.callCtx, environs.StartInstanceParams{Placement: "zone=zone2"})
	c.Assert(err, gc.ErrorMatches, `availability zone "zone2" not valid`)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environSuite) TestDeriveAvailabilityZonesInvalidPlacement(c *gc.C) {
	env := s.makeEnviron()
	zones, err := env.DeriveAvailabilityZones(s.callCtx, environs.StartInstanceParams{Placement: "notzone=anything"})
	c.Assert(err, gc.ErrorMatches, "unknown placement directive: notzone=anything")
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environSuite) TestDeriveAvailabilityZonesNoPlacement(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("zone1", "the grass is greener in zone1")
	env := s.makeEnviron()
	zones, err := env.DeriveAvailabilityZones(s.callCtx, environs.StartInstanceParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 0)
}

func (s *environSuite) TestStartInstanceAvailZone(c *gc.C) {
	// Add a node for the started instance.
	s.newNode(c, "thenode1", "host1", map[string]interface{}{"zone": "test-available"})
	s.addSubnet(c, 1, 1, "thenode1")
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	inst, err := s.testStartInstanceAvailZone(c, "test-available")
	c.Assert(err, jc.ErrorIsNil)
	zone, err := inst.(maasInstance).zone()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zone, gc.Equals, "test-available")
}

func (s *environSuite) TestStartInstanceAvailZoneUnknown(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	_, err := s.testStartInstanceAvailZone(c, "test-unknown")
	c.Assert(err, gc.Not(jc.Satisfies), environs.IsAvailabilityZoneIndependent)
}

func (s *environSuite) testStartInstanceAvailZone(c *gc.C, zone string) (instances.Instance, error) {
	env := s.bootstrap(c)
	params := environs.StartInstanceParams{ControllerUUID: s.controllerUUID, AvailabilityZone: zone}
	result, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
	if err != nil {
		return nil, err
	}
	return result.Instance, nil
}

func (s *environSuite) TestStartInstanceZoneIndependentError(c *gc.C) {
	s.testMAASObject.TestServer.AddZone("test-available", "description")
	env := s.bootstrap(c)
	params := environs.StartInstanceParams{
		ControllerUUID: s.controllerUUID,
		Placement:      "foo=bar",
	}
	_, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
	c.Assert(err, jc.Satisfies, environs.IsAvailabilityZoneIndependent)
}

func (s *environSuite) TestStartInstanceUnmetConstraints(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", nil)
	s.addSubnet(c, 1, 1, "thenode1")
	params := environs.StartInstanceParams{ControllerUUID: s.controllerUUID, Constraints: constraints.MustParse("mem=8G")}
	_, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
	c.Assert(err, gc.ErrorMatches, "failed to acquire node: .* 409.*")
}

func (s *environSuite) TestStartInstanceConstraints(c *gc.C) {
	env := s.bootstrap(c)
	s.newNode(c, "thenode1", "host1", nil)
	s.addSubnet(c, 1, 1, "thenode1")
	s.newNode(c, "thenode2", "host2", map[string]interface{}{"memory": 8192})
	s.addSubnet(c, 2, 2, "thenode2")
	params := environs.StartInstanceParams{ControllerUUID: s.controllerUUID, Constraints: constraints.MustParse("mem=8G")}
	result, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
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
	s.addSubnet(c, 1, 1, "thenode1")
	params := environs.StartInstanceParams{ControllerUUID: s.controllerUUID,
		Volumes: []storage.VolumeParams{
			{Tag: names.NewVolumeTag("1"), Size: 2000000},
			{Tag: names.NewVolumeTag("3"), Size: 2000000},
		}}
	result, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
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
	s.addSubnet(c, 1, 1, "thenode1")
	params := environs.StartInstanceParams{ControllerUUID: s.controllerUUID,
		Volumes: []storage.VolumeParams{
			{Tag: names.NewVolumeTag("1"), Size: 2000000},
			{Tag: names.NewVolumeTag("3"), Size: 2000000},
		}}
	_, err := testing.StartInstanceWithParams(env, s.callCtx, "1", params)
	c.Assert(err, gc.ErrorMatches, "requested 2 storage volumes. 0 returned")
	operations := s.testMAASObject.TestServer.NodesOperations()
	c.Check(operations, gc.DeepEquals, []string{"acquire", "acquire", "release"})
	c.Assert(s.testMAASObject.TestServer.OwnedNodes()["node0"], jc.IsTrue)
	c.Assert(s.testMAASObject.TestServer.OwnedNodes()["thenode1"], jc.IsFalse)
}

func (s *environSuite) TestGetAvailabilityZones(c *gc.C) {
	env := s.makeEnviron()

	zones, err := env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
	c.Assert(zones, gc.IsNil)

	s.testMAASObject.TestServer.AddZone("whatever", "andever")
	zones, err = env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
	c.Assert(zones[0].Available(), jc.IsTrue)

	// A successful result is cached, currently for the lifetime
	// of the Environ. This will change if/when we have long-lived
	// Environs to cut down repeated IaaS requests.
	s.testMAASObject.TestServer.AddZone("somewhere", "outthere")
	zones, err = env.AvailabilityZones(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(zones, gc.HasLen, 1)
	c.Assert(zones[0].Name(), gc.Equals, "whatever")
}

func (s *environSuite) newNode(c *gc.C, nodename, hostname string, attrs map[string]interface{}) {
	allAttrs := map[string]interface{}{
		"system_id":     nodename,
		"hostname":      hostname,
		"architecture":  fmt.Sprintf("%s/generic", arch.HostArch()),
		"memory":        1024,
		"cpu_count":     1,
		"zone":          map[string]interface{}{"name": "test_zone", "description": "description"},
		"interface_set": exampleParsedInterfaceSetJSON,
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
	s.addSubnet(c, 9, 9, "node0")
	s.setupFakeTools(c)
	env := s.makeEnviron()
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callCtx, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			Placement:            "bootstrap-host",
			AdminSecret:          testing.AdminSecret,
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: constraints.MustParse("mem=1G"),
		})
	c.Assert(err, jc.ErrorIsNil)
	return env
}

func (s *environSuite) TestReleaseContainerAddresses(c *gc.C) {
	s.testMAASObject.TestServer.AddDevice(&gomaasapi.TestDevice{
		SystemId:     "device1",
		MACAddresses: []string{"mac1"},
	})
	s.testMAASObject.TestServer.AddDevice(&gomaasapi.TestDevice{
		SystemId:     "device2",
		MACAddresses: []string{"mac2"},
	})
	s.testMAASObject.TestServer.AddDevice(&gomaasapi.TestDevice{
		SystemId:     "device3",
		MACAddresses: []string{"mac3"},
	})

	env := s.makeEnviron()
	err := env.ReleaseContainerAddresses(s.callCtx,
		[]network.ProviderInterfaceInfo{
			{MACAddress: "mac1"},
			{MACAddress: "mac3"},
			{MACAddress: "mac4"},
		})
	c.Assert(err, jc.ErrorIsNil)

	var systemIds []string
	for systemId := range s.testMAASObject.TestServer.Devices() {
		systemIds = append(systemIds, systemId)
	}
	c.Assert(systemIds, gc.DeepEquals, []string{"device2"})
}

func (s *environSuite) TestReleaseContainerAddresses_HandlesDupes(c *gc.C) {
	s.testMAASObject.TestServer.AddDevice(&gomaasapi.TestDevice{
		SystemId:     "device1",
		MACAddresses: []string{"mac1", "mac2"},
	})
	s.testMAASObject.TestServer.AddDevice(&gomaasapi.TestDevice{
		SystemId:     "device3",
		MACAddresses: []string{"mac3"},
	})

	env := s.makeEnviron()
	err := env.ReleaseContainerAddresses(s.callCtx,
		[]network.ProviderInterfaceInfo{
			{MACAddress: "mac1"},
			{MACAddress: "mac2"},
		})
	c.Assert(err, jc.ErrorIsNil)

	var systemIds []string
	for systemId := range s.testMAASObject.TestServer.Devices() {
		systemIds = append(systemIds, systemId)
	}
	c.Assert(systemIds, gc.DeepEquals, []string{"device3"})
}

func (s *environSuite) TestAdoptResources(c *gc.C) {
	s.addNode(allocatedNode)
	env := s.makeEnviron()
	// Shouldn't do anything in MAAS1.
	err := env.AdoptResources(s.callCtx, "other-controller", version.MustParse("3.2.1"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestUsingNonVersionURLForAPI(c *gc.C) {
	var gotURL *url.URL
	configuredURL := s.testMAASObject.TestServer.URL
	_, err := s.makeEnvironWithURL(
		configuredURL,
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			gotURL = client.URL()
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL.String(), gc.Equals, configuredURL+"/api/1.0/")
}

func (s *environSuite) TestUsingVersionURLForAPI(c *gc.C) {
	var gotURL *url.URL
	configuredURL := s.testMAASObject.TestServer.URL + "/api/1.0/"
	_, err := s.makeEnvironWithURL(
		configuredURL,
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			gotURL = client.URL()
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL.String(), gc.Equals, configuredURL)
}

func (s *environSuite) TestUsingUnknownVersionURLForAPI(c *gc.C) {
	var gotURL *url.URL
	configuredURL := s.testMAASObject.TestServer.URL + "/api/3.0/"
	_, err := s.makeEnvironWithURL(
		configuredURL,
		func(client *gomaasapi.MAASObject, serverURL string) (set.Strings, error) {
			gotURL = client.URL()
			return set.NewStrings("network-deployment-ubuntu"), nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURL.String(), gc.Equals, configuredURL)
}
