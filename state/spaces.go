// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/network"
)

// Space represents the state of a juju network space.
type Space struct {
	st  *State
	doc spaceDoc
}

type spaceDoc struct {
	DocId      string `bson:"_id"`
	Id         string `bson:"spaceid"`
	Life       Life   `bson:"life"`
	Name       string `bson:"name"`
	IsPublic   bool   `bson:"is-public"`
	ProviderId string `bson:"providerid,omitempty"`
}

// Id returns the space ID.
func (s *Space) Id() string {
	return s.doc.Id
}

// Life returns whether the space is Alive, Dying or Dead.
func (s *Space) Life() Life {
	return s.doc.Life
}

// String implements fmt.Stringer.
func (s *Space) String() string {
	return s.doc.Name
}

// Name returns the name of the Space.
func (s *Space) Name() string {
	return s.doc.Name
}

// IsPublic returns whether the space is public or not.
func (s *Space) IsPublic() bool {
	return s.doc.IsPublic
}

// ProviderId returns the provider id of the space. This will be the empty
// string except on substrates that directly support spaces.
func (s *Space) ProviderId() network.Id {
	return network.Id(s.doc.ProviderId)
}

// Subnets returns all the subnets associated with the Space.
func (s *Space) Subnets() ([]*Subnet, error) {
	id := s.Id()

	subnetsCollection, closer := s.st.db().GetCollection(subnetsC)
	defer closer()

	var doc subnetDoc
	var results []*Subnet
	// We ignore space-name field for FAN subnets...
	iter := subnetsCollection.Find(
		bson.D{{"space-id", id}, bson.DocElem{Name: "fan-local-underlay", Value: bson.D{{"$exists", false}}}}).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		subnet := &Subnet{s.st, doc, id}
		results = append(results, subnet)
		// ...and then add them explicitly as descendants of underlay network.
		childIter := subnetsCollection.Find(bson.D{{"fan-local-underlay", doc.CIDR}}).Iter()
		for childIter.Next(&doc) {
			subnet := &Subnet{s.st, doc, id}
			results = append(results, subnet)
		}
	}
	if err := iter.Close(); err != nil {
		return nil, errors.Annotatef(err, "cannot fetch subnets")
	}
	return results, nil
}

// NetworkSpace maps the space fields into a network.SpaceInfo.
func (s *Space) NetworkSpace() (network.SpaceInfo, error) {
	subnets, err := s.Subnets()
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}

	mappedSubnets := make([]network.SubnetInfo, len(subnets))
	for i, subnet := range subnets {
		mappedSubnet := subnet.networkSubnet()
		mappedSubnet.SpaceID = s.Id()
		mappedSubnet.SpaceName = s.Name()
		mappedSubnets[i] = mappedSubnet
	}

	return network.SpaceInfo{
		ID:         s.Id(),
		Name:       network.SpaceName(s.Name()),
		ProviderId: s.ProviderId(),
		Subnets:    mappedSubnets,
	}, nil
}

