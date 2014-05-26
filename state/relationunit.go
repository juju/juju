// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/names"
)

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
var ErrCannotEnterScope = stderrors.New("cannot enter scope: unit or relation is not alive")

// ErrCannotEnterScopeYet indicates that a relation unit failed to enter its
// scope due to a required and pre-existing subordinate unit that is not Alive.
// Once that subordinate has been removed, a new one can be created.
var ErrCannotEnterScopeYet = stderrors.New("cannot enter scope yet: non-alive subordinate unit has not been removed")

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
func (ru *RelationUnit) EnterScope(settings map[string]interface{}) error {
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
	// * TODO(fwereade): check unit status == params.StatusStarted (this
	//   breaks a bunch of tests in a boring but noisy-to-fix way, and is
	//   being saved for a followup).
	unitName, relationKey := ru.unit.doc.Name, ru.relation.doc.Key
	ops := []txn.Op{{
		C:      ru.st.units.Name,
		Id:     unitName,
		Assert: isAliveDoc,
	}, {
		C:      ru.st.relations.Name,
		Id:     relationKey,
		Assert: isAliveDoc,
		Update: bson.D{{"$inc", bson.D{{"unitcount", 1}}}},
	}}

	// * Create the unit settings in this relation, if they do not already
	//   exist; or completely overwrite them if they do. This must happen
	//   before we create the scope doc, because the existence of a scope doc
	//   is considered to be a guarantee of the existence of a settings doc.
	settingsChanged := func() (bool, error) { return false, nil }
	if count, err := ru.st.settings.FindId(ruKey).Count(); err != nil {
		return err
	} else if count == 0 {
		ops = append(ops, createSettingsOp(ru.st, ruKey, settings))
	} else {
		var rop txn.Op
		rop, settingsChanged, err = replaceSettingsOp(ru.st, ruKey, settings)
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
		Insert: relationScopeDoc{Key: ruKey},
	})

	// * If the unit should have a subordinate, and does not, create it.
	var existingSubName string
	if subOps, subName, err := ru.subordinateOps(); err != nil {
		return err
	} else {
		existingSubName = subName
		ops = append(ops, subOps...)
	}

	// Now run the complete transaction, or figure out why we can't.
	if err := ru.st.runTransaction(ops); err != txn.ErrAborted {
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

	// Maybe a subordinate used to exist, but is no longer alive. If that is
	// case, we will be unable to enter scope until that unit is gone.
	if existingSubName != "" {
		if alive, err := isAlive(ru.st.units, existingSubName); err != nil {
			return err
		} else if !alive {
			return ErrCannotEnterScopeYet
		}
	}

	// It's possible that there was a pre-existing settings doc whose version
	// has changed under our feet, preventing us from clearing it properly; if
	// that is the case, something is seriously wrong (nobody else should be
	// touching that doc under our feet) and we should bail out.
	prefix := fmt.Sprintf("cannot enter scope for unit %q in relation %q: ", ru.unit, ru.relation)
	if changed, err := settingsChanged(); err != nil {
		return err
	} else if changed {
		return fmt.Errorf(prefix + "concurrent settings change detected")
	}

	// Apparently, all our assertions should have passed, but the txn was
	// aborted: something is really seriously wrong.
	return fmt.Errorf(prefix + "inconsistent state in EnterScope")
}

