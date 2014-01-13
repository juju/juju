package peergrouper

import (
	"fmt"
	"net"
	"sort"

	"launchpad.net/juju-core/replicaset"
	"launchpad.net/loggo"
)

var logger = loggo.GetLogger("juju.worker.peergrouper")

type peerGroupInfo struct {
	machines  []*machine
	statuses  []replicaset.Status
	members   []replicaset.Member
	mongoPort int
}

type machine struct {
	id        string
	candidate bool
	host      string

	// Set by desiredPeerGroup
	voting bool
}

//// getPeerGroupInfo collates current session information about the
//// mongo peer group with information from state machines.
//func getPeerGroupInfo(st *state.State, ms []*state.Machine) (*peerGroupInfo, error) {
//	session := st.MongoSession()
//	info := &peerGroupInfo{}
//	var err error
//	info.statuses, err = replicaset.CurrentStatus(session)
//	if err != nil {
//		return nil, fmt.Errorf("cannot get replica set status: %v", err)
//	}
//	info.members, err = replicaset.CurrentMembers(session)
//	if err != nil {
//		return nil, fmt.Errorf("cannot get replica set members: %v", err)
//	}
//	for _, m := range ms {
//		info.machines = append(info.machines, &machine{
//			id:        m.Id(),
//			candidate: m.IsCandidate(),
//			host: instance.SelectInternalAddress(m.Addresses(), false),
//		})
//	}
//	return info, nil
//}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}

// desiredPeerGroup returns the mongo peer group according to the given
// servers and a map with an element for each machine in info.machines
// specifying whether that machine has been configured as voting. It may
// return (nil, nil, nil) if the current group is already correct.
func desiredPeerGroup(info *peerGroupInfo) ([]replicaset.Member, map[*machine]bool, error) {
	changed := false
	members, extra, maxId := info.membersMap()
	logger.Infof("got members: %#v", members)
	logger.Infof("extra: %#v", extra)
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
	// 4) bomb out "run in circles, scream and shout"
	// 5) do nothing "nothing to see here"
	for _, member := range extra {
		if member.Votes == nil || *member.Votes > 0 {
			return nil, nil, fmt.Errorf("voting non-machine member found in peer group")
		}
		changed = true
	}
	statuses := info.statusesMap(members)

	machineVoting := make(map[*machine]bool)
	var toRemoveVote, toAddVote, toKeep []*machine
	for _, m := range info.machines {
		member := members[m]
		isVoting := member != nil && (member.Votes == nil || *member.Votes > 0)
		machineVoting[m] = isVoting
		switch {
		case m.candidate && isVoting:
			toKeep = append(toKeep, m)
		case m.candidate && !isVoting:
			if status, ok := statuses[m]; ok && isReady(status) {
				toAddVote = append(toAddVote, m)
			} else {
				toKeep = append(toKeep, m)
			}
		case !m.candidate && isVoting:
			toRemoveVote = append(toRemoveVote, m)
		case !m.candidate && !isVoting:
			toKeep = append(toKeep, m)
		}
	}
	// sort machines to be added and removed so that we
	// get deterministic behaviour when testing. Earlier
	// entries will be dealt with preferentially, so we could
	// potentially sort by some other metric in each case.
	sort.Sort(byId(toRemoveVote))
	sort.Sort(byId(toAddVote))
	sort.Sort(byId(toKeep))

	setVoting := func(m *machine, voting bool) {
		setMemberVoting(members[m], voting)
		machineVoting[m] = voting
		changed = true
	}

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
	for _, m := range toKeep {
		if members[m] == nil && m.host != "" {
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
		}
	}
	// Make sure all members' machine addresses are up to date.
	for _, m := range info.machines {
		if m.host == "" {
			continue
		}
		// TODO ensure that replicset works correctly with IPv6 [host]:port addresses.
		hostPort := net.JoinHostPort(m.host, fmt.Sprint(info.mongoPort))
		if hostPort != members[m].Address {
			members[m].Address = hostPort
			changed = true
		}
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

func isReady(status replicaset.Status) bool {
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

func (info *peerGroupInfo) membersMap() (members map[*machine]*replicaset.Member, extra []replicaset.Member, maxId int) {
	maxId = -1
	members = make(map[*machine]*replicaset.Member)
	for _, member := range info.members {
		member := member
		var found *machine
		if mid, ok := member.Tags["juju-machine-id"]; ok {
			for _, m := range info.machines {
				if m.id == mid {
					found = m
				}
			}
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

func (info *peerGroupInfo) statusesMap(members map[*machine]*replicaset.Member) map[*machine]replicaset.Status {
	statuses := make(map[*machine]replicaset.Status)
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
