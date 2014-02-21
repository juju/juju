// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/url"

	gc "launchpad.net/gocheck"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtesting "launchpad.net/juju-core/environs/testing"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
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

func (suite *environSuite) setupFakeProviderStateFile(c *gc.C) {
	suite.testMAASObject.TestServer.NewFile(bootstrap.StateFile, []byte("test file content"))
}

func (suite *environSuite) setupFakeTools(c *gc.C) {
	stor := NewStorage(suite.makeEnviron())
	envtesting.UploadFakeTools(c, stor)
}

func (suite *environSuite) addNode(jsonText string) instance.Id {
	node := suite.testMAASObject.TestServer.NewNode(jsonText)
	resourceURI, _ := node.GetField("resource_uri")
	return instance.Id(resourceURI)
}

func (suite *environSuite) TestInstancesReturnsInstances(c *gc.C) {
	id := suite.addNode(allocatedNode)
	instances, err := suite.makeEnviron().Instances([]instance.Id{id})

	c.Check(err, gc.IsNil)
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

	c.Check(err, gc.IsNil)
	c.Assert(instances, gc.HasLen, 1)
	c.Assert(instances[0].Id(), gc.Equals, id)
}

func (suite *environSuite) TestAllInstancesReturnsEmptySliceIfNoInstance(c *gc.C) {
	instances, err := suite.makeEnviron().AllInstances()

	c.Check(err, gc.IsNil)
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
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	// The bootstrap node has been acquired and started.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Check(found, gc.Equals, true)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// Test the instance id is correctly recorded for the bootstrap node.
	// Check that the state holds the id of the bootstrap machine.
	stateData, err := bootstrap.LoadState(env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)
	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, stateData.StateInstances[0])

	// Create node 1: it will be used as instance number 1.
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node1", "hostname": "host1"}`)
	// TODO(wallyworld) - test instance metadata
	instance, _ := testing.AssertStartInstance(c, env, "1")
	c.Assert(err, gc.IsNil)
	c.Check(instance, gc.NotNil)

	// The instance number 1 has been acquired and started.
	actions, found = operations["node1"]
	c.Assert(found, gc.Equals, true)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// The value of the "user data" parameter used when starting the node
	// contains the run cmd used to write the machine information onto
	// the node's filesystem.
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node1"]
	c.Assert(found, gc.Equals, true)
	c.Assert(len(nodeRequestValues), gc.Equals, 2)
	userData := nodeRequestValues[1].Get("user_data")
	decodedUserData, err := decodeUserData(userData)
	c.Assert(err, gc.IsNil)
	info := machineInfo{"host1"}
	cloudinitRunCmd, err := info.cloudinitRunCmd()
	c.Assert(err, gc.IsNil)
	data, err := goyaml.Marshal(cloudinitRunCmd)
	c.Assert(err, gc.IsNil)
	c.Check(string(decodedUserData), gc.Matches, "(.|\n)*"+string(data)+"(\n|.)*")

	// Trash the tools and try to start another instance.
	envtesting.RemoveTools(c, env.Storage())
	instance, _, err = testing.StartInstance(env, "2")
	c.Check(instance, gc.IsNil)
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func stringp(val string) *string {
	return &val
}

func (suite *environSuite) TestAcquireNode(c *gc.C) {
	stor := NewStorage(suite.makeEnviron())
	fakeTools := envtesting.MustUploadFakeToolsVersions(stor, version.Current)[0]
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)

	_, _, err := env.acquireNode(constraints.Value{}, tools.List{fakeTools})

	c.Check(err, gc.IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Assert(found, gc.Equals, true)
	c.Check(actions, gc.DeepEquals, []string{"acquire"})
}

func (suite *environSuite) TestAcquireNodeTakesConstraintsIntoAccount(c *gc.C) {
	stor := NewStorage(suite.makeEnviron())
	fakeTools := envtesting.MustUploadFakeToolsVersions(stor, version.Current)[0]
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	constraints := constraints.Value{Arch: stringp("arm"), Mem: uint64p(1024)}

	_, _, err := env.acquireNode(constraints, tools.List{fakeTools})

	c.Check(err, gc.IsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, gc.Equals, true)
	c.Assert(nodeRequestValues[0].Get("arch"), gc.Equals, "arm")
	c.Assert(nodeRequestValues[0].Get("mem"), gc.Equals, "1024")
}

