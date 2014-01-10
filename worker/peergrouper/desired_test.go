package peergrouper

import (
	"fmt"
	"strconv"
	"strings"
	stdtesting "testing"

	"code.google.com/p/rog-go/deepdiff"
	gc "launchpad.net/gocheck"
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
	expectErr     string
}{{
	about:    "single machine, no change",
	machines: mkMachines("11c"),
	members:  mkMembers("1v"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about:    "extra member with nil Vote",
	machines: mkMachines("11c"),
	members:  mkMembers("1v 2vT"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member found in peer group",
}, {
	about:    "extra member with >1 votes",
	machines: mkMachines("11c"),
	members: append(mkMembers("1v"), replicaset.Member{
		Id:      2,
		Votes:   newInt(2),
		Address: "0.1.2.12:1234",
	}),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member found in peer group",
}, {
	about:    "new machine with no associated member",
	machines: mkMachines("11c 12c"),
	members:  mkMembers("1v"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true, false},
	expectMembers: mkMembers("1v 2"),
}, {
	about:    "one machine has become ready to vote  (-> no change)",
	machines: mkMachines("11c 12c"),
	members:  mkMembers("1v 2"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true, false},
	expectMembers: nil,
}, {
	about:    "two machines have become ready to vote (-> added)",
	machines: mkMachines("11c 12c 13c"),
	members:  mkMembers("1v 2 3"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      3,
		Address: "0.1.2.13:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true, true, true},
	expectMembers: mkMembers("1v 2v 3v"),
}, {
	about:    "three machines have become ready to vote (-> 2 added)",
	machines: mkMachines("11c 12c 13c 14c"),
	members:  mkMembers("1v 2 3 4"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      3,
		Address: "0.1.2.13:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      4,
		Address: "0.1.2.14:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true, true, true, false},
	expectMembers: mkMembers("1v 2v 3v 4"),
}, {
	about:    "one machine ready to lose vote with no others -> no change",
	machines: mkMachines("11"),
	members: []replicaset.Member{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Tags:    memberTag("11"),
	}},
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about:    "two machines ready to lose vote -> votes removed",
	machines: mkMachines("11 12c 13"),
	members:  mkMembers("1v 2v 3v"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      3,
		Address: "0.1.2.13:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{false, true, false},
	expectMembers: mkMembers("1 2v 3"),
}, {
	about:    "one machine removed as state server -> removed from members",
	machines: mkMachines("11c"),
	members:  mkMembers("1v 2"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:    1234,
	expectVoting: []bool{true},
	expectMembers: []replicaset.Member{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Tags:    memberTag("11"),
	}},
}, {
	about:    "candidates can take the vote of a non-candidates when they're ready",
	machines: mkMachines("11c 12c 13 14c"),
	members:  mkMembers("1v 2v 3v 4"),
	statuses: []replicaset.Status{{
		Id:      1,
		Address: "0.1.2.11:1234",
		Healthy: true,
		State:   replicaset.PrimaryState,
	}, {
		Id:      2,
		Address: "0.1.2.12:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      3,
		Address: "0.1.2.13:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}, {
		Id:      4,
		Address: "0.1.2.13:1234",
		Healthy: true,
		State:   replicaset.SecondaryState,
	}},
	mongoPort:     1234,
	expectVoting:  []bool{true, true, false, true},
	expectMembers: mkMembers("1v 2v 3 4v"),
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
			_, err := deepdiff.DeepDiff(members, test.expectMembers)
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

// mkMachines returns a slice of *machine based on
// the given description.
// Each machine in the description is white-space separated
// and holds the decimal machine id followed by an optional
// "c" if the machine is a candidate.
func mkMachines(description string) []*machine {
	descrs := parseDescr(description)
	ms := make([]*machine, len(descrs))
	for i, d := range descrs {
		ms[i] = &machine{
			id:        fmt.Sprint(d.id),
			host:      fmt.Sprintf("0.1.2.%d", d.id),
			candidate: strings.Contains(d.flags, "c"),
		}
	}
	return ms
}

// mkMembers returns a slice of *replicaset.Member
// based on the given description.
// Each member in the description is white-space separated
// and holds the decimal replica-set id optionally followed by the characters:
//	- 'v' if the member is voting.
// 	- 'T' if the member has no associated machine tags.
// Unless the T flag is specified, the machine tag
// will be the replica-set id + 10.
func mkMembers(description string) []replicaset.Member {
	descrs := parseDescr(description)
	ms := make([]replicaset.Member, len(descrs))
	for i, d := range descrs {
		machineId := d.id + 10
		m := replicaset.Member{
			Id:      d.id,
			Address: fmt.Sprintf("0.1.2.%d:1234", machineId),
			Tags:    memberTag(fmt.Sprint(machineId)),
		}
		if !strings.Contains(d.flags, "v") {
			m.Priority = newFloat64(0)
			m.Votes = newInt(0)
		}
		if strings.Contains(d.flags, "T") {
			m.Tags = nil
		}
		ms[i] = m
	}
	return ms
}

type descr struct {
	id    int
	flags string
}

func isNotDigit(r rune) bool {
	return r < '0' || r > '9'
}

var parseDescrTests = []struct {
	descr  string
	expect []descr
}{{
	descr:  "",
	expect: []descr{},
}, {
	descr:  "0",
	expect: []descr{{id: 0}},
}, {
	descr:  "1foo",
	expect: []descr{{id: 1, flags: "foo"}},
}, {
	descr: "10c  5 6443arble ",
	expect: []descr{{
		id:    10,
		flags: "c",
	}, {
		id: 5,
	}, {
		id:    6443,
		flags: "arble",
	}},
}}

func (*desiredPeerGroupSuite) TestParseDescr(c *gc.C) {
	for i, test := range parseDescrTests {
		c.Logf("test %d. %q", i, test.descr)
		c.Assert(parseDescr(test.descr), gc.DeepEquals, test.expect)
	}
}

func parseDescr(s string) []descr {
	fields := strings.Fields(s)
	descrs := make([]descr, len(fields))
	for i, field := range fields {
		d := &descrs[i]
		i := strings.IndexFunc(field, isNotDigit)
		if i == -1 {
			i = len(field)
		}
		id, err := strconv.Atoi(field[0:i])
		if err != nil {
			panic(fmt.Errorf("bad field %q", field))
		}
		d.id = id
		d.flags = field[i:]
	}
	return descrs
}
