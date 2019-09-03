// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/mongo"
)

type Subnet struct {
	st  *State
	doc subnetDoc
	// spaceID is the space id from the subnet's FanLocalUnderlay, or this subnet's space id,
	spaceID string
}

type subnetDoc struct {
	DocID             string   `bson:"_id"`
	TxnRevno          int64    `bson:"txn-revno"`
	ID                string   `bson:"subnet-id"`
	ModelUUID         string   `bson:"model-uuid"`
	Life              Life     `bson:"life"`
	ProviderId        string   `bson:"providerid,omitempty"`
	ProviderNetworkId string   `bson:"provider-network-id,omitempty"`
	CIDR              string   `bson:"cidr"`
	VLANTag           int      `bson:"vlantag,omitempty"`
	AvailabilityZones []string `bson:"availability-zones,omitempty"`
	IsPublic          bool     `bson:"is-public,omitempty"`
	SpaceID           string   `bson:"space-id,omitempty"`
	FanLocalUnderlay  string   `bson:"fan-local-underlay,omitempty"`
	FanOverlay        string   `bson:"fan-overlay,omitempty"`
}

// Life returns whether the subnet is Alive, Dying or Dead.
func (s *Subnet) Life() Life {
	return s.doc.Life
}

// ID returns the unique id for the subnet, for other entities to reference it.
func (s *Subnet) ID() string {
	return s.doc.ID
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

// IsPublic returns true if the subnet is public.
func (s *Subnet) IsPublic() bool {
	return s.doc.IsPublic
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
	if s.spaceID == "" {
		return network.DefaultSpaceName
	}
	sp, err := s.st.SpaceByID(s.spaceID)
	if err != nil {
		logger.Errorf("error finding space %q: %s", s.spaceID, err)
		return network.DefaultSpaceName
	}
	return sp.Name()
}

// SpaceID returns the id of the space the subnet is associated with. If the subnet is
// not associated with a space it will be network.DefaultSpaceId.
func (s *Subnet) SpaceID() string {
	return s.spaceID
}

// ProviderNetworkId returns the provider id of the network containing
// this subnet.
func (s *Subnet) ProviderNetworkId() network.Id {
	return network.Id(s.doc.ProviderNetworkId)
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
	if err = s.setSpace(subnets); err != nil {
		return err
	}
	return nil
}

func (s *Subnet) setSpace(subnets mongo.Collection) error {
	s.spaceID = s.doc.SpaceID
	if s.doc.FanLocalUnderlay == "" {
		return nil
	}
	if subnets == nil {
		// Some callers have the mongo subnet collection already,
		// some do not.
		var closer SessionCloser
		subnets, closer = s.st.db().GetCollection(subnetsC)
		defer closer()
	}
	overlayDoc := &subnetDoc{}
	// TODO: (hml) 2019-08-06
	// Rethink the bson logic once multiple subnets can have the same cidr.
	err := subnets.Find(bson.M{"cidr": s.doc.FanLocalUnderlay}).One(overlayDoc)
	if err == mgo.ErrNotFound {
		logger.Errorf("unable to update spaceID for subnet %q %q: underlay network %q: %s",
			s.doc.ID, s.doc.CIDR, s.doc.FanLocalUnderlay, err.Error())
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "underlay network %v for FAN %v", s.doc.FanLocalUnderlay, s.doc.CIDR)
	}
	s.spaceID = overlayDoc.SpaceID
	return nil
}

// Update adds new info to the subnet based on provided info.  Currently no
// data is changed, unless it is the default space from MAAS.  There are
// restrictions to the additions allowed:
//   - no change to CIDR, more work to determine how to handle needs to
// be done.
//   - no change to ProviderId nor ProviderNetworkID, these are immutable.
func (s *Subnet) Update(args network.SubnetInfo) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			if err := s.Refresh(); err != nil {
				if errors.IsNotFound(err) {
					return nil, errors.Errorf("ProviderId %q not unique", args.ProviderId)
				}
				return nil, errors.Trace(err)
			}
		}
		makeSpaceNameUpdate, err := s.updateSpaceName(args.SpaceName)
		if err != nil {
			return nil, err
		}
		var bsonSet bson.D
		if makeSpaceNameUpdate {
			// TODO (hml) 2019-07-25
			// Update for SpaceID once SubnetInfo Updated
			sp, err := s.st.Space(args.SpaceName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			bsonSet = append(bsonSet, bson.DocElem{"space-id", sp.Id()})
		}
		if len(args.AvailabilityZones) > 0 {
			currentAZ := set.NewStrings(args.AvailabilityZones...)
			newAZ := currentAZ.Difference(set.NewStrings(s.doc.AvailabilityZones...))
			if !newAZ.IsEmpty() {
				bsonSet = append(bsonSet, bson.DocElem{"availability-zones", append(s.doc.AvailabilityZones, newAZ.Values()...)})
			}
		}
		if s.doc.VLANTag == 0 && args.VLANTag > 0 {
			bsonSet = append(bsonSet, bson.DocElem{"vlantag", args.VLANTag})
		}
		if len(bsonSet) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		return []txn.Op{{
			C:      subnetsC,
			Id:     s.doc.DocID,
			Assert: bson.D{{"txn-revno", s.doc.TxnRevno}},
			Update: bson.D{{"$set", bsonSet}},
		}}, nil
	}
	return errors.Trace(s.st.db().Run(buildTxn))
}

