package maas

import (
	"bytes"
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type EnvironSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironSuite))

// getTestConfig creates a customized sample MAAS provider configuration.
func getTestConfig(name, server, oauth, secret string) *config.Config {
	ecfg, err := newConfig(map[string]interface{}{
		"name":         name,
		"maas-server":  server,
		"maas-oauth":   oauth,
		"admin-secret": secret,
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
		"maas-server":     suite.testMAASObject.URL().String(),
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
	authClient, _ := gomaasapi.NewAuthenticatedClient(anotherServer, anotherOauth)
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
	specificStorage := storage.(*maasStorage)
	c.Check(specificStorage.environUnlocked, Equals, env)
}

func (suite *EnvironSuite) TestPublicStorageIsNotImplemented(c *C) {
	env := suite.makeEnviron()
	c.Check(env.PublicStorage(), IsNil)
}

func (suite *EnvironSuite) TestStartInstanceStartsInstance(c *C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")

	instance, err := suite.environ.StartInstance(resourceURI, nil, nil, nil)

	c.Check(err, IsNil)
	c.Check(string(instance.Id()), Equals, resourceURI)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	actions, found := operations["test"]
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

func (suite *EnvironSuite) TestStopInstancesStopsInstances(c *C) {
	instance1 := suite.getInstance("test1")
	instance2 := suite.getInstance("test2")
	suite.getInstance("test3")
	instances := []environs.Instance{instance1, instance2}

	err := suite.environ.StopInstances(instances)

	c.Check(err, IsNil)
	operations := suite.testMAASObject.TestServer.NodeOperations()
	expectedOperations := map[string][]string{"test1": {"stop"}, "test2": {"stop"}}
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

func (suite *EnvironSuite) TestQuiesceStateFileFailsOnBrokenStateFile(c *C) {
	const content = "@#$(*&Y%!"
	reader := bytes.NewReader([]byte(content))
	env := suite.makeEnviron()
	err := env.Storage().Put(stateFile, reader, int64(len(content)))
	c.Assert(err, IsNil)

	err = env.quiesceStateFile()

	c.Check(err, Not(IsNil))
}

func (suite *EnvironSuite) TestBootstrap(c *C) {
	env := suite.makeEnviron()

	err := env.Bootstrap(true, []byte{}, []byte{})
	// TODO: Get this to succeed.
	unused(err)
	// c.Assert(err, IsNil)

	// TODO: Verify a simile of success.
}
