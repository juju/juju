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

// peerGroupInfo holds information used in attempting to determine a Mongo
// peer group.
type peerGroupInfo struct {
	// Maps below are keyed on machine ID.

	// Trackers for known controller machines sourced from the peergrouper
	// worker.
	machines map[string]*machineTracker

	// Replica-set members sourced from the Mongo session that are recognised by
	// their association with known machines.
	recognised map[string]replicaset.Member

	// Replica-set member statuses sourced from the Mongo session.
	statuses map[string]replicaset.MemberStatus

	extra       []replicaset.Member
	maxMemberId int
	mongoPort   int
	haSpace     network.SpaceName
}

func newPeerGroupInfo(
	machines map[string]*machineTracker,
	statuses []replicaset.MemberStatus,
	members []replicaset.Member,
	mongoPort int,
	haSpace network.SpaceName,
) (*peerGroupInfo, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("current member set is empty")
	}

	info := peerGroupInfo{
		machines:    machines,
		statuses:    make(map[string]replicaset.MemberStatus),
		recognised:  make(map[string]replicaset.Member),
		maxMemberId: -1,
		mongoPort:   mongoPort,
		haSpace:     haSpace,
	}

	// Iterate over the input members and associate them with a machine if
	// possible; add any unassociated members to the "extra" slice.
	// Link the statuses with the machine IDs where associated.
	// Keep track of the highest member ID that we observe.
	for _, m := range members {
		found := false
		if id, ok := m.Tags[jujuMachineKey]; ok {
			if machines[id] != nil {
				info.recognised[id] = m
				found = true
			}

			// This invariably makes for N^2, but we anticipate small N.
			for _, sts := range statuses {
				if sts.Id == m.Id {
					info.statuses[id] = sts
				}
			}
		}
		if !found {
			info.extra = append(info.extra, m)
		}

		if m.Id > info.maxMemberId {
			info.maxMemberId = m.Id
		}
	}

	return &info, nil
}

// getLogMessage generates a nicely formatted log message from the known peer
// group information.
func (info *peerGroupInfo) getLogMessage() string {
	lines := []string{
		fmt.Sprintf("calculating desired peer group\ndesired voting members: (maxId: %d)", info.maxMemberId),
	}

	template := "\n   %#v: rs_id=%d, rs_addr=%s"
	for id, rm := range info.recognised {
		lines = append(lines, fmt.Sprintf(template, info.machines[id], rm.Id, rm.Address))
	}

	if len(info.extra) > 0 {
		lines = append(lines, "\nother members:")

		template := "\n   rs_id=%d, rs_addr=%s, tags=%v, vote=%t"
		for _, em := range info.extra {
			vote := em.Votes != nil && *em.Votes > 0
			lines = append(lines, fmt.Sprintf(template, em.Id, em.Address, em.Tags, vote))
		}
	}

	return strings.Join(lines, "")
}

// initNewReplicaSet creates a new machine ID indexed map of known replica-set
// members to use as the basis for a newly calculated replica-set.
func (info *peerGroupInfo) initNewReplicaSet() map[string]*replicaset.Member {
	rs := make(map[string]*replicaset.Member, len(info.recognised))
	for id := range info.recognised {
		// Local-scoped variable required here,
		// or the same pointer to the loop variable is used each time.
		m := info.recognised[id]
		rs[id] = &m
	}
	return rs
}

// desiredPeerGroup returns the mongo peer group according to the given
// servers and a map with an element for each machine in info.machines
// specifying whether that machine has been configured as voting. It will
// return a nil member list and error if the current group is already
// correct, though the voting map will be still be returned in that case.
func desiredPeerGroup(info *peerGroupInfo) (map[string]*replicaset.Member, map[string]bool, error) {
	logger.Debugf(info.getLogMessage())

	// We may find extra peer group members if the machines have been removed
	// or their controller status removed.
	// This should only happen if they had been set to non-voting before
	// removal, in which case we want to remove them from the members list.
	// If we find a member that is still configured to vote, it is an error.
	// TODO: There are some other possibilities for what to do in that case.
	// 1) Leave them untouched, but deal with others as usual (ignore).
	// 2) Leave them untouched and deal with others, but make sure the extras
	//    are not eligible to be primary.
	// 3) Remove them.
	// 4) Do nothing.
	changed, err := checkExtraMembers(info.extra)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Determine the addresses to be used for replica-set communication.
	addrs, err := getMongoAddresses(info)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	newMembers := info.initNewReplicaSet()
	toRemoveVote, toAddVote, toKeep := possiblePeerGroupChanges(info, newMembers)

	// Set up initial record of machine votes. Any changes after
	// this will trigger a peer group election.
	machineVoting := make(map[string]bool)
	for id, m := range newMembers {
		machineVoting[id] = isVotingMember(m)
	}

	setVoting := func(id string, voting bool) {
		setMemberVoting(newMembers[id], voting)
		machineVoting[id] = voting
		changed = true
	}
	adjustVotes(toRemoveVote, toAddVote, setVoting)

	addNewMembers(newMembers, toKeep, info.maxMemberId, setVoting, addrs)
	if updateAddresses(newMembers, addrs) {
		changed = true
	}

	if !changed {
		return nil, machineVoting, nil
	}
	return newMembers, machineVoting, nil
}

