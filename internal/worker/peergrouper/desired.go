// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/replicaset/v3"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

// jujuNodeKey is the key for the tag where we save a member's node id.
const jujuNodeKey = "juju-machine-id"

// peerGroupInfo holds information used in attempting to determine a Mongo
// peer group.
type peerGroupInfo struct {
	// Maps below are keyed on node ID.

	// controllers holds the controllerTrackers for known controller nodes sourced from the peergrouper
	// worker. Indexed by node.Id()
	controllers map[string]*controllerTracker

	// Replica-set members sourced from the Mongo session that are recognised by
	// their association with known controller nodes.
	recognised map[string]replicaset.Member

	// Replica-set member statuses sourced from the Mongo session.
	statuses map[string]replicaset.MemberStatus

	toRemove    []replicaset.Member
	extra       []replicaset.Member
	maxMemberId int
	mongoPort   int
}

// desiredChanges tracks the specific changes we are asking to be made to the peer group.
type desiredChanges struct {
	// isChanged is set False if the existing peer group is already in a valid configuration.
	isChanged bool

	// stepDownPrimary is set if we want to remove the vote from the Mongo Primary. This is specially flagged,
	// because you have to ask the primary to step down before you can remove its vote.
	stepDownPrimary bool

	// members is the map of Id to replicaset.Member for the desired list of controller nodes in the replicaset.
	members map[string]*replicaset.Member
}

// peerGroupChanges tracks the process of computing the desiredChanges to the peer group.
type peerGroupChanges struct {
	// info is the input state we will be processing
	info *peerGroupInfo

	// this block all represents active processing state
	toRemoveVote                []string
	toAddVote                   []string
	toKeepVoting                []string
	toKeepNonVoting             []string
	toKeepCreateNonVotingMember []string

	// desired tracks the final changes to the peer group that we want to make
	desired desiredChanges
}

func newPeerGroupInfo(
	controllers map[string]*controllerTracker,
	statuses []replicaset.MemberStatus,
	members []replicaset.Member,
	mongoPort int,
) (*peerGroupInfo, error) {
	if len(members) == 0 {
		return nil, fmt.Errorf("current member set is empty")
	}

	info := peerGroupInfo{
		controllers: controllers,
		statuses:    make(map[string]replicaset.MemberStatus),
		recognised:  make(map[string]replicaset.Member),
		maxMemberId: -1,
		mongoPort:   mongoPort,
	}

	// Iterate over the input members and associate them with a controller if
	// possible; add any non-juju unassociated members to the "extra" slice.
	// Unassociated members with the juju machine id tag are to be removed.
	// Link the statuses with the controller node IDs where associated.
	// Keep track of the highest member ID that we observe.
	for _, m := range members {
		if m.Id > info.maxMemberId {
			info.maxMemberId = m.Id
		}

		controllerId, ok := m.Tags[jujuNodeKey]
		if !ok {
			info.extra = append(info.extra, m)
			continue
		}
		found := false
		if node, got := controllers[controllerId]; got {
			info.recognised[controllerId] = m
			found = node.host.Life() != state.Dead
		}

		// This invariably makes for N^2, but we anticipate small N.
		for _, sts := range statuses {
			if sts.Id == m.Id {
				info.statuses[controllerId] = sts
			}
		}
		if !found {
			info.toRemove = append(info.toRemove, m)
		}
	}

	return &info, nil
}

// isPrimary returns true if the given controller node id is the mongo primary.
func (info *peerGroupInfo) isPrimary(workerControllerId string) (bool, error) {
	primaryNodeId := -1
	// Current status of replicaset contains node state.
	// Here we determine node id of the primary node.
	for _, m := range info.statuses {
		if m.State == replicaset.PrimaryState {
			primaryNodeId = m.Id
			break
		}
	}
	if primaryNodeId == -1 {
		return false, errors.NotFoundf("HA primary machine")
	}

	for _, m := range info.recognised {
		if m.Id == primaryNodeId {
			if primaryControllerId, ok := m.Tags[jujuNodeKey]; ok {
				return primaryControllerId == workerControllerId, nil
			}
		}
	}
	return false, errors.NotFoundf("HA primary machine")
}

