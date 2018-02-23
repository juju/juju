// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sort"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/network"
)

// jujuMachineKey is the key for the tag where we save the member's juju machine id.
const jujuMachineKey = "juju-machine-id"

// machineMembers is a type alias for conveniently repeating this usage.
type machineMembers = map[*machineTracker]*replicaset.Member

// peerGroupInfo holds information that may contribute to
// a peer group.
type peerGroupInfo struct {
	machineTrackers map[string]*machineTracker // id -> machine
	statuses        []replicaset.MemberStatus
	members         []replicaset.Member
	mongoPort       int
	haSpace         network.SpaceName
}

// membersMap returns the replica-set members inside info keyed
// by machine. Any members that do not have a corresponding
// machine are returned in extra.
// The maximum replica-set id is returned in maxId.
func (info *peerGroupInfo) membersMap() (members machineMembers, extra []replicaset.Member, maxId int) {
	maxId = -1
	members = make(machineMembers)
	for key := range info.members {
		// key is used instead of value to have a loop scoped member value
		member := info.members[key]
		mid, ok := member.Tags[jujuMachineKey]
		var found *machineTracker
		if ok {
			found = info.machineTrackers[mid]
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
func (info *peerGroupInfo) statusesMap(members machineMembers) map[*machineTracker]replicaset.MemberStatus {
	statuses := make(map[*machineTracker]replicaset.MemberStatus)
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

// desiredPeerGroup returns the mongo peer group according to the given
// servers and a map with an element for each machine in info.machines
// specifying whether that machine has been configured as voting. It will
// return a nil member list and error if the current group is already
// correct, though the voting map will be still be returned in that case.
func desiredPeerGroup(info *peerGroupInfo) ([]replicaset.Member, map[*machineTracker]bool, error) {
	if len(info.members) == 0 {
		return nil, nil, fmt.Errorf("current member set is empty")
	}

	members, extra, maxId := info.membersMap()
	logger.Debugf(getLogMessage(members, extra, maxId))

	// We may find extra peer group members if the machines
	// have been removed or their controller status removed.
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
	changed, err := checkExtraMembers(extra)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Determine the addresses to be used for replica-set communication.
	addrs, err := getMongoAddresses(info)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	toRemoveVote, toAddVote, toKeep := possiblePeerGroupChanges(info, members)

	// Set up initial record of machine votes. Any changes after
	// this will trigger a peer group election.
	machineVoting := make(map[*machineTracker]bool)
	for _, m := range info.machineTrackers {
		member := members[m]
		machineVoting[m] = member != nil && isVotingMember(member)
	}
	setVoting := func(m *machineTracker, voting bool) {
		setMemberVoting(members[m], voting)
		machineVoting[m] = voting
		changed = true
	}
	adjustVotes(toRemoveVote, toAddVote, setVoting)

	addNewMembers(members, toKeep, maxId, setVoting, addrs)
	if updateAddresses(members, addrs) {
		changed = true
	}
	if !changed {
		return nil, machineVoting, nil
	}
	var memberSet []replicaset.Member
	for _, member := range members {
		memberSet = append(memberSet, *member)
	}
	return memberSet, machineVoting, nil
}

// getLogMessage generates a nicely formatted log message for the input
// replica-set arguments.
func getLogMessage(members machineMembers, extra []replicaset.Member, maxId int) string {
	lines := make([]string, 0)
	lines = append(lines, fmt.Sprintf("calculated desired peer group\ndesired voting members: (maxId: %d)", maxId))
	for tracker, replMem := range members {
		lines = append(lines, fmt.Sprintf("\n   %#v: rs_id=%d, rs_addr=%s", tracker, replMem.Id, replMem.Address))
	}
	if len(extra) > 0 {
		lines = append(lines, "\nother members:")
		for _, replMem := range extra {
			vote := replMem.Votes != nil && *replMem.Votes > 0
			lines = append(lines, fmt.Sprintf("\n   rs_id=%d, rs_addr=%s, tags=%v, vote=%t",
				replMem.Id, replMem.Address, replMem.Tags, vote))
		}
	}
	return strings.Join(lines, "")
}

// checkExtraMembers checks to see if any of the input members, identified as
// not being associated with machines, is set as a voter in the peer group.
// If any have, an error is returned.
// The boolean indicates whether any extra members were present at all.
func checkExtraMembers(extra []replicaset.Member) (bool, error) {
	for _, member := range extra {
		if isVotingMember(&member) {
			return false, fmt.Errorf("voting non-machine member %#v found in peer group", member)
		}
	}
	return len(extra) > 0, nil
}

func isVotingMember(member *replicaset.Member) bool {
	return member.Votes == nil || *member.Votes > 0
}

// getMongoAddresses gets an address suitable for Mongo peer group
// communication for each tracked machine.
// An error will be returned if more that one address is found for a machine
// and there is no HA space is configured.
func getMongoAddresses(info *peerGroupInfo) (
	map[*machineTracker]string, error,
) {
	addrs := make(map[*machineTracker]string, len(info.machineTrackers))
	for _, m := range info.machineTrackers {
		var err error
		if addrs[m], err = m.SelectMongoAddress(info.mongoPort, info.haSpace); err != nil {
			return addrs, errors.Trace(err)
		}
	}
	return addrs, nil
}

// possiblePeerGroupChanges returns a set of slices classifying all the
// existing machines according to how their vote might move.
// toRemoveVote holds machines whose vote should be removed;
// toAddVote holds machines which are ready to vote;
// toKeep holds machines with no desired change to their voting status
// (this includes machines that are not yet represented in the peer group).
func possiblePeerGroupChanges(info *peerGroupInfo, members machineMembers) (
	toRemoveVote, toAddVote, toKeep []*machineTracker,
) {
	statuses := info.statusesMap(members)

	logger.Debugf("assessing possible peer group changes:")
	for _, m := range info.machineTrackers {
		member := members[m]
		wantsVote := m.WantsVote()
		isVoting := member != nil && isVotingMember(member)
		switch {
		case wantsVote && isVoting:
			logger.Debugf("machine %q is already voting", m.Id())
			toKeep = append(toKeep, m)
		case wantsVote && !isVoting:
			if status, ok := statuses[m]; ok && isReady(status) {
				logger.Debugf("machine %q is a potential voter", m.Id())
				toAddVote = append(toAddVote, m)
			} else {
				logger.Debugf("machine %q is not ready (status: %v, healthy: %v)", m.Id(), status.State, status.Healthy)
				toKeep = append(toKeep, m)
			}
		case !wantsVote && isVoting:
			logger.Debugf("machine %q is a potential non-voter", m.Id())
			toRemoveVote = append(toRemoveVote, m)
		case !wantsVote && !isVoting:
			logger.Debugf("machine %q does not want the vote", m.Id())
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

// adjustVotes adjusts the votes of the given machines, taking
// care not to let the total number of votes become even at
// any time. It calls setVoting to change the voting status
// of a machine.
func adjustVotes(toRemoveVote, toAddVote []*machineTracker, setVoting func(*machineTracker, bool)) {
	// Remove voting members if they can be replaced by
	// candidates that are ready. This does not affect
	// the total number of votes.
	nReplace := min(len(toRemoveVote), len(toAddVote))
	for i := 0; i < nReplace; i++ {
		from := toRemoveVote[i]
		to := toAddVote[i]
		setVoting(from, false)
		setVoting(to, true)
	}
	toAddVote = toAddVote[nReplace:]
	toRemoveVote = toRemoveVote[nReplace:]

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

// addNewMembers adds new members from toKeep to the given set of members,
// allocating ids from maxId upwards.
// It calls setVoting to set the voting status of each new member.
func addNewMembers(
	members machineMembers,
	toKeep []*machineTracker,
	maxId int,
	setVoting func(*machineTracker, bool),
	addrs map[*machineTracker]string,
) {
	for _, m := range toKeep {
		if members[m] != nil {
			continue
		}
		if addrs[m] == "" {
			logger.Debugf("ignoring machine %q with no address", m.Id())
			continue
		}
		// This machine was not previously in the members list,
		// so add it (as non-voting). We maintain the
		// id manually to make it easier for tests.
		maxId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuMachineKey: m.Id(),
			},
			Id: maxId,
		}
		members[m] = member
		setVoting(m, false)
	}
}

// updateAddresses updates the members' addresses from the machines' addresses.
// It reports whether any changes have been made.
func updateAddresses(members machineMembers, addrs map[*machineTracker]string) bool {
	changed := false

	// Make sure all members' machine addresses are up to date.
	for m, addr := range addrs {
		if addr == "" {
			continue
		}
		// TODO ensure that replicaset works correctly with IPv6 [host]:port addresses.
		if addr != members[m].Address {
			members[m].Address = addr
			changed = true
		}
	}
	return changed
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

type byId []*machineTracker

func (l byId) Len() int           { return len(l) }
func (l byId) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }
func (l byId) Less(i, j int) bool { return l[i].Id() < l[j].Id() }

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
