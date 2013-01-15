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
	"strings"
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

type epSlice []Endpoint

var roleOrder = map[RelationRole]int{
	RoleRequirer: 0,
	RoleProvider: 1,
	RolePeer:     2,
}

func (eps epSlice) Len() int      { return len(eps) }
func (eps epSlice) Swap(i, j int) { eps[i], eps[j] = eps[j], eps[i] }
func (eps epSlice) Less(i, j int) bool {
	ep1 := eps[i]
	ep2 := eps[j]
	if ep1.RelationRole != ep2.RelationRole {
		return roleOrder[ep1.RelationRole] < roleOrder[ep2.RelationRole]
	}
	return ep1.String() < ep2.String()
}

// relationDoc is the internal representation of a Relation in MongoDB.
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

// Refresh refreshes the contents of the relation from the underlying
// state. It returns a NotFoundError if the relation has been removed.
func (r *Relation) Refresh() error {
	doc := relationDoc{}
	err := r.st.relations.FindId(r.doc.Key).One(&doc)
	if err == mgo.ErrNotFound {
		return notFound("relation %v", r)
	}
	if err != nil {
		return fmt.Errorf("cannot refresh relation %v: %v", r, err)
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
	defer trivial.ErrorContextf(&err, "cannot destroy relation %q", r)
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			r.doc.Life = Dying
		}
	}()
	rel := &Relation{r.st, r.doc}
	for attempt := 0; attempt < 5; attempt++ {
		ops, _, err := rel.destroyOps("")
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			return nil
		}
		if err := rel.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
			return err
		}
		if err := rel.Refresh(); IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return fmt.Errorf("units being added during relation removal; shouldn't happen, please contact juju-dev@lists.ubuntu.com")
}

func (r *Relation) destroyOps(destroyingService string) ([]txn.Op, bool, error) {
	if r.doc.Life != Alive {
		return nil, false, nil
	}
	if r.doc.UnitCount == 0 {
		mode := modeDestroyRelation
		if destroyingService != "" {
			mode = modeDestroyService
		}
		ops, err := r.removeOps(mode, destroyingService)
		if err != nil {
			return nil, false, err
		}
		return ops, true, nil
	}
	return []txn.Op{{
		C:      r.st.relations.Name,
		Id:     r.doc.Key,
		Assert: D{{"life", Alive}, {"unitcount", D{{"$gt", 0}}}},
		Update: D{{"$set", D{{"life", Dying}}}},
	}}, false, nil
}

type removeMode string

const (
	modeDestroyService  removeMode = "destroy-service"
	modeDestroyRelation removeMode = "destroy-relation"
	modeLeaveScope      removeMode = "leave-scope"
)

func (r *Relation) removeOps(mode removeMode, serviceName string) ([]txn.Op, error) {
	var ops []txn.Op
	for _, ep := range r.doc.Endpoints {
		epOps, err := r.endpointRemoveOps(mode, serviceName, ep)
		if err != nil {
			return nil, err
		}
		ops = append(ops, epOps...)
	}
	relOp := txn.Op{
		C:      r.st.relations.Name,
		Id:     r.doc.Key,
		Remove: true,
	}
	if mode == modeLeaveScope {
		relOp.Assert = D{{"life", Dying}, {"unitcount", 1}}
	} else {
		relOp.Assert = D{{"life", Alive}, {"unitcount", 0}}
	}
	ops = append(ops, relOp)
	cDoc := &cleanupDoc{
		Id:     bson.NewObjectId(),
		Kind:   "settings",
		Prefix: fmt.Sprintf("r#%d#", r.Id()),
	}
	return append(ops, txn.Op{
		C:      r.st.cleanups.Name,
		Id:     cDoc.Id,
		Insert: cDoc,
	}), nil
}

