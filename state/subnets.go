// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
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

	// SpaceName is the name of the space the subnet is associated with. It
	// can be empty if the subnet is in the default space.
	SpaceName string
}

type Subnet struct {
	st  *State
	doc subnetDoc
}

type subnetDoc struct {
	DocID             string `bson:"_id"`
	EnvUUID           string `bson:"env-uuid"`
	Life              Life   `bson:"life"`
	ProviderId        string `bson:"providerid,omitempty"`
	CIDR              string `bson:"cidr"`
	AllocatableIPHigh string `bson:"allocatableiphigh,omitempty"`
	AllocatableIPLow  string `bson:"allocatableiplow,omitempty"`
	VLANTag           int    `bson:"vlantag,omitempty"`
	AvailabilityZone  string `bson:"availabilityzone,omitempty"`
	IsPublic          bool   `bson:"is-public,omitempty"`
	// TODO(dooferlad 2015-08-03): add an upgrade step to insert IsPublic=false
	SpaceName string `bson:"space-name,omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

// ID returns the unique id for the subnet, for other entities to reference it
func (s *Subnet) ID() string {
	return s.doc.DocID
}

// String implements fmt.Stringer.
func (s *Subnet) String() string {
	return s.CIDR()
}

// GoString implements fmt.GoStringer.
func (s *Subnet) GoString() string {
	return s.String()
}

// EnsureDead sets the Life of the subnet to Dead, if it's Alive. It
// does nothing otherwise.
func (s *Subnet) EnsureDead() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set subnet %q to dead", s)

	if s.doc.Life == Dead {
		return nil
	}

	ops := []txn.Op{{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		Assert: isAliveDoc,
	}}
	if err = s.st.runTransaction(ops); err != nil {
		// Ignore ErrAborted if it happens, otherwise return err.
		return onAbort(err, nil)
	}
	s.doc.Life = Dead
	return nil
}

// Remove removes a dead subnet. If the subnet is not dead it returns an error.
// It also removes any IP addresses associated with the subnet.
func (s *Subnet) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove subnet %q", s)

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
	if err = iter.Close(); err != nil {
		return errors.Annotate(err, "cannot read addresses")
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

// SpaceName returns the space the subnet is associated with. If the subnet is
// in the default space it will be the empty string.
func (s *Subnet) SpaceName() string {
	return s.doc.SpaceName
}

// Validate validates the subnet, checking the CIDR, VLANTag and
// AllocatableIPHigh and Low, if present.
func (s *Subnet) Validate() error {
	var mask *net.IPNet
	var err error
	if s.doc.CIDR != "" {
		_, mask, err = net.ParseCIDR(s.doc.CIDR)
		if err != nil {
			return errors.Trace(err)
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
		return errors.NotFoundf("subnet %q", s)
	}
	if err != nil {
		return errors.Errorf("cannot refresh subnet %q: %v", s, err)
	}
	return nil
}

// PickNewAddress returns a new IPAddress that isn't in use for the subnet.
// The address starts with AddressStateUnknown, for later allocation.
// This will fail if the subnet is not alive.
func (s *Subnet) PickNewAddress() (*IPAddress, error) {
	for {
		addr, err := s.attemptToPickNewAddress()
		if err == nil {
			return addr, err
		}
		if !errors.IsAlreadyExists(err) {
			return addr, err
		}
	}
}

// attemptToPickNewAddress will try to pick a new address. It can fail
// with AlreadyExists due to a race condition between fetching the
// list of addresses already in use and allocating a new one. If the
// subnet is not alive, it will also fail. It is called in a loop by
// PickNewAddress until it gets one or there are no more available!
func (s *Subnet) attemptToPickNewAddress() (*IPAddress, error) {
	if s.doc.Life != Alive {
		return nil, errors.Errorf("cannot pick address: subnet %q is not alive", s)
	}
	high := s.doc.AllocatableIPHigh
	low := s.doc.AllocatableIPLow
	if low == "" || high == "" {
		return nil, errors.Errorf("no allocatable IP addresses for subnet %q", s)
	}

	// convert low and high to decimals as the bounds
	lowDecimal, err := network.IPv4ToDecimal(net.ParseIP(low))
	if err != nil {
		// these addresses are validated so should never happen
		return nil, errors.Annotatef(err, "invalid AllocatableIPLow %q for subnet %q", low, s)
	}
	highDecimal, err := network.IPv4ToDecimal(net.ParseIP(high))
	if err != nil {
		// these addresses are validated so should never happen
		return nil, errors.Annotatef(err, "invalid AllocatableIPHigh %q for subnet %q", high, s)
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
		value, err := network.IPv4ToDecimal(net.ParseIP(doc.Value))
		if err != nil {
			continue
		}
		allocated[value] = true
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotatef(err, "cannot read addresses of subnet %q", s)
	}

	// Check that the number of addresses in use is less than the
	// difference between low and high - i.e. we haven't exhausted all
	// possible addresses.
	if len(allocated) >= int(highDecimal-lowDecimal)+1 {
		return nil, errors.Errorf("allocatable IP addresses exhausted for subnet %q", s)
	}

	// pick a new random decimal between the low and high bounds that
	// doesn't match an existing one
	newDecimal := pickAddress(lowDecimal, highDecimal, allocated)

	// convert it back to a dotted-quad
	newIP := network.DecimalToIPv4(newDecimal)
	newAddr := network.NewAddress(newIP.String())

	// and create a new IPAddress from it and return it
	return s.st.AddIPAddress(newAddr, s.ID())
}

// pickAddress will pick a number, representing an IPv4 address, between low
// and high (inclusive) that isn't in the allocated map. There must be at least
// one available address between low and high and not in allocated.
// e.g. pickAddress(uint32(2700), uint32(2800), map[uint32]bool{uint32(2701): true})
// The allocated map is just being used as a set of unavailable addresses, so
// the bool value isn't significant.
var pickAddress = func(low, high uint32, allocated map[uint32]bool) uint32 {
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