// checkExtraMembers checks to see if any of the input members, identified as
// not being associated with machines, is set as a voter in the peer group.
// If any have, an error is returned.
// The boolean indicates whether any extra members were present at all.
func checkExtraMembers(extra []replicaset.Member) (bool, error) {
	for _, member := range extra {
		if isVotingMember(&member) {
			return true, fmt.Errorf("voting non-machine member %v found in peer group", member)
		}
	}
	return len(extra) > 0, nil
}

func isVotingMember(m *replicaset.Member) bool {
	v := m.Votes
	return v == nil || *v > 0
}

// getMongoAddresses gets an address suitable for Mongo peer group
// communication for each tracked machine.
// An error will be returned if more that one address is found for a machine
// and there is no HA space is configured.
func getMongoAddresses(info *peerGroupInfo) (map[string]string, error) {
	addrs := make(map[string]string, len(info.machines))
	for id, m := range info.machines {
		var err error
		if addrs[id], err = m.SelectMongoAddress(info.mongoPort, info.haSpace); err != nil {
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
func possiblePeerGroupChanges(
	info *peerGroupInfo,
	members map[string]*replicaset.Member,
) (toRemoveVote, toAddVote, toKeep []string) {
	logger.Debugf("assessing possible peer group changes:")
	for id, m := range info.machines {
		member := members[id]
		isVoting := member != nil && isVotingMember(member)
		wantsVote := m.WantsVote()
		switch {
		case wantsVote && isVoting:
			logger.Debugf("machine %q is already voting", id)
			toKeep = append(toKeep, id)
		case wantsVote && !isVoting:
			if status, ok := info.statuses[id]; ok && isReady(status) {
				logger.Debugf("machine %q is a potential voter", id)
				toAddVote = append(toAddVote, id)
			} else {
				logger.Debugf("machine %q is not ready (status: %v, healthy: %v)", id, status.State, status.Healthy)
				toKeep = append(toKeep, id)
			}
		case !wantsVote && isVoting:
			logger.Debugf("machine %q is a potential non-voter", id)
			toRemoveVote = append(toRemoveVote, id)
		case !wantsVote && !isVoting:
			logger.Debugf("machine %q does not want the vote", id)
			toKeep = append(toKeep, id)
		}
	}
	logger.Debugf("assessed")

	// sort machines to be added and removed so that we get deterministic
	// behaviour when testing.
	// Earlier entries will be dealt with preferentially, so we could
	// potentially sort by some other metric in each case.
	sort.Strings(toRemoveVote)
	sort.Strings(toAddVote)
	sort.Strings(toKeep)
	return toRemoveVote, toAddVote, toKeep
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

// adjustVotes adjusts the votes of the given machines, taking care not to let
// the total number of votes become even at any time.
// It calls setVoting to change the voting status of a machine.
func adjustVotes(toRemoveVote, toAddVote []string, setVoting func(string, bool)) {
	// Remove voting members if they can be replaced by candidates that are
	// ready. This does not affect the total number of votes.
	nReplace := min(len(toRemoveVote), len(toAddVote))
	for i := 0; i < nReplace; i++ {
		setVoting(toRemoveVote[i], false)
		setVoting(toAddVote[i], true)
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

// addNewMembers adds new members from toKeep to the new replica-set,
// allocating IDs from maxId upwards.
// It calls setVoting to set the voting status of each new member.
func addNewMembers(
	members map[string]*replicaset.Member,
	toKeep []string,
	maxId int,
	setVoting func(string, bool),
	addrs map[string]string,
) {
	for _, id := range toKeep {
		if members[id] != nil {
			continue
		}
		if addrs[id] == "" {
			logger.Debugf("ignoring machine %q with no address", id)
			continue
		}
		// This machine was not previously in the members list,
		// so add it (as non-voting).
		// We maintain the ID manually to make it easier for tests.
		maxId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuMachineKey: id,
			},
			Id: maxId,
		}
		members[id] = member
		setVoting(id, false)
	}
}

// updateAddresses updates the member addresses in the new replica-set with
// those determined by getMongoAddresses, where they differ.
// The return indicates whether any changes were made.
func updateAddresses(members map[string]*replicaset.Member, addrs map[string]string) bool {
	changed := false

	// Make sure all members' machine addresses are up to date.
	for id, addr := range addrs {
		if addr == "" {
			continue
		}
		if addr != members[id].Address {
			members[id].Address = addr
			changed = true
		}
	}
	return changed
}

func isReady(status replicaset.MemberStatus) bool {
	return status.Healthy && (status.State == replicaset.PrimaryState ||
		status.State == replicaset.SecondaryState)
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
