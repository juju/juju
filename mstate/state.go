// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package mstate

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/url"
)

// Tools describes a particular set of juju tools and where to find them.
type Tools struct {
	version.Binary
	URL string
}

// State represents the state of an environment
// managed by juju.
type State struct {
	db        *mgo.Database
	cfgnodes  *mgo.Collection
	charms    *mgo.Collection
	machines  *mgo.Collection
	relations *mgo.Collection
	services  *mgo.Collection
	units     *mgo.Collection
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (m *Machine, err error) {
	defer trivial.ErrorContextf(&err, "cannot add a new machine")
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
	return newMachine(s, &mdoc), nil
}

// RemoveMachine removes the machine with the the given id.
func (s *State) RemoveMachine(id int) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove machine %d", id)
	m, err := s.Machine(id)
	if err != nil {
		return err
	}
	if m.doc.Life != Dead {
		panic(fmt.Errorf("machine %d is not dead", id))
	}
	sel := bson.D{
		{"_id", id},
		{"life", Dead},
	}
	err = s.machines.Remove(sel)
	if err != nil {
		return err
	}
	return nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	sel := bson.D{}
	err = s.machines.Find(sel).Select(bson.D{{"_id", 1}}).All(&mdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all machines: %v", err)
	}
	for _, doc := range mdocs {
		machines = append(machines, newMachine(s, &doc))
	}
	return
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	mdoc := &machineDoc{}
	sel := bson.D{{"_id", id}}
	err := s.machines.Find(sel).One(mdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot get machine %d: %v", id, err)
	}
	return newMachine(s, mdoc), nil
}

// AddCharm adds the ch charm with curl to the state.  bundleUrl must be
// set to a URL where the bundle for ch may be downloaded from.
// On success the newly added charm state is returned.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
	cdoc := &charmDoc{
		URL:          curl,
		Meta:         ch.Meta(),
		Config:       ch.Config(),
		BundleURL:    bundleURL,
		BundleSha256: bundleSha256,
	}
	err = s.charms.Insert(cdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm %q: %v", curl, err)
	}
	return newCharm(s, cdoc)
}

// Charm returns the charm with the given URL.
func (s *State) Charm(curl *charm.URL) (*Charm, error) {
	cdoc := &charmDoc{}
	err := s.charms.Find(bson.D{{"_id", curl}}).One(cdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot get charm %q: %v", curl, err)
	}

	return newCharm(s, cdoc)
}

// AddService creates a new service state with the given unique name
// and the charm state.
func (s *State) AddService(name string, ch *Charm) (service *Service, err error) {
	sdoc := serviceDoc{
		Name:     name,
		CharmURL: ch.URL(),
		Life:     Alive,
	}
	err = s.services.Insert(sdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot add service %q:", name, err)
	}
	return &Service{st: s, doc: sdoc}, nil
}

// RemoveService removes a service from the state. It will also remove all
// its units and break any of its existing relations.
func (s *State) RemoveService(svc *Service) (err error) {
	// TODO Integrate with txn and do lifecycle properly.
	defer trivial.ErrorContextf(&err, "cannot remove service %q", svc)

	if svc.doc.Life != Dead {
		panic(fmt.Errorf("service %q is not dead", svc))
	}
	// Remove relations first, to minimize unwanted hook executions.
	rels, err := svc.Relations()
	if err != nil {
		return err
	}
	for _, rel := range rels {
		err = rel.Die()
		if err != nil {
			return err
		}
		err = s.RemoveRelation(rel)
		if err != nil {
			return err
		}
	}
	// TODO Will be deleted with proper lifecycle integration.
	units, err := svc.AllUnits()
	if err != nil {
		return err
	}
	for _, unit := range units {
		err = unit.Die()
		if err != nil {
			return err
		}
		if err = svc.RemoveUnit(unit); err != nil {
			return err
		}
	}
	// Remove the service.
	sel := bson.D{
		{"_id", svc.doc.Name},
		{"life", Dead},
	}
	err = s.services.Remove(sel)
	if err != nil {
		return err
	}
	return nil
}

// Service returns a service state by name.
func (s *State) Service(name string) (service *Service, err error) {
	sdoc := serviceDoc{}
	sel := bson.D{{"_id", name}}
	err = s.services.Find(sel).One(&sdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot get service %q: %v", name, err)
	}
	return &Service{st: s, doc: sdoc}, nil
}

// AllServices returns all deployed services in the environment.
func (s *State) AllServices() (services []*Service, err error) {
	sdocs := []serviceDoc{}
	err = s.services.Find(bson.D{}).All(&sdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all services")
	}
	for _, v := range sdocs {
		services = append(services, &Service{st: s, doc: v})
	}
	return services, nil
}

// AddRelation creates a new relation with the given endpoints.
func (s *State) AddRelation(endpoints ...RelationEndpoint) (r *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot add relation %q", relationKey(endpoints))
	switch len(endpoints) {
	case 1:
		if endpoints[0].RelationRole != RolePeer {
			return nil, fmt.Errorf("single endpoint must be a peer relation")
		}
	case 2:
		if !endpoints[0].CanRelateTo(&endpoints[1]) {
			return nil, fmt.Errorf("endpoints do not relate")
		}
	default:
		return nil, fmt.Errorf("cannot relate %d endpoints", len(endpoints))
	}

	var scope charm.RelationScope
	for _, v := range endpoints {
		if v.RelationScope == charm.ScopeContainer {
			scope = charm.ScopeContainer
		}
		// BUG(aram): potential race in the time between getting the service
		// to validate the endpoint and actually writting the relation
		// into MongoDB; the service might have disappeared.
		_, err = s.Service(v.ServiceName)
		if err != nil {
			return nil, err
		}
	}
	if scope == charm.ScopeContainer {
		for i := range endpoints {
			endpoints[i].RelationScope = scope
		}
	}
	id, err := s.sequence("relation")
	if err != nil {
		return nil, err
	}
	doc := relationDoc{
		Id:        id,
		Key:       relationKey(endpoints),
		Endpoints: endpoints,
		Life:      Alive,
	}
	err = s.relations.Insert(doc)
	if err != nil {
		return nil, err
	}
	return newRelation(s, &doc), nil
}

// Relation returns the existing relation with the given endpoints.
func (s *State) Relation(endpoints ...RelationEndpoint) (r *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot get relation %q", relationKey(endpoints))

	doc := relationDoc{}
	err = s.relations.Find(bson.D{{"key", relationKey(endpoints)}}).One(&doc)
	if err != nil {
		return nil, err
	}
	return newRelation(s, &doc), nil
}

// RemoveRelation removes the supplied relation.
func (s *State) RemoveRelation(r *Relation) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove relation %q", r.doc.Key)

	if r.doc.Life != Dead {
		panic(fmt.Errorf("relation %q is not dead", r))
	}
	sel := bson.D{
		{"_id", r.doc.Id},
		{"life", Dead},
	}
	err = s.relations.Remove(sel)
	if err != nil {
		return err
	}
	return nil
}

// Unit returns a unit by name.
func (s *State) Unit(name string) (*Unit, error) {
	doc := unitDoc{}
	err := s.units.FindId(name).One(&doc)
	if err != nil {
		return nil, fmt.Errorf("cannot get unit %q: %v", name, err)
	}
	return newUnit(s, &doc), nil
}
