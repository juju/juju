package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"strconv"
)

// Service represents the state of a service.
type Service struct {
	st  *State
	doc serviceDoc
}

// serviceDoc represents the internal state of a service in MongoDB.
type serviceDoc struct {
	Name       string `bson:"_id"`
	CharmURL   *charm.URL
	ForceCharm bool
	Life       Life
	UnitSeq    int
	Exposed    bool
}

func newService(st *State, doc *serviceDoc) *Service {
	return &Service{st: st, doc: *doc}
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.doc.Name
}

// Life returns whether the service is Alive, Dying or Dead.
func (s *Service) Life() Life {
	return s.doc.Life
}

// Kill sets the service lifecycle to Dying if it is Alive.
// It does nothing otherwise.
func (s *Service) Kill() error {
	err := ensureLife(s.st, s.st.services, s.doc.Name, Dying, "service")
	if err != nil {
		return err
	}
	s.doc.Life = Dying
	return nil
}

// Die sets the service lifecycle to Dead if it is Alive or Dying.
// It does nothing otherwise.
func (s *Service) Die() error {
	err := ensureLife(s.st, s.st.services, s.doc.Name, Dead, "service")
	if err != nil {
		return err
	}
	s.doc.Life = Dead
	return nil
}

// IsExposed returns whether this service is exposed. The explicitly open
// ports (with open-port) for exposed services may be accessed from machines
// outside of the local deployment network. See SetExposed and ClearExposed.
func (s *Service) IsExposed() (bool, error) {
	return s.doc.Exposed, nil
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	ops := []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: D{{"life", Alive}},
		Update: D{{"$set", D{{"exposed", true}}}},
	}}
	err := s.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set exposed flag for service %q: %v", s, deadOnAbort(err))
	}
	s.doc.Exposed = true
	return nil
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	ops := []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: D{{"life", Alive}},
		Update: D{{"$set", D{{"exposed", false}}}},
	}}
	err := s.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot clear exposed flag for service %q: %v", s, deadOnAbort(err))
	}
	s.doc.Exposed = false
	return nil
}

// Charm returns the service's charm and whether units should upgrade to that
// charm even if they are in an error state.
func (s *Service) Charm() (ch *Charm, force bool, err error) {
	ch, err = s.st.Charm(s.doc.CharmURL)
	if err != nil {
		return nil, false, err
	}
	return ch, s.doc.ForceCharm, nil
}

// SetCharm changes the charm for the service. New units will be started with
// this charm, and existing units will be upgraded to use it. If force is true,
// units will be upgraded even if they are in an error state.
func (s *Service) SetCharm(ch *Charm, force bool) (err error) {
	ops := []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: D{{"life", Alive}},
		Update: D{{"$set", D{{"charmurl", ch.URL()}, {"forcecharm", force}}}},
	}}
	err = s.st.runner.Run(ops, "", nil)
	if err != nil {
		return fmt.Errorf("cannot set charm for service %q: %v", s, deadOnAbort(err))
	}
	s.doc.CharmURL = ch.URL()
	s.doc.ForceCharm = force
	return nil
}

// String returns the service name.
func (s *Service) String() string {
	return s.doc.Name
}

func (s *Service) Refresh() error {
	err := s.st.services.FindId(s.doc.Name).One(&s.doc)
	if err != nil {
		return fmt.Errorf("cannot refresh service %v: %v", s, err)
	}
	return nil
}

// newUnitName returns the next unit name.
func (s *Service) newUnitName() (string, error) {
	sel := D{{"_id", s.doc.Name}, {"life", Alive}}
	change := mgo.Change{Update: D{{"$inc", D{{"unitseq", 1}}}}}
	result := serviceDoc{}
	_, err := s.st.services.Find(sel).Apply(change, &result)
	if err != nil {
		return "", err
	}
	name := s.doc.Name + "/" + strconv.Itoa(result.UnitSeq)
	return name, nil
}

