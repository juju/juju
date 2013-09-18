// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/utils"
)

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []Endpoint) string {
	eps := epSlice{}
	for _, ep := range endpoints {
		eps = append(eps, ep)
	}
	sort.Sort(eps)
	names := []string{}
	for _, ep := range eps {
		names = append(names, ep.String())
	}
	return strings.Join(names, " ")
}

// relationDoc is the internal representation of a Relation in MongoDB.
// Note the correspondence with RelationInfo in state/api/params.
type relationDoc struct {
	Key       string `bson:"_id"`
	Id        int
	Endpoints []Endpoint
	Life      Life
	UnitCount int
}

// Relation represents a relation between one or two service endpoints.
type Relation struct {
	st  *State
	doc relationDoc
}

func newRelation(st *State, doc *relationDoc) *Relation {
	return &Relation{
		st:  st,
		doc: *doc,
	}
}

func (r *Relation) String() string {
	return r.doc.Key
}

// Tag returns a name identifying the relation that is safe to use
// as a file name.
func (r *Relation) Tag() string {
	return names.RelationTag(r.doc.Key)
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies IsNotFound if the relation has been
// removed.
func (r *Relation) Refresh() error {
	doc := relationDoc{}
	err := r.st.relations.FindId(r.doc.Key).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("relation %v", r)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh relation %v: %v", r, err)
	}
	if r.doc.Id != doc.Id {
		// The relation has been destroyed and recreated. This is *not* the
		// same relation; if we pretend it is, we run the risk of violating
		// the lifecycle-only-advances guarantee.
		return errors.NotFoundf("relation %v", r)
	}
	r.doc = doc
	return nil
}

// Life returns the relation's current life state.
func (r *Relation) Life() Life {
	return r.doc.Life
}

