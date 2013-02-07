package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gomaasapi"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

type EnvironSuite struct {
	ProviderSuite
}

var _ = Suite(new(EnvironSuite))

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
	input := `{"system_id": "test"}`
	suite.testMAASObject.TestServer.NewNode(input)
	instances, err := suite.environ.Instances([]state.InstanceId{})

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
