package state

import (
	"errors"
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
	"sort"
	"strconv"
)

// Service represents the state of a service.
type Service struct {
	st  *State
	doc serviceDoc
	annotator
}

// serviceDoc represents the internal state of a service in MongoDB.
type serviceDoc struct {
	Name          string `bson:"_id"`
	CharmURL      *charm.URL
	ForceCharm    bool
	Life          Life
	UnitSeq       int
	UnitCount     int
	RelationCount int
	Exposed       bool
	TxnRevno      int64 `bson:"txn-revno"`
	Annotations   map[string]string
}

func newService(st *State, doc *serviceDoc) *Service {
	ann := annotator{st: st, coll: st.services.Name, id: doc.Name}
	svc := &Service{st: st, doc: *doc, annotator: ann}
	svc.annotator.annotations = &svc.doc.Annotations
	return svc
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.doc.Name
}

// Annotations returns the service annotations.
func (s *Service) Annotations() map[string]string {
	return s.doc.Annotations
}

// globalKey returns the global database key for the service.
func (s *Service) globalKey() string {
	return "s#" + s.doc.Name
}

// Life returns whether the service is Alive, Dying or Dead.
func (s *Service) Life() Life {
	return s.doc.Life
}

var errRefresh = errors.New("cannot determine relation destruction operations; please refresh the service")

// Destroy ensures that the service and all its relations will be removed at
// some point; if the service has no units, and no relation involving the
// service has any units in scope, they are all removed immediately.
func (s *Service) Destroy() (err error) {
	defer trivial.ErrorContextf(&err, "cannot destroy service %q", s)
	defer func() {
		if err != nil {
			// This is a white lie; the document might actually be removed.
			s.doc.Life = Dying
		}
	}()
	svc := &Service{st: s.st, doc: s.doc}
	for i := 0; i < 5; i++ {
		ops, err := svc.destroyOps()
		switch {
		case err == errRefresh:
		case err == errAlreadyDying:
			return nil
		case err != nil:
			return err
		default:
			if err := svc.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
				return err
			}
		}
		if err := svc.Refresh(); IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return ErrExcessiveContention
}

// destroyOps returns the operations required to destroy the service. If it
// returns errRefresh, the service should be refreshed and the destruction
// operations recalculated.
func (s *Service) destroyOps() ([]txn.Op, error) {
	if s.doc.Life == Dying {
		return nil, errAlreadyDying
	}
	rels, err := s.Relations()
	if err != nil {
		return nil, err
	}
	if len(rels) != s.doc.RelationCount {
		// This is just an early bail out. The relations obtained may still
		// be wrong, but that situation will be caught by a combination of
		// asserts on relationcount and on each known relation, below.
		return nil, errRefresh
	}
	var ops []txn.Op
	removeCount := 0
	for _, rel := range rels {
		relOps, isRemove, err := rel.destroyOps(s.doc.Name)
		if err == errAlreadyDying {
			relOps = []txn.Op{{
				C:      s.st.relations.Name,
				Id:     rel.doc.Key,
				Assert: D{{"life", Dying}},
			}}
		} else if err != nil {
			return nil, err
		}
		if isRemove {
			removeCount++
		}
		ops = append(ops, relOps...)
	}
	// If the service has no units, and all its known relations will be
	// removed, the service can also be removed.
	if s.doc.UnitCount == 0 && s.doc.RelationCount == removeCount {
		hasLastRefs := D{{"life", Alive}, {"unitcount", 0}, {"relationcount", removeCount}}
		return append(ops, s.removeOps(hasLastRefs)...), nil
	}
	// If any units of the service exist, or if any known relation was not
	// removed (because it had units in scope, or because it was Dying, which
	// implies the same condition), service removal will be handled as a
	// consequence of the removal of the last unit or relation referencing it.
	notLastRefs := D{
		{"life", Alive},
		{"$or", []D{
			{{"unitcount", D{{"$gt", 0}}}},
			{{"relationcount", s.doc.RelationCount}},
		}},
	}
	update := D{{"$set", D{{"life", Dying}}}}
	if removeCount != 0 {
		decref := D{{"$inc", D{{"relationcount", -removeCount}}}}
		update = append(update, decref...)
	}
	return append(ops, txn.Op{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: notLastRefs,
		Update: update,
	}), nil
}

// removeOps returns the operations required to remove the service. Supplied
// asserts will be included in the operation on the service document.
func (s *Service) removeOps(asserts D) []txn.Op {
	return []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: asserts,
		Remove: true,
	}, {
		C:      s.st.settings.Name,
		Id:     s.globalKey(),
		Remove: true,
	}, {
		C:      s.st.constraints.Name,
		Id:     s.globalKey(),
		Remove: true,
	}}
}