func (suite *environSuite) TestAcquireNodePassedAgentName(c *gc.C) {
	stor := NewStorage(suite.makeEnviron())
	fakeTools := envtesting.MustUploadFakeToolsVersions(stor, version.Current)[0]
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)

	_, _, err := env.acquireNode(constraints.Value{}, tools.List{fakeTools})

	c.Check(err, gc.IsNil)
	requestValues := suite.testMAASObject.TestServer.NodeOperationRequestValues()
	nodeRequestValues, found := requestValues["node0"]
	c.Assert(found, gc.Equals, true)
	c.Assert(nodeRequestValues[0].Get("agent_name"), gc.Equals, exampleAgentName)
}

func (*environSuite) TestConvertConstraints(c *gc.C) {
	var testValues = []struct {
		constraints    constraints.Value
		expectedResult url.Values
	}{
		{constraints.Value{Arch: stringp("arm")}, url.Values{"arch": {"arm"}}},
		{constraints.Value{CpuCores: uint64p(4)}, url.Values{"cpu_count": {"4"}}},
		{constraints.Value{Mem: uint64p(1024)}, url.Values{"mem": {"1024"}}},
		// CpuPower is ignored.
		{constraints.Value{CpuPower: uint64p(1024)}, url.Values{}},
		// RootDisk is ignored.
		{constraints.Value{RootDisk: uint64p(8192)}, url.Values{}},
		{constraints.Value{Tags: &[]string{"foo", "bar"}}, url.Values{"tags": {"foo,bar"}}},
		{constraints.Value{Arch: stringp("arm"), CpuCores: uint64p(4), Mem: uint64p(1024), CpuPower: uint64p(1024), RootDisk: uint64p(8192), Tags: &[]string{"foo", "bar"}}, url.Values{"arch": {"arm"}, "cpu_count": {"4"}, "mem": {"1024"}, "tags": {"foo,bar"}}},
	}
	for _, test := range testValues {
		c.Check(convertConstraints(test.constraints), gc.DeepEquals, test.expectedResult)
	}
}

func (suite *environSuite) getInstance(systemId string) *maasInstance {
	input := `{"system_id": "` + systemId + `"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	return &maasInstance{maasObject: &node, environ: suite.makeEnviron()}
}

func (suite *environSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	suite.getInstance("test1")

	err := suite.makeEnviron().StopInstances([]instance.Instance{})
	c.Check(err, gc.IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	c.Check(operations, gc.DeepEquals, map[string][]string{})
}

func (suite *environSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	instance1 := suite.getInstance("test1")
	instance2 := suite.getInstance("test2")
	suite.getInstance("test3")
	instances := []instance.Instance{instance1, instance2}

	err := suite.makeEnviron().StopInstances(instances)

	c.Check(err, gc.IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"release"}, "test2": {"release"}}
	c.Check(operations, gc.DeepEquals, expectedOperations)
}

func (suite *environSuite) TestStateInfo(c *gc.C) {
	env := suite.makeEnviron()
	hostname := "test"
	input := `{"system_id": "system_id", "hostname": "` + hostname + `"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	testInstance := &maasInstance{maasObject: &node, environ: suite.makeEnviron()}
	err := bootstrap.SaveState(
		env.Storage(),
		&bootstrap.BootstrapState{StateInstances: []instance.Id{testInstance.Id()}})
	c.Assert(err, gc.IsNil)

	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)

	cfg := env.Config()
	statePortSuffix := fmt.Sprintf(":%d", cfg.StatePort())
	apiPortSuffix := fmt.Sprintf(":%d", cfg.APIPort())
	c.Assert(stateInfo.Addrs, gc.DeepEquals, []string{hostname + statePortSuffix})
	c.Assert(apiInfo.Addrs, gc.DeepEquals, []string{hostname + apiPortSuffix})
}

func (suite *environSuite) TestStateInfoFailsIfNoStateInstances(c *gc.C) {
	env := suite.makeEnviron()

	_, _, err := env.StateInfo()

	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
}