// RemoveSpaceOps returns txn.Ops to remove the space
func (s *Space) RemoveSpaceOps() []txn.Op {
	return []txn.Op{
		{
			C:      spacesC,
			Id:     s.doc.DocId,
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

// RenameSpaceOps returns the database transaction operations required to
// rename the input space `fromName` to input `toName`.
func (s *Space) RenameSpaceOps(toName string) []txn.Op {
	renameSpaceOps := []txn.Op{{
		C:      spacesC,
		Id:     s.doc.DocId,
		Update: bson.D{{"$set", bson.D{{"name", toName}}}},
	}}
	return renameSpaceOps
}

// AddSpace creates and returns a new space.
func (st *State) AddSpace(
	name string, providerId network.Id, subnetIDs []string, isPublic bool) (newSpace *Space, err error,
) {
	defer errors.DeferredAnnotatef(&err, "adding space %q", name)
	if !names.IsValidSpace(name) {
		return nil, errors.NewNotValid(nil, "invalid space name")
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.SpaceByName(name); err != nil {
			if !errors.IsNotFound(err) {
				return nil, errors.Annotatef(err, "checking for existing space")
			}
		} else {
			return nil, errors.AlreadyExistsf("space %q", name)
		}

		for _, subnetId := range subnetIDs {
			subnet, err := st.Subnet(subnetId)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if subnet.FanLocalUnderlay() != "" {
				return nil, errors.Errorf(
					"cannot set space for FAN subnet %q - it is always inherited from underlay", subnet.CIDR())
			}
		}

		// The ops will assert that the ID is unique,
		// but we check explicitly in order to return an indicative error.
		if providerId != "" {
			exists, err := st.networkEntityGlobalKeyExists("space", providerId)
			if err != nil {
				return nil, errors.Trace(err)
			}
			if exists {
				return nil, errors.Errorf("provider ID %q not unique", providerId)
			}
		}

		ops, err := st.addSpaceWithSubnetsTxnOps(name, providerId, subnetIDs, isPublic)
		return ops, errors.Trace(err)
	}

	err = st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot add space to the model: %v", err)
		return nil, errors.Trace(err)
	}

	space, err := st.SpaceByName(name)
	return space, errors.Trace(err)
}

func (st *State) addSpaceWithSubnetsTxnOps(
	name string, providerId network.Id, subnetIDs []string, isPublic bool,
) ([]txn.Op, error) {
	// Space with ID zero is the default space; start at 1.
	seq, err := sequenceWithMin(st, "space", 1)
	if err != nil {
		return nil, err
	}
	id := strconv.Itoa(seq)

	ops := st.addSpaceTxnOps(id, name, providerId, isPublic)

	for _, subnetID := range subnetIDs {
		// TODO:(mfoord) once we have refcounting for subnets we should
		// also assert that the refcount is zero as moving the space of a
		// subnet in use is not permitted.
		ops = append(ops, txn.Op{
			C:      subnetsC,
			Id:     subnetID,
			Assert: bson.D{bson.DocElem{Name: "fan-local-underlay", Value: bson.D{{"$exists", false}}}},
			Update: bson.D{{"$set", bson.D{{"space-id", id}}}},
		})
	}

	return ops, nil
}

func (st *State) addSpaceTxnOps(id, name string, providerId network.Id, isPublic bool) []txn.Op {
	doc := spaceDoc{
		DocId:      st.docID(id),
		Id:         id,
		Life:       Alive,
		Name:       name,
		IsPublic:   isPublic,
		ProviderId: string(providerId),
	}

	ops := []txn.Op{{
		C:      spacesC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}

	if providerId != "" {
		ops = append(ops, st.networkEntityGlobalKeyOp("space", providerId))
	}

	return ops
}

// Space returns a space from state that matches the input ID.
// An error is returned if the space does not exist or if there was a problem
// accessing its information.
func (st *State) Space(id string) (*Space, error) {
	spaces, closer := st.db().GetCollection(spacesC)
	defer closer()

	var doc spaceDoc
	err := spaces.Find(bson.M{"spaceid": id}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("space id %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get space id %q", id)
	}
	return &Space{st, doc}, nil
}

// SpaceByName returns a space from state that matches the input name.
// An error is returned if the space does not exist or if there was a problem
// accessing its information.
func (st *State) SpaceByName(name string) (*Space, error) {
	spaces, closer := st.db().GetCollection(spacesC)
	defer closer()

	var doc spaceDoc
	err := spaces.Find(bson.M{"name": name}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("space %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get space %q", name)
	}
	return &Space{st, doc}, nil
}

// AllSpaceInfos return SpaceInfos for all spaces in the model.
func (st *State) AllSpaceInfos() (network.SpaceInfos, error) {
	spaces, err := st.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make(network.SpaceInfos, len(spaces))
	for i, space := range spaces {
		if result[i], err = space.NetworkSpace(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// AllSpaces returns all spaces for the model.
func (st *State) AllSpaces() ([]*Space, error) {
	spacesCollection, closer := st.db().GetCollection(spacesC)
	defer closer()

	var docs []spaceDoc
	err := spacesCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all spaces")
	}
	spaces := make([]*Space, len(docs))
	for i, doc := range docs {
		spaces[i] = &Space{st: st, doc: doc}
	}
	return spaces, nil
}

// EnsureDead sets the Life of the space to Dead, if it's Alive. If the space is
// already Dead, no error is returned. When the space is no longer Alive or
// already removed, errNotAlive is returned.
func (s *Space) EnsureDead() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set space %q to dead", s)

	if s.doc.Life == Dead {
		return nil
	}

	ops := []txn.Op{{
		C:      spacesC,
		Id:     s.doc.DocId,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
		Assert: isAliveDoc,
	}}

	txnErr := s.st.db().RunTransaction(ops)
	if txnErr == nil {
		s.doc.Life = Dead
		return nil
	}
	return onAbort(txnErr, spaceNotAliveErr)
}

// Remove removes a Dead space. If the space is not Dead or it is already
// removed, an error is returned.
func (s *Space) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove space %q", s)

	if s.doc.Life != Dead {
		return errors.New("space is not dead")
	}

	ops := []txn.Op{{
		C:      spacesC,
		Id:     s.doc.Id,
		Remove: true,
		Assert: isDeadDoc,
	}}
	if s.ProviderId() != "" {
		ops = append(ops, s.st.networkEntityGlobalKeyRemoveOp("space", s.ProviderId()))
	}

	txnErr := s.st.db().RunTransaction(ops)
	if txnErr == nil {
		return nil
	}
	return onAbort(txnErr, errors.New("not found or not dead"))
}

// Refresh: refreshes the contents of the Space from the underlying state. It
// returns an error that satisfies errors.IsNotFound if the Space has been
// removed.
func (s *Space) Refresh() error {
	spaces, closer := s.st.db().GetCollection(spacesC)
	defer closer()

	var doc spaceDoc
	err := spaces.FindId(s.doc.Id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("space %q", s)
	} else if err != nil {
		return errors.Errorf("cannot refresh space %q: %v", s, err)
	}
	s.doc = doc
	return nil
}

// createDefaultSpaceOp returns a transaction operation
// that creates the default space (id=0).
func (st *State) createDefaultSpaceOp() txn.Op {
	return txn.Op{
		C:  spacesC,
		Id: st.docID(network.AlphaSpaceId),
		Insert: spaceDoc{
			Id:       network.AlphaSpaceId,
			Life:     Alive,
			Name:     network.AlphaSpaceName,
			IsPublic: true,
		},
	}
}
