package maas

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"testing"
)

type _MAASUtilTestSuite struct{}

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&_MAASUtilTestSuite{})

func (s *_MAASUtilTestSuite) TestExtractSystemId(c *C) {
	instanceId := state.InstanceId("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, Equals, "system_id")
}

func (s *_MAASUtilTestSuite) TestGetSystemIdValues(c *C) {
	instanceId1 := state.InstanceId("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := state.InstanceId("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []state.InstanceId{instanceId1, instanceId2}

	values := getSystemIdValues(instanceIds)

	c.Check(values["id"], DeepEquals, []string{"system_id1", "system_id2"})
}
