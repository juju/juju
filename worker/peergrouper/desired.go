// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/replicaset"

	"github.com/juju/juju/network"
	"github.com/juju/juju/status"
)

// jujuMachineKey is the key for the tag where we save a member's machine id.
const jujuMachineKey = "juju-machine-id"

// peerGroupInfo holds information used in attempting to determine a Mongo
// peer group.
type peerGroupInfo struct {
	// Maps below are keyed on machine ID.

	// machines holds the machineTrackers for known controller machines sourced from the peergrouper
	// worker. Indexed by machine.Id()
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

type peerGroupChanges struct {
	isChanged                   bool
	toRemoveVote                []string
	toAddVote                   []string
	toKeepVoting                []string
	toKeepNonVoting             []string
	toKeepCreateNonVotingMember []string
	machineVoting               map[string]bool
	members                     map[string]*replicaset.Member
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

// desiredPeerGroup returns a new Mongo peer-group calculated from the input
// peerGroupInfo.
// Returned are the new members indexed by machine ID, and a map indicating
// which machines are set as voters in the new new peer-group.
// If the new peer-group is does not differ from that indicated by the input
// peerGroupInfo, a nil member map is returned along with the correct voters
// map.
// An error is returned if:
//   1) There are members unrecognised by machine association,
//      and any of these are set as voters.
//   2) There is no HA space configured and any machines have multiple
//      cloud-local addresses.
func desiredPeerGroup(info *peerGroupInfo) (map[string]*replicaset.Member, map[string]bool, error) {
	logger.Debugf(info.getLogMessage())

	peerChanges := peerGroupChanges{
		isChanged:     false,
		machineVoting: map[string]bool{},
		members:       map[string]*replicaset.Member{},
	}

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
	err := peerChanges.checkExtraMembers(info.extra)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	peerChanges.members = info.initNewReplicaSet()
	peerChanges.possiblePeerGroupChanges(info)
	peerChanges.reviewPeerGroupChanges(info)
	peerChanges.createNonVotingMember(&info.maxMemberId)

	// Set up initial record of machine votes. Any changes after
	// this will trigger a peer group election.
	peerChanges.getMachinesVoting()
	peerChanges.adjustVotes()

	if err := peerChanges.updateAddresses(info); err != nil {
		return nil, nil, errors.Trace(err)
	}

	if !peerChanges.isChanged {
		return nil, peerChanges.machineVoting, nil
	}
	return peerChanges.members, peerChanges.machineVoting, nil
}

// checkExtraMembers checks to see if any of the input members, identified as
// not being associated with machines, is set as a voter in the peer group.
// If any have, an error is returned.
// The boolean indicates whether any extra members were present at all.
func (p *peerGroupChanges) checkExtraMembers(extra []replicaset.Member) error {
	for _, member := range extra {
		if isVotingMember(&member) {
			return fmt.Errorf("voting non-machine member %v found in peer group", member)
		}
	}
	if len(extra) > 0 {
		p.isChanged = true
	}
	return nil
}

// possiblePeerGroupChanges returns a set of slices classifying all the
// existing machines according to how their vote might move.
// toRemoveVote holds machines whose vote should be removed;
// toAddVote holds machines which are ready to vote;
// toKeep holds machines with no desired change to their voting status
// (this includes machines that are not yet represented in the peer group).
func (p *peerGroupChanges) possiblePeerGroupChanges(info *peerGroupInfo) {
	machineIds := make([]string, 0, len(info.machines))
	for id := range info.machines {
		machineIds = append(machineIds, id)
	}
	sort.Strings(machineIds)
	logger.Debugf("assessing possible peer group changes:")
	for _, id := range machineIds {
		m := info.machines[id]
		member := p.members[id]
		isVoting := member != nil && isVotingMember(member)
		wantsVote := m.WantsVote()
		switch {
		case wantsVote && isVoting:
			logger.Debugf("machine %q is already voting", id)
			p.toKeepVoting = append(p.toKeepVoting, id)
		case wantsVote && !isVoting:
			if status, ok := info.statuses[id]; ok && isReady(status) {
				logger.Debugf("machine %q is a potential voter", id)
				p.toAddVote = append(p.toAddVote, id)
			} else if member != nil {
				logger.Debugf("machine %q exists but is not ready (status: %v, healthy: %v)",
					id, status.State, status.Healthy)
				p.toKeepNonVoting = append(p.toKeepNonVoting, id)
			} else {
				logger.Debugf("machine %q does not exist and is not ready (status: %v, healthy: %v)",
					id, status.State, status.Healthy)
				p.toKeepCreateNonVotingMember = append(p.toKeepCreateNonVotingMember, id)
			}
		case !wantsVote && isVoting:
			logger.Debugf("machine %q is a potential non-voter", id)
			p.toRemoveVote = append(p.toRemoveVote, id)
		case !wantsVote && !isVoting:
			logger.Debugf("machine %q does not want the vote", id)
			p.toKeepNonVoting = append(p.toKeepNonVoting, id)
		}
	}
	logger.Debugf("assessed")
}

func isReady(status replicaset.MemberStatus) bool {
	return status.Healthy && (status.State == replicaset.PrimaryState ||
		status.State == replicaset.SecondaryState)
}

// reviewPeerGroupChanges adds some extra logic after creating
// possiblePeerGroupChanges to safely add or remove machines, keeping the
// correct odd number of voters peer structure, and preventing the primary from
// demotion.
func (p *peerGroupChanges) reviewPeerGroupChanges(info *peerGroupInfo) {
	currVoters := 0
	for _, m := range p.members {
		if isVotingMember(m) {
			currVoters += 1
		}
	}
	keptVoters := currVoters - len(p.toRemoveVote)
	if (keptVoters+len(p.toAddVote))%2 == 1 {
		logger.Debugf("number of voters is odd")
		// if this is true we will create an odd number of voters
		return
	}
	if len(p.toAddVote) > 0 {
		logger.Debugf("number of voters is even, trim last member from toAddVote")
		p.toAddVote = p.toAddVote[:len(p.toAddVote)-1]
		return
	}
	// we must remove an extra peer
	// make sure we don't pick the primary to be removed.
	if keptVoters == 0 {
		// we are asking to remove all voters, a clear 'odd' number of voters
		// to preserve is to just keep the current primary.
		logger.Debugf("remove all voters, preserve primary voter")
		var tempToRemove []string
		for _, id := range p.toRemoveVote {
			isPrimary := isPrimaryMember(info, id)
			if !isPrimary {
				tempToRemove = append(tempToRemove, id)
			}
		}
		p.toRemoveVote = tempToRemove
	} else {
		for i, id := range p.toKeepVoting {
			if !isPrimaryMember(info, id) {
				p.toRemoveVote = append(p.toRemoveVote, id)
				if i == len(p.toKeepVoting)-1 {
					p.toKeepVoting = p.toKeepVoting[:i]
				} else {
					p.toKeepVoting = append(p.toKeepVoting[:i], p.toKeepVoting[i+1:]...)
				}
				break
			}
		}
	}
}

func isVotingMember(m *replicaset.Member) bool {
	v := m.Votes
	return v == nil || *v > 0
}

func isPrimaryMember(info *peerGroupInfo, id string) bool {
	return info.statuses[id].State == replicaset.PrimaryState
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

// adjustVotes removes and adds votes to the members via setVoting.
func (p *peerGroupChanges) adjustVotes() {
	setVoting := func(memberIds []string, voting bool) {
		for _, id := range memberIds {
			setMemberVoting(p.members[id], voting)
			p.machineVoting[id] = voting
		}
	}

	if len(p.toAddVote) > 0 ||
		len(p.toRemoveVote) > 0 ||
		len(p.toKeepCreateNonVotingMember) > 0 {
		p.isChanged = true
	}
	setVoting(p.toAddVote, true)
	setVoting(p.toRemoveVote, false)
	setVoting(p.toKeepCreateNonVotingMember, false)
}

// createMembers from a list of member IDs, instantiate a new replica-set
// member and add it to members map with the given ID.
func (p *peerGroupChanges) createNonVotingMember(
	maxId *int,
) {
	for _, id := range p.toKeepCreateNonVotingMember {
		logger.Debugf("create member with id %q", id)
		*maxId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuMachineKey: id,
			},
			Id: *maxId,
		}
		setMemberVoting(member, false)
		p.members[id] = member
	}
	for _, id := range p.toKeepNonVoting {
		if p.members[id] != nil {
			continue
		}
		logger.Debugf("create member with id %q", id)
		*maxId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuMachineKey: id,
			},
			Id: *maxId,
		}
		setMemberVoting(member, false)
		p.members[id] = member
	}
}