// IsExposed returns whether this service is exposed. The explicitly open
// ports (with open-port) for exposed services may be accessed from machines
// outside of the local deployment network. See SetExposed and ClearExposed.
func (s *Service) IsExposed() bool {
	return s.doc.Exposed
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	return s.setExposed(true)
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	return s.setExposed(false)
}

func (s *Service) setExposed(exposed bool) (err error) {
	ops := []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"exposed", exposed}}}},
	}}
	if err := s.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set exposed flag for service %q to %v: %v", s, exposed, onAbort(err, errNotAlive))
	}
	s.doc.Exposed = exposed
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

// CharmURL returns the service's charm URL, and whether units should upgrade
// to the charm with that URL even if they are in an error state.
func (s *Service) CharmURL() (curl *charm.URL, force bool) {
	return s.doc.CharmURL, s.doc.ForceCharm
}

// Endpoints returns the service's currently available relation endpoints.
func (s *Service) Endpoints() (eps []Endpoint, err error) {
	ch, _, err := s.Charm()
	if err != nil {
		return nil, err
	}
	collect := func(role RelationRole, rels map[string]charm.Relation) {
		for name, rel := range rels {
			eps = append(eps, Endpoint{
				ServiceName:   s.doc.Name,
				Interface:     rel.Interface,
				RelationName:  name,
				RelationRole:  role,
				RelationScope: rel.Scope,
			})
		}
	}
	meta := ch.Meta()
	collect(RolePeer, meta.Peers)
	collect(RoleProvider, meta.Provides)
	collect(RoleRequirer, meta.Requires)
	collect(RoleProvider, map[string]charm.Relation{
		"juju-info": {
			Interface: "juju-info",
			Scope:     charm.ScopeGlobal,
		},
	})
	sort.Sort(epSlice(eps))
	return eps, nil
}

// Endpoint returns the relation endpoint with the supplied name, if it exists.
func (s *Service) Endpoint(relationName string) (Endpoint, error) {
	eps, err := s.Endpoints()
	if err != nil {
		return Endpoint{}, err
	}
	for _, ep := range eps {
		if ep.RelationName == relationName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("service %q has no %q relation", s, relationName)
}

// SetCharm changes the charm for the service. New units will be started with
// this charm, and existing units will be upgraded to use it. If force is true,
// units will be upgraded even if they are in an error state.
func (s *Service) SetCharm(ch *Charm, force bool) (err error) {
	ops := []txn.Op{{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"charmurl", ch.URL()}, {"forcecharm", force}}}},
	}}
	if err := s.st.runner.Run(ops, "", nil); err != nil {
		return fmt.Errorf("cannot set charm for service %q: %v", s, onAbort(err, errNotAlive))
	}
	s.doc.CharmURL = ch.URL()
	s.doc.ForceCharm = force
	return nil
}

// String returns the service name.
func (s *Service) String() string {
	return s.doc.Name
}

// Refresh refreshes the contents of the Service from the underlying
// state. It returns an error that satisfies IsNotFound if the service has
// been removed.
func (s *Service) Refresh() error {
	err := s.st.services.FindId(s.doc.Name).One(&s.doc)
	if err == mgo.ErrNotFound {
		return NotFoundf("service %q", s)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh service %q: %v", s, err)
	}
	return nil
}

// newUnitName returns the next unit name.
func (s *Service) newUnitName() (string, error) {
	change := mgo.Change{Update: D{{"$inc", D{{"unitseq", 1}}}}}
	result := serviceDoc{}
	if _, err := s.st.services.Find(D{{"_id", s.doc.Name}}).Apply(change, &result); err == mgo.ErrNotFound {
		return "", NotFoundf("service %q", s)
	} else if err != nil {
		return "", fmt.Errorf("cannot increment unit sequence: %v", err)
	}
	name := s.doc.Name + "/" + strconv.Itoa(result.UnitSeq)
	return name, nil
}