func (r *Relation) endpointRemoveOps(mode removeMode, serviceName string, ep Endpoint) ([]txn.Op, error) {
	name := ep.ServiceName
	switch mode {
	case modeDestroyRelation:
	case modeDestroyService:
		if name == serviceName {
			// The service destroy op is handling its own changes; don't interfere.
			return nil, nil
		}
	case modeLeaveScope:
		if name != serviceName {
			// If we hold the last reference to a Dying relation, it can be
			// removed directly without further assertions, because nothing
			// can add a reference to a Dying service.
			svc, err := r.st.Service(name)
			if err != nil {
				return nil, err
			}
			if svc.doc.Life == Dying && svc.doc.UnitCount == 0 && svc.doc.RelationCount == 1 {
				return svc.removeOps(nil), nil
			}
		}
	}
	return []txn.Op{{
		C:      r.st.services.Name,
		Id:     name,
		Update: D{{"$inc", D{{"relationcount", -1}}}},
	}}, nil
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
	role := local.RelationRole.counterpartRole()
	var eps []Endpoint
	for _, ep := range r.doc.Endpoints {
		if ep.RelationRole == role {
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
	if ep.RelationScope == charm.ScopeContainer {
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

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st       *State
	relation *Relation
	unit     *Unit
	endpoint Endpoint
	scope    string
}

// Relation returns the relation associated with the unit.
func (ru *RelationUnit) Relation() *Relation {
	return ru.relation
}

// Endpoint returns the relation endpoint that defines the unit's
// participation in the relation.
func (ru *RelationUnit) Endpoint() Endpoint {
	return ru.endpoint
}

// ErrCannotEnterScope indicates that a relation unit failed to enter its scope
// due to either the unit or the relation not being Alive.
var ErrCannotEnterScope = errors.New("cannot enter scope: unit or relation is not alive")

// ErrCannotEnterScopeYet indicates that a relation unit failed to enter its
// scope due to a required and pre-existing subordinate unit that is not Alive.
// Once that subordinate has been removed, a new one can be created.
var ErrCannotEnterScopeYet = errors.New("cannot enter scope yet: non-alive subordinate unit has not been removed")

// EnterScope ensures that the unit has entered its scope in the relation and
// that its relation settings contain its private address. When the unit has
// already entered its relation scope, EnterScope will report success but make
// no changes to state; otherwise, it is an error for either the relation or
// the unit not to be Alive. Once a unit has entered a scope, it stays in scope
// without further intervention; the relation will not be able to become Dead
// until all units have departed its scopes.
//
// If the unit is a principal and the relation has container scope, EnterScope
// will also create the required subordinate unit, if it does not already exist;
// this is because there's no point having a principal in scope if there is no
// corresponding subordinate to join it.
func (ru *RelationUnit) EnterScope() error {
	// Verify that the unit is not already in scope, and abort without error
	// if it is.
	relationKey, err := ru.key(ru.unit.Name())
	if err != nil {
		return err
	}
	desc := fmt.Sprintf("unit %q in relation %q", ru.unit, ru.relation)
	if count, err := ru.st.relationScopes.FindId(relationKey).Count(); err != nil {
		return fmt.Errorf("cannot examine scope for %s: %v", desc, err)
	} else if count != 0 {
		return nil
	}

	// Collect the operations that are always required to enter scope.
	unitName := ru.unit.doc.Name
	ops := []txn.Op{{
		C:      ru.st.relationScopes.Name,
		Id:     relationKey,
		Assert: txn.DocMissing,
		Insert: relationScopeDoc{relationKey},
	}, {
		C:      ru.st.relations.Name,
		Id:     ru.relation.doc.Key,
		Assert: isAliveDoc,
		Update: D{{"$inc", D{{"unitcount", 1}}}},
	}, {
		C:      ru.st.units.Name,
		Id:     unitName,
		Assert: isAliveDoc,
	}}

	// Collect the operations necessary to create the unit settings in this
	// relation, if they do not already exist.
	if _, err := readSettings(ru.st, relationKey); IsNotFound(err) {
		address, ok := ru.unit.PrivateAddress()
		if !ok {
			return fmt.Errorf("cannot initialize state for %s: private address not set", desc)
		}
		ops = append([]txn.Op{{
			C:      ru.st.settings.Name,
			Id:     relationKey,
			Assert: txn.DocMissing,
			Insert: map[string]interface{}{"private-address": address},
		}}, ops...)
	} else if err != nil {
		return fmt.Errorf("cannot check settings for %s: %v", desc, err)
	}

	// If the unit should have a subordinate, and does not, collect the
	// operations necessary to create it.
	var existingSubName string
	if ru.unit.IsPrincipal() && ru.endpoint.RelationScope == charm.ScopeContainer {
		related, err := ru.relation.RelatedEndpoints(ru.endpoint.ServiceName)
		if err != nil {
			return err
		} else if len(related) != 1 {
			return fmt.Errorf("expected single related endpoint, got %v", related)
		}
		serviceName := related[0].ServiceName
		var lDoc lifeDoc
		selSubordinate := D{{"service", serviceName}, {"principal", unitName}}
		if err := ru.st.units.Find(selSubordinate).One(&lDoc); err == mgo.ErrNotFound {
			service, err := ru.st.Service(serviceName)
			if err != nil {
				return err
			}
			_, subOps, err := service.addUnitOps(unitName, true)
			if err != nil {
				return err
			}
			ops = append(ops, subOps...)
		} else if err != nil {
			return err
		} else if lDoc.Life != Alive {
			return ErrCannotEnterScopeYet
		} else {
			existingSubName = lDoc.Id
			ops = append(ops, txn.Op{
				C:      ru.st.units.Name,
				Id:     existingSubName,
				Assert: isAliveDoc,
			})
		}
	}

	// Run the complete transaction, or figure out why we can't.
	if err := ru.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
		return err
	}
	if count, err := ru.st.relationScopes.FindId(relationKey).Count(); err != nil {
		return fmt.Errorf("cannot examine scope for %s: %v", desc, err)
	} else if count != 0 {
		// The scope document exists, so we're already in scope; the txn was
		// aborted by one of the DocMissing checks.
		return nil
	}

	// If there's no scope document, the abort should be a consequence of
	// one of the isAliveDoc checks: find out which. (Note that there is no
	// need for additional checks if we're trying to create a subordinate
	// unit: this could fail due to the subordinate service's not being Alive,
	// but this case will always be caught by the check for the relation's
	// life (because a relation cannot be Alive if its services are not).)
	if alive, err := isAlive(ru.st.units, ru.unit.doc.Name); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}
	if alive, err := isAlive(ru.st.relations, relationKey); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}

	// Maybe a subordinate used to exist, but is no longer alive. If that is
	// case, we will be unable to enter scope until that unit is gone.
	if existingSubName != "" {
		if alive, err := isAlive(ru.st.units, existingSubName); err != nil {
			return err
		} else if !alive {
			return ErrCannotEnterScopeYet
		}
	}

	// Apparently, all our assertions should have passed, but the txn was
	// aborted: something is badly wrong.
	return fmt.Errorf("cannot enter scope for %s: inconsistent state", desc)
}

