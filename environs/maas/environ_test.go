// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"encoding/base64"
	"fmt"
	"net/url"

	gc "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/goyaml"

	"launchpad.net/juju-core/agent/tools"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

type environSuite struct {
	providerSuite
}

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

// makeEnviron creates a functional maasEnviron for a test.  Its configuration
// is a bit arbitrary and none of the test code's business.
func (suite *environSuite) makeEnviron() *maasEnviron {
	config, err := config.New(map[string]interface{}{
		"name":            suite.environ.Name(),
		"type":            "maas",
		"admin-secret":    "local-secret",
		"authorized-keys": "foo",
		"agent-version":   version.CurrentNumber().String(),
		"maas-oauth":      "a:b:c",
		"maas-server":     suite.testMAASObject.TestServer.URL,
		// These are not needed by MAAS, but juju-core breaks without them. Needs
		// fixing there.
		"ca-cert":        testing.CACert,
		"ca-private-key": testing.CAKey,
	})
	if err != nil {
		panic(err)
	}
	env, err := NewEnviron(config)
	if err != nil {
		panic(err)
	}
	return env
}

func (suite *environSuite) setupFakeProviderStateFile(c *gc.C) {
	suite.testMAASObject.TestServer.NewFile(environs.StateFile, []byte("test file content"))
}

func (suite *environSuite) setupFakeTools(c *gc.C) {
	storage := NewStorage(suite.environ)
	envtesting.UploadFakeTools(c, storage)
}

func (*environSuite) TestSetConfigValidatesFirst(c *gc.C) {
	// SetConfig() validates the config change and disallows, for example,
	// changes in the environment name.
	server := "http://maas.testing.invalid"
	oauth := "a:b:c"
	secret := "pssst"
	oldCfg := getTestConfig("old-name", server, oauth, secret)
	newCfg := getTestConfig("new-name", server, oauth, secret)
	env, err := NewEnviron(oldCfg)
	c.Assert(err, gc.IsNil)

	// SetConfig() fails, even though both the old and the new config are
	// individually valid.
	err = env.SetConfig(newCfg)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, ".*cannot change name.*")

	// The old config is still in place.  The new config never took effect.
	c.Check(env.Name(), gc.Equals, "old-name")
}

func (*environSuite) TestSetConfigUpdatesConfig(c *gc.C) {
	name := "test env"
	cfg := getTestConfig(name, "http://maas2.testing.invalid", "a:b:c", "secret")
	env, err := NewEnviron(cfg)
	c.Check(err, gc.IsNil)
	c.Check(env.name, gc.Equals, "test env")

	anotherServer := "http://maas.testing.invalid"
	anotherOauth := "c:d:e"
	anotherSecret := "secret2"
	cfg2 := getTestConfig(name, anotherServer, anotherOauth, anotherSecret)
	errSetConfig := env.SetConfig(cfg2)
	c.Check(errSetConfig, gc.IsNil)
	c.Check(env.name, gc.Equals, name)
	authClient, _ := gomaasapi.NewAuthenticatedClient(anotherServer, anotherOauth, apiVersion)
	maas := gomaasapi.NewMAAS(*authClient)
	MAASServer := env.maasClientUnlocked
	c.Check(MAASServer, gc.DeepEquals, maas)
}

func (*environSuite) TestNewEnvironSetsConfig(c *gc.C) {
	name := "test env"
	cfg := getTestConfig(name, "http://maas.testing.invalid", "a:b:c", "secret")

	env, err := NewEnviron(cfg)

	c.Check(err, gc.IsNil)
	c.Check(env.name, gc.Equals, name)
}

