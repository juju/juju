package maas

import (
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

type EnvironSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironSuite))

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
func (suite *EnvironSuite) makeEnviron() *maasEnviron {
	config, err := config.New(map[string]interface{}{
		"name":            suite.environ.Name(),
		"type":            "maas",
		"admin-secret":    "local-secret",
		"authorized-keys": "foo",
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
		"maas-oauth":      "a:b:c",
		"maas-server":     suite.testMAASObject.TestServer.URL,
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

func (suite *EnvironSuite) setupFakeProviderStateFile(c *C) {
	suite.testMAASObject.TestServer.NewFile("provider-state", []byte("test file content"))
}

func (suite *EnvironSuite) setupFakeTools(c *C) {
	storage := NewStorage(suite.environ)
	envtesting.PutFakeTools(c, storage)
}

func (EnvironSuite) TestSetConfigUpdatesConfig(c *C) {
	cfg := getTestConfig("test env", "http://maas2.example.com", "a:b:c", "secret")
	env, err := NewEnviron(cfg)
	c.Check(err, IsNil)
	c.Check(env.name, Equals, "test env")

	anotherName := "another name"
	anotherServer := "http://maas.example.com"
	anotherOauth := "c:d:e"
	anotherSecret := "secret2"
	cfg2 := getTestConfig(anotherName, anotherServer, anotherOauth, anotherSecret)
	errSetConfig := env.SetConfig(cfg2)
	c.Check(errSetConfig, IsNil)
	c.Check(env.name, Equals, anotherName)
	authClient, _ := gomaasapi.NewAuthenticatedClient(anotherServer, anotherOauth, "1.0")
	maas := gomaasapi.NewMAAS(*authClient)
	MAASServer := env.maasClientUnlocked
	c.Check(MAASServer, DeepEquals, maas)
}

func (EnvironSuite) TestNewEnvironSetsConfig(c *C) {
	name := "test env"
	cfg := getTestConfig(name, "http://maas.example.com", "a:b:c", "secret")

	env, err := NewEnviron(cfg)

	c.Check(err, IsNil)
	c.Check(env.name, Equals, name)
}

func (suite *EnvironSuite) TestInstancesReturnsInstances(c *C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")
	instanceIds := []state.InstanceId{state.InstanceId(resourceURI)}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, IsNil)
	c.Check(len(instances), Equals, 1)
	c.Check(string(instances[0].Id()), Equals, resourceURI)
}

func (suite *EnvironSuite) TestInstancesReturnsNilIfEmptyParameter(c *C) {
	// Instances returns nil if the given parameter is empty.
	input := `{"system_id": "test"}`
	suite.testMAASObject.TestServer.NewNode(input)
	instances, err := suite.environ.Instances([]state.InstanceId{})

	c.Check(err, IsNil)
	c.Check(instances, IsNil)
}

func (suite *EnvironSuite) TestInstancesReturnsNilIfNilParameter(c *C) {
	// Instances returns nil if the given parameter is nil.
	input := `{"system_id": "test"}`
	suite.testMAASObject.TestServer.NewNode(input)
	instances, err := suite.environ.Instances(nil)

	c.Check(err, IsNil)
	c.Check(instances, IsNil)
}

func (suite *EnvironSuite) TestAllInstancesReturnsAllInstances(c *C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")

	instances, err := suite.environ.AllInstances()

	c.Check(err, IsNil)
	c.Check(len(instances), Equals, 1)
	c.Check(string(instances[0].Id()), Equals, resourceURI)
}

func (suite *EnvironSuite) TestAllInstancesReturnsEmptySliceIfNoInstance(c *C) {
	instances, err := suite.environ.AllInstances()

	c.Check(err, IsNil)
	c.Check(len(instances), Equals, 0)
}

func (suite *EnvironSuite) TestInstancesReturnsErrorIfPartialInstances(c *C) {
	input1 := `{"system_id": "test"}`
	node1 := suite.testMAASObject.TestServer.NewNode(input1)
	resourceURI1, _ := node1.GetField("resource_uri")
	input2 := `{"system_id": "test2"}`
	suite.testMAASObject.TestServer.NewNode(input2)
	instanceId1 := state.InstanceId(resourceURI1)
	instanceId2 := state.InstanceId("unknown systemID")
	instanceIds := []state.InstanceId{instanceId1, instanceId2}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, Equals, environs.ErrPartialInstances)
	c.Check(len(instances), Equals, 1)
	c.Check(string(instances[0].Id()), Equals, resourceURI1)
}

func (suite *EnvironSuite) TestStorageReturnsStorage(c *C) {
	env := suite.makeEnviron()
	storage := env.Storage()
	c.Check(storage, NotNil)
	// The Storage object is really a maasStorage.
	specificStorage := storage.(*maasStorage)
	// Its environment pointer refers back to its environment.
	c.Check(specificStorage.environUnlocked, Equals, env)
}

func (suite *EnvironSuite) TestPublicStorageReturnsEmptyStorage(c *C) {
	env := suite.makeEnviron()
	storage := env.PublicStorage()
	c.Assert(storage, NotNil)
	c.Check(storage, Equals, environs.EmptyStorage)
}

// fakeWriteCertAndKey is a stub for the writeCertAndKey to Bootstrap() that
// always returns an error.  It should never be called.
func fakeWriteCertAndKey(name string, cert, key []byte) error {
	return fmt.Errorf("unexpected call to writeCertAndKey")
}

func (suite *EnvironSuite) TestStartInstanceStartsInstance(c *C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	// Create node 0: it will be used as the bootstrap node.
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node0", "hostname": "host0"}`)
	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
	// The bootstrap node has been started.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["node0"]
	c.Check(found, Equals, true)
	c.Check(actions, DeepEquals, []string{"start"})

	// Create node 1: it will be used as instance number 1.
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "node1", "hostname": "host1"}`)
	stateInfo, apiInfo, err := env.StateInfo()
	c.Assert(err, IsNil)
	stateInfo.Tag = "machine-1"
	apiInfo.Tag = "machine-1"
	series := version.Current.Series
	instance, err := env.StartInstance("1", series, constraints.Value{}, stateInfo, apiInfo)
	c.Assert(err, IsNil)
	c.Check(instance, NotNil)

	// The instance number 1 has been started.
	actions, found = operations["node1"]
	c.Check(found, Equals, true)
	c.Check(actions, DeepEquals, []string{"start"})
}

func (suite *EnvironSuite) getInstance(systemId string) *maasInstance {
	input := `{"system_id": "` + systemId + `"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	return &maasInstance{&node, suite.environ}
}

func (suite *EnvironSuite) TestStopInstancesReturnsIfParameterEmpty(c *C) {
	suite.getInstance("test1")

	err := suite.environ.StopInstances([]environs.Instance{})
	c.Check(err, IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	c.Check(operations, DeepEquals, map[string][]string{})
}

func (suite *EnvironSuite) TestStopInstancesStopsAndReleasesInstances(c *C) {
	instance1 := suite.getInstance("test1")
	instance2 := suite.getInstance("test2")
	suite.getInstance("test3")
	instances := []environs.Instance{instance1, instance2}

	err := suite.environ.StopInstances(instances)

	c.Check(err, IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"release"}, "test2": {"release"}}
	c.Check(operations, DeepEquals, expectedOperations)
}

func (suite *EnvironSuite) TestQuiesceStateFileIsHappyWithoutStateFile(c *C) {
	err := suite.makeEnviron().quiesceStateFile()
	c.Check(err, IsNil)
}

func (suite *EnvironSuite) TestQuiesceStateFileFailsWithStateFile(c *C) {
	env := suite.makeEnviron()
	err := env.saveState(&bootstrapState{})
	c.Assert(err, IsNil)

	err = env.quiesceStateFile()

	c.Check(err, Not(IsNil))
}

func (suite *EnvironSuite) TestStateInfo(c *C) {
	env := suite.makeEnviron()
	hostname := "test"
	input := `{"system_id": "system_id", "hostname": "` + hostname + `"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	instance := &maasInstance{&node, suite.environ}
	err := env.saveState(&bootstrapState{StateInstances: []state.InstanceId{instance.Id()}})
	c.Assert(err, IsNil)

	stateInfo, apiInfo, err := env.StateInfo()

	c.Assert(err, IsNil)
	c.Assert(stateInfo.Addrs, DeepEquals, []string{hostname + mgoPortSuffix})
	c.Assert(apiInfo.Addrs, DeepEquals, []string{hostname + apiPortSuffix})
}

func (suite *EnvironSuite) TestStateInfoFailsIfNoStateInstances(c *C) {
	env := suite.makeEnviron()

	_, _, err := env.StateInfo()

	c.Check(err, FitsTypeOf, &environs.NotFoundError{})
}

func (suite *EnvironSuite) TestQuiesceStateFileFailsOnBrokenStateFile(c *C) {
	const content = "@#$(*&Y%!"
	reader := bytes.NewReader([]byte(content))
	env := suite.makeEnviron()
	err := env.Storage().Put(stateFile, reader, int64(len(content)))
	c.Assert(err, IsNil)

	err = env.quiesceStateFile()

	c.Check(err, Not(IsNil))
}

func (suite *EnvironSuite) TestDestroy(c *C) {
	env := suite.makeEnviron()
	suite.getInstance("test1")
	instance := suite.getInstance("test2")
	data := makeRandomBytes(10)
	suite.testMAASObject.TestServer.NewFile("filename", data)
	storage := env.Storage()

	err := env.Destroy([]environs.Instance{instance})

	c.Check(err, IsNil)
	// Instances have been stopped.
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"release"}, "test2": {"release"}}
	c.Check(operations, DeepEquals, expectedOperations)
	// Files have been cleaned up.
	listing, err := storage.List("")
	c.Assert(err, IsNil)
	c.Check(listing, DeepEquals, []string{})
}

// It would be nice if we could unit-test Bootstrap() in more detail, but
// at the time of writing that would require more support from gomaasapi's
// testing service than we have.
func (suite *EnvironSuite) TestBootstrapSucceeds(c *C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "thenode"}`)
	cert := []byte{1, 2, 3}
	key := []byte{4, 5, 6}

	err := env.Bootstrap(constraints.Value{}, cert, key)
	c.Assert(err, IsNil)
}

func (suite *EnvironSuite) TestBootstrapFailsIfNoNodes(c *C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	cert := []byte{1, 2, 3}
	key := []byte{4, 5, 6}
	err := env.Bootstrap(constraints.Value{}, cert, key)
	// Since there are no nodes, the attempt to allocate one returns a
	// 409: Conflict.
	c.Check(err, ErrorMatches, ".*409.*")
}

func (suite *EnvironSuite) TestBootstrapIntegratesWithEnvirons(c *C) {
	suite.setupFakeTools(c)
	env := suite.makeEnviron()
	suite.testMAASObject.TestServer.NewNode(`{"system_id": "bootstrapnode"}`)

	// environs.Bootstrap calls Environ.Bootstrap.  This works.
	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
}

func (suite *EnvironSuite) TestAssignmentPolicy(c *C) {
	env := suite.makeEnviron()

	c.Check(env.AssignmentPolicy(), Equals, state.AssignUnused)
}
