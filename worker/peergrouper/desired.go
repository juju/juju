// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sort"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/replicaset"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

// peerGroupInfo holds information that may contribute to
// a peer group.
type peerGroupInfo struct {
	machines map[string]*machine // id -> machine
	statuses []replicaset.MemberStatus
	members  []replicaset.Member
}

// desiredPeerGroup returns the mongo peer group according to the given
// servers and a map with an element for each machine in info.machines
// specifying whether that machine has been configured as voting. It may
// return (nil, nil, nil) if the current group is already correct.
func desiredPeerGroup(info *peerGroupInfo) ([]replicaset.Member, map[*machine]bool, error) {
	if len(info.members) == 0 {
		return nil, nil, fmt.Errorf("current member set is empty")
	}
	changed := false
	members, extra, maxId := info.membersMap()
	logger.Debugf("calculating desired peer group")
	logger.Debugf("members: %#v", members)
	logger.Debugf("extra: %#v", extra)
	logger.Debugf("maxId: %v", maxId)

	// We may find extra peer group members if the machines
	// have been removed or their state server status removed.
	// This should only happen if they had been set to non-voting
	// before removal, in which case we want to remove it
	// from the members list. If we find a member that's still configured
	// to vote, it's an error.
	// TODO There are some other possibilities
	// for what to do in that case.
	// 1) leave them untouched, but deal
	// with others as usual "i didn't see that bit"
	// 2) leave them untouched, deal with others,
	// but make sure the extras aren't eligible to
	// be primary.
	// 3) remove them "get rid of bad rubbish"
	// 4) do nothing "nothing to see here"
	for _, member := range extra {
		if member.Votes == nil || *member.Votes > 0 {
			return nil, nil, fmt.Errorf("voting non-machine member %#v found in peer group", member)
		}
		changed = true
	}

	toRemoveVote, toAddVote, toKeep := possiblePeerGroupChanges(info, members)

	// Set up initial record of machine votes. Any changes after
	// this will trigger a peer group election.
	machineVoting := make(map[*machine]bool)
	for _, m := range info.machines {
		if member := members[m]; member != nil && isVotingMember(member) {
			machineVoting[m] = true
		}
	}
	setVoting := func(m *machine, voting bool) {
		setMemberVoting(members[m], voting)
		machineVoting[m] = voting
		changed = true
	}
	adjustVotes(toRemoveVote, toAddVote, setVoting)

	addNewMembers(members, toKeep, maxId, setVoting)
	if updateAddresses(members, info.machines) {
		changed = true
	}
	if !changed {
		return nil, nil, nil
	}
	var memberSet []replicaset.Member
	for _, member := range members {
		memberSet = append(memberSet, *member)
	}
	return memberSet, machineVoting, nil
}

func isVotingMember(member *replicaset.Member) bool {
	return member.Votes == nil || *member.Votes > 0
}

// possiblePeerGroupChanges returns a set of slices
// classifying all the existing machines according to
// how their vote might move.
// toRemoveVote holds machines whose vote should
// be removed; toAddVote holds machines which are
// ready to vote; toKeep holds machines with no desired
// change to their voting status (this includes machines
// that are not yet represented in the peer group).
func possiblePeerGroupChanges(
	info *peerGroupInfo,
	members map[*machine]*replicaset.Member,
) (toRemoveVote, toAddVote, toKeep []*machine) {
	statuses := info.statusesMap(members)

	logger.Debugf("assessing possible peer group changes:")
	for _, m := range info.machines {
		member := members[m]
		isVoting := member != nil && isVotingMember(member)
		switch {
		case m.wantsVote && isVoting:
			logger.Debugf("machine %q is already voting", m.id)
			toKeep = append(toKeep, m)
		case m.wantsVote && !isVoting:
			if status, ok := statuses[m]; ok && isReady(status) {
				logger.Debugf("machine %q is a potential voter", m.id)
				toAddVote = append(toAddVote, m)
			} else {
				logger.Debugf("machine %q is not ready (has status: %v)", m.id, ok)
				toKeep = append(toKeep, m)
			}
		case !m.wantsVote && isVoting:
			logger.Debugf("machine %q is a potential non-voter", m.id)
			toRemoveVote = append(toRemoveVote, m)
		case !m.wantsVote && !isVoting:
			logger.Debugf("machine %q does not want the vote", m.id)
			toKeep = append(toKeep, m)
		}
	}
	logger.Debugf("assessed")
	// sort machines to be added and removed so that we
	// get deterministic behaviour when testing. Earlier
	// entries will be dealt with preferentially, so we could
	// potentially sort by some other metric in each case.
	sort.Sort(byId(toRemoveVote))
	sort.Sort(byId(toAddVote))
	sort.Sort(byId(toKeep))
	return toRemoveVote, toAddVote, toKeep
}

