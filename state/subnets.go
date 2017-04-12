// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/network"
)

// SubnetInfo describes a single subnet.
type SubnetInfo struct {
	// ProviderId is a provider-specific id for the subnet. This may be empty.
	ProviderId network.Id

	// ProviderNetworkId is the id of the network containing this
	// subnet from the provider's perspective. It can be empty if the
	// provider doesn't support distinct networks.
	ProviderNetworkId network.Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for normal
	// networks. It's defined by IEEE 802.1Q standard.
	VLANTag int

	// AvailabilityZone describes which availability zone this subnet is in. It can
	// be empty if the provider does not support availability zones.
	AvailabilityZone string

	// SpaceName is the name of the space the subnet is associated with. It
	// can be empty if the subnet is not associated with a space yet.
	SpaceName string
}

type Subnet struct {
	st  *State
	doc subnetDoc
}

type subnetDoc struct {
	DocID             string `bson:"_id"`
	ModelUUID         string `bson:"model-uuid"`
	Life              Life   `bson:"life"`
	ProviderId        string `bson:"providerid,omitempty"`
	ProviderNetworkId string `bson:"provider-network-id,omitempty"`
	CIDR              string `bson:"cidr"`
	VLANTag           int    `bson:"vlantag,omitempty"`
	AvailabilityZone  string `bson:"availabilityzone,omitempty"`
	// TODO: add IsPublic to SubnetArgs, add an IsPublic method and add
	// IsPublic to migration import/export.
	IsPublic  bool   `bson:"is-public,omitempty"`
	SpaceName string `bson:"space-name,omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

// ID returns the unique id for the subnet, for other entities to reference it.
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

// EnsureDead sets the Life of the subnet to Dead, if it's Alive. If the subnet
// is already Dead, no error is returned. When the subnet is no longer Alive or
// already removed, errNotAlive is returned.
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

	txnErr := s.st.runTransaction(ops)
	if txnErr == nil {
		s.doc.Life = Dead
		return nil
	}
	return onAbort(txnErr, errNotAlive)
}

// Remove removes a Dead subnet. If the subnet is not Dead or it is already
// removed, an error is returned. On success, all IP addresses added to the
// subnet are also removed.
func (s *Subnet) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove subnet %q", s)

	if s.doc.Life != Dead {
		return errors.New("subnet is not dead")
	}

	ops := []txn.Op{{
		C:      subnetsC,
		Id:     s.doc.DocID,
		Remove: true,
		Assert: isDeadDoc,
	}}
	if s.doc.ProviderId != "" {
		op := s.st.networkEntityGlobalKeyRemoveOp("subnet", s.ProviderId())
		ops = append(ops, op)
	}

	txnErr := s.st.runTransaction(ops)
	if txnErr == nil {
		return nil
	}
	return onAbort(txnErr, errors.New("not found or not dead"))
}

// ProviderId returns the provider-specific id of the subnet.
func (s *Subnet) ProviderId() network.Id {
	return network.Id(s.doc.ProviderId)
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

// AvailabilityZone returns the availability zone of the subnet. If the subnet
// is not associated with an availability zone it will be the empty string.
func (s *Subnet) AvailabilityZone() string {
	return s.doc.AvailabilityZone
}

// SpaceName returns the space the subnet is associated with. If the subnet is
// not associated with a space it will be the empty string.
func (s *Subnet) SpaceName() string {
	return s.doc.SpaceName
}

// ProviderNetworkId returns the provider id of the network containing
// this subnet.
func (s *Subnet) ProviderNetworkId() network.Id {
	return network.Id(s.doc.ProviderNetworkId)
}

// Validate validates the subnet, checking the CIDR, and VLANTag, if present.
func (s *Subnet) Validate() error {
	if s.doc.CIDR != "" {
		_, _, err := net.ParseCIDR(s.doc.CIDR)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		return errors.Errorf("missing CIDR")
	}

	if s.doc.VLANTag < 0 || s.doc.VLANTag > 4094 {
		return errors.Errorf("invalid VLAN tag %d: must be between 0 and 4094", s.doc.VLANTag)
	}

	return nil
}

// Refresh refreshes the contents of the Subnet from the underlying
// state. It an error that satisfies errors.IsNotFound if the Subnet has
// been removed.
func (s *Subnet) Refresh() error {
	subnets, closer := s.st.db().GetCollection(subnetsC)
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

// AddSubnet creates and returns a new subnet
func (st *State) AddSubnet(args SubnetInfo) (subnet *Subnet, err error) {
	defer errors.DeferredAnnotatef(&err, "adding subnet %q", args.CIDR)

	subnet, err = st.newSubnetFromArgs(args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops := st.addSubnetOps(args)
	ops = append(ops, assertModelActiveOp(st.ModelUUID()))
	buildTxn := func(attempt int) ([]txn.Op, error) {

		if attempt != 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			if _, err = st.Subnet(args.CIDR); err == nil {
				return nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
			}
			if err := subnet.Refresh(); err != nil {
				if errors.IsNotFound(err) {
					return nil, errors.Errorf("ProviderId %q not unique", args.ProviderId)
				}
				return nil, errors.Trace(err)
			}
		}
		return ops, nil
	}
	err = st.run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return subnet, nil
}

func (st *State) newSubnetFromArgs(args SubnetInfo) (*Subnet, error) {
	subnetID := st.docID(args.CIDR)
	subDoc := subnetDoc{
		DocID:             subnetID,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        string(args.ProviderId),
		ProviderNetworkId: string(args.ProviderNetworkId),
		AvailabilityZone:  args.AvailabilityZone,
		SpaceName:         args.SpaceName,
	}
	subnet := &Subnet{doc: subDoc, st: st}
	err := subnet.Validate()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return subnet, nil
}

func (st *State) addSubnetOps(args SubnetInfo) []txn.Op {
	subnetID := st.docID(args.CIDR)
	subDoc := subnetDoc{
		DocID:             subnetID,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        string(args.ProviderId),
		ProviderNetworkId: string(args.ProviderNetworkId),
		AvailabilityZone:  args.AvailabilityZone,
		SpaceName:         args.SpaceName,
	}
	ops := []txn.Op{
		{
			C:      subnetsC,
			Id:     subnetID,
			Assert: txn.DocMissing,
			Insert: subDoc,
		},
	}
	if args.ProviderId != "" {
		ops = append(ops, st.networkEntityGlobalKeyOp("subnet", args.ProviderId))
	}
	return ops
}

// Subnet returns the subnet specified by the cidr.
func (st *State) Subnet(cidr string) (*Subnet, error) {
	subnets, closer := st.db().GetCollection(subnetsC)
	defer closer()

	doc := &subnetDoc{}
	err := subnets.FindId(cidr).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("subnet %q", cidr)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get subnet %q", cidr)
	}
	return &Subnet{st, *doc}, nil
}

// AllSubnets returns all known subnets in the model.
func (st *State) AllSubnets() (subnets []*Subnet, err error) {
	subnetsCollection, closer := st.db().GetCollection(subnetsC)
	defer closer()

	docs := []subnetDoc{}
	err = subnetsCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all subnets")
	}
	for _, doc := range docs {
		subnets = append(subnets, &Subnet{st, doc})
	}
	return subnets, nil
}
