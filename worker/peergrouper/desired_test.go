package peergrouper

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/testing/testbase"
	deepdiff "code.google.com/p/rog-go/deepdiff"
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
	expectErr     string
}{{
	about: "single machine, no change",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about: "extra member with nil Vote",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member found in peer group",
}, {
	about: "extra member with >1 votes",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:      11,
		Votes:   newInt(2),
		Address: "0.1.2.2:1234",
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member found in peer group",
}, {
	about: "new machine with no associated member",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}, {
		id:        "2",
		candidate: true,
		host:      "0.1.2.2",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true, false},
	expectMembers: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:       11,
		Address:  "0.1.2.2:1234",
		Tags:     memberTag("2"),
		Votes:    newInt(0),
		Priority: newFloat64(0),
	}},
}, {
	about: "one machine has become ready to vote  (-> no change)",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}, {
		id:        "2",
		candidate: true,
		host:      "0.1.2.2",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Tags:    memberTag("2"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true, false},
	expectMembers: nil,
}, {
	about: "two machines have become ready to vote (-> added)",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}, {
		id:        "2",
		candidate: true,
		host:      "0.1.2.2",
	}, {
		id:	"3",
		candidate: true,
		host: "0.1.2.3",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Tags:    memberTag("2"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	},  {
		Id:      12,
		Address: "0.1.2.3:1234",
		Tags:    memberTag("3"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      12,
		Address: "0.1.2.3:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true, true, true},
	expectMembers: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:       11,
		Address:  "0.1.2.2:1234",
		Tags:     memberTag("2"),
	}, {
		Id:       12,
		Address:  "0.1.2.3:1234",
		Tags:     memberTag("3"),
	}},
}, {
	about: "three machines have become ready to vote (-> 2 added)",
	machines: []*machine{{
		id:        "1",
		candidate: true,
		host:      "0.1.2.1",
	}, {
		id:        "2",
		candidate: true,
		host:      "0.1.2.2",
	}, {
		id:	"3",
		candidate: true,
		host: "0.1.2.3",
	}, {
		id:	"4",
		candidate: true,
		host: "0.1.2.4",
	}},
	members: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Tags:    memberTag("2"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}, {
		Id:      12,
		Address: "0.1.2.3:1234",
		Tags:    memberTag("3"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}, {
		Id:      13,
		Address: "0.1.2.4:1234",
		Tags:    memberTag("3"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}},
	statuses: []replicaset.Status{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      11,
		Address: "0.1.2.2:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      12,
		Address: "0.1.2.3:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      13,
		Address: "0.1.2.4:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true, true, true, false},
	expectMembers: []replicaset.Member{{
		Id:      10,
		Address: "0.1.2.1:1234",
		Tags:    memberTag("1"),
	}, {
		Id:       11,
		Address:  "0.1.2.2:1234",
		Tags:     memberTag("2"),
	}, {
		Id:       12,
		Address:  "0.1.2.3:1234",
		Tags:     memberTag("3"),
	}, {
		Id:       13,
		Address:  "0.1.2.4:1234",
		Tags:     memberTag("4"),
		Votes: newInt(0),
		Priority: newFloat64(0),
	}},
}}

func memberTag(id string) map[string]string {
	return map[string]string{
		"juju-machine-id": id,
	}
}

func (*desiredPeerGroupSuite) TestDesiredPeerGroup(c *gc.C) {
	for i, test := range desiredPeerGroupTests {
		c.Logf("\ntest %d: %s", i, test.about)
		info := &peerGroupInfo{
			machines:  test.machines,
			statuses:  test.statuses,
			members:   test.members,
			mongoPort: test.mongoPort,
		}
		members, err := desiredPeerGroup(info)
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			c.Assert(members, gc.IsNil)
			continue
		}
		if !c.Check(members, gc.DeepEquals, test.expectMembers) {
			_, err := deepdiff.DeepEqual(members, test.expectMembers)
			c.Fatalf("diff err: %v", err)
		}
		for i, m := range info.machines {
			c.Assert(m.voting, gc.Equals, test.expectVoting[i], gc.Commentf("machine %s", m.id))
		}
		if len(members) > 0 {
			// Make sure that when the members are set as
			// required, that there's no further change
			// if desiredPeerGroup is called again.
			info.members = members
			members, err := desiredPeerGroup(info)
			c.Assert(members, gc.IsNil)
			c.Assert(err, gc.IsNil)
		}
	}
}

func newInt(i int) *int {
	return &i
}

func newFloat64(f float64) *float64 {
	return &f
}