func (suite *environSuite) TestInstancesReturnsInstances(c *gc.C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")
	instanceIds := []instance.Id{instance.Id(resourceURI)}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(string(instances[0].Id()), gc.Equals, resourceURI)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfEmptyParameter(c *gc.C) {
	input := `{"system_id": "test"}`
	suite.testMAASObject.TestServer.NewNode(input)
	instances, err := suite.environ.Instances([]instance.Id{})

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNilParameter(c *gc.C) {
	input := `{"system_id": "test"}`
	suite.testMAASObject.TestServer.NewNode(input)
	instances, err := suite.environ.Instances(nil)

	c.Check(err, gc.Equals, environs.ErrNoInstances)
	c.Check(instances, gc.IsNil)
}

func (suite *environSuite) TestInstancesReturnsErrNoInstancesIfNoneFound(c *gc.C) {
	_, err := suite.environ.Instances([]instance.Id{"unknown"})
	c.Check(err, gc.Equals, environs.ErrNoInstances)
}

func (suite *environSuite) TestAllInstancesReturnsAllInstances(c *gc.C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")

	instances, err := suite.environ.AllInstances()

	c.Check(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(string(instances[0].Id()), gc.Equals, resourceURI)
}

func (suite *environSuite) TestAllInstancesReturnsEmptySliceIfNoInstance(c *gc.C) {
	instances, err := suite.environ.AllInstances()

	c.Check(err, gc.IsNil)
	c.Check(len(instances), gc.Equals, 0)
}

func (suite *environSuite) TestInstancesReturnsErrorIfPartialInstances(c *gc.C) {
	input1 := `{"system_id": "test"}`
	node1 := suite.testMAASObject.TestServer.NewNode(input1)
	resourceURI1, _ := node1.GetField("resource_uri")
	input2 := `{"system_id": "test2"}`
	suite.testMAASObject.TestServer.NewNode(input2)
	instanceId1 := instance.Id(resourceURI1)
	instanceId2 := instance.Id("unknown systemID")
	instanceIds := []instance.Id{instanceId1, instanceId2}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, gc.Equals, environs.ErrPartialInstances)
	c.Check(len(instances), gc.Equals, 1)
	c.Check(string(instances[0].Id()), gc.Equals, resourceURI1)
}

func (suite *environSuite) TestStorageReturnsStorage(c *gc.C) {
	env := suite.makeEnviron()
	storage := env.Storage()
	c.Check(storage, gc.NotNil)
	// The Storage object is really a maasStorage.
	specificStorage := storage.(*maasStorage)
	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environUnlocked, gc.Equals, env)
}

func (suite *environSuite) TestPublicStorageReturnsEmptyStorage(c *gc.C) {
	env := suite.makeEnviron()
	storage := env.PublicStorage()
	c.Assert(storage, gc.NotNil)
	c.Check(storage, gc.Equals, environs.EmptyStorage)
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
	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	// The bootstrap node has been acquired and started.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Check(found, gc.Equals, true)
	c.Check(actions, gc.DeepEquals, []string{"acquire", "start"})

	// Test the instance id is correctly recorded for the bootstrap node.
	// Check that the state holds the id of the bootstrap machine.
	stateData, err := environs.LoadState(env.Storage())
	c.Assert(err, gc.IsNil)
	c.Assert(stateData.StateInstances, gc.HasLen, 1)
	insts, err := env.AllInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(insts, gc.HasLen, 1)
	c.Check(insts[0].Id(), gc.Equals, stateData.StateInstances[0])

	// Create node 1: it will be used as instance number 1.
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node1", "hostname": "host1"}`)
	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)
	stateInfo.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	series := version.Current.Series
	nonce := "12345"
	// TODO(wallyworld) - test instance metadata
	instance, _, err := env.StartInstance("1", nonce, series, constraints.Value{}, stateInfo, apiInfo)
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
	instance, _, err = env.StartInstance("2", "fake-nonce", series, constraints.Value{}, stateInfo, apiInfo)
	c.Check(instance, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "no tools available")
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func uint64p(val uint64) *uint64 {
	return &val
}

func stringp(val string) *string {
	return &val
}

func (suite *environSuite) TestAcquireNode(c *gc.C) {
	storage := NewStorage(suite.environ)
	fakeTools := envtesting.MustUploadFakeToolsVersion(storage, version.Current)
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
	storage := NewStorage(suite.environ)
	fakeTools := envtesting.MustUploadFakeToolsVersion(storage, version.Current)
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
		{constraints.Value{Arch: stringp("arm"), CpuCores: uint64p(4), Mem: uint64p(1024), CpuPower: uint64p(1024)}, url.Values{"arch": {"arm"}, "cpu_count": {"4"}, "mem": {"1024"}}},
	}
	for _, test := range testValues {
		c.Check(convertConstraints(test.constraints), gc.DeepEquals, test.expectedResult)
	}
}

