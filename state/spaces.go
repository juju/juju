// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"net"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// Space represents the state of a space.
// A space is a security subdivision of a network. In practice, a space
// is a collection of related subnets that have no firewalls between
// each other, and that have the same ingress and egress policies.
type Space struct {
	st  *State
	doc spaceDoc
}

type spaceDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`
	Life    Life   `bson:"life"`

	Subnets  []string `bson:"subnets"`
	Name     string   `bson:"name"`
	IsPublic bool     `bson:"is-public"`
}

// Life returns whether the space is Alive, Dying or Dead.
func (s *Space) Life() Life {
	return s.doc.Life
}

// ID returns the unique id for the space, for other entities to reference it
func (s *Space) ID() string {
	return s.doc.DocID
}

// String implements fmt.Stringer.
func (s *Space) String() string {
	return s.doc.Name
}

// AddSpace creates and returns a new space
func (st *State) AddSpace(name string, subnets []string, isPublic bool) (newSpace *Space, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add space %q", name)

	spaceID := st.docID(name)
	spaceDoc := spaceDoc{
		DocID:    spaceID,
		EnvUUID:  st.EnvironUUID(),
		Life:     Alive,
		Name:     name,
		Subnets:  subnets,
		IsPublic: isPublic,
	}
	newSpace = &Space{doc: spaceDoc, st: st}

	if err = newSpace.validate(); err != nil {
		return nil, err
	}

	ops := []txn.Op{{
		C:      spacesC,
		Id:     spaceID,
		Assert: txn.DocMissing,
		Insert: spaceDoc,
	}}

	if err = st.runTransaction(ops); err == txn.ErrAborted {
		if _, err = st.Space(name); err == nil {
			return nil, errors.AlreadyExistsf("space %q", name)
		}
	}
	return newSpace, errors.Trace(err)
}

// Space returns a space from state that matches the provided name. An error
// is returned if the space doesn't exist or if there was a problem accessing
// its information.
func (st *State) Space(name string) (*Space, error) {
	spaces, closer := st.getCollection(spacesC)
	defer closer()

	var doc spaceDoc
	err := spaces.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("space %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get space %q", name)
	}
	return &Space{st, doc}, nil
}

// AllSpaces returns all spaces from state.
func (st *State) AllSpaces() ([]*Space, error) {
	spacesCollection, closer := st.getCollection(spacesC)
	defer closer()

	docs := []spaceDoc{}
	var spaces []*Space
	err := spacesCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all spaces")
	}
	for _, doc := range docs {
		spaces = append(spaces, &Space{st: st, doc: doc})
	}
	return spaces, nil
}

// EnsureDead sets the Life of the space to Dead, if it's Alive. It
// does nothing otherwise.
func (s *Space) EnsureDead() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set space %q to dead", s)

	if s.doc.Life == Dead {
		return nil
	}

	ops := []txn.Op{{
		C:      spacesC,
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

// Remove removes a dead space. If the space is not dead it returns an error.
func (s *Space) Remove() (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot remove space %q", s)

	if s.doc.Life != Dead {
		return errors.New("space is not dead")
	}

	ops := []txn.Op{{
		C:      spacesC,
		Id:     s.doc.DocID,
		Remove: true,
		Assert: notDeadDoc,
	}}

	err = s.st.runTransaction(ops)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

// Refresh: refreshes the contents of the Space from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the Space has
// been removed.
func (s *Space) Refresh() error {
	spaces, closer := s.st.getCollection(spacesC)
	defer closer()

	err := spaces.FindId(s.doc.DocID).One(&s.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("space %q", s)
	}
	if err != nil {
		return errors.Errorf("cannot refresh space %q: %v", s, err)
	}
	return nil
}

// validate validates the space, checking the validity of its subnets.
func (s *Space) validate() error {
	if !names.IsValidSpace(s.doc.Name) {
		return errors.NewNotValid(nil, "invalid space name")
	}

	// We need at least one subnet
	if len(s.doc.Subnets) == 0 {
		return errors.NewNotValid(nil, "at least one subnet required")
	}

	for _, cidr := range s.doc.Subnets {
		// Check that CIDRs are valid
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return errors.Trace(err)
		}

		// Check that CIDRs match a subnet entry in state
		if _, err := s.st.Subnet(cidr); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}
