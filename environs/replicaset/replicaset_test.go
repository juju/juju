package replicaset

import (
	"testing"

	gc "launchpad.net/gocheck"

	//"labix.org/v2/mgo"

	coretesting "launchpad.net/juju-core/testing"
)

func TestPackage(t *testing.T) {
	t.Log("Starting gocheck")
	gc.TestingT(t)
}

type MongoSuite struct{}

var _ = gc.Suite(&MongoSuite{})

func (s *MongoSuite) TestParseConstraints(c *gc.C) {
	name := "juju"
	num := 3
	addrs := make([]string, 0, num)
	servers := make([]*coretesting.MgoInstance, 0, num)
	members := make([]Member, 0, num)
	c.Logf("Starting %d mongo instances", num)
	for x := 0; x < num; x++ {
		inst := &coretesting.MgoInstance{}
		inst.Params = []string{"--replSet", name}
		servers = append(servers, inst)
		err := inst.Start()
		c.Assert(err, gc.IsNil)
		c.Logf("Started mongo at %s", inst.Addr)
		//defer inst.Destroy()
		addrs = append(addrs, inst.Addr)
		members = append(members, Member{Id: x, Address: inst.Addr})
	}
	c.Logf("Dialing server %v", servers[0])
	session := servers[0].MgoDialDirect()
	defer session.Close()

	c.Logf("Initiating the replica set")
	err := Initiate(session, servers[0].Addr, name)
	c.Assert(err, gc.IsNil)

	expectedConfig := replicaConfig{
		Name:    name,
		Version: 1,
		Members: members[:1],
	}

	cfg, err := getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)

	// add should automatically skip the pre-existing member 0
	Add(session, members...)

	expectedConfig = replicaConfig{
		Name:    name,
		Version: 2,
		Members: members,
	}

	c.Log("Log dir: " + servers[0].Dir)

	cfg, err = getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)
}