// subordinateOps returns any txn operations necessary to ensure sane
// subordinate state when entering scope. If a required subordinate unit
// exists and is Alive, its name will be returned as well; if one exists
// but is not Alive, ErrCannotEnterScopeYet is returned.
func (ru *RelationUnit) subordinateOps() ([]txn.Op, string, error) {
	if !ru.unit.IsPrincipal() || ru.endpoint.Scope != charm.ScopeContainer {
		return nil, "", nil
	}
	related, err := ru.relation.RelatedEndpoints(ru.endpoint.ServiceName)
	if err != nil {
		return nil, "", err
	}
	if len(related) != 1 {
		return nil, "", fmt.Errorf("expected single related endpoint, got %v", related)
	}
	serviceName, unitName := related[0].ServiceName, ru.unit.doc.Name
	selSubordinate := bson.D{{"service", serviceName}, {"principal", unitName}}
	var lDoc lifeDoc
	if err := ru.st.units.Find(selSubordinate).One(&lDoc); err == mgo.ErrNotFound {
		service, err := ru.st.Service(serviceName)
		if err != nil {
			return nil, "", err
		}
		_, ops, err := service.addUnitOps(unitName, nil)
		return ops, "", err
	} else if err != nil {
		return nil, "", err
	} else if lDoc.Life != Alive {
		return nil, "", ErrCannotEnterScopeYet
	}
	return []txn.Op{{
		C:      ru.st.units.Name,
		Id:     lDoc.Id,
		Assert: isAliveDoc,
	}}, lDoc.Id, nil
}

// PrepareLeaveScope causes the unit to be reported as departed by watchers,
// but does not *actually* leave the scope, to avoid triggering relation
// cleanup.
func (ru *RelationUnit) PrepareLeaveScope() error {
	key, err := ru.key(ru.unit.Name())
	if err != nil {
		return err
	}
	if count, err := ru.st.relationScopes.FindId(key).Count(); err != nil {
		return err
	} else if count == 0 {
		return nil
	}
	ops := []txn.Op{{
		C:      ru.st.relationScopes.Name,
		Id:     key,
		Update: bson.D{{"$set", bson.D{{"departing", true}}}},
	}}
	return ru.st.runTransaction(ops)
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
	// Destroy changes the Life attribute in memory (units could join before
	// the database is actually changed).
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
				Assert: bson.D{{"life", Alive}},
				Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
			})
		} else if ru.relation.doc.UnitCount > 1 {
			ops = append(ops, txn.Op{
				C:      ru.st.relations.Name,
				Id:     ru.relation.doc.Key,
				Assert: bson.D{{"unitcount", bson.D{{"$gt", 1}}}},
				Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
			})
		} else {
			relOps, err := ru.relation.removeOps("", ru.unit)
			if err != nil {
				return err
			}
			ops = append(ops, relOps...)
		}
		if err = ru.st.runTransaction(ops); err != txn.ErrAborted {
			if err != nil {
				return fmt.Errorf("cannot leave scope for %s: %v", desc, err)
			}
			return err
		}
		if err := ru.relation.Refresh(); errors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	return fmt.Errorf("cannot leave scope for %s: inconsistent state", desc)
}

// InScope returns whether the relation unit has entered scope and not left it.
func (ru *RelationUnit) InScope() (bool, error) {
	return ru.inScope(nil)
}

// Joined returns whether the relation unit has entered scope and neither left
// it nor prepared to leave it.
func (ru *RelationUnit) Joined() (bool, error) {
	return ru.inScope(bson.D{{"departing", bson.D{{"$ne", true}}}})
}

// inScope returns whether a scope document exists satisfying the supplied
// selector.
func (ru *RelationUnit) inScope(sel bson.D) (bool, error) {
	key, err := ru.key(ru.unit.Name())
	if err != nil {
		return false, err
	}
	sel = append(sel, bson.D{{"_id", key}}...)
	count, err := ru.st.relationScopes.Find(sel).Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// WatchScope returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (ru *RelationUnit) WatchScope() *RelationScopeWatcher {
	role := counterpartRole(ru.endpoint.Role)
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
	defer errors.Maskf(&err, "cannot read settings for unit %q in relation %q", uname, ru.relation)
	if !names.IsUnit(uname) {
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
	parts := []string{ru.scope, string(ep.Role), uname}
	return strings.Join(parts, "#"), nil
}

// relationScopeDoc represents a unit which is in a relation scope.
// The relation, container, role, and unit are all encoded in the key.
type relationScopeDoc struct {
	Key       string `bson:"_id"`
	Departing bool
}

func (d *relationScopeDoc) unitName() string {
	return unitNameFromScopeKey(d.Key)
}

func unitNameFromScopeKey(key string) string {
	parts := strings.Split(key, "#")
	return parts[len(parts)-1]
}
