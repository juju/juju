// Copyright 2012, 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/network"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
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

func (s *Subnet) Refresh() error {
	subnets, closer := s.st.getCollection(subnetsC)
	defer closer()

	err := subnets.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("subnet %v", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh subnet %v: %v", s, err)
	}
	return nil
}

func (s *Subnet) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy subnet %v", s)
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			s.doc.Life = Dying
		}
	}()
	sub := &Subnet{st: s.st, doc: s.doc}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := sub.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		switch ops, err := sub.destroyOps(); err {
		case errRefresh:
		case errAlreadyDying:
			return nil, jujutxn.ErrNoOperations
		case nil:
			return ops, nil
		default:
			return nil, err
		}
		return nil, jujutxn.ErrTransientFailure
	}
	return s.st.run(buildTxn)
}

func (s *Subnet) destroyOps() ([]txn.Op, error) {
	// XXX we also need to destroy related addresses
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	ops := []txn.Op{{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Remove: true,
	}}
	return ops, nil
}

func (s *Subnet) globalKey() string {
	return "sn#" + s.doc.CIDR
}