// LeaveScope signals that the unit has left its scope in the relation.
// After the unit has left its relation scope, it is no longer a member
// of the relation; if the relation is dying when its last member unit
// leaves, it is removed immediately. It is not an error to leave a scope
// that the unit is not, or never was, a member of.
func (ru *RelationUnit) LeaveScope() error {
	key, err := ru.key(ru.unit.Name())
	if err != nil {
		return err
	}
	// The logic below is involved because we remove a dying relation
	// with the last unit that leaves a scope in it. It handles three
	// possible cases:
	//
	// 1. Relation is alive: just leave the scope.
	//
	// 2. Relation is dying, and other units remain: just leave the scope.
	//
	// 3. Relation is dying, and this is the last unit: leave the scope
	//    and remove the relation.
	//
	// In each of those cases, proper assertions are done to guarantee
	// that the condition observed is still valid when the transaction is
	// applied. If an abort happens, it observes the new condition and
	// retries. In theory, a worst case will try at most all of the
	// conditions once, because units cannot join a scope once its relation
	// is dying.
	//
	// Keep in mind that in the first iteration of the loop it's possible
	// to have a Dying relation with a smaller-than-real unit count, because
	// EnsureDying changes the Life attribute in memory (units could join
	// before the database is actually changed).
	desc := fmt.Sprintf("unit %q in relation %q", ru.unit, ru.relation)
	for attempt := 0; attempt < 3; attempt++ {
		count, err := ru.st.relationScopes.FindId(key).Count()
		if err != nil {
			return fmt.Errorf("cannot examine scope for %s: %v", desc, err)
		} else if count == 0 {
			return nil
		}
		ops := []txn.Op{{
			C:      ru.st.relationScopes.Name,
			Id:     key,
			Assert: txn.DocExists,
			Remove: true,
		}}
		if ru.relation.doc.Life == Alive {
			ops = append(ops, txn.Op{
				C:      ru.st.relations.Name,
				Id:     ru.relation.doc.Key,
				Assert: D{{"life", Alive}},
				Update: D{{"$inc", D{{"unitcount", -1}}}},
			})
		} else if ru.relation.doc.UnitCount > 1 {
			ops = append(ops, txn.Op{
				C:      ru.st.relations.Name,
				Id:     ru.relation.doc.Key,
				Assert: D{{"unitcount", D{{"$gt", 1}}}},
				Update: D{{"$inc", D{{"unitcount", -1}}}},
			})
		} else {
			relOps, err := ru.relation.removeOps(modeLeaveScope, ru.unit.doc.Service)
			if err != nil {
				return err
			}
			ops = append(ops, relOps...)
		}
		if err = ru.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
			if err != nil {
				return fmt.Errorf("cannot leave scope for %s: %v", desc, err)
			}
			return err
		}
		if err := ru.relation.Refresh(); IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return fmt.Errorf("cannot leave scope for %s: inconsistent state", desc)
}

