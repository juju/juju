// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	stdtesting "testing"

	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/replicaset"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type desiredPeerGroupSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&desiredPeerGroupSuite{})

const mongoPort = 1234

var desiredPeerGroupTests = []struct {
	about    string
	machines []*machine
	statuses []replicaset.MemberStatus
	members  []replicaset.Member

	expectMembers []replicaset.Member
	expectVoting  []bool
	expectErr     string
}{{
	// Note that this should never happen - mongo
	// should always be bootstrapped with at least a single
	// member in its member-set.
	about:     "no members - error",
	expectErr: "current member set is empty",
}, {
	about:    "one machine, two more proposed members",
	machines: mkMachines("10v 11v 12v"),
	statuses: mkStatuses("0p"),
	members:  mkMembers("0v"),

	expectMembers: mkMembers("0v 1 2"),
	expectVoting:  []bool{true, false, false},
}, {
	about:         "single machine, no change",
	machines:      mkMachines("11v"),
	members:       mkMembers("1v"),
	statuses:      mkStatuses("1p"),
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about:        "extra member with nil Vote",
	machines:     mkMachines("11v"),
	members:      mkMembers("1v 2vT"),
	statuses:     mkStatuses("1p 2s"),
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member.* found in peer group",
}, {
	about:    "extra member with >1 votes",
	machines: mkMachines("11v"),
	members: append(mkMembers("1v"), replicaset.Member{
		Id:      2,
		Votes:   newInt(2),
		Address: "0.1.2.12:1234",
	}),
	statuses:     mkStatuses("1p 2s"),
	expectVoting: []bool{true},
	expectErr:    "voting non-machine member.* found in peer group",
}, {
	about:         "new machine with no associated member",
	machines:      mkMachines("11v 12v"),
	members:       mkMembers("1v"),
	statuses:      mkStatuses("1p"),
	expectVoting:  []bool{true, false},
	expectMembers: mkMembers("1v 2"),
}, {
	about:         "one machine has become ready to vote  (-> no change)",
	machines:      mkMachines("11v 12v"),
	members:       mkMembers("1v 2"),
	statuses:      mkStatuses("1p 2s"),
	expectVoting:  []bool{true, false},
	expectMembers: nil,
}, {
	about:         "two machines have become ready to vote (-> added)",
	machines:      mkMachines("11v 12v 13v"),
	members:       mkMembers("1v 2 3"),
	statuses:      mkStatuses("1p 2s 3s"),
	expectVoting:  []bool{true, true, true},
	expectMembers: mkMembers("1v 2v 3v"),
}, {
	about:         "two machines have become ready to vote but one is not healthy (-> no change)",
	machines:      mkMachines("11v 12v 13v"),
	members:       mkMembers("1v 2 3"),
	statuses:      mkStatuses("1p 2s 3sH"),
	expectVoting:  []bool{true, false, false},
	expectMembers: nil,
}, {
	about:         "three machines have become ready to vote (-> 2 added)",
	machines:      mkMachines("11v 12v 13v 14v"),
	members:       mkMembers("1v 2 3 4"),
	statuses:      mkStatuses("1p 2s 3s 4s"),
	expectVoting:  []bool{true, true, true, false},
	expectMembers: mkMembers("1v 2v 3v 4"),
}, {
	about:         "one machine ready to lose vote with no others -> no change",
	machines:      mkMachines("11"),
	members:       mkMembers("1v"),
	statuses:      mkStatuses("1p"),
	expectVoting:  []bool{true},
	expectMembers: nil,
}, {
	about:         "two machines ready to lose vote -> votes removed",
	machines:      mkMachines("11 12v 13"),
	members:       mkMembers("1v 2v 3v"),
	statuses:      mkStatuses("1p 2p 3p"),
	expectVoting:  []bool{false, true, false},
	expectMembers: mkMembers("1 2v 3"),
}, {
	about:         "machines removed as state server -> removed from members",
	machines:      mkMachines("11v"),
	members:       mkMembers("1v 2 3"),
	statuses:      mkStatuses("1p 2s 3s"),
	expectVoting:  []bool{true},
	expectMembers: mkMembers("1v"),
}, {
	about:         "a candidate can take the vote of a non-candidate when they're ready",
	machines:      mkMachines("11v 12v 13 14v"),
	members:       mkMembers("1v 2v 3v 4"),
	statuses:      mkStatuses("1p 2s 3s 4s"),
	expectVoting:  []bool{true, true, false, true},
	expectMembers: mkMembers("1v 2v 3 4v"),
}, {
	about:         "several candidates can take non-candidates' votes",
	machines:      mkMachines("11v 12v 13 14 15 16v 17v 18v"),
	members:       mkMembers("1v 2v 3v 4v 5v 6 7 8"),
	statuses:      mkStatuses("1p 2s 3s 4s 5s 6s 7s 8s"),
	expectVoting:  []bool{true, true, false, false, false, true, true, true},
	expectMembers: mkMembers("1v 2v 3 4 5 6v 7v 8v"),
}, {
	about: "a changed machine address should propagate to the members",
	machines: append(mkMachines("11v 12v"), &machine{
		id:        "13",
		wantsVote: true,
		hostPort:  "0.1.99.13:1234",
	}),
	statuses:     mkStatuses("1s 2p 3p"),
	members:      mkMembers("1v 2v 3v"),
	expectVoting: []bool{true, true, true},
	expectMembers: append(mkMembers("1v 2v"), replicaset.Member{
		Id:      3,
		Address: "0.1.99.13:1234",
		Tags:    memberTag("13"),
	}),
}, {
	about: "a machine's address is ignored if it changes to empty",
	machines: append(mkMachines("11v 12v"), &machine{
		id:        "13",
		wantsVote: true,
		hostPort:  "",
	}),
	statuses:      mkStatuses("1s 2p 3p"),
	members:       mkMembers("1v 2v 3v"),
	expectVoting:  []bool{true, true, true},
	expectMembers: nil,
}}

