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
	// spaceID is either the space id from the subnet's FanLocalUnderlay,
	// or this subnet's space ID.
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

// ID returns the unique ID for the subnet.
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

// EnsureDead sets the Life of the subnet to Dead if it is Alive.
// If the subnet is already Dead, no error is returned.
// When the subnet is no longer Alive or already removed,
// errNotAlive is returned.
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

// ProviderId returns the provider-specific ID of the subnet.
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

// AvailabilityZones returns the availability zones of the subnet.
// If the subnet is not associated with an availability zones
// it will return an the empty slice.
func (s *Subnet) AvailabilityZones() []string {
	return s.doc.AvailabilityZones
}

// SpaceName returns the space the subnet is associated with.  If no
// space is associated, return the default space and log an error.
func (s *Subnet) SpaceName() string {
	if s.spaceID == "" {
		logger.Errorf("subnet %q has no spaceID", s.spaceID)
		return network.AlphaSpaceName
	}
	sp, err := s.st.Space(s.spaceID)
	if err != nil {
		logger.Errorf("error finding space %q: %s", s.spaceID, err)
		return network.AlphaSpaceName
	}
	return sp.Name()
}

// SpaceID returns the ID of the space the subnet is associated with.
// If the subnet is not associated with a space it will return
// network.AlphaSpaceId.
func (s *Subnet) SpaceID() string {
	return s.spaceID
}

// UpdateSpaceOps returns operations that will ensure that
// the subnet is in the input space, provided the space exists.
func (s *Subnet) UpdateSpaceOps(spaceID string) []txn.Op {
	if s.spaceID == spaceID {
		return nil
	}
	return []txn.Op{
		{
			C:      spacesC,
			Id:     s.st.docID(spaceID),
			Assert: txn.DocExists,
		},
		{
			C:      subnetsC,
			Id:     s.doc.DocID,
			Update: bson.D{{"$set", bson.D{{"space-id", spaceID}}}},
			Assert: isAliveDoc,
		},
	}
}

// ProviderNetworkId returns the provider id of the network containing
// this subnet.
func (s *Subnet) ProviderNetworkId() network.Id {
	return network.Id(s.doc.ProviderNetworkId)
}

// Refresh refreshes the contents of the Subnet from the underlying
// state. It an error that satisfies errors.IsNotFound if the SubnetByCIDR has
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
		// Some callers have the mongo subnet collection already; some do not.
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

// Update adds new info to the subnet based on input info.
// Currently no data is changed unless it is the "undefined" space from MAAS.
// There are restrictions on the additions allowed:
//   - No change to CIDR; more work is required to determine how to handle.
//   - No change to ProviderId nor ProviderNetworkID; these are immutable.
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
			sp, err := s.st.SpaceByName(args.SpaceName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			bsonSet = append(bsonSet, bson.DocElem{Name: "space-id", Value: sp.Id()})
		}
		if len(args.AvailabilityZones) > 0 {
			currentAZ := set.NewStrings(args.AvailabilityZones...)
			newAZ := currentAZ.Difference(set.NewStrings(s.doc.AvailabilityZones...))
			if !newAZ.IsEmpty() {
				bsonSet = append(bsonSet,
					bson.DocElem{Name: "availability-zones", Value: append(s.doc.AvailabilityZones, newAZ.Values()...)})
			}
		}
		if s.doc.VLANTag == 0 && args.VLANTag > 0 {
			bsonSet = append(bsonSet, bson.DocElem{Name: "vlantag", Value: args.VLANTag})
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
	sp, err := s.st.Space(s.doc.SpaceID)
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
		spaceNameChange = sp.doc.ProviderId == "-1" || sp.doc.Id == network.AlphaSpaceId
	}
	// TODO (hml) 2019-07-25
	// Update when there is a s.doc.spaceID, which has done the calculation of
	// ID from the CIDR or FAN.
	return spaceNameChange && spaceName != "" && s.doc.FanLocalUnderlay == "", nil
}

