package mstate

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/charm"
	"strconv"
)

// Service represents the state of a service.
type Service struct {
	st   *State
	name string
}

// serviceDoc represents the internal state of a service in MongoDB.
type serviceDoc struct {
	Name     string `bson:"_id"`
	CharmURL *charm.URL
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.name
}

// CharmURL returns the charm URL this service is supposed to use.
func (s *Service) CharmURL() (url *charm.URL, err error) {
	sdoc := &serviceDoc{}
	err = s.st.services.Find(bson.D{{"_id", s.name}}).One(sdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get the charm URL of service %q: %v", s, err)
	}
	return sdoc.CharmURL, nil
}

// SetCharmURL changes the charm URL for the service.
func (s *Service) SetCharmURL(url *charm.URL) (err error) {
	change := bson.D{{"$set", bson.D{{"charmurl", url}}}}
	err = s.st.services.Update(bson.D{{"_id", s.name}}, change)
	if err != nil {
		return fmt.Errorf("can't set the charm URL of service %q: %v", s, err)
	}
	return nil
}

// Charm returns the service's charm.
func (s *Service) Charm() (*Charm, error) {
	url, err := s.CharmURL()
	if err != nil {
		return nil, err
	}
	return s.st.Charm(url)
}

// String returns the service name.
func (s *Service) String() string {
	return s.Name()
}

// newUnitName returns the next unit name.
func (s *Service) newUnitName() (string, error) {
	id, err := s.st.sequence(s.Name())
	if err != nil {
		return "", err
	}
	name := s.name + "/" + strconv.Itoa(id)
	return name, nil
}

// addUnit adds the named unit, which is part of unitSet.
func (s *Service) addUnit(name string, principal string) (*Unit, error) {
	udoc := unitDoc{
		Name:      name,
		Service:   s.name,
		Principal: principal,
	}
	err := s.st.units.Insert(udoc)
	if err != nil {
		return nil, fmt.Errorf("can't add unit to service %q", s)
	}
	return newUnit(s.st, &udoc), nil
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (unit *Unit, err error) {
	ch, err := s.Charm()
	if err != nil {
		return nil, fmt.Errorf("can't add unit to service %q: %v", err)
	}
	if ch.Meta().Subordinate {
		return nil, fmt.Errorf("cannot directly add units to subordinate service %q", s)
	}
	name, err := s.newUnitName()
	if err != nil {
		return nil, fmt.Errorf("can't add unit to service %q: %v", err)
	}
	return s.addUnit(name, "")
}

// AddUnitSubordinateTo adds a new subordinate unit to the service,
// subordinate to principal.
func (s *Service) AddUnitSubordinateTo(principal *Unit) (*Unit, error) {
	ch, err := s.Charm()
	if err != nil {
		return nil, fmt.Errorf("can't add unit to service %q: %v", err)
	}
	if !ch.Meta().Subordinate {
		return nil, fmt.Errorf("can't add unit of principal service %q as a subordinate of %q", s, principal)
	}
	if !principal.IsPrincipal() {
		return nil, errors.New("a subordinate unit must be added to a principal unit")
	}
	name, err := s.newUnitName()
	if err != nil {
		return nil, fmt.Errorf("can't add unit to service %q: %v", err)
	}
	return s.addUnit(name, principal.Name())
}

// RemovesUnit removes the given unit from s.
func (s *Service) RemoveUnit(unit *Unit) error {
	sel := bson.D{
		{"_id", unit.Name()},
		{"service", s.name},
	}
	err := s.st.units.Remove(sel)
	if err != nil {
		return fmt.Errorf("can't remove unit %q: %v", unit, err)
	}
	// TODO unassign from machine if currently assigned.
	return nil
}

func (s *Service) unitDoc(name string) (*unitDoc, error) {
	udoc := &unitDoc{}
	sel := bson.D{
		{"_id", name},
		{"service", s.name},
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
		return nil, fmt.Errorf("can't get unit %q from service %q: %v", name, s.name, err)
	}
	return newUnit(s.st, udoc), nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() (units []*Unit, err error) {
	docs := []unitDoc{}
	err = s.st.units.Find(bson.D{{"service", s.name}}).All(&docs)
	if err != nil {
		return nil, fmt.Errorf("can't get all units from service %q: %v", err)
	}
	for i := range docs {
		units = append(units, newUnit(s.st, &docs[i]))
	}
	return units, nil
}