func (p *peerGroupChanges) getMachinesVoting() {
	for id, m := range p.members {
		p.machineVoting[id] = isVotingMember(m)
	}
}

// updateAddresses updates the member addresses in the new replica-set, using
// the HA space if one is configured.
func (p *peerGroupChanges) updateAddresses(info *peerGroupInfo) error {
	var err error
	if info.haSpace == "" {
		err = p.updateAddressesFromInternal(info)
	} else {
		err = p.updateAddressesFromSpace(info)
	}
	return errors.Annotate(err, "updating member addresses")
}

const multiAddressMessage = "multiple usable addresses found" +
	"\nrun \"juju config juju-ha-space=<name>\" to set a space for Mongo peer communication"

// updateAddressesFromInternal attempts to update each member with a
// cloud-local address from the machine.
// If there is a single cloud local address available, it is used.
// If there are multiple addresses, then a check is made to ensure that:
//   - the member was previously in the replica-set and;
//   - the previous address used for replication is still available.
// If the check is satisfied, then a warning is logged and no change is made.
// Otherwise an error is returned to indicate that a HA space must be
// configured in order to proceed. Such machines have their status set to
// indicate that they require intervention.
func (p *peerGroupChanges) updateAddressesFromInternal(info *peerGroupInfo) error {
	var multipleAddresses []string

	for id := range p.members {
		m := info.machines[id]
		hostPorts := m.GetPotentialMongoHostPorts(info.mongoPort)
		addrs := network.SelectInternalHostPorts(hostPorts, false)

		// This should not happen because SelectInternalHostPorts will choose a
		// public address when there are no cloud-local addresses.
		// Zero addresses would mean the machine is completely inaccessible.
		// We ignore this outcome and leave the address alone.
		if len(addrs) == 0 {
			continue
		}

		// Unique address; we can use this for Mongo peer communication.
		member := p.members[id]
		if len(addrs) == 1 {
			addr := addrs[0]
			logger.Debugf("machine %q selected address %q by scope from %v", id, addr, hostPorts)

			if member.Address != addr {
				member.Address = addr
				p.isChanged = true
			}
			continue
		}

		// Multiple potential Mongo addresses.
		// Checks are required in order to use it as a peer.
		unchanged := false
		if _, ok := info.recognised[id]; ok {
			for _, addr := range addrs {
				if member.Address == addr {
					logger.Warningf("%s\npreserving member with unchanged address %q", multiAddressMessage, addr)
					unchanged = true
					break
				}
			}
		}

		// If this member was not previously in the replica-set, or if its
		// address has changed, we enforce the policy of requiring a
		// configured HA space when there are multiple cloud-local addresses.
		if !unchanged {
			multipleAddresses = append(multipleAddresses, id)
			if err := m.stm.SetStatus(getStatusInfo(multiAddressMessage)); err != nil {
				return errors.Trace(err)
			}
		}
	}

	if len(multipleAddresses) > 0 {
		ids := strings.Join(multipleAddresses, ", ")
		return fmt.Errorf("juju-ha-space is not set and these machines have more than one usable address: %s"+
			"\nrun \"juju config juju-ha-space=<name>\" to set a space for Mongo peer communication", ids)
	}
	return nil
}

