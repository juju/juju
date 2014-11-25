// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/network"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

// SubnetInfo describes a single network.
type SubnetInfo struct {
	// ProviderId is a provider-specific network id.
	ProviderId network.Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AllocatableIPHigh and Low describe the allocatable portion of the
	// subnet. The remainder, if any, is reserved by the provider.
	AllocatableIPHigh string
	AllocatableIPLow  string

	AvailabilityZone string
}

type Subnet struct {
	st  *State
	doc subnetDoc
}

type subnetDoc struct {
	DocID             string `bson:"_id"`
	EnvUUID           string `bson:"env-uuid"`
	Life              Life
	ProviderId        string
	CIDR              string
	AllocatableIPHigh string
	AllocatableIPLow  string
	VLANTag           int    `bson:",omitempty"`
	AvailabilityZone  string `bson:",omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

func (s *Subnet) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy subnet %v", s)
	if s.doc.Life == Dying {
		return jujutxn.ErrNoOperations
	}

	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			s.doc.Life = Dying
		}
	}()

	ops := []txn.Op{{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Remove: true,
	}}
	return s.st.runTransaction(ops)
}