func (s *Subnet) updateSpaceName(spaceName string) (bool, error) {
	var spaceNameChange bool
	sp, err := s.st.SpaceByID(s.doc.SpaceID)
	switch {
	case err != nil && !errors.IsNotFound(err):
		return false, errors.Trace(err)
	case errors.IsNotFound(err):
		spaceNameChange = true
	case err == nil:
		// Only change space name it's a default one at this time.
		//
		// The undefined space from MAAS has a providerId of -1.
		// The juju default space will be 0.
		spaceNameChange = sp.doc.ProviderId == "-1" || sp.doc.Id == network.DefaultSpaceId
	}
	// TODO (hml) 2019-07-25
	// Update when there is a s.doc.spaceID, which has done the calculation of
	// ID from the CIDR or FAN.
	return spaceNameChange && spaceName != "" && s.doc.FanLocalUnderlay == "", nil
}

// SubnetUpdate adds new info to the subnet based on provided info.
func (st *State) SubnetUpdate(args network.SubnetInfo) error {
	s, err := st.Subnet(args.CIDR)
	if err != nil {
		return errors.Trace(err)
	}
	return s.Update(args)
}

// AddSubnet creates and returns a new subnet.
func (st *State) AddSubnet(args network.SubnetInfo) (subnet *Subnet, err error) {
	defer errors.DeferredAnnotatef(&err, "adding subnet %q", args.CIDR)

	if err := args.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var seq int
	seq, err = sequence(st, "subnet")
	if err != nil {
		return nil, errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt != 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			if _, err = st.SubnetByID(subnet.ID()); err == nil {
				return nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
			}
			if err := subnet.Refresh(); err != nil {
				if errors.IsNotFound(err) {
					return nil, errors.Errorf("provider ID %q not unique", args.ProviderId)
				}
				return nil, errors.Trace(err)
			}
		}
		var ops []txn.Op
		var subDoc subnetDoc
		subDoc, ops, err = st.addSubnetOps(strconv.Itoa(seq), args)
		if err != nil {
			return nil, errors.Trace(err)
		}
		subnet = &Subnet{st: st, doc: subDoc}
		ops = append(ops, assertModelActiveOp(st.ModelUUID()))
		return ops, nil
	}
	err = st.db().Run(buildTxn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := subnet.setSpace(nil); err != nil {
		return nil, errors.Trace(err)
	}
	return subnet, nil
}

func (st *State) addSubnetOps(id string, args network.SubnetInfo) (subnetDoc, []txn.Op, error) {
	unique, err := st.uniqueSubnet(args.CIDR, string(args.ProviderId))
	if err != nil {
		return subnetDoc{}, nil, errors.Trace(err)
	}
	if !unique {
		return subnetDoc{}, nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
	}
	if args.SpaceID == "" && args.SpaceName == network.DefaultSpaceName {
		// Ensure the subnet is added to the default space
		// if none is defined for the subnet.
		args.SpaceID = network.DefaultSpaceId
	}
	subDoc := subnetDoc{
		DocID:             st.docID(id),
		ID:                id,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        string(args.ProviderId),
		ProviderNetworkId: string(args.ProviderNetworkId),
		AvailabilityZones: args.AvailabilityZones,
		SpaceID:           args.SpaceID,
		FanLocalUnderlay:  args.FanLocalUnderlay(),
		FanOverlay:        args.FanOverlay(),
		IsPublic:          args.IsPublic,
	}
	ops := []txn.Op{
		{
			C:      subnetsC,
			Id:     subDoc.DocID,
			Assert: txn.DocMissing,
			Insert: subDoc,
		},
	}
	if args.ProviderId != "" {
		ops = append(ops, st.networkEntityGlobalKeyOp("subnet", args.ProviderId))
	}
	return subDoc, ops, nil
}

func (st *State) uniqueSubnet(cidr, providerID string) (bool, error) {
	subnets, closer := st.db().GetCollection(subnetsC)
	defer closer()

	pID := bson.D{{"providerid", providerID}}
	if providerID == "" {
		pID = bson.D{{"providerid", bson.D{{"$exists", false}}}}
	}

	count, err := subnets.Find(
		bson.D{{"$and",
			[]bson.D{
				{{"cidr", cidr}},
				pID,
			},
		}}).Count()

	if err == mgo.ErrNotFound {
		return false, errors.NotFoundf("subnet cidr %q", cidr)
	}
	if err != nil {
		return false, errors.Annotatef(err, "cannot get subnet cidr %q", cidr)
	}
	return count == 0, nil
}

// TODO (hml) 2019-08-06
// This will need to be updated or removed once cidrs
// are no longer unique identifiers of juju subnets.
//
// Subnet returns the subnet specified by the cidr.
func (st *State) Subnet(cidr string) (*Subnet, error) {
	return st.subnet(bson.M{"cidr": cidr}, cidr)
}

// SubnetByID returns the subnet specified by the id.
func (st *State) SubnetByID(id string) (*Subnet, error) {
	return st.subnet(bson.M{"subnet-id": id}, id)
}

func (st *State) subnet(exp bson.M, thing string) (*Subnet, error) {
	subnets, closer := st.db().GetCollection(subnetsC)
	defer closer()

	var doc subnetDoc
	err := subnets.Find(exp).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("subnet %q", thing)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get subnet %q", thing)
	}
	subnet := &Subnet{st: st, doc: doc}
	if err := subnet.setSpace(subnets); err != nil {
		return nil, err
	}
	return subnet, nil
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
		cidrToSpace[doc.CIDR] = doc.SpaceID
	}
	for _, doc := range docs {
		spaceID := doc.SpaceID
		if doc.FanLocalUnderlay != "" {
			if space, ok := cidrToSpace[doc.FanLocalUnderlay]; ok {
				spaceID = space
			}
		}
		subnets = append(subnets, &Subnet{st: st, doc: doc, spaceID: spaceID})
	}
	return subnets, nil
}