// updateAddressesFromSpace updates the member addresses based on the
// configured HA space.
// If no addresses are available for any of the machines, then such machines
// have their status set and are included in the detail of the returned error.
func (p *peerGroupChanges) updateAddressesFromSpace(info *peerGroupInfo) error {
	space := info.haSpace
	var noAddresses []string

	for id := range p.members {
		m := info.machines[id]
		addr, err := m.SelectMongoAddressFromSpace(info.mongoPort, space)
		if err != nil {
			if errors.IsNotFound(err) {
				noAddresses = append(noAddresses, id)
				msg := fmt.Sprintf("no addresses in configured juju-ha-space %q", space)
				if err := m.stm.SetStatus(getStatusInfo(msg)); err != nil {
					return errors.Trace(err)
				}
				continue
			}
			return errors.Trace(err)
		}
		if addr != p.members[id].Address {
			p.members[id].Address = addr
			p.isChanged = true
		}
	}

	if len(noAddresses) > 0 {
		ids := strings.Join(noAddresses, ", ")
		return fmt.Errorf("no usable Mongo addresses found in configured juju-ha-space %q for machines: %s", space, ids)
	}
	return nil
}

// getStatusInfo creates and returns a StatusInfo instance for use as a machine
// status. The *machine* status is not ideal for conveying this information,
// which is a really a characteristic of its role as a controller application.
// For this reason we leave the status as "Started" and supplement with an
// appropriate message.
// This is subject to change if/when controller status is represented in its
// own right.
func getStatusInfo(msg string) status.StatusInfo {
	now := time.Now()
	return status.StatusInfo{
		Status:  status.Started,
		Message: msg,
		Since:   &now,
	}
}
