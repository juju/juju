package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
)

func (suite *_MAASProviderTestSuite) TestInstancesReturnsInstances(c *C) {
	input := `{"system_id": "test"}`
	node := suite.testMAASObject.TestServer.NewNode(input)
	resourceURI, _ := node.GetField("resource_uri")
	instanceIds := []state.InstanceId{"test"}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, IsNil)
	c.Check(len(instances), Equals, 1)
	c.Check(string(instances[0].Id()), Equals, resourceURI)
}

func (suite *_MAASProviderTestSuite) TestInstancesReturnsNilIfEmptyParameter(c *C) {
	instances, err := suite.environ.Instances([]state.InstanceId{})

	c.Check(err, IsNil)
	c.Check(instances, DeepEquals, []environs.Instance{})
}

func (suite *_MAASProviderTestSuite) TestInstancesReturnsErrorIfPartialInstances(c *C) {
	input1 := `{"system_id": "test"}`
	node1 := suite.testMAASObject.TestServer.NewNode(input1)
	input2 := `{"system_id": "test2"}`
	suite.testMAASObject.TestServer.NewNode(input2)
	resourceURI1, _ := node1.GetField("resource_uri")
	instanceId1 := state.InstanceId("test")
	instanceId2 := state.InstanceId("unknown systemID")
	instanceIds := []state.InstanceId{instanceId1, instanceId2}

	instances, err := suite.environ.Instances(instanceIds)

	c.Check(err, Equals, environs.ErrPartialInstances)
	c.Check(len(instances), Equals, 1)
	c.Check(string(instances[0].Id()), Equals, resourceURI1)
}
