package mstate

import (
	"fmt"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/charm"
)

// Service represents the state of a service.
type Service struct {
	st   *State
	name string
}

// serviceDoc represents the internal state of a service in MongoDB.
type serviceDoc struct {
	Name     string `bson:"_id"`
	CharmUrl *charm.URL
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
	return sdoc.CharmUrl, nil
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
