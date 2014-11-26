// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// SubnetInfo describes a single subnet.
type SubnetInfo struct {
	// ProviderId is a provider-specific network id. This may be empty.
	ProviderId string

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AllocatableIPHigh and Low describe the allocatable portion of the
	// subnet. The remainder, if any, is reserved by the provider.
	// Either both of these must be set or neither, if they're empty it
	// means that none of the subnet is allocatable.
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

	VLANTag          int    `bson:",omitempty"`
	AvailabilityZone string `bson:",omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

func (s *Subnet) EnsureDead() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy subnet %v", s)
	if s.doc.Life == Dead {
		return nil
	}

	defer func() {
		if err == nil {
			s.doc.Life = Dead
		}
	}()

	ops := []txn.Op{{
		C:  subnetsC,
		Id: s.doc.DocID,
		Update: bson.D{{"$set", bson.D{
			{"Life", Dead},
		}}},
	}}
	return s.st.runTransaction(ops)
}

func (s *Subnet) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy subnet %v", s)
	if s.doc.Life != Dead {
		return errors.New("subnet is not dead")
	}
	ops := []txn.Op{{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Remove: true,
	}}
	return s.st.runTransaction(ops)
}

// ProviderId returns the provider-specific id of the subnet.
func (s *Subnet) ProviderId() string {
	return s.doc.ProviderId
}

// CIDR returns the subnet CIDR (e.g. 192.168.50.0/24).
func (s *Subnet) CIDR() string {
	return s.doc.CIDR
}

// VLANTag returns the subnet VLAN tag. It's a number between 1 and
// 4094 for VLANs and 0 if the network is not a VLAN.
func (s *Subnet) VLANTag() int {
	return s.doc.VLANTag
}

// AllocatableIPLow returns the lowest allocatable IP address in the subnet
func (s *Subnet) AllocatableIPLow() string {
	return s.doc.AllocatableIPLow
}

// AllocatableIPHigh returns the hightest allocatable IP address in the subnet.
func (s *Subnet) AllocatableIPHigh() string {
	return s.doc.AllocatableIPHigh
}

// AvailabilityZone returns the availability zone of the subnet. If the subnet
// is not associated with an availability zone it will be the empty string.
func (s *Subnet) AvailabilityZone() string {
	return s.doc.AvailabilityZone
}