func (suite *environSuite) getInstance(systemId string) *maasInstance {
	input := `{"system_id": "` + systemId + `"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	return &maasInstance{&node, suite.environ}
}

func (suite *environSuite) TestStopInstancesReturnsIfParameterEmpty(c *gc.C) {
	suite.getInstance("test1")

	err := suite.environ.StopInstances([]instance.Instance{})
	c.Check(err, gc.IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	c.Check(operations, gc.DeepEquals, map[string][]string{})
}

func (suite *environSuite) TestStopInstancesStopsAndReleasesInstances(c *gc.C) {
	instance1 := suite.getInstance("test1")
	instance2 := suite.getInstance("test2")
	suite.getInstance("test3")
	instances := []instance.Instance{instance1, instance2}

	err := suite.environ.StopInstances(instances)

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
	testInstance := &maasInstance{&node, suite.environ}
	err := environs.SaveState(
		env.Storage(),
		&environs.BootstrapState{StateInstances: []instance.Id{testInstance.Id()}})
	c.Assert(err, gc.IsNil)

	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, gc.IsNil)

	config := env.Config()
	statePortSuffix := fmt.Sprintf(":%d", config.StatePort())
	apiPortSuffix := fmt.Sprintf(":%d", config.APIPort())
	c.Assert(stateInfo.Addrs, gc.DeepEquals, []string{hostname + statePortSuffix})
	c.Assert(apiInfo.Addrs, gc.DeepEquals, []string{hostname + apiPortSuffix})
}

func (suite *environSuite) TestStateInfoFailsIfNoStateInstances(c *gc.C) {
	env := suite.makeEnviron()

	_, _, err := env.StateInfo()

	c.Check(err, jc.Satisfies, errors.IsNotBootstrapped)
}

func (suite *environSuite) TestDestroy(c *gc.C) {
	env := suite.makeEnviron()
	suite.getInstance("test1")
	testInstance := suite.getInstance("test2")
	data := makeRandomBytes(10)
	suite.testMAASObject.TestServer.NewFile("filename", data)
	storage := env.Storage()

	err := env.Destroy([]instance.Instance{testInstance})

	c.Check(err, gc.IsNil)
	// Instances have been stopped.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"release"}, "test2": {"release"}}
	c.Check(operations, gc.DeepEquals, expectedOperations)
	// Files have been cleaned up.
	listing, err := storage.List("")
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
	err := env.Bootstrap(constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (suite *environSuite) TestBootstrapFailsIfNoTools(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	// Can't RemoveAllTools, no public storage.
	envtesting.RemoveTools(c, env.Storage())
	err := env.Bootstrap(constraints.Value{})
	c.Check(err, gc.ErrorMatches, "no tools available")
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func (suite *environSuite) TestBootstrapFailsIfNoNodes(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	err := env.Bootstrap(constraints.Value{})
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, gc.ErrorMatches, ".*409.*")
}

func (suite *environSuite) TestBootstrapIntegratesWithEnvirons(c *gc.C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "bootstrapnode", "hostname": "host"}`)

	// environs.Bootstrap calls Environ.Bootstrap.  This works.
	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}
