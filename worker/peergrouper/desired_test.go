package peergrouper

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/testing/testbase"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type desiredPeerGroupSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&desiredPeerGroupSuite{})

var desiredPeerGroupTests = []struct {
	about     string
	machines  []*machine
	statuses  []replicaset.Status
	members   []replicaset.Member
	mongoPort int

	expectMembers []replicaset.Member
	expectVoting  []bool
	err           string
}{{
	about: "single machine, no change",
	machines: []*machine{{
		id:        "0",
		candidate: true,
		addresses: mkAddrs("0.1.2.3"),
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.3:1234",
		Tags:    memberTag("0"),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.3:1234",
		Self:    true,
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about: "extra voting member }

func memberTag(id string) map[string]string {
	return map[string]string{
		"juju-machine-id": id,
	}
}

func mkAddrs(addr string) []instance.Address {
	return []instance.Address{{
		Value:        addr,
		Type:         instance.Ipv4Address,
		NetworkScope: instance.NetworkCloudLocal,
	}}
}

func (*desiredPeerGroupSuite) TestDesiredPeerGroup(c *gc.C) {
	for i, test := range desiredPeerGroupTests {
		c.Logf("%d: %s\n", i, test.about)
		info := &peerGroupInfo{
			machines:  test.machines,
			statuses:  test.statuses,
			members:   test.members,
			mongoPort: test.mongoPort,
		}
		members, err := desiredPeerGroup(info)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(members, gc.IsNil)
			continue
		}
		c.Assert(members, gc.DeepEquals, test.expectMembers)
		for i, m := range info.machines {
			c.Assert(m.voting, gc.Equals, test.expectVoting[i])
		}
	}
}