func (suite *environSuite) TestDestroy(c *gc.C) {
	env := suite.makeEnviron()
	suite.getInstance("test1")
	data := makeRandomBytes(10)
	suite.testMAASObject.TestServer.NewFile("filename", data)
	stor := env.Storage()

	err := env.Destroy()
	c.Check(err, gc.IsNil)

	// Instances have been stopped.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"release"}}
	c.Check(operations, gc.DeepEquals, expectedOperations)
	// Files have been cleaned up.
	listing, err := storage.List(stor, "")
	c.Assert(err, gc.IsNil)
	c.Check(listing, gc.DeepEquals, []string{})
}

// It would be nice if we could unit-test Bootstrap() in more detail, but
// at the time of writing that would require more support from gomaasapi's
// testing service than we have.
func (suite *environSuite) TestBootstrapSucceeds(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "thenode", "hostname": "host"}`)
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (suite *environSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	// Can't RemoveAllTools, no public storage.
	envtesting.RemoveTools(c, env.Storage())
	// Disable auto-uploading by setting the agent version.
	cfg, err := env.Config().Apply(map[string]interface{}{
		"agent-version": version.Current.Number.String(),
	})
	c.Assert(err, gc.IsNil)
	err = env.SetConfig(cfg)
	c.Assert(err, gc.IsNil)
	err = bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Check(err, gc.ErrorMatches, "cannot find bootstrap tools.*")
}

func (suite *environSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, gc.ErrorMatches, ".*409.*")
}

func (suite *environSuite) TestBootstrapIntegratesWithEnvirons(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "bootstrapnode", "hostname": "host"}`)

	// bootstrap.Bootstrap calls Environ.Bootstrap.  This works.
	err := bootstrap.Bootstrap(coretesting.Context(c), env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func assertSourceContents(c *gc.C, source simplestreams.DataSource, filename string, content []byte) {
	rc, _, err := source.Fetch(filename)
	c.Assert(err, gc.IsNil)
	defer rc.Close()
	retrieved, err := ioutil.ReadAll(rc)
	c.Assert(err, gc.IsNil)
	c.Assert(retrieved, gc.DeepEquals, content)
}

func (suite *environSuite) assertGetImageMetadataSources(c *gc.C, stream, officialSourcePath string) {
	// Make an env configured with the stream.
	testAttrs := maasEnvAttrs
	testAttrs = testAttrs.Merge(coretesting.Attrs{
		"maas-server": suite.testMAASObject.TestServer.URL,
	})
	if stream != "" {
		testAttrs = testAttrs.Merge(coretesting.Attrs{
			"image-stream": stream,
		})
	}
	attrs := coretesting.FakeConfig().Merge(testAttrs)
	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	env, err := NewEnviron(cfg)
	c.Assert(err, gc.IsNil)

	// Add a dummy file to storage so we can use that to check the
	// obtained source later.
	data := makeRandomBytes(10)
	stor := NewStorage(env)
	err = stor.Put("images/filename", bytes.NewBuffer([]byte(data)), int64(len(data)))
	c.Assert(err, gc.IsNil)
	sources, err := imagemetadata.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 2)
	assertSourceContents(c, sources[0], "filename", data)
	url, err := sources[1].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.Equals, fmt.Sprintf("http://cloud-images.ubuntu.com/%s/", officialSourcePath))
}

func (suite *environSuite) TestGetImageMetadataSources(c *gc.C) {
	suite.assertGetImageMetadataSources(c, "", "releases")
	suite.assertGetImageMetadataSources(c, "released", "releases")
	suite.assertGetImageMetadataSources(c, "daily", "daily")
}

func (suite *environSuite) TestGetToolsMetadataSources(c *gc.C) {
	env := suite.makeEnviron()
	// Add a dummy file to storage so we can use that to check the
	// obtained source later.
	data := makeRandomBytes(10)
	stor := NewStorage(env)
	err := stor.Put("tools/filename", bytes.NewBuffer([]byte(data)), int64(len(data)))
	c.Assert(err, gc.IsNil)
	sources, err := envtools.GetMetadataSources(env)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	assertSourceContents(c, sources[0], "filename", data)
}