// getLogMessage generates a nicely formatted log message from the known peer
// group information.
func (info *peerGroupInfo) getLogMessage() string {
	lines := []string{
		fmt.Sprintf("calculating desired peer group\ndesired voting members: (maxId: %d)", info.maxMemberId),
	}

	template := "\n   %#v: rs_id=%d, rs_addr=%s, rs_primary=%v"
	ids := make([]string, 0, len(info.recognised))
	for id := range info.recognised {
		ids = append(ids, id)
	}
	sortAsInts(ids)
	for _, id := range ids {
		rm := info.recognised[id]
		isPrimary := isPrimaryMember(info, id)
		lines = append(lines, fmt.Sprintf(template, info.controllers[id], rm.Id, rm.Address, isPrimary))
	}

	if len(info.toRemove) > 0 {
		lines = append(lines, "\nmembers to remove:")
		template := "\n   rs_id=%d, rs_addr=%s, tags=%v, vote=%t"
		for _, em := range info.toRemove {
			vote := em.Votes != nil && *em.Votes > 0
			lines = append(lines, fmt.Sprintf(template, em.Id, em.Address, em.Tags, vote))
		}
	}

	if len(info.extra) > 0 {
		lines = append(lines, "\nother non-juju  members:")
		template := "\n   rs_id=%d, rs_addr=%s, tags=%v, vote=%t"
		for _, em := range info.extra {
			vote := em.Votes != nil && *em.Votes > 0
			lines = append(lines, fmt.Sprintf(template, em.Id, em.Address, em.Tags, vote))
		}
	}

	return strings.Join(lines, "")
}

// initNewReplicaSet creates a new node ID indexed map of known replica-set
// members to use as the basis for a newly calculated replica-set.
func (p *peerGroupChanges) initNewReplicaSet() map[string]*replicaset.Member {
	rs := make(map[string]*replicaset.Member, len(p.info.recognised))
	for id := range p.info.recognised {
		// Local-scoped variable required here,
		// or the same pointer to the loop variable is used each time.
		m := p.info.recognised[id]
		rs[id] = &m
	}
	return rs
}

// desiredPeerGroup returns a new Mongo peer-group calculated from the input
// peerGroupInfo.
// Returned are the new members indexed by node ID, and a map indicating
// which controller nodes are set as voters in the new peer-group.
// If the new peer-group does not differ from that indicated by the input
// peerGroupInfo, a nil member map is returned along with the correct voters
// map.
// An error is returned if:
//  1. There are members unrecognised by controller node association,
//     and any of these are set as voters.
//  2. There is no HA space configured and any nodes have multiple
//     cloud-local addresses.
func desiredPeerGroup(info *peerGroupInfo) (desiredChanges, error) {
	logger.Debugf(context.TODO(), info.getLogMessage())

	peerChanges := peerGroupChanges{
		info: info,
		desired: desiredChanges{
			isChanged:       false,
			stepDownPrimary: false,
			members:         map[string]*replicaset.Member{},
		},
	}
	return peerChanges.computeDesiredPeerGroup()
}

func (p *peerGroupChanges) computeDesiredPeerGroup() (desiredChanges, error) {

	// We may find extra peer group members if the controller nodes have been
	// removed or their controller status removed.
	// This should only happen if they had been set to non-voting before
	// removal, in which case we want to remove them from the members list.
	// If we find a member that is still configured to vote, it is an error.
	// TODO: There are some other possibilities for what to do in that case.
	// 1) Leave them untouched, but deal with others as usual (ignore).
	// 2) Leave them untouched and deal with others, but make sure the extras
	//    are not eligible to be primary.
	// 3) Remove them.
	// 4) Do nothing.
	err := p.checkExtraMembers()
	if err != nil {
		return desiredChanges{}, errors.Trace(err)
	}

	p.desired.members = p.initNewReplicaSet()
	p.possiblePeerGroupChanges()
	p.reviewPeerGroupChanges()
	p.createNonVotingMember()

	// Set up initial record of controller node votes. Any changes after
	// this will trigger a peer group election.
	p.adjustVotes()

	if err := p.updateAddressesFromInternal(); err != nil {
		return desiredChanges{}, errors.Trace(err)
	}

	return p.desired, nil
}

