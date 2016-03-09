// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/replicaset"
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
)

// environMongo implements state/lease.Mongo to expose environ-filtered mongo
// capabilities to the lease package.
type environMongo struct {
	state *State
}

// GetCollection is part of the lease.Mongo interface.
func (m *environMongo) GetCollection(name string) (mongo.Collection, func()) {
	return m.state.getCollection(name)
}

// RunTransaction is part of the lease.Mongo interface.
func (m *environMongo) RunTransaction(buildTxn jujutxn.TransactionSource) error {
	return m.state.run(buildTxn)
}

// Mongo Upgrade

// HAMember holds information that identifies one member
// of HA.
type HAMember struct {
	Tag           string
	PublicAddress network.Address
	Series        string
}

// UpgradeMongoParams holds information that identifies
// the machines part of HA.
type UpgradeMongoParams struct {
	RsMembers []replicaset.Member

	Master  HAMember
	Members []HAMember
}

// SetUpgradeMongoMode writes a value in the state server to be picked up
// by api servers to know that there is an upgrade ready to happen.
func (st *State) SetUpgradeMongoMode(v mongo.Version) (UpgradeMongoParams, error) {
	currentInfo, err := st.ControllerInfo()
	if err != nil {
		return UpgradeMongoParams{}, errors.Annotate(err, "could not obtain current controller information")
	}
	result := UpgradeMongoParams{}
	machines := []*Machine{}
	for _, mID := range currentInfo.VotingMachineIds {
		m, err := st.Machine(mID)
		if err != nil {
			return UpgradeMongoParams{}, errors.Annotate(err, "cannot change all the replicas")
		}
		isMaster, err := mongo.IsMaster(st.session, m)
		if err != nil {
			return UpgradeMongoParams{}, errors.Annotatef(err, "cannot determine if machine %q is master", mID)
		}
		paddr, err := m.PublicAddress()
		if err != nil {
			return UpgradeMongoParams{}, errors.Annotatef(err, "cannot obtain public address for machine: %v", m)
		}
		tag := m.Tag()
		mtag := tag.(names.MachineTag)
		member := HAMember{
			Tag:           mtag.Id(),
			PublicAddress: paddr,
			Series:        m.Series(),
		}
		if isMaster {
			result.Master = member
		} else {
			result.Members = append(result.Members, member)
		}
		machines = append(machines, m)
	}
	rsMembers, err := replicaset.CurrentMembers(st.session)
	if err != nil {
		return UpgradeMongoParams{}, errors.Annotate(err, "cannot obtain current replicaset members")
	}
	masterRs, err := replicaset.MasterHostPort(st.session)
	if err != nil {
		return UpgradeMongoParams{}, errors.Annotate(err, "cannot determine master on replicaset members")
	}
	for _, m := range rsMembers {
		if m.Address != masterRs {
			result.RsMembers = append(result.RsMembers, m)
		}
	}
	for _, m := range machines {
		if err := m.SetStopMongoUntilVersion(v); err != nil {
			return UpgradeMongoParams{}, errors.Annotate(err, "cannot trigger replica shutdown")
		}
	}
	return result, nil
}

// ResumeReplication will add all passed members to replicaset.
func (st *State) ResumeReplication(members []replicaset.Member) error {
	return replicaset.Add(st.session, members...)
}
