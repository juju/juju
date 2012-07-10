// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package mstate

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/charm"
	"net/url"
)

type Life int

const (
	Alive Life = 1 + iota
	Dying
	Dead
)

// State represents the state of an environment
// managed by juju.
type State struct {
	db       *mgo.Database
	charms   *mgo.Collection
	machines *mgo.Collection
	services *mgo.Collection
	units    *mgo.Collection
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (m *Machine, err error) {
	defer errorContextf(&err, "can't add a new machine")
	id, err := s.sequence("machine")
	if err != nil {
		return nil, err
	}
	mdoc := machineDoc{
		Id:   id,
		Life: Alive,
	}
	err = s.machines.Insert(mdoc)
	if err != nil {
		return nil, err
	}
	return &Machine{st: s, id: id}, nil
}

// RemoveMachine removes the machine with the the given id.
func (s *State) RemoveMachine(id int) error {
	sel := bson.D{{"_id", id}, {"life", Alive}}
	change := bson.D{{"$set", bson.D{{"life", Dying}}}}
	err := s.machines.Update(sel, change)
	if err != nil {
		return fmt.Errorf("can't remove machine %d", id)
	}
	return nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	sel := bson.D{{"life", Alive}}
	err = s.machines.Find(sel).Select(bson.D{{"_id", 1}}).All(&mdocs)
	if err != nil {
		return nil, fmt.Errorf("can't get all machines: %v", err)
	}
	for _, v := range mdocs {
		machines = append(machines, &Machine{st: s, id: v.Id})
	}
	return
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	mdoc := &machineDoc{}
	sel := bson.D{{"_id", id}, {"life", Alive}}
	err := s.machines.Find(sel).One(mdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get machine %d: %v", id, err)
	}
	return &Machine{st: s, id: mdoc.Id}, nil
}

// AddCharm adds the ch charm with curl to the state.  bundleUrl must be
// set to a URL where the bundle for ch may be downloaded from.
// On success the newly added charm state is returned.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
	cdoc := &charmDoc{
		URL:          curl,
		Meta:         ch.Meta(),
		Config:       ch.Config(),
		BundleURL:    bundleURL.String(),
		BundleSha256: bundleSha256,
	}
	err = s.charms.Insert(cdoc)
	if err != nil {
		return nil, fmt.Errorf("can't add charm %q: %v", curl, err)
	}
	return newCharm(s, cdoc)
}

// Charm returns the charm with the given URL.
func (s *State) Charm(curl *charm.URL) (*Charm, error) {
	cdoc := &charmDoc{}
	err := s.charms.Find(bson.D{{"_id", curl}}).One(cdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get charm %q: %v", curl, err)
	}

	return newCharm(s, cdoc)
}

// AddService creates a new service state with the given unique name
// and the charm state.
func (s *State) AddService(name string, ch *Charm) (service *Service, err error) {
	sdoc := &serviceDoc{
		Name:     name,
		CharmURL: ch.URL(),
		Life:     Alive,
	}
	err = s.services.Insert(sdoc)
	if err != nil {
		return nil, fmt.Errorf("can't add service %q:", name, err)
	}
	return &Service{st: s, name: name}, nil
}

// RemoveService removes a service from the state. It will also remove all
// its units and break any of its existing relations.
func (s *State) RemoveService(svc *Service) (err error) {
	sel := bson.D{{"_id", svc.name}, {"life", Alive}}
	change := bson.D{{"$set", bson.D{{"life", Dying}}}}
	err = s.services.Update(sel, change)
	if err != nil {
		return fmt.Errorf("can't remove service %s: %v", svc, err)
	}
	// TODO Remove units and break relations.
	return
}

// Service returns a service state by name.
func (s *State) Service(name string) (service *Service, err error) {
	sdoc := &serviceDoc{}
	sel := bson.D{{"_id", name}, {"life", Alive}}
	err = s.services.Find(sel).One(sdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get service %q: %v", name, err)
	}
	return &Service{st: s, name: name}, nil
}

// AllServices returns all deployed services in the environment.
func (s *State) AllServices() (services []*Service, err error) {
	sdocs := []serviceDoc{}
	err = s.services.Find(bson.D{{"life", Alive}}).All(&sdocs)
	if err != nil {
		return nil, fmt.Errorf("can't get all services")
	}
	for _, v := range sdocs {
		services = append(services, &Service{st: s, name: v.Name})
	}
	return services, nil
}
