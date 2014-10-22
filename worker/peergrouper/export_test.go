// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package peergrouper

import (
	"github.com/juju/juju/network"
	"github.com/juju/juju/replicaset"
)

const JujuMachineTag = jujuMachineTag

var (
	DesiredPeerGroup = desiredPeerGroup
	NewPublisher     = newPublisher
	NewWorker        = newWorker
)

func NewMachine(id string, wantsVote bool, mongoHostPorts []network.HostPort) *Machine {
	return &Machine{
		id:             id,
		wantsVote:      wantsVote,
		mongoHostPorts: mongoHostPorts,
	}
}

func (m *Machine) Id() string {
	return m.id
}

func NewPeerGroupInfo(machines map[string]*Machine, statuses []replicaset.MemberStatus, members []replicaset.Member) *PeerGroupInfo {
	return &PeerGroupInfo{
		machines: machines,
		statuses: statuses,
		members:  members,
	}
}

func (p *PeerGroupInfo) SetMembers(members []replicaset.Member) {
	p.members = members
}