// addUnit adds the named unit.
func (s *Service) addUnit(name string, principal string) (*Unit, error) {
	udoc := unitDoc{
		Name:      name,
		Service:   s.doc.Name,
		Principal: principal,
		Life:      Alive,
	}
	ops := []txn.Op{{
		C:      s.st.units.Name,
		Id:     udoc.Name,
		Assert: txn.DocMissing,
		Insert: udoc,
	}}
	err := s.st.runner.Run(ops, "", nil)
	if err != nil {
		return nil, fmt.Errorf("cannot add unit to service %q", s)
	}
	return newUnit(s.st, &udoc), nil
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (unit *Unit, err error) {
	ch, _, err := s.Charm()
	if err != nil {
		return nil, fmt.Errorf("cannot add unit to service %q: %v", err)
	}
	if ch.Meta().Subordinate {
		return nil, fmt.Errorf("cannot directly add units to subordinate service %q", s)
	}
	name, err := s.newUnitName()
	if err != nil {
		return nil, fmt.Errorf("cannot add unit to service %q: %v", err)
	}
	return s.addUnit(name, "")
}

// AddUnitSubordinateTo adds a new subordinate unit to the service,
// subordinate to principal.
func (s *Service) AddUnitSubordinateTo(principal *Unit) (*Unit, error) {
	ch, _, err := s.Charm()
	if err != nil {
		return nil, fmt.Errorf("cannot add unit to service %q: %v", err)
	}
	if !ch.Meta().Subordinate {
		return nil, fmt.Errorf("cannot add unit of principal service %q as a subordinate of %q", s, principal)
	}
	if !principal.IsPrincipal() {
		return nil, errors.New("a subordinate unit must be added to a principal unit")
	}
	name, err := s.newUnitName()
	if err != nil {
		return nil, fmt.Errorf("cannot add unit to service %q: %v", err)
	}
	return s.addUnit(name, principal.Name())
}

// RemoveUnit removes the given unit from s.
func (s *Service) RemoveUnit(u *Unit) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove unit %q", u)
	if u.doc.Life != Dead {
		return errors.New("unit is not dead")
	}
	if u.doc.Service != s.doc.Name {
		return fmt.Errorf("unit is not assigned to service %q", s)
	}
	ops := []txn.Op{{
		C:      s.st.units.Name,
		Id:     u.doc.Name,
		Assert: D{{"life", Dead}},
		Remove: true,
	}}
	err = s.st.runner.Run(ops, "", nil)
	if err != nil {
		return deadOnAbort(err)
	}
	return nil
}

func (s *Service) unitDoc(name string) (*unitDoc, error) {
	udoc := &unitDoc{}
	sel := D{
		{"_id", name},
		{"service", s.doc.Name},
		{"life", Alive},
	}
	err := s.st.units.Find(sel).One(udoc)
	if err != nil {
		return nil, err
	}
	return udoc, nil
}

// Unit returns the service's unit with name.
func (s *Service) Unit(name string) (*Unit, error) {
	udoc, err := s.unitDoc(name)
	if err != nil {
		return nil, fmt.Errorf("cannot get unit %q from service %q: %v", name, s.doc.Name, err)
	}
	return newUnit(s.st, udoc), nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() (units []*Unit, err error) {
	docs := []unitDoc{}
	sel := D{{"service", s.doc.Name}, {"life", Alive}}
	err = s.st.units.Find(sel).All(&docs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all units from service %q: %v", err)
	}
	for i := range docs {
		units = append(units, newUnit(s.st, &docs[i]))
	}
	return units, nil
}

// Relations returns a Relation for every relation the service is in.
func (s *Service) Relations() (relations []*Relation, err error) {
	defer trivial.ErrorContextf(&err, "can't get relations for service %q", s)
	sel := D{
		{"life", Alive},
		{"endpoints.servicename", s.doc.Name},
	}
	docs := []relationDoc{}
	err = s.st.relations.Find(sel).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, v := range docs {
		relations = append(relations, newRelation(s.st, &v))
	}
	return relations, nil
}

// Config returns the configuration node for the service.
func (s *Service) Config() (config *ConfigNode, err error) {
	config, err = readConfigNode(s.st, "s/"+s.Name())
	if err != nil {
		return nil, fmt.Errorf("cannot get configuration of service %q: %v", s, err)
	}
	return config, nil
}