// updateAddresses updates the members' addresses from the machines' addresses.
// It reports whether any changes have been made.
func updateAddresses(members map[*machine]*replicaset.Member, machines map[string]*machine) bool {
	changed := false
	// Make sure all members' machine addresses are up to date.
	for _, m := range machines {
		if m.hostPort == "" {
			continue
		}
		// TODO ensure that replicaset works correctly with IPv6 [host]:port addresses.
		if m.hostPort != members[m].Address {
			members[m].Address = m.hostPort
			changed = true
		}
	}
	return changed
}

// adjustVotes adjusts the votes of the given machines, taking
// care not to let the total number of votes become even at
// any time. It calls setVoting to change the voting status
// of a machine.
func adjustVotes(toRemoveVote, toAddVote []*machine, setVoting func(*machine, bool)) {
	// Remove voting members if they can be replaced by
	// candidates that are ready. This does not affect
	// the total number of votes.
	nreplace := min(len(toRemoveVote), len(toAddVote))
	for i := 0; i < nreplace; i++ {
		from := toRemoveVote[i]
		to := toAddVote[i]
		setVoting(from, false)
		setVoting(to, true)
	}
	toAddVote = toAddVote[nreplace:]
	toRemoveVote = toRemoveVote[nreplace:]

	// At this point, one or both of toAdd or toRemove is empty, so
	// we can adjust the voting-member count by an even delta,
	// maintaining the invariant that the total vote count is odd.
	if len(toAddVote) > 0 {
		toAddVote = toAddVote[0 : len(toAddVote)-len(toAddVote)%2]
		for _, m := range toAddVote {
			setVoting(m, true)
		}
	} else {
		toRemoveVote = toRemoveVote[0 : len(toRemoveVote)-len(toRemoveVote)%2]
		for _, m := range toRemoveVote {
			setVoting(m, false)
		}
	}
}

// addNewMembers adds new members from toKeep
// to the given set of members, allocating ids from
// maxId upwards. It calls setVoting to set the voting
// status of each new member.
func addNewMembers(
	members map[*machine]*replicaset.Member,
	toKeep []*machine,
	maxId int,
	setVoting func(*machine, bool),
) {
	for _, m := range toKeep {
		if members[m] == nil && m.hostPort != "" {
			// This machine was not previously in the members list,
			// so add it (as non-voting). We maintain the
			// id manually to make it easier for tests.
			maxId++
			member := &replicaset.Member{
				Tags: map[string]string{
					"juju-machine-id": m.id,
				},
				Id: maxId,
			}
			members[m] = member
			setVoting(m, false)
		} else if m.hostPort == "" {
			logger.Debugf("ignoring machine %q with no address", m.id)
		}
	}
}

func isReady(status replicaset.MemberStatus) bool {
	return status.Healthy && (status.State == replicaset.PrimaryState ||
		status.State == replicaset.SecondaryState)
}

func setMemberVoting(member *replicaset.Member, voting bool) {
	if voting {
		member.Votes = nil
		member.Priority = nil
	} else {
		votes := 0
		member.Votes = &votes
		priority := 0.0
		member.Priority = &priority
	}
}

type byId []*machine

func (l byId) Len() int           { return len(l) }
func (l byId) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l byId) Less(i, j int) bool { return l[i].id < l[j].id }

// membersMap returns the replica-set members inside info keyed
// by machine. Any members that do not have a corresponding
// machine are returned in extra.
// The maximum replica-set id is returned in maxId.
func (info *peerGroupInfo) membersMap() (members map[*machine]*replicaset.Member, extra []replicaset.Member, maxId int) {
	maxId = -1
	members = make(map[*machine]*replicaset.Member)
	for _, member := range info.members {
		member := member
		mid, ok := member.Tags["juju-machine-id"]
		var found *machine
		if ok {
			found = info.machines[mid]
		}
		if found != nil {
			members[found] = &member
		} else {
			extra = append(extra, member)
		}
		if member.Id > maxId {
			maxId = member.Id
		}
	}
	return members, extra, maxId
}

// statusesMap returns the statuses inside info keyed by machine.
// The provided members map holds the members keyed by machine,
// as returned by membersMap.
func (info *peerGroupInfo) statusesMap(members map[*machine]*replicaset.Member) map[*machine]replicaset.MemberStatus {
	statuses := make(map[*machine]replicaset.MemberStatus)
	for _, status := range info.statuses {
		for m, member := range members {
			if member.Id == status.Id {
				statuses[m] = status
				break
			}
		}
	}
	return statuses
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