// NetworkSubnet maps the subnet fields into a network.SubnetInfo.
func (s *Subnet) NetworkSubnet() network.SubnetInfo {
	var fanInfo *network.FanCIDRs
	if s.doc.FanLocalUnderlay != "" || s.doc.FanOverlay != "" {
		fanInfo = &network.FanCIDRs{
			FanLocalUnderlay: s.doc.FanLocalUnderlay,
			FanOverlay:       s.doc.FanOverlay,
		}
	}

	sInfo := network.SubnetInfo{
		ID:                network.Id(s.doc.ID),
		CIDR:              s.doc.CIDR,
		ProviderId:        network.Id(s.doc.ProviderId),
		ProviderNetworkId: network.Id(s.doc.ProviderNetworkId),
		VLANTag:           s.doc.VLANTag,
		AvailabilityZones: s.doc.AvailabilityZones,
		FanInfo:           fanInfo,
		IsPublic:          s.doc.IsPublic,
		SpaceID:           s.doc.SpaceID,
		// SpaceName and ProviderSpaceID are populated by Space.NetworkSpace().
		// For now, we do not look them up here.
	}

	// If this is a fan overlay, it will have a (numeric) space ID of 0.
	// This is because it inherits the space of its underlay.
	// In this case we replace it with an empty string so as not to cause
	// confusion.
	// Space.NetworkSpace() sets this correctly as expected.
	// TODO (manadart 2020-04-09): We will probably need to populate this at
	// some point.
	if sInfo.FanLocalUnderlay() != "" {
		sInfo.SpaceID = ""
	}

	return sInfo
}

// AllSubnetInfos returns SubnetInfos for all subnets in the model.
func (st *State) AllSubnetInfos() (network.SubnetInfos, error) {
	subs, err := st.AllSubnets()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make(network.SubnetInfos, len(subs))
	for i, sub := range subs {
		result[i] = sub.NetworkSubnet()
	}
	return result, nil
}

// SubnetUpdate adds new info to the subnet based on provided info.
func (st *State) SubnetUpdate(args network.SubnetInfo) error {
	s, err := st.SubnetByCIDR(args.CIDR)
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
			if _, err = st.Subnet(subnet.ID()); err == nil {
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
	if args.SpaceID == "" {
		// Ensure the subnet is added to the default space
		// if none is defined for the subnet.
		args.SpaceID = network.AlphaSpaceId
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

// Subnet returns the subnet identified by the input ID,
// or an error if it is not found.
func (st *State) Subnet(id string) (*Subnet, error) {
	subnets, err := st.subnets(bson.M{"subnet-id": id})
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving subnet with ID %q", id)
	}
	if len(subnets) == 0 {
		return nil, errors.NotFoundf("subnet %q", id)
	}
	return subnets[0], nil
}

// SubnetByCIDR returns a unique subnet matching the input CIDR.
// If no unique match is achieved, an error is returned.
// TODO (manadart 2020-03-11): As of this date, CIDR remains a unique
// identifier for a subnet due to how we constrain provider networking
// implementations. When this changes, callers relying on this method to return
// a unique match will need attention.
// Usage of this method should probably be phased out.
func (st *State) SubnetByCIDR(cidr string) (*Subnet, error) {
	subnets, err := st.subnets(bson.M{"cidr": cidr})
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving subnet with CIDR %q", cidr)
	}
	if len(subnets) == 0 {
		return nil, errors.NotFoundf("subnet %q", cidr)
	}
	if len(subnets) > 1 {
		return nil, errors.Errorf("multiple subnets matching %q", cidr)
	}
	return subnets[0], nil
}

// SubnetsByCIDR returns the subnets matching the input CIDR.
func (st *State) SubnetsByCIDR(cidr string) ([]*Subnet, error) {
	subnets, err := st.subnets(bson.M{"cidr": cidr})
	return subnets, errors.Annotatef(err, "retrieving subnets with CIDR %q", cidr)
}

func (st *State) subnets(exp bson.M) ([]*Subnet, error) {
	col, closer := st.db().GetCollection(subnetsC)
	defer closer()

	var docs []subnetDoc
	err := col.Find(exp).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, nil
	}

	subnets := make([]*Subnet, len(docs))
	for i, doc := range docs {
		subnets[i] = &Subnet{st: st, doc: doc}
		if err := subnets[i].setSpace(col); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return subnets, nil
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
