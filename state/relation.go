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
// are currently in scope, it will be removed immediately. It is an error to
// destroy a relation more than once.
func (r *Relation) Destroy() (err error) {
	defer trivial.ErrorContextf(&err, "cannot destroy relation %q", r)
	// In *theory*, there is no upper bound to the number of attempts we could
	// legitimately make here; it is not inconsistent for a relation to flip
	// between 0 and 1 units in scope, indefinitely, and perfectly timed to
	// abort every transaction we attempt below. In practice, this situation
	// is very unlikely; if as many as 5 attempts have failed, we can be almost
	// certain that corrupt state is indicated.
	for attempt := 0; attempt < 5; attempt++ {
		if r.doc.Life != Alive {
			return nil
		}
		if r.doc.UnitCount == 0 {
			ops := r.removeOps(D{{"unitcount", 0}})
			if err := r.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
				return err
			}
		} else {
			ops := []txn.Op{{
				C:      r.st.relations.Name,
				Id:     r.doc.Key,
				Assert: D{{"life", Alive}, {"unitcount", D{{"$gt", 0}}}},
				Update: D{{"$set", D{{"life", Dying}}}},
			}}
			if err := r.st.runner.Run(ops, "", nil); err == nil {
				r.doc.Life = Dying
				return nil
			} else if err != txn.ErrAborted {
				return err
			}
		}
		if err := r.Refresh(); IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return fmt.Errorf("units being added during relation removal; shouldn't happen, please contact juju-dev@lists.ubuntu.com")
}

