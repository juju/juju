// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/juju/replicaset/v3"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/state"
)

type desiredPeerGroupSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&desiredPeerGroupSuite{})

const (
	mongoPort         = 1234
	apiPort           = 5678
	controllerAPIPort = 9876
)

type desiredPeerGroupTest struct {
	about    string
	machines []*controllerTracker
	statuses []replicaset.MemberStatus
	members  []replicaset.Member

	expectChanged  bool
	expectStepDown bool
	expectMembers  []replicaset.Member
	expectVoting   []bool
	expectErr      string
}

// TestMember mirrors replicaset.Member but simplifies the structure
// so that test assertions are easier to understand.
//
// See http://docs.mongodb.org/manual/reference/replica-configuration/
// for more details
type TestMember struct {
	// Id is a unique id for a member in a set.
	Id int

	// Address holds the network address of the member,
	// in the form hostname:port.
	Address string

	// Priority determines eligibility of a member to become primary.
	// This value is optional; it defaults to 1.
	Priority float64

	// Tags store additional information about a replica member, often used for
	// customizing read preferences and write concern.
	Tags map[string]string

	// Votes controls the number of votes a server has in a replica set election.
	// This value is optional; it defaults to 1.
	Votes int
}

func memberToTestMember(m replicaset.Member) TestMember {

	priority := 1.0
	if m.Priority != nil {
		priority = *m.Priority
	}
	votes := 1
	if m.Votes != nil {
		votes = *m.Votes
	}
	return TestMember{
		Id:       m.Id,
		Address:  m.Address,
		Priority: priority,
		Tags:     m.Tags,
		Votes:    votes,
	}
}

func membersToTestMembers(m []replicaset.Member) []TestMember {
	if m == nil {
		return nil
	}
	result := make([]TestMember, len(m))
	for i, member := range m {
		result[i] = memberToTestMember(member)
	}
	return result
}