// checkExtraMembers checks to see if any of the input members, identified as
// not being associated with controller nodes, is set as a voter in the peer group.
// If any have, an error is returned.
// The boolean indicates whether any extra members were present at all.
func (p *peerGroupChanges) checkExtraMembers() error {
	// Note: (jam 2018-04-18) With the new "juju remove-controller --force" it is much easier to get into this situation
	// because an active controller that is in the replicaset would get removed while it still had voting rights.
	// Given that Juju is in control of the replicaset we don't really just 'accept' that some other node has a vote.
	// *maybe* we could allow non-voting members that would be used by 3rd parties to provide a warm database backup.
	// But I think the right answer is probably to downgrade unknown members from voting.
	// Note: (wallyworld) notwithstanding the above, each controller runs its own peer grouper worker. The
	// mongo primary will remove nodes as needed from the replicaset. There will be a short time where
	// Juju managed nodes will not yet be accounted for by the other secondary workers. These are accounted
	// for in the 'toRemove' list.
	for _, member := range p.info.extra {
		if isVotingMember(&member) {
			return fmt.Errorf("non juju voting member %v found in peer group", member)
		}
	}
	if len(p.info.toRemove) > 0 || len(p.info.extra) > 0 {
		p.desired.isChanged = true
	}
	return nil
}

// sortAsInts converts all the vals to an integer to sort them as numbers instead of strings
// If any of the values are not valid integers, they will be sorted as strings, and added to the end
// the slice will be sorted in place.
// (generally this should only be used for strings we expect to represent ints, but we don't want to error if
// something isn't an int.)
func sortAsInts(vals []string) {
	asInts := make([]int, 0, len(vals))
	extra := []string{}
	for _, val := range vals {
		asInt, err := strconv.Atoi(val)
		if err != nil {
			extra = append(extra, val)
		} else {
			asInts = append(asInts, asInt)
		}
	}
	sort.Ints(asInts)
	sort.Strings(extra)
	i := 0
	for _, asInt := range asInts {
		vals[i] = strconv.Itoa(asInt)
		i++
	}
	for _, val := range extra {
		vals[i] = val
		i++
	}
}

// possiblePeerGroupChanges returns a set of slices classifying all the
// existing controller nodes according to how their vote might move.
// toRemoveVote holds nodes whose vote should be removed;
// toAddVote holds nodes which are ready to vote;
// toKeep holds nodes with no desired change to their voting status
// (this includes nodes that are not yet represented in the peer group).
func (p *peerGroupChanges) possiblePeerGroupChanges() {
	nodeIds := make([]string, 0, len(p.info.controllers))
	for id := range p.info.controllers {
		nodeIds = append(nodeIds, id)
	}
	sortAsInts(nodeIds)
	logger.Debugf(context.TODO(), "assessing possible peer group changes:")
	for _, id := range nodeIds {
		m := p.info.controllers[id]
		member := p.desired.members[id]
		if m.host.Life() != state.Alive {
			if _, ok := p.desired.members[id]; !ok {
				// Dead machine already removed from replicaset.
				continue
			}
			if isPrimaryMember(p.info, id) {
				p.desired.stepDownPrimary = true
			}
			delete(p.desired.members, id)
			p.desired.isChanged = true
			continue
		}
		isVoting := member != nil && isVotingMember(member)
		wantsVote := true
		switch {
		case wantsVote && isVoting:
			logger.Debugf(context.TODO(), "node %q is already voting", id)
			p.toKeepVoting = append(p.toKeepVoting, id)
		case wantsVote && !isVoting:
			if status, ok := p.info.statuses[id]; ok && isReady(status) {
				logger.Debugf(context.TODO(), "node %q is a potential voter", id)
				p.toAddVote = append(p.toAddVote, id)
			} else if member != nil {
				logger.Debugf(context.TODO(), "node %q exists but is not ready (status: %v, healthy: %v)",
					id, status.State, status.Healthy)
				p.toKeepNonVoting = append(p.toKeepNonVoting, id)
			} else {
				logger.Debugf(context.TODO(), "node %q does not exist and is not ready (status: %v, healthy: %v)",
					id, status.State, status.Healthy)
				p.toKeepCreateNonVotingMember = append(p.toKeepCreateNonVotingMember, id)
			}
		case !wantsVote && isVoting:
			p.toRemoveVote = append(p.toRemoveVote, id)
			if isPrimaryMember(p.info, id) {
				p.desired.stepDownPrimary = true
				logger.Debugf(context.TODO(), "primary node %q is a potential non-voter", id)
			} else {
				logger.Debugf(context.TODO(), "node %q is a potential non-voter", id)
			}
		case !wantsVote && !isVoting:
			logger.Debugf(context.TODO(), "node %q does not want the vote", id)
			p.toKeepNonVoting = append(p.toKeepNonVoting, id)
		}
	}
	logger.Debugf(context.TODO(), "assessed")
}