// Destroy ensures that the relation will be removed at some point; if no units
// are currently in scope, it will be removed immediately.
func (r *Relation) Destroy() (err error) {
	defer utils.ErrorContextf(&err, "cannot destroy relation %q", r)
	if len(r.doc.Endpoints) == 1 && r.doc.Endpoints[0].Role == charm.RolePeer {
		return fmt.Errorf("is a peer relation")
	}
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			r.doc.Life = Dying
		}
	}()
	rel := &Relation{r.st, r.doc}
	// In this context, aborted transactions indicate that the number of units
	// in scope have changed between 0 and not-0. The chances of 5 successive
	// attempts each hitting this change -- which is itself an unlikely one --
	// are considered to be extremely small.
	for attempt := 0; attempt < 5; attempt++ {
		ops, _, err := rel.destroyOps("")
		if err == errAlreadyDying {
			return nil
		} else if err != nil {
			return err
		}
		if err := rel.st.runTransaction(ops); err != txn.ErrAborted {
			return err
		}
		if err := rel.Refresh(); errors.IsNotFoundError(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return ErrExcessiveContention
}

var errAlreadyDying = stderrors.New("entity is already dying and cannot be destroyed")

// destroyOps returns the operations necessary to destroy the relation, and
// whether those operations will lead to the relation's removal. These
// operations may include changes to the relation's services; however, if
// ignoreService is not empty, no operations modifying that service will
// be generated.
func (r *Relation) destroyOps(ignoreService string) (ops []txn.Op, isRemove bool, err error) {
	if r.doc.Life != Alive {
		return nil, false, errAlreadyDying
	}
	if r.doc.UnitCount == 0 {
		removeOps, err := r.removeOps(ignoreService, nil)
		if err != nil {
			return nil, false, err
		}
		return removeOps, true, nil
	}
	return []txn.Op{{
		C:      r.st.relations.Name,
		Id:     r.doc.Key,
		Assert: D{{"life", Alive}, {"unitcount", D{{"$gt", 0}}}},
		Update: D{{"$set", D{{"life", Dying}}}},
	}}, false, nil
}

// removeOps returns the operations necessary to remove the relation. If
// ignoreService is not empty, no operations affecting that service will be
// included; if departingUnit is not nil, this implies that the relation's
// services may be Dying and otherwise unreferenced, and may thus require
// removal themselves.
func (r *Relation) removeOps(ignoreService string, departingUnit *Unit) ([]txn.Op, error) {
	relOp := txn.Op{
		C:      r.st.relations.Name,
		Id:     r.doc.Key,
		Remove: true,
	}
	if departingUnit != nil {
		relOp.Assert = D{{"life", Dying}, {"unitcount", 1}}
	} else {
		relOp.Assert = D{{"life", Alive}, {"unitcount", 0}}
	}
	ops := []txn.Op{relOp}
	for _, ep := range r.doc.Endpoints {
		if ep.ServiceName == ignoreService {
			continue
		}
		var asserts D
		hasRelation := D{{"relationcount", D{{"$gt", 0}}}}
		if departingUnit == nil {
			// We're constructing a destroy operation, either of the relation
			// or one of its services, and can therefore be assured that both
			// services are Alive.
			asserts = append(hasRelation, isAliveDoc...)
		} else if ep.ServiceName == departingUnit.ServiceName() {
			// This service must have at least one unit -- the one that's
			// departing the relation -- so it cannot be ready for removal.
			cannotDieYet := D{{"unitcount", D{{"$gt", 0}}}}
			asserts = append(hasRelation, cannotDieYet...)
		} else {
			// This service may require immediate removal.
			svc := &Service{st: r.st}
			hasLastRef := D{{"life", Dying}, {"unitcount", 0}, {"relationcount", 1}}
			removable := append(D{{"_id", ep.ServiceName}}, hasLastRef...)
			if err := r.st.services.Find(removable).One(&svc.doc); err == nil {
				ops = append(ops, svc.removeOps(hasLastRef)...)
				continue
			} else if err != mgo.ErrNotFound {
				return nil, err
			}
			// If not, we must check that this is still the case when the
			// transaction is applied.
			asserts = D{{"$or", []D{
				{{"life", Alive}},
				{{"unitcount", D{{"$gt", 0}}}},
				{{"relationcount", D{{"$gt", 1}}}},
			}}}
		}
		ops = append(ops, txn.Op{
			C:      r.st.services.Name,
			Id:     ep.ServiceName,
			Assert: asserts,
			Update: D{{"$inc", D{{"relationcount", -1}}}},
		})
	}
	cleanupOp := r.st.newCleanupOp("settings", fmt.Sprintf("r#%d#", r.Id()))
	return append(ops, cleanupOp), nil
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different services.
func (r *Relation) Id() int {
	return r.doc.Id
}

// Endpoint returns the endpoint of the relation for the named service.
// If the service is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(serviceName string) (Endpoint, error) {
	for _, ep := range r.doc.Endpoints {
		if ep.ServiceName == serviceName {
			return ep, nil
		}
	}
	return Endpoint{}, fmt.Errorf("service %q is not a member of %q", serviceName, r)
}

// RelatedEndpoints returns the endpoints of the relation r with which
// units of the named service will establish relations. If the service
// is not part of the relation r, an error will be returned.
func (r *Relation) RelatedEndpoints(serviceName string) ([]Endpoint, error) {
	local, err := r.Endpoint(serviceName)
	if err != nil {
		return nil, err
	}
	role := counterpartRole(local.Role)
	var eps []Endpoint
	for _, ep := range r.doc.Endpoints {
		if ep.Role == role {
			eps = append(eps, ep)
		}
	}
	if eps == nil {
		return nil, fmt.Errorf("no endpoints of %q relate to service %q", r, serviceName)
	}
	return eps, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	ep, err := r.Endpoint(u.doc.Service)
	if err != nil {
		return nil, err
	}
	scope := []string{"r", strconv.Itoa(r.doc.Id)}
	if ep.Scope == charm.ScopeContainer {
		container := u.doc.Principal
		if container == "" {
			container = u.doc.Name
		}
		scope = append(scope, container)
	}
	return &RelationUnit{
		st:       r.st,
		relation: r,
		unit:     u,
		endpoint: ep,
		scope:    strings.Join(scope, "#"),
	}, nil
}
