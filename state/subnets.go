// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
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
	// means that none of the subnet is allocatable. If present they must
	// be valid IP addresses within the subnet CIDR.
	AllocatableIPHigh string
	AllocatableIPLow  string

	// AvailabilityZone describes which availability zone this subnet is in. It can
	// be empty if the provider does not support availability zones.
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
	ProviderId        string `bson:",omitempty"`
	CIDR              string
	AllocatableIPHigh string `bson:",omitempty"`
	AllocatableIPLow  string `bson:",omitempty"`

	VLANTag          int    `bson:",omitempty"`
	AvailabilityZone string `bson:",omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

// ID returns the unique id for the subnet, for other entities to reference it
func (s *Subnet) ID() string {
	return s.doc.DocID
}

// EnsureDead sets the Life of the subnet to Dead
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
		C:      subnetsC,
		Id:     s.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		Assert: isAliveDoc,
	}}
	return s.st.runTransaction(ops)
}

// Remove removes a dead subnet. If the subnet is not dead it returns an error.
// It also removes any IP addresses associated with the subnet.
func (s *Subnet) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot destroy subnet %v", s)
	if s.doc.Life != Dead {
		return errors.New("subnet is not dead")
	}
	addresses, closer := s.st.getCollection(ipaddressesC)
	defer closer()

	ops := []txn.Op{}
	id := s.ID()
	var doc struct {
		DocID string `bson:"_id"`
	}
	iter := addresses.Find(bson.D{{"subnetid", id}}).Iter()
	for iter.Next(&doc) {
		ops = append(ops, txn.Op{
			C:      ipaddressesC,
			Id:     doc.DocID,
			Remove: true,
		})
	}

	ops = append(ops, txn.Op{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Remove: true,
	})
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

// Validate validates the subnet, checking the CIDR, VLANTag and
// AllocatableIPHigh and Low, if present.
func (s *Subnet) Validate() error {
	var mask *net.IPNet
	var err error
	if s.doc.CIDR != "" {
		_, mask, err = net.ParseCIDR(s.doc.CIDR)
		if err != nil {
			return errors.Annotatef(err, "invalid CIDR")
		}
	} else {
		return errors.Errorf("missing CIDR")
	}
	if s.doc.VLANTag < 0 || s.doc.VLANTag > 4094 {
		return errors.Errorf("invalid VLAN tag %d: must be between 0 and 4094", s.doc.VLANTag)
	}
	present := func(str string) bool {
		return str != ""
	}
	either := present(s.doc.AllocatableIPLow) || present(s.doc.AllocatableIPHigh)
	both := present(s.doc.AllocatableIPLow) && present(s.doc.AllocatableIPHigh)

	if either && !both {
		return errors.Errorf("either both AllocatableIPLow and AllocatableIPHigh must be set or neither set")
	}

	// TODO (mfoord 26-11-2014) we could also validate that the IPs are the
	// same type (IPv4 or IPv6) and that IPLow is lower than or equal to
	// IPHigh.
	if s.doc.AllocatableIPHigh != "" {
		highIP := net.ParseIP(s.doc.AllocatableIPHigh)
		if highIP == nil || !mask.Contains(highIP) {
			return errors.Errorf("invalid AllocatableIPHigh %q", s.doc.AllocatableIPHigh)
		}
		lowIP := net.ParseIP(s.doc.AllocatableIPLow)
		if lowIP == nil || !mask.Contains(lowIP) {
			return errors.Errorf("invalid AllocatableIPLow %q", s.doc.AllocatableIPLow)
		}
	}
	return nil
}

// Refresh refreshes the contents of the Subnet from the underlying
// state. It an error that satisfies errors.IsNotFound if the Subnet has
// been removed.
func (s *Subnet) Refresh() error {
	subnets, closer := s.st.getCollection(subnetsC)
	defer closer()

	err := subnets.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("subnet %v", s)
	}
	if err != nil {
		return errors.Errorf("cannot refresh subnet %v: %v", s, err)
	}
	return nil
}

// PickNewAddress returns a new IPAddress that isn't in use for the subnet.
// The address starts with AddressStateUnknown, for later allocation.
func (s *Subnet) PickNewAddress() (*IPAddress, error) {
	high := s.doc.AllocatableIPHigh
	low := s.doc.AllocatableIPLow
	if low == "" || high == "" {
		return nil, errors.New("No available IP addresses")
	}

	// convert low and high to decimals (dottedQuadToNum) as the bounds
	lowDecimal, err := ipToDecimal(low)
	if err != nil {
		// these addresses are validated so should never happen
		return nil, errors.Errorf("invalid AllocatableIPLow %q", low)
	}
	highDecimal, err := ipToDecimal(high)
	if err != nil {
		// these addresses are validated so should never happen
		return nil, errors.Errorf("invalid AllocatableIPHigh %q", high)
	}

	// find all addresses for this subnet and convert them to decimals
	addresses, closer := s.st.getCollection(ipaddressesC)
	defer closer()

	id := s.ID()
	var doc struct {
		Value string
	}
	allocated := make(map[uint32]bool)
	iter := addresses.Find(bson.D{{"subnetid", id}}).Iter()
	for iter.Next(&doc) {
		// skip invalid values. Can't happen anyway as we validate.
		value, err := ipToDecimal(doc.Value)
		if err != nil {
			continue
		}
		allocated[value] = true
	}

	// Check that the number of addresses in use is less than the
	// difference between low and high - i.e. we haven't exhausted all
	// possible addresses.
	if len(allocated) >= int(highDecimal-lowDecimal)+1 {
		return nil, errors.New("IP addresses exhausted")
	}

	// pick a new random decimal between the low and high bounds that
	// doesn't match an existing one
	newDecimal := pickAddress(lowDecimal, highDecimal, allocated)

	// convert it back to a dotted-quad
	newIP := decimalToIP(newDecimal)
	newAddr := network.NewAddress(newIP, network.ScopeUnknown)

	// and create a new IPAddress from it and return it
	return s.st.AddIPAddress(newAddr, s.ID())
}

func pickAddress(low, high uint32, allocated map[uint32]bool) uint32 {
	// +1 because Int63n will pick a number up to, but not including, the
	// bounds we provide.
	bounds := uint32(high-low) + 1
	if bounds == 1 {
		// we've already checked that there is a free IP address, so
		// this must be it!
		return low
	}
	for {
		inBounds := rand.Int63n(int64(bounds))
		value := uint32(inBounds) + low
		if _, ok := allocated[value]; !ok {
			return value
		}
	}
}

func decimalToIP(addr uint32) string {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, addr)
	return net.IP(bytes).String()
}

func ipToDecimal(ipv4Addr string) (uint32, error) {
	ip := net.ParseIP(ipv4Addr).To4()
	if ip == nil {
		return 0, fmt.Errorf("%q is not a valid IPv4 Address.", ipv4Addr)
	}
	return binary.BigEndian.Uint32([]byte(ip)), nil
}
