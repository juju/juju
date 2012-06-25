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

// State represents the state of an environment
// managed by juju.
type State struct {
	db       *mgo.Database
	machines *mgo.Collection
	charms   *mgo.Collection
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (m *Machine, err error) {
	defer errorContextf(&err, "can't add a new machine")
	id, err := s.sequence("machine")
	if err != nil {
		return nil, err
	}
	err = s.machines.Insert(bson.D{{"_id", id}})
	if err != nil {
		return nil, err
	}
	return &Machine{st: s, id: id}, nil
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id int) error {
	err := s.machines.Remove(bson.D{{"_id", id}})
	if err != nil {
		return fmt.Errorf("can't remove machine %d", id)
	}
	return nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	err = s.machines.Find(nil).Select(bson.D{{"_id", 1}}).All(&mdocs)
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
	err := s.machines.Find(bson.D{{"_id", id}}).One(mdoc)
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
		Url:          curl,
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

// Charm returns a charm by the given id.
func (s *State) Charm(curl *charm.URL) (stch *Charm, err error) {
	cdoc := &charmDoc{}
	err = s.charms.Find(bson.D{{"_id", curl}}).One(cdoc)
	if err != nil {
		return nil, fmt.Errorf("can't get charm %q: %v", curl, err)
	}

	return newCharm(s, cdoc)
}