func isReady(status replicaset.MemberStatus) bool {
	return status.Healthy && (status.State == replicaset.PrimaryState ||
		status.State == replicaset.SecondaryState)
}

// reviewPeerGroupChanges adds some extra logic after creating
// possiblePeerGroupChanges to safely add or remove controller nodes, keeping the
// correct odd number of voters peer structure, and preventing the primary from
// demotion.
func (p *peerGroupChanges) reviewPeerGroupChanges() {
	currVoters := 0
	for _, m := range p.desired.members {
		if isVotingMember(m) {
			currVoters += 1
		}
	}
	keptVoters := currVoters - len(p.toRemoveVote)
	if keptVoters == 0 {
		// to keep no voters means to step down the primary without a replacement, which is not possible.
		// So restore the current primary. Once there is another member to work with after reconfiguring, we will then
		// be able to ask the current primary to step down, and then we can finally remove it.
		var tempToRemove []string
		for _, id := range p.toRemoveVote {
			isPrimary := isPrimaryMember(p.info, id)
			if !isPrimary {
				tempToRemove = append(tempToRemove, id)
			} else {
				logger.Debugf(context.TODO(), "asked to remove all voters, preserving primary voter %q", id)
				p.desired.stepDownPrimary = false
			}
		}
		p.toRemoveVote = tempToRemove
	}
	newCount := keptVoters + len(p.toAddVote)
	if (newCount)%2 == 1 {
		logger.Debugf(context.TODO(), "number of voters is odd")
		// if this is true we will create an odd number of voters
		return
	}
	if len(p.toAddVote) > 0 {
		last := p.toAddVote[len(p.toAddVote)-1]
		logger.Debugf(context.TODO(), "number of voters would be even, not adding %q to maintain odd", last)
		p.toAddVote = p.toAddVote[:len(p.toAddVote)-1]
		return
	}
	// we must remove an extra peer
	// make sure we don't pick the primary to be removed.
	for i, id := range p.toKeepVoting {
		if !isPrimaryMember(p.info, id) {
			p.toRemoveVote = append(p.toRemoveVote, id)
			logger.Debugf(context.TODO(), "removing vote from %q to maintain odd number of voters", id)
			if i == len(p.toKeepVoting)-1 {
				p.toKeepVoting = p.toKeepVoting[:i]
			} else {
				p.toKeepVoting = append(p.toKeepVoting[:i], p.toKeepVoting[i+1:]...)
			}
			break
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
			setMemberVoting(p.desired.members[id], voting)
		}
	}

	if len(p.toAddVote) > 0 ||
		len(p.toRemoveVote) > 0 ||
		len(p.toKeepCreateNonVotingMember) > 0 {
		p.desired.isChanged = true
	}
	setVoting(p.toAddVote, true)
	setVoting(p.toRemoveVote, false)
	setVoting(p.toKeepCreateNonVotingMember, false)
}

