// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st            *State
	relation      *Relation
	unitName      string
	isPrincipal   bool
	checkUnitLife bool
	endpoint      Endpoint
	scope         string
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
	db, closer := ru.st.newDB()
	defer closer()
	relationScopes, closer := db.GetCollection(relationScopesC)
	defer closer()

	// Verify that the unit is not already in scope, and abort without error
	// if it is.
	ruKey := ru.key()
	if count, err := relationScopes.FindId(ruKey).Count(); err != nil {
		return err
	} else if count != 0 {
		return nil
	}

	// Collect the operations necessary to enter scope, as follows:
	// * Check unit and relation state, and incref the relation.
	// * TODO(fwereade): check unit status == params.StatusActive (this
	//   breaks a bunch of tests in a boring but noisy-to-fix way, and is
	//   being saved for a followup).
	relationDocID := ru.relation.doc.DocID
	var ops []txn.Op
	if ru.checkUnitLife {
		ops = append(ops, txn.Op{
			C:      unitsC,
			Id:     ru.unitName,
			Assert: isAliveDoc,
		})
	}
	ops = append(ops, txn.Op{
		C:      relationsC,
		Id:     relationDocID,
		Assert: isAliveDoc,
		Update: bson.D{{"$inc", bson.D{{"unitcount", 1}}}},
	})

	// * Create the unit settings in this relation, if they do not already
	//   exist; or completely overwrite them if they do. This must happen
	//   before we create the scope doc, because the existence of a scope doc
	//   is considered to be a guarantee of the existence of a settings doc.
	settingsChanged := func() (bool, error) { return false, nil }
	settingsColl, closer := db.GetCollection(settingsC)
	defer closer()
	if count, err := settingsColl.FindId(ruKey).Count(); err != nil {
		return err
	} else if count == 0 {
		ops = append(ops, createSettingsOp(settingsC, ruKey, settings))
	} else {
		var rop txn.Op
		rop, settingsChanged, err = replaceSettingsOp(ru.st, settingsC, ruKey, settings)
		if err != nil {
			return err
		}
		ops = append(ops, rop)
	}

	// * Create the scope doc.
	ops = append(ops, txn.Op{
		C:      relationScopesC,
		Id:     ruKey,
		Assert: txn.DocMissing,
		Insert: relationScopeDoc{
			Key: ruKey,
		},
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
	if count, err := relationScopes.FindId(ruKey).Count(); err != nil {
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
	relations, closer := db.GetCollection(relationsC)
	defer closer()
	if alive, err := isAliveWithSession(relations, relationDocID); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}
	if ru.checkUnitLife {
		units, closer := db.GetCollection(unitsC)
		defer closer()
		if alive, err := isAliveWithSession(units, ru.unitName); err != nil {
			return err
		} else if !alive {
			return ErrCannotEnterScope
		}

		// Maybe a subordinate used to exist, but is no longer alive. If that is
		// case, we will be unable to enter scope until that unit is gone.
		if existingSubName != "" {
			if alive, err := isAliveWithSession(units, existingSubName); err != nil {
				return err
			} else if !alive {
				return ErrCannotEnterScopeYet
			}
		}
	}

	// It's possible that there was a pre-existing settings doc whose version
	// has changed under our feet, preventing us from clearing it properly; if
	// that is the case, something is seriously wrong (nobody else should be
	// touching that doc under our feet) and we should bail out.
	prefix := fmt.Sprintf("cannot enter scope for unit %q in relation %q: ", ru.unitName, ru.relation)
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
	units, closer := ru.st.getCollection(unitsC)
	defer closer()

	if !ru.isPrincipal || ru.endpoint.Scope != charm.ScopeContainer {
		return nil, "", nil
	}
	related, err := ru.relation.RelatedEndpoints(ru.endpoint.ApplicationName)
	if err != nil {
		return nil, "", err
	}
	if len(related) != 1 {
		return nil, "", fmt.Errorf("expected single related endpoint, got %v", related)
	}
	applicationname, unitName := related[0].ApplicationName, ru.unitName
	selSubordinate := bson.D{{"application", applicationname}, {"principal", unitName}}
	var lDoc lifeDoc
	if err := units.Find(selSubordinate).One(&lDoc); err == mgo.ErrNotFound {
		application, err := ru.st.Application(applicationname)
		if err != nil {
			return nil, "", err
		}
		_, ops, err := application.addUnitOps(unitName, nil)
		return ops, "", err
	} else if err != nil {
		return nil, "", err
	} else if lDoc.Life != Alive {
		return nil, "", ErrCannotEnterScopeYet
	}
	return []txn.Op{{
		C:      unitsC,
		Id:     lDoc.Id,
		Assert: isAliveDoc,
	}}, lDoc.Id, nil
}

// PrepareLeaveScope causes the unit to be reported as departed by watchers,
// but does not *actually* leave the scope, to avoid triggering relation
// cleanup.
func (ru *RelationUnit) PrepareLeaveScope() error {
	relationScopes, closer := ru.st.getCollection(relationScopesC)
	defer closer()

	key := ru.key()
	if count, err := relationScopes.FindId(key).Count(); err != nil {
		return err
	} else if count == 0 {
		return nil
	}
	ops := []txn.Op{{
		C:      relationScopesC,
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
	relationScopes, closer := ru.st.getCollection(relationScopesC)
	defer closer()

	key := ru.key()
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
	desc := fmt.Sprintf("unit %q in relation %q", ru.unitName, ru.relation)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := ru.relation.Refresh(); errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			} else if err != nil {
				return nil, err
			}
		}
		count, err := relationScopes.FindId(key).Count()
		if err != nil {
			return nil, fmt.Errorf("cannot examine scope for %s: %v", desc, err)
		} else if count == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		ops := []txn.Op{{
			C:      relationScopesC,
			Id:     key,
			Assert: txn.DocExists,
			Remove: true,
		}}
		if ru.relation.doc.Life == Alive {
			ops = append(ops, txn.Op{
				C:      relationsC,
				Id:     ru.relation.doc.DocID,
				Assert: bson.D{{"life", Alive}},
				Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
			})
		} else if ru.relation.doc.UnitCount > 1 {
			ops = append(ops, txn.Op{
				C:      relationsC,
				Id:     ru.relation.doc.DocID,
				Assert: bson.D{{"unitcount", bson.D{{"$gt", 1}}}},
				Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
			})
		} else {
			relOps, err := ru.relation.removeOps("", ru.unitName)
			if err != nil {
				return nil, err
			}
			ops = append(ops, relOps...)
		}
		return ops, nil
	}
	if err := ru.st.run(buildTxn); err != nil {
		return errors.Annotatef(err, "cannot leave scope for %s", desc)
	}
	return nil
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
	relationScopes, closer := ru.st.getCollection(relationScopesC)
	defer closer()

	sel = append(sel, bson.D{{"_id", ru.key()}}...)
	count, err := relationScopes.Find(sel).Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// WatchScope returns a watcher which notifies of counterpart units
// entering and leaving the unit's scope.
func (ru *RelationUnit) WatchScope() *RelationScopeWatcher {
	role := counterpartRole(ru.endpoint.Role)
	return watchRelationScope(ru.st, ru.scope, role, ru.unitName)
}

func watchRelationScope(
	st *State, scope string, role charm.RelationRole, ignore string,
) *RelationScopeWatcher {
	scope = scope + "#" + string(role)
	return newRelationScopeWatcher(st, scope, ignore)
}

// Settings returns a Settings which allows access to the unit's settings
// within the relation.
func (ru *RelationUnit) Settings() (*Settings, error) {
	return readSettings(ru.st, settingsC, ru.key())
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's service is not part of the
// relation, or the settings are invalid; but mere non-existence of the
// unit is not grounds for an error, because the unit settings are
// guaranteed to persist for the lifetime of the relation, regardless
// of the lifetime of the unit.
func (ru *RelationUnit) ReadSettings(uname string) (m map[string]interface{}, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot read settings for unit %q in relation %q", uname, ru.relation)
	if !names.IsValidUnit(uname) {
		return nil, fmt.Errorf("%q is not a valid unit name", uname)
	}
	key, err := ru.unitKey(uname)
	if err != nil {
		return nil, err
	}
	node, err := readSettings(ru.st, settingsC, key)
	if err != nil {
		return nil, err
	}
	return node.Map(), nil
}

// unitKey returns a string, based on the relation and the supplied unit name,
// which is used as a key for that unit within this relation in the settings,
// presence, and relationScopes collections.
func (ru *RelationUnit) unitKey(uname string) (string, error) {
	uparts := strings.Split(uname, "/")
	sname := uparts[0]
	ep, err := ru.relation.Endpoint(sname)
	if err != nil {
		return "", err
	}
	return ru._key(string(ep.Role), uname), nil
}

// key returns a string, based on the relation and the current unit name,
// which is used as a key for that unit within this relation in the settings,
// presence, and relationScopes collections.
func (ru *RelationUnit) key() string {
	return ru._key(string(ru.endpoint.Role), ru.unitName)
}

func (ru *RelationUnit) _key(role, unitname string) string {
	parts := []string{ru.scope, role, unitname}
	return strings.Join(parts, "#")
}

// relationScopeDoc represents a unit which is in a relation scope.
// The relation, container, role, and unit are all encoded in the key.
type relationScopeDoc struct {
	DocID     string `bson:"_id"`
	Key       string `bson:"key"`
	ModelUUID string `bson:"model-uuid"`
	Departing bool
}

func (d *relationScopeDoc) unitName() string {
	return unitNameFromScopeKey(d.Key)
}

func unitNameFromScopeKey(key string) string {
	parts := strings.Split(key, "#")
	return parts[len(parts)-1]
}