// addUnitOps returns a unique name for a new unit, and a list of txn operations
// necessary to create that unit. The principalName param must be non-empty if
// and only if s is a subordinate service. Only one subordinate of a given
// service will be assigned to a given principal.
func (s *Service) addUnitOps(principalName string) (string, []txn.Op, error) {
	ch, _, err := s.Charm()
	if err != nil {
		return "", nil, err
	}
	if subordinate := ch.Meta().Subordinate; subordinate && principalName == "" {
		return "", nil, fmt.Errorf("service is a subordinate")
	} else if !subordinate && principalName != "" {
		return "", nil, fmt.Errorf("service is not a subordinate")
	}
	name, err := s.newUnitName()
	if err != nil {
		return "", nil, err
	}
	udoc := &unitDoc{
		Name:      name,
		Service:   s.doc.Name,
		Life:      Alive,
		Status:    UnitPending,
		Principal: principalName,
	}
	ops := []txn.Op{{
		C:      s.st.units.Name,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: udoc,
	}, {
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Assert: isAliveDoc,
		Update: D{{"$inc", D{{"unitcount", 1}}}},
	}}
	if principalName != "" {
		ops = append(ops, txn.Op{
			C:  s.st.units.Name,
			Id: principalName,
			Assert: append(isAliveDoc, bson.DocElem{
				"subordinates", D{{"$not", bson.RegEx{Pattern: "^" + s.doc.Name + "/"}}},
			}),
			Update: D{{"$addToSet", D{{"subordinates", name}}}},
		})
	}
	return name, ops, nil
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (unit *Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot add unit to service %q", s)
	name, ops, err := s.addUnitOps("")
	if err != nil {
		return nil, err
	}
	if err := s.st.runner.Run(ops, "", nil); err == txn.ErrAborted {
		if alive, err := isAlive(s.st.services, s.doc.Name); err != nil {
			return nil, err
		} else if !alive {
			return nil, fmt.Errorf("service is not alive")
		}
		return nil, fmt.Errorf("inconsistent state")
	} else if err != nil {
		return nil, err
	}
	return s.Unit(name)
}

var ErrExcessiveContention = errors.New("state changing too quickly; try again soon")

func (s *Service) removeUnitOps(u *Unit) []txn.Op {
	var ops []txn.Op
	if u.doc.Principal != "" {
		ops = append(ops, txn.Op{
			C:      s.st.units.Name,
			Id:     u.doc.Principal,
			Assert: txn.DocExists,
			Update: D{{"$pull", D{{"subordinates", u.doc.Name}}}},
		})
	} else if u.doc.MachineId != "" {
		ops = append(ops, txn.Op{
			C:      s.st.machines.Name,
			Id:     u.doc.MachineId,
			Assert: txn.DocExists,
			Update: D{{"$pull", D{{"principals", u.doc.Name}}}},
		})
	}
	ops = append(ops, txn.Op{
		C:      s.st.units.Name,
		Id:     u.doc.Name,
		Assert: txn.DocExists,
		Remove: true,
	})
	if s.doc.Life == Dying && s.doc.RelationCount == 0 && s.doc.UnitCount == 1 {
		hasLastRef := D{{"life", Dying}, {"relationcount", 0}, {"unitcount", 1}}
		return append(ops, s.removeOps(hasLastRef)...)
	}
	svcOp := txn.Op{
		C:      s.st.services.Name,
		Id:     s.doc.Name,
		Update: D{{"$inc", D{{"unitcount", -1}}}},
	}
	if s.doc.Life == Alive {
		svcOp.Assert = D{{"life", Alive}, {"unitcount", D{{"$gt", 0}}}}
	} else {
		svcOp.Assert = D{{"life", Dying}, {"unitcount", D{{"$gt", 1}}}}
	}
	return append(ops, svcOp)
}

// Unit returns the service's unit with name.
func (s *Service) Unit(name string) (*Unit, error) {
	if !IsUnitName(name) {
		return nil, fmt.Errorf("%q is not a valid unit name", name)
	}
	udoc := &unitDoc{}
	sel := D{{"_id", name}, {"service", s.doc.Name}}
	if err := s.st.units.Find(sel).One(udoc); err != nil {
		return nil, fmt.Errorf("cannot get unit %q from service %q: %v", name, s.doc.Name, err)
	}
	return newUnit(s.st, udoc), nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() (units []*Unit, err error) {
	docs := []unitDoc{}
	err = s.st.units.Find(D{{"service", s.doc.Name}}).All(&docs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all units from service %q: %v", s, err)
	}
	for i := range docs {
		units = append(units, newUnit(s.st, &docs[i]))
	}
	return units, nil
}

// Relations returns a Relation for every relation the service is in.
func (s *Service) Relations() (relations []*Relation, err error) {
	defer trivial.ErrorContextf(&err, "can't get relations for service %q", s)
	docs := []relationDoc{}
	err = s.st.relations.Find(D{{"endpoints.servicename", s.doc.Name}}).All(&docs)
	if err != nil {
		return nil, err
	}
	for _, v := range docs {
		relations = append(relations, newRelation(s.st, &v))
	}
	return relations, nil
}

// Config returns the configuration node for the service.
func (s *Service) Config() (config *Settings, err error) {
	config, err = readSettings(s.st, s.globalKey())
	if err != nil {
		return nil, fmt.Errorf("cannot get configuration of service %q: %v", s, err)
	}
	return config, nil
}

// Constraints returns the current service constraints.
func (s *Service) Constraints() (Constraints, error) {
	return readConstraints(s.st, s.globalKey())
}

// SetConstraints replaces the current service constraints.
func (s *Service) SetConstraints(cons Constraints) error {
	return writeConstraints(s.st, s.globalKey(), cons)
}