func desiredPeerGroupTests(ipVersion TestIPVersion) []desiredPeerGroupTest {
	return []desiredPeerGroupTest{
		{
			about:         "one controller, one more proposed member",
			machines:      mkMachines("10v 11v", ipVersion),
			statuses:      mkStatuses("0p", ipVersion),
			members:       mkMembers("0v", ipVersion),
			expectMembers: mkMembers("0v 1", ipVersion),
			expectVoting:  []bool{true, false},
			expectChanged: true,
		}, {
			about:         "one controller, two more proposed members",
			machines:      mkMachines("10v 11v 12v", ipVersion),
			statuses:      mkStatuses("0p", ipVersion),
			members:       mkMembers("0v", ipVersion),
			expectMembers: mkMembers("0v 1 2", ipVersion),
			expectVoting:  []bool{true, false, false},
			expectChanged: true,
		}, {
			about:         "single controller, no change",
			machines:      mkMachines("11v", ipVersion),
			members:       mkMembers("1v", ipVersion),
			statuses:      mkStatuses("1p", ipVersion),
			expectVoting:  []bool{true},
			expectMembers: mkMembers("1v", ipVersion),
			expectChanged: false,
		}, {
			about:        "extra member with nil Vote",
			machines:     mkMachines("11v", ipVersion),
			members:      mkMembers("1vT 2v", ipVersion),
			statuses:     mkStatuses("1p 2s", ipVersion),
			expectVoting: []bool{true},
			expectErr:    "non juju voting member.* found in peer group",
		}, {
			about:    "extra member with >1 votes",
			machines: mkMachines("11v", ipVersion),
			members: append(mkMembers("1vT", ipVersion), replicaset.Member{
				Id:    2,
				Votes: newInt(2),
				Address: net.JoinHostPort(
					fmt.Sprintf(ipVersion.formatHost, 12),
					fmt.Sprint(mongoPort),
				),
			}),
			statuses:     mkStatuses("1p 2s", ipVersion),
			expectVoting: []bool{true},
			expectErr:    "non juju voting member.* found in peer group",
		}, {
			about:         "one controller has become ready to vote (no change)",
			machines:      mkMachines("11v 12v", ipVersion),
			members:       mkMembers("1v 2", ipVersion),
			statuses:      mkStatuses("1p 2s", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2", ipVersion),
			expectChanged: false,
		}, {
			about:         "two machines have become ready to vote (-> added)",
			machines:      mkMachines("11v 12v 13v", ipVersion),
			members:       mkMembers("1v 2 3", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true, true, true},
			expectMembers: mkMembers("1v 2v 3v", ipVersion),
			expectChanged: true,
		}, {
			about:         "one controller has become ready to vote but one is not healthy",
			machines:      mkMachines("11v 12v", ipVersion),
			members:       mkMembers("1v 2", ipVersion),
			statuses:      mkStatuses("1p 2sH", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2", ipVersion),
			expectChanged: false,
		}, {
			about:         "two machines have become ready to vote but one is not healthy (-> no change)",
			machines:      mkMachines("11v 12v 13v", ipVersion),
			members:       mkMembers("1v 2 3", ipVersion),
			statuses:      mkStatuses("1p 2s 3sH", ipVersion),
			expectVoting:  []bool{true, false, false},
			expectMembers: mkMembers("1v 2 3", ipVersion),
			expectChanged: false,
		}, {
			about:         "three machines have become ready to vote (-> 2 added)",
			machines:      mkMachines("11v 12v 13v 14v", ipVersion),
			members:       mkMembers("1v 2 3 4", ipVersion),
			statuses:      mkStatuses("1p 2s 3s 4s", ipVersion),
			expectVoting:  []bool{true, true, true, false},
			expectMembers: mkMembers("1v 2v 3v 4", ipVersion),
			expectChanged: true,
		}, {
			about:         "one controller ready to lose vote with no others -> no change",
			machines:      mkMachines("11", ipVersion),
			members:       mkMembers("1v", ipVersion),
			statuses:      mkStatuses("1p", ipVersion),
			expectVoting:  []bool{true},
			expectMembers: mkMembers("1v", ipVersion),
			expectChanged: false,
		}, {
			about:         "one controller ready to lose vote -> votes removed from secondaries",
			machines:      mkMachines("11v 12v 13", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1s 2p 3s", ipVersion),
			expectVoting:  []bool{false, true, false},
			expectMembers: mkMembers("1 2v 3", ipVersion),
			expectChanged: true,
		}, {
			about:         "two machines ready to lose vote -> votes removed",
			machines:      mkMachines("11 12v 13", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1s 2p 3s", ipVersion),
			expectVoting:  []bool{false, true, false},
			expectMembers: mkMembers("1 2v 3", ipVersion),
			expectChanged: true,
		}, {
			about:         "machines removed as controller -> removed from members",
			machines:      mkMachines("11v", ipVersion),
			members:       mkMembers("1v 2 3", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true},
			expectMembers: mkMembers("1v", ipVersion),
			expectChanged: true,
		}, {
			about: "controller dead -> removed from members",
			machines: append(mkMachines("11v 12v", ipVersion), &controllerTracker{
				id: "13",
				addresses: []network.SpaceAddress{{
					MachineAddress: network.MachineAddress{
						Value: ipVersion.extraHost,
						Type:  ipVersion.addressType,
						Scope: network.ScopeCloudLocal,
					},
				}},
				host: mkController(state.Dead),
			}),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2s", ipVersion),
			expectChanged: true,
		}, {
			about:         "controller removed as controller -> removed from member",
			machines:      mkMachines("11v 12", ipVersion),
			members:       mkMembers("1v 2 3", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2", ipVersion),
			expectChanged: true,
		}, {
			about:         "a candidate can take the vote of a non-candidate when they're ready",
			machines:      mkMachines("11v 12v 13 14v", ipVersion),
			members:       mkMembers("1v 2v 3v 4", ipVersion),
			statuses:      mkStatuses("1p 2s 3s 4s", ipVersion),
			expectVoting:  []bool{true, true, false, true},
			expectMembers: mkMembers("1v 2v 3 4v", ipVersion),
			expectChanged: true,
		}, {
			about:         "several candidates can take non-candidates' votes",
			machines:      mkMachines("11v 12v 13 14 15 16v 17v 18v", ipVersion),
			members:       mkMembers("1v 2v 3v 4v 5v 6 7 8", ipVersion),
			statuses:      mkStatuses("1p 2s 3s 4s 5s 6s 7s 8s", ipVersion),
			expectVoting:  []bool{true, true, false, false, false, true, true, true},
			expectMembers: mkMembers("1v 2v 3 4 5 6v 7v 8v", ipVersion),
			expectChanged: true,
		}, {
			about: "a changed controller address should propagate to the members",
			machines: append(mkMachines("11v 12v", ipVersion), &controllerTracker{
				id:        "13",
				wantsVote: true,
				addresses: []network.SpaceAddress{{
					MachineAddress: network.MachineAddress{
						Value: ipVersion.extraHost,
						Type:  ipVersion.addressType,
						Scope: network.ScopeCloudLocal,
					},
				}},
				host: mkController(state.Alive),
			}),
			statuses:     mkStatuses("1s 2p 3s", ipVersion),
			members:      mkMembers("1v 2v 3v", ipVersion),
			expectVoting: []bool{true, true, true},
			expectMembers: append(mkMembers("1v 2v", ipVersion), replicaset.Member{
				Id:      3,
				Address: net.JoinHostPort(ipVersion.extraHost, fmt.Sprint(mongoPort)),
				Tags:    memberTag("13"),
			}),
			expectChanged: true,
		}, {
			about: "a controller's address is ignored if it changes to empty",
			machines: append(mkMachines("11v 12v", ipVersion), &controllerTracker{
				id:        "13",
				wantsVote: true,
				host:      mkController(state.Alive),
			}),
			statuses:      mkStatuses("1s 2p 3s", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			expectVoting:  []bool{true, true, true},
			expectMembers: mkMembers("1v 2v 3v", ipVersion),
			expectChanged: false,
		}, {
			about:         "two voting members removes vote from secondary (first member)",
			machines:      mkMachines("11v 12v", ipVersion),
			members:       mkMembers("1v 2v", ipVersion),
			statuses:      mkStatuses("1s 2p", ipVersion),
			expectVoting:  []bool{false, true},
			expectMembers: mkMembers("1 2v", ipVersion),
			expectChanged: true,
		}, {
			about:         "two voting members removes vote from secondary (second member)",
			machines:      mkMachines("11v 12v", ipVersion),
			members:       mkMembers("1v 2v", ipVersion),
			statuses:      mkStatuses("1p 2s", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2", ipVersion),
			expectChanged: true,
		}, {
			about:         "three voting members one ready to loose voting -> no consensus",
			machines:      mkMachines("11v 12v 13", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true, false, false},
			expectMembers: mkMembers("1v 2 3", ipVersion),
			expectChanged: true,
		}, {
			about:         "three voting members remove one, to only one voting member left",
			machines:      mkMachines("11v 12", ipVersion),
			members:       mkMembers("1v 2v 3", ipVersion),
			statuses:      mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:  []bool{true, false},
			expectMembers: mkMembers("1v 2", ipVersion),
			expectChanged: true,
		}, {
			about:         "three voting members remove all, keep primary",
			machines:      mkMachines("11 12 13", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1s 2s 3p", ipVersion),
			expectVoting:  []bool{false, false, true},
			expectMembers: mkMembers("1 2 3v", ipVersion),
			expectChanged: true,
		}, {
			about:         "add controller, non-voting still add it to the replica set",
			machines:      mkMachines("11v 12v 13v 14", ipVersion),
			members:       mkMembers("1v 2v 3v", ipVersion),
			statuses:      mkStatuses("1s 2s 3p", ipVersion),
			expectVoting:  []bool{true, true, true, false},
			expectMembers: mkMembers("1v 2v 3v 4", ipVersion),
			expectChanged: true,
		}, {
			about:          "remove primary controller",
			machines:       mkMachines("11 12v 13v", ipVersion),
			members:        mkMembers("1v 2v 3v", ipVersion),
			statuses:       mkStatuses("1p 2s 3s", ipVersion),
			expectVoting:   []bool{false, false, true},
			expectMembers:  mkMembers("1 2 3v", ipVersion),
			expectStepDown: true,
			expectChanged:  true,
		},
	}
}

func (s *desiredPeerGroupSuite) TestDesiredPeerGroupIPv4(c *gc.C) {
	s.doTestDesiredPeerGroup(c, testIPv4)
}

func (s *desiredPeerGroupSuite) TestDesiredPeerGroupIPv6(c *gc.C) {
	s.doTestDesiredPeerGroup(c, testIPv6)
}

func (s *desiredPeerGroupSuite) doTestDesiredPeerGroup(c *gc.C, ipVersion TestIPVersion) {
	for ti, test := range desiredPeerGroupTests(ipVersion) {
		c.Logf("\ntest %d: %s", ti, test.about)
		trackerMap := make(map[string]*controllerTracker)
		for _, m := range test.machines {
			c.Assert(trackerMap[m.Id()], gc.IsNil)
			trackerMap[m.Id()] = m
		}

		info, err := newPeerGroupInfo(trackerMap, test.statuses, test.members, mongoPort)
		c.Assert(err, jc.ErrorIsNil)

		desired, err := desiredPeerGroup(info)
		if test.expectErr != "" {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			c.Assert(desired.members, gc.IsNil)
			c.Assert(desired.isChanged, jc.IsFalse)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info, gc.NotNil)

		members := make([]replicaset.Member, 0, len(desired.members))
		for _, m := range desired.members {
			members = append(members, *m)
		}

		sort.Sort(membersById(members))
		c.Assert(desired.isChanged, gc.Equals, test.expectChanged)
		c.Assert(desired.stepDownPrimary, gc.Equals, test.expectStepDown)
		c.Assert(membersToTestMembers(members), jc.DeepEquals, membersToTestMembers(test.expectMembers))
		for i, m := range test.machines {
			if m.host.Life() == state.Dead {
				continue
			}
			var vote, votePresent bool
			for _, member := range desired.members {
				controllerId, ok := member.Tags[jujuNodeKey]
				c.Assert(ok, jc.IsTrue)
				if controllerId == m.Id() {
					votePresent = true
					vote = isVotingMember(member)
					break
				}
			}
			c.Check(votePresent, jc.IsTrue)
			c.Check(vote, gc.Equals, test.expectVoting[i], gc.Commentf("controller %s", m.Id()))
		}

		// Assure ourselves that the total number of desired votes is odd in
		// all circumstances.
		c.Assert(countVotes(members)%2, gc.Equals, 1)

		// Make sure that when the members are set as required, that there
		// is no further change if desiredPeerGroup is called again.
		info, err = newPeerGroupInfo(trackerMap, test.statuses, members, mongoPort)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(info, gc.NotNil)

		desired, err = desiredPeerGroup(info)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(desired.isChanged, jc.IsFalse)
		c.Assert(desired.stepDownPrimary, jc.IsFalse)
		countPrimaries := 0
		c.Assert(err, gc.IsNil)
		for i, m := range test.machines {
			var vote, votePresent bool
			for _, member := range desired.members {
				controllerId, ok := member.Tags[jujuNodeKey]
				c.Assert(ok, jc.IsTrue)
				if controllerId == m.Id() {
					votePresent = true
					vote = isVotingMember(member)
					break
				}
			}
			if m.host.Life() == state.Dead {
				c.Assert(votePresent, jc.IsFalse)
				continue
			}
			c.Check(votePresent, jc.IsTrue)
			c.Check(vote, gc.Equals, test.expectVoting[i], gc.Commentf("controller %s", m.Id()))
			if isPrimaryMember(info, m.Id()) {
				countPrimaries += 1
			}
		}
		c.Assert(countPrimaries, gc.Equals, 1)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *desiredPeerGroupSuite) TestIsPrimary(c *gc.C) {
	machines := mkMachines("11v 12v 13v", testIPv4)
	trackerMap := make(map[string]*controllerTracker)
	for _, m := range machines {
		c.Assert(trackerMap[m.Id()], gc.IsNil)
		trackerMap[m.Id()] = m
	}
	members := mkMembers("1v 2v 3v", testIPv4)
	statuses := mkStatuses("1p 2s 3s", testIPv4)
	info, err := newPeerGroupInfo(trackerMap, statuses, members, mongoPort)
	c.Assert(err, jc.ErrorIsNil)
	isPrimary, err := info.isPrimary("11")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isPrimary, jc.IsTrue)
	isPrimary, err = info.isPrimary("12")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isPrimary, jc.IsFalse)
}

func (s *desiredPeerGroupSuite) TestNewPeerGroupInfoErrWhenNoMembers(c *gc.C) {
	_, err := newPeerGroupInfo(nil, nil, nil, 666)
	c.Check(err, gc.ErrorMatches, "current member set is empty")
}

func (s *desiredPeerGroupSuite) TestCheckExtraMembersReturnsErrorWhenVoterFound(c *gc.C) {
	v := 1
	peerChanges := peerGroupChanges{
		info: &peerGroupInfo{extra: []replicaset.Member{{Votes: &v}}},
	}
	err := peerChanges.checkExtraMembers()
	c.Check(err, gc.ErrorMatches, "non juju voting member .+ found in peer group")
}

func (s *desiredPeerGroupSuite) TestCheckExtraMembersReturnsTrueWhenCheckMade(c *gc.C) {
	v := 0
	peerChanges := peerGroupChanges{
		info: &peerGroupInfo{extra: []replicaset.Member{{Votes: &v}}},
	}
	err := peerChanges.checkExtraMembers()
	c.Check(peerChanges.desired.isChanged, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
}

func (s *desiredPeerGroupSuite) TestCheckToRemoveMembersReturnsTrueWhenCheckMade(c *gc.C) {
	peerChanges := peerGroupChanges{
		info: &peerGroupInfo{toRemove: []replicaset.Member{{}}},
	}
	err := peerChanges.checkExtraMembers()
	c.Check(peerChanges.desired.isChanged, jc.IsTrue)
	c.Check(err, jc.ErrorIsNil)
}

func (s *desiredPeerGroupSuite) TestCheckExtraMembersReturnsFalseWhenEmpty(c *gc.C) {
	peerChanges := peerGroupChanges{
		info: &peerGroupInfo{},
	}
	err := peerChanges.checkExtraMembers()
	c.Check(peerChanges.desired.isChanged, jc.IsFalse)
	c.Check(err, jc.ErrorIsNil)
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

func mkController(life state.Life) *fakeController {
	ctrl := &fakeController{}
	ctrl.val.Set(controllerDoc{life: life})
	return ctrl
}

// mkMachines returns a slice of *machineTracker based on
// the given description.
// Each controller in the description is white-space separated
// and holds the decimal controller id followed by an optional
// "v" if the controller wants a vote.
func mkMachines(description string, ipVersion TestIPVersion) []*controllerTracker {
	descrs := parseDescr(description)
	ms := make([]*controllerTracker, len(descrs))
	for i, d := range descrs {
		ms[i] = &controllerTracker{
			id: fmt.Sprint(d.id),
			addresses: []network.SpaceAddress{{
				MachineAddress: network.MachineAddress{
					Value: fmt.Sprintf(ipVersion.formatHost, d.id),
					Type:  ipVersion.addressType,
					Scope: network.ScopeCloudLocal,
				},
			}},
			host:      mkController(state.Alive),
			wantsVote: strings.Contains(d.flags, "v"),
		}
	}
	return ms
}

func memberTag(id string) map[string]string {
	return map[string]string{jujuNodeKey: id}
}

// mkMembers returns a slice of replicaset.Member based on the given
// description.
// Each member in the description is white-space separated and holds the decimal
// replica-set id optionally followed by the characters:
//   - 'v' if the member is voting.
//   - 'T' if the member has no associated controller tags.
//
// Unless the T flag is specified, the controller tag
// will be the replica-set id + 10.
func mkMembers(description string, ipVersion TestIPVersion) []replicaset.Member {
	descrs := parseDescr(description)
	ms := make([]replicaset.Member, len(descrs))
	for i, d := range descrs {
		machineId := d.id + 10
		m := replicaset.Member{
			Id: d.id,
			Address: net.JoinHostPort(
				fmt.Sprintf(ipVersion.formatHost, machineId),
				fmt.Sprint(mongoPort),
			),
			Tags: memberTag(fmt.Sprint(machineId)),
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

// mkStatuses returns a slice of replicaset.MemberStatus based on the given
// description.
// Each member in the description is white-space separated  and holds the
// decimal replica-set id optionally followed by the characters:
//   - 'H' if the instance is not healthy.
//   - 'p' if the instance is in PrimaryState
//   - 's' if the instance is in SecondaryState
func mkStatuses(description string, ipVersion TestIPVersion) []replicaset.MemberStatus {
	descrs := parseDescr(description)
	ss := make([]replicaset.MemberStatus, len(descrs))
	for i, d := range descrs {
		machineId := d.id + 10
		s := replicaset.MemberStatus{
			Id: d.id,
			Address: net.JoinHostPort(
				fmt.Sprintf(ipVersion.formatHost, machineId),
				fmt.Sprint(mongoPort),
			),
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

func assertMembers(c *gc.C, obtained interface{}, expected []replicaset.Member) {
	c.Assert(obtained, gc.FitsTypeOf, []replicaset.Member{})
	// Avoid mutating the obtained slice: because it's usually retrieved
	// directly from the memberWatcher voyeur.Value,
	// mutation can cause races.
	obtainedMembers := deepCopy(obtained).([]replicaset.Member)
	sort.Sort(membersById(obtainedMembers))
	sort.Sort(membersById(expected))
	c.Assert(membersToTestMembers(obtainedMembers), jc.DeepEquals, membersToTestMembers(expected))
}

type membersById []replicaset.Member

func (l membersById) Len() int           { return len(l) }
func (l membersById) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l membersById) Less(i, j int) bool { return l[i].Id < l[j].Id }

// AssertAPIHostPorts asserts of two sets of network.SpaceHostPort slices are the same.
func AssertAPIHostPorts(c *gc.C, got, want []network.SpaceHostPorts) {
	c.Assert(got, gc.HasLen, len(want))
	sort.Sort(hostPortSliceByHostPort(got))
	sort.Sort(hostPortSliceByHostPort(want))
	c.Assert(got, gc.DeepEquals, want)
}

type hostPortSliceByHostPort []network.SpaceHostPorts

func (h hostPortSliceByHostPort) Len() int      { return len(h) }
func (h hostPortSliceByHostPort) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h hostPortSliceByHostPort) Less(i, j int) bool {
	a, b := h[i], h[j]
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	for i := range a {
		av, bv := a[i], b[i]
		if av.Value != bv.Value {
			return av.Value < bv.Value
		}
		if av.Port() != bv.Port() {
			return av.Port() < bv.Port()
		}
	}
	return false
}

type sortAsIntsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sortAsIntsSuite{})

func checkIntSorted(c *gc.C, vals, expected []string) {
	// we sort in place, so leave 'vals' alone and copy to another slice
	copied := append([]string(nil), vals...)
	sortAsInts(copied)
	c.Check(copied, gc.DeepEquals, expected)
}

func (*sortAsIntsSuite) TestAllInts(c *gc.C) {
	checkIntSorted(c, []string{"1", "10", "2", "20"}, []string{"1", "2", "10", "20"})
}

func (*sortAsIntsSuite) TestStrings(c *gc.C) {
	checkIntSorted(c, []string{"a", "c", "b", "X"}, []string{"X", "a", "b", "c"})
}

func (*sortAsIntsSuite) TestMixed(c *gc.C) {
	checkIntSorted(c, []string{"1", "20", "10", "2", "2d", "c", "b", "X"},
		[]string{"1", "2", "10", "20", "2d", "X", "b", "c"})
}
