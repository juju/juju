package replicaset

import (
	"testing"

	gc "launchpad.net/gocheck"

	"labix.org/v2/mgo"

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
	num := 6
	addrs := make([]string, 0, num)
	servers := make([]*coretesting.MgoInstance, 0, num)
	members := make([]Member, 0, num)
	for x := 0; x < num; x++ {
		inst := &coretesting.MgoInstance{}
		inst.Params = []string{"--replSet", name}
		servers = append(servers, inst)
		err := inst.Start()
		c.Assert(err, gc.IsNil)
		addrs = append(addrs, inst.Addr)
		members = append(members, Member{Id: x, Address: inst.Addr})
	}
	t := true
	members[5].Arbiter = &t
	session := servers[0].MgoDialDirect()
	defer session.Close()

	err := Initiate(session, servers[0].Addr, name)
	c.Assert(err, gc.IsNil)

	// need to set mode to strong so that we wait for the write to succeed
	// before reading and thus ensure that we're getting consistent reads.
	session.SetMode(mgo.Strong, false)
	expectedConfig := replicaConfig{
		Name:    name,
		Version: 1,
		Members: members[:1],
	}

	cfg, err := getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)

	// Add all but the last one (the arbiter)
	// This should automatically skip the pre-existing member 0
	err = Add(session, members[0:5]...)
	c.Assert(err, gc.IsNil)

	expectedConfig = replicaConfig{
		Name:    name,
		Version: 2,
		Members: members,
	}

	cfg, err = getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)

	// Now remove the last two Members
	err = Remove(session, addrs[3:]...)
	c.Assert(err, gc.IsNil)

	expectedConfig = replicaConfig{
		Name:    name,
		Version: 3,
		Members: members[0:3],
	}

	cfg, err = getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)

	// now let's mix it up and set the new members to a mix of the previous
	// plus the new arbiter
	mems := []Member{members[4], members[2], members[0], members[5]}

	// reset this guy's ID to amke sure it gets set corrcetly
	members[4].Id = 0

	err = Set(session, mems)
	c.Assert(err, gc.IsNil)

	// any new members will get an id of max(existing_ids...)+1
	members[4].Id = 4
	members[5].Id = 5

	expectedConfig = replicaConfig{
		Name:    name,
		Version: 4,
		Members: mems,
	}

	cfg, err = getConfig(session)
	c.Assert(err, gc.IsNil)
	c.Assert(*cfg, gc.DeepEquals, expectedConfig)

}