// WatchScope returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (ru *RelationUnit) WatchScope() *RelationScopeWatcher {
	role := ru.endpoint.RelationRole.counterpartRole()
	scope := ru.scope + "#" + string(role)
	return newRelationScopeWatcher(ru.st, scope, ru.unit.Name())
}

// Settings returns a Settings which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*Settings, error) {
	key, err := ru.key(ru.unit.Name())
	if err != nil {
		return nil, err
	}
	return readSettings(ru.st, key)
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's service is not part of the
// relation, or the settings are invalid; but mere non-existence of the
// unit is not grounds for an error, because the unit settings are
// guaranteed to persist for the lifetime of the relation, regardless
// of the lifetime of the unit.
func (ru *RelationUnit) ReadSettings(uname string) (m map[string]interface{}, err error) {
	defer trivial.ErrorContextf(&err, "cannot read settings for unit %q in relation %q", uname, ru.relation)
	if !IsUnitName(uname) {
		return nil, fmt.Errorf("%q is not a valid unit name", uname)
	}
	key, err := ru.key(uname)
	if err != nil {
		return nil, err
	}
	node, err := readSettings(ru.st, key)
	if err != nil {
		return nil, err
	}
	return node.Map(), nil
}

// key returns a string, based on the relation and the supplied unit name,
// which is used as a key for that unit within this relation in the settings,
// presence, and relationScopes collections.
func (ru *RelationUnit) key(uname string) (string, error) {
	uparts := strings.Split(uname, "/")
	sname := uparts[0]
	ep, err := ru.relation.Endpoint(sname)
	if err != nil {
		return "", err
	}
	parts := []string{ru.scope, string(ep.RelationRole), uname}
	return strings.Join(parts, "#"), nil
}

// relationScopeDoc represents a unit which is in a relation scope.
// The relation, container, role, and unit are all encoded in the key.
type relationScopeDoc struct {
	Key string `bson:"_id"`
}

func (d *relationScopeDoc) unitName() string {
	parts := strings.Split(d.Key, "#")
	return parts[len(parts)-1]
}