func (*desiredPeerGroupSuite) TestDesiredPeerGroup(c *gc.C) {
	for i, test := range desiredPeerGroupTests {
		c.Logf("\ntest %d: %s", i, test.about)
		machineMap := make(map[string]*machine)
		for _, m := range test.machines {
			c.Assert(machineMap[m.id], gc.IsNil)
			machineMap[m.id] = m
		}
		info := &peerGroupInfo{
			machines: machineMap,
			statuses: test.statuses,
			members:  test.members,
		}
		members, voting, err := desiredPeerGroup(info)
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			c.Assert(members, gc.IsNil)
			continue
		}
		sort.Sort(membersById(members))
		c.Assert(members, jc.DeepEquals, test.expectMembers)
		if len(members) == 0 {
			continue
		}
		for i, m := range test.machines {
			c.Assert(voting[m], gc.Equals, test.expectVoting[i], gc.Commentf("machine %s", m.id))
		}
		// Assure ourselves that the total number of desired votes is odd in
		// all circumstances.
		c.Assert(countVotes(members)%2, gc.Equals, 1)

		// Make sure that when the members are set as
		// required, that there's no further change
		// if desiredPeerGroup is called again.
		info.members = members
		members, voting, err = desiredPeerGroup(info)
		c.Assert(members, gc.IsNil)
		c.Assert(voting, gc.IsNil)
		c.Assert(err, gc.IsNil)
	}
}

func countVotes(members []replicaset.Member) int {
	tot := 0
	for _, m := range members {
		v := 1
		if m.Votes != nil {
			v = *m.Votes
		}
		tot += v
	}
	return tot
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
// "v" if the machine wants a vote.
func mkMachines(description string) []*machine {
	descrs := parseDescr(description)
	ms := make([]*machine, len(descrs))
	for i, d := range descrs {
		ms[i] = &machine{
			id:        fmt.Sprint(d.id),
			hostPort:  fmt.Sprintf("0.1.2.%d:%d", d.id, mongoPort),
			wantsVote: strings.Contains(d.flags, "v"),
		}
	}
	return ms
}

func memberTag(id string) map[string]string {
	return map[string]string{"juju-machine-id": id}
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
			Address: fmt.Sprintf("0.1.2.%d:%d", machineId, mongoPort),
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

var stateFlags = map[rune]replicaset.MemberState{
	'p': replicaset.PrimaryState,
	's': replicaset.SecondaryState,
}

// mkStatuses returns a slice of *replicaset.Member
// based on the given description.
// Each member in the description is white-space separated
// and holds the decimal replica-set id optionally followed by the
// characters:
// 	- 'H' if the instance is not healthy.
//	- 'p' if the instance is in PrimaryState
//	- 's' if the instance is in SecondaryState
func mkStatuses(description string) []replicaset.MemberStatus {
	descrs := parseDescr(description)
	ss := make([]replicaset.MemberStatus, len(descrs))
	for i, d := range descrs {
		machineId := d.id + 10
		s := replicaset.MemberStatus{
			Id:      d.id,
			Address: fmt.Sprintf("0.1.2.%d:%d", machineId, mongoPort),
			Healthy: !strings.Contains(d.flags, "H"),
			State:   replicaset.UnknownState,
		}
		for _, r := range d.flags {
			if state, ok := stateFlags[r]; ok {
				s.State = state
			}
		}
		ss[i] = s
	}
	return ss
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
		c.Assert(parseDescr(test.descr), jc.DeepEquals, test.expect)
	}
}

// parseDescr parses white-space separated fields of the form
// <id><flags> into descr structures.
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

type membersById []replicaset.Member

func (l membersById) Len() int           { return len(l) }
func (l membersById) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l membersById) Less(i, j int) bool { return l[i].Id < l[j].Id }