// createMembers from a list of member IDs, instantiate a new replica-set
// member and add it to members map with the given ID.
func (p *peerGroupChanges) createNonVotingMember() {
	for _, id := range p.toKeepCreateNonVotingMember {
		logger.Debugf(context.TODO(), "create member with id %q", id)
		p.info.maxMemberId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuNodeKey: id,
			},
			Id: p.info.maxMemberId,
		}
		setMemberVoting(member, false)
		p.desired.members[id] = member
	}
	for _, id := range p.toKeepNonVoting {
		if p.desired.members[id] != nil {
			continue
		}
		logger.Debugf(context.TODO(), "create member with id %q", id)
		p.info.maxMemberId++
		member := &replicaset.Member{
			Tags: map[string]string{
				jujuNodeKey: id,
			},
			Id: p.info.maxMemberId,
		}
		setMemberVoting(member, false)
		p.desired.members[id] = member
	}
}

// updateAddressesFromInternal attempts to update each member with a
// cloud-local address from the node.
// If there is a single cloud local address available, it is used.
// If there are multiple addresses, then a check is made to ensure that:
//   - the member was previously in the replica-set and;
//   - the previous address used for replication is still available.
//
// If the check is satisfied, no change is made.
// Otherwise, one single address is selected from the list and the member
// address is updated.
func (p *peerGroupChanges) updateAddressesFromInternal() error {
	ids := p.sortedMemberIds()

	for _, id := range ids {
		m := p.info.controllers[id]
		hostPorts := m.GetPotentialMongoHostPorts(p.info.mongoPort)

		if len(hostPorts) > 1 {
			// ignore /32 and /64 addresses, which can be associated with a virtual IP (see https://bugs.launchpad.net/juju/+bug/2073986)
			hostPorts = slices.DeleteFunc(hostPorts, func(addr network.SpaceHostPort) bool {
				if addr.AddressType() == network.IPv6Address {
					return strings.HasSuffix(addr.AddressCIDR(), "/128")
				}
				return strings.HasSuffix(addr.AddressCIDR(), "/32")
			})
		}
		addrs := hostPorts.AllMatchingScope(network.ScopeMatchCloudLocal)

		// This should not happen because SelectInternalHostPorts will choose a
		// public address when there are no cloud-local addresses.
		// Zero addresses would mean the node is completely inaccessible.
		// We ignore this outcome and leave the address alone.
		if len(addrs) == 0 {
			continue
		}

		// Unique address; we can use this for Mongo peer communication.
		member := p.desired.members[id]
		if len(addrs) == 1 {
			addr := addrs[0]
			logger.Debugf(context.TODO(), "node %q selected address %q by scope from %v", id, addr, hostPorts)

			if member.Address != addr {
				member.Address = addr
				p.desired.isChanged = true
			}
			continue
		}

		// Multiple potential Mongo addresses.
		// Checks are required in order to use it as a peer.
		unchanged := false
		if _, ok := p.info.recognised[id]; ok {
			for _, addr := range addrs {
				if member.Address == addr {
					unchanged = true
					break
				}
			}
		}

		// If this member was not previously in the replica-set, or if
		// its address has changed, we simply select the first one when
		// there are multiple cloud-local addresses.
		if !unchanged {
			member.Address = selectSingleAddress(addrs)
			p.desired.isChanged = true
		}
	}

	return nil
}

// selectSingleAddress selects only one address from list of addresses. The
// selection is done in a consistent fashion by simply sorting the list of
// addresses and then selecting the first one.
func selectSingleAddress(addrs []string) string {
	slices.Sort(addrs)
	return addrs[0]
}

// sortedMemberIds returns the list of p.desired.members in integer-sorted order
func (p *peerGroupChanges) sortedMemberIds() []string {
	memberIds := make([]string, 0, len(p.desired.members))
	for id := range p.desired.members {
		memberIds = append(memberIds, id)
	}
	sortAsInts(memberIds)
	return memberIds
}

// getStatusInfo creates and returns a StatusInfo instance for use as a controller
// status. The *controller* status is not ideal for conveying this information,
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
