// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

type Subnet struct {
	st        *State
	doc       subnetDoc
	spaceName string
}

type subnetDoc struct {
	DocID             string   `bson:"_id"`
	ModelUUID         string   `bson:"model-uuid"`
	Life              Life     `bson:"life"`
	ProviderId        string   `bson:"providerid,omitempty"`
	ProviderNetworkId string   `bson:"provider-network-id,omitempty"`
	CIDR              string   `bson:"cidr"`
	VLANTag           int      `bson:"vlantag,omitempty"`
	AvailabilityZones []string `bson:"availability-zones,omitempty"`
	// TODO: add IsPublic to SubnetArgs, add an IsPublic method and add
	// IsPublic to migration import/export.
	IsPublic         bool   `bson:"is-public,omitempty"`
	SpaceName        string `bson:"space-name,omitempty"`
	FanLocalUnderlay string `bson:"fan-local-underlay,omitempty"`
	FanOverlay       string `bson:"fan-overlay,omitempty"`
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

func (s *Subnet) FanOverlay() string {
	return s.doc.FanOverlay
}

func (s *Subnet) FanLocalUnderlay() string {
	return s.doc.FanLocalUnderlay
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

	txnErr := s.st.db().RunTransaction(ops)
	if txnErr == nil {
		s.doc.Life = Dead
		return nil
	}
	return onAbort(txnErr, subnetNotAliveErr)
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

	txnErr := s.st.db().RunTransaction(ops)
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

// AvailabilityZones returns the availability zones of the subnet. If the subnet
// is not associated with an availability zones it will be the empty slice.
func (s *Subnet) AvailabilityZones() []string {
	return s.doc.AvailabilityZones
}

// SpaceName returns the space the subnet is associated with. If the subnet is
// not associated with a space it will be the empty string.
func (s *Subnet) SpaceName() string {
	return s.spaceName
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
	s.spaceName = s.doc.SpaceName
	if s.doc.FanLocalUnderlay != "" {
		overlayDoc := &subnetDoc{}
		err = subnets.FindId(s.doc.FanLocalUnderlay).One(overlayDoc)
		if err != nil {
			return errors.Annotatef(err, "finding underlay network %v for FAN %v", s.doc.FanLocalUnderlay, s.doc.CIDR)
		} else {
			s.spaceName = overlayDoc.SpaceName
		}
	}
	return nil
}

// AddSubnet creates and returns a new subnet
func (st *State) AddSubnet(args network.SubnetInfo) (subnet *Subnet, err error) {
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
					return nil, errors.Errorf("provider ID %q not unique", args.ProviderId)
				}
				return nil, errors.Trace(err)
			}
		}
		return ops, nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return subnet, nil
}

func (st *State) newSubnetFromArgs(args network.SubnetInfo) (*Subnet, error) {
	subnetID := st.docID(args.CIDR)
	subDoc := subnetDoc{
		DocID:             subnetID,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        string(args.ProviderId),
		ProviderNetworkId: string(args.ProviderNetworkId),
		AvailabilityZones: args.AvailabilityZones,
		SpaceName:         args.SpaceName,
		FanLocalUnderlay:  args.FanLocalUnderlay(),
		FanOverlay:        args.FanOverlay(),
	}
	subnet := &Subnet{doc: subDoc, st: st, spaceName: args.SpaceName}
	err := subnet.Validate()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return subnet, nil
}

func (st *State) addSubnetOps(args network.SubnetInfo) []txn.Op {
	subnetID := st.docID(args.CIDR)
	subDoc := subnetDoc{
		DocID:             subnetID,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        string(args.ProviderId),
		ProviderNetworkId: string(args.ProviderNetworkId),
		AvailabilityZones: args.AvailabilityZones,
		SpaceName:         args.SpaceName,
		FanLocalUnderlay:  args.FanLocalUnderlay(),
		FanOverlay:        args.FanOverlay(),
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

	var doc subnetDoc
	err := subnets.FindId(cidr).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("subnet %q", cidr)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get subnet %q", cidr)
	}
	spaceName := doc.SpaceName
	if doc.FanLocalUnderlay != "" {
		overlayDoc := &subnetDoc{}
		err = subnets.FindId(doc.FanLocalUnderlay).One(overlayDoc)
		if err != nil {
			return nil, errors.Annotatef(
				err, "Can't find underlay network %v for FAN %v", doc.FanLocalUnderlay, doc.CIDR)
		} else {
			spaceName = overlayDoc.SpaceName
		}
	}

	return &Subnet{st, doc, spaceName}, nil
}

// AllSubnets returns all known subnets in the model.
func (st *State) AllSubnets() (subnets []*Subnet, err error) {
	subnetsCollection, closer := st.db().GetCollection(subnetsC)
	defer closer()

	var docs []subnetDoc
	err = subnetsCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all subnets")
	}
	cidrToSpace := make(map[string]string)
	for _, doc := range docs {
		cidrToSpace[doc.CIDR] = doc.SpaceName
	}
	for _, doc := range docs {
		spaceName := doc.SpaceName
		if doc.FanLocalUnderlay != "" {
			if space, ok := cidrToSpace[doc.FanLocalUnderlay]; ok {
				spaceName = space
			}
		}
		subnets = append(subnets, &Subnet{st, doc, spaceName})
	}
	return subnets, nil
}