// removeOps returns the operations that must occur when a relation is removed.
// The only assertions made in the returned list are those supplied, which will
// be applied to the relation document.
func (r *Relation) removeOps(asserts D) []txn.Op {
	cDoc := &cleanupDoc{
		Id:     bson.NewObjectId(),
		Kind:   "settings",
		Prefix: fmt.Sprintf("r#%d#", r.Id()),
	}
	ops := []txn.Op{{
		C:      r.st.cleanups.Name,
		Id:     cDoc.Id,
		Insert: cDoc,
	}, {
		C:      r.st.relations.Name,
		Id:     r.doc.Key,
		Assert: asserts,
		Remove: true,
	}}
	for _, ep := range r.doc.Endpoints {
		ops = append(ops, txn.Op{
			C:      r.st.services.Name,
			Id:     ep.ServiceName,
			Update: D{{"$inc", D{{"relationcount", -1}}}},
		})
	}
	return ops
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

// PrivateAddress returns the private address of the unit and whether it is valid.
func (ru *RelationUnit) PrivateAddress() (string, bool) {
	return ru.unit.PrivateAddress()
}

// ErrCannotEnterScope indicates that a relation unit failed to enter its scope
// due to either the unit or the relation not being Alive.
var ErrCannotEnterScope = errors.New("cannot enter scope: unit or relation is not alive")

// EnterScope ensures that the unit has entered its scope in the relation.
// When the unit has already entered its relation scope, EnterScope will report
// success but make no changes to state.
//
// Otherwise, assuming both the relation and the unit are alive, it will enter
// scope and create or overwrite the unit's settings in the relation according
// to the supplied map.
//
// If the unit is a principal and the relation has container scope, EnterScope
// will also create the required subordinate unit, if it does not already exist;
// this is because there's no point having a principal in scope if there is no
// corresponding subordinate to join it.
//
// Once a unit has entered a scope, it stays in scope without further
// intervention; the relation will not be able to become Dead until all units
// have departed its scopes.
func (ru *RelationUnit) EnterScope(enterSettings map[string]interface{}) error {
	// Verify that the unit is not already in scope, and abort without error
	// if it is.
	ruKey, err := ru.key(ru.unit.Name())
	if err != nil {
		return err
	}
	if count, err := ru.st.relationScopes.FindId(ruKey).Count(); err != nil {
		return err
	} else if count != 0 {
		return nil
	}

	// Collect the operations necessary to enter scope, as follows:
	// * Check unit and relation state, and incref the relation.
	unitName, relationKey := ru.unit.doc.Name, ru.relation.doc.Key
	ops := []txn.Op{{
		C:      ru.st.units.Name,
		Id:     unitName,
		Assert: isAliveDoc,
	}, {
		C:      ru.st.relations.Name,
		Id:     relationKey,
		Assert: isAliveDoc,
		Update: D{{"$inc", D{{"unitcount", 1}}}},
	}}

	// * Create the unit settings in this relation, if they do not already
	//   exist; or completely overwrite them if they do. This must happen
	//   before we create the scope doc, because the existence of a scope doc
	//   is considered to be a guarantee of the existence of a settings doc.
	if enterSettings == nil {
		enterSettings = map[string]interface{}{}
	}
	settingsChanged := func() (bool, error) { return false, nil }
	if count, err := ru.st.settings.FindId(ruKey).Count(); err != nil {
		return err
	} else if count == 0 {
		ops = append(ops, createSettingsOp(ru.st, ruKey, enterSettings))
	} else {
		var rop txn.Op
		rop, settingsChanged, err = replaceSettingsOp(ru.st, ruKey, enterSettings)
		if err != nil {
			return err
		}
		ops = append(ops, rop)
	}

	// * Create the scope doc.
	ops = append(ops, txn.Op{
		C:      ru.st.relationScopes.Name,
		Id:     ruKey,
		Assert: txn.DocMissing,
		Insert: relationScopeDoc{ruKey},
	})

	// * If the unit should have a subordinate, and does not, create it.
	if subOps, err := ru.createSubordinateOps(); err != nil {
		return err
	} else {
		ops = append(ops, subOps...)
	}

	// Now run the complete transaction, or figure out why we can't.
	if err := ru.st.runner.Run(ops, "", nil); err != txn.ErrAborted {
		return err
	}
	if count, err := ru.st.relationScopes.FindId(ruKey).Count(); err != nil {
		return err
	} else if count != 0 {
		// The scope document exists, so we're actually already in scope.
		return nil
	}
	// The relation or unit might no longer be Alive. (Note that there is no
	// need for additional checks if we're trying to create a subordinate
	// unit: this could fail due to the subordinate service's not being Alive,
	// but this case will always be caught by the check for the relation's
	// life (because a relation cannot be Alive if its services are not).)
	if alive, err := isAlive(ru.st.units, unitName); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}
	if alive, err := isAlive(ru.st.relations, relationKey); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}
	// It's possible that there was a pre-existing settings doc whose version
	// has changed under our feet, preventing us from clearing it properly; if
	// that is the case, something is seriously wrong (nobody else should be
	// touching that doc under our feet) and we should bail out.
	t := fmt.Sprintf("cannot enter scope for unit %q in relation %q", ru.unit, ru.relation) + ": %v"
	if changed, err := settingsChanged(); err != nil {
		return err
	} else if changed {
		return fmt.Errorf(t, "concurrent settings change detected")
	}
	// Apparently, all our assertions should have passed, but the txn was
	// aborted: something is really seriously wrong.
	return fmt.Errorf(t, "inconsistent state")
}

// createSubordinateOps returns any txn operations necessary to ensure sane
// subordinate state when entering scope.
func (ru *RelationUnit) createSubordinateOps() ([]txn.Op, error) {
	if !ru.unit.IsPrincipal() || ru.endpoint.RelationScope != charm.ScopeContainer {
		return nil, nil
	}
	related, err := ru.relation.RelatedEndpoints(ru.endpoint.ServiceName)
	if err != nil {
		return nil, err
	} else if len(related) != 1 {
		return nil, fmt.Errorf("expected single related endpoint, got %v", related)
	}
	serviceName, unitName := related[0].ServiceName, ru.unit.doc.Name
	selSubordinate := D{{"service", serviceName}, {"principal", unitName}}
	if n, err := ru.st.units.Find(selSubordinate).Count(); err != nil {
		return nil, err
	} else if n == 0 {
		service, err := ru.st.Service(serviceName)
		if err != nil {
			return nil, err
		}
		_, subOps, err := service.addUnitOps(unitName, true)
		return subOps, err
	} else {
		// TODO BUG: if a subordinate exists, but is Dying, it won't be
		// recreated later; and if it is Alive we need to assert that it
		// still is when the txn is flushed.
	}
	return nil, nil
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
			asserts := D{{"life", Dying}, {"unitcount", 1}}
			ops = append(ops, ru.relation.removeOps(asserts)...)
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
