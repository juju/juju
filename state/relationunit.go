// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"
	"github.com/kr/pretty"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	stateerrors "github.com/juju/juju/state/errors"
)

var rulogger = logger.Child("relationunit")

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st          *State
	relation    *Relation
	unitName    string
	isPrincipal bool
	endpoint    relation.Endpoint
	scope       string

	// isLocalUnit is true for relation units representing
	// the local side of a cross model relation, or for
	// any 2 units in a non cross model relation.
	isLocalUnit bool
}

// Relation returns the relation associated with the unit.
func (ru *RelationUnit) Relation() *Relation {
	return ru.relation
}

// Endpoint returns the relation endpoint that defines the unit's
// participation in the relation.
func (ru *RelationUnit) Endpoint() relation.Endpoint {
	return ru.endpoint
}

// UnitName returns the name of the unit in the relation.
func (ru *RelationUnit) UnitName() string {
	return ru.unitName
}

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
func (ru *RelationUnit) EnterScope(
	settings map[string]interface{},
) error {
	db, dbCloser := ru.st.newDB()
	defer dbCloser()
	relationScopes, rsCloser := db.GetCollection(relationScopesC)
	defer rsCloser()
	ruKey := ru.key()
	relationDocID := ru.relation.doc.DocID

	var settingsChanged func() (bool, error)
	var existingSubName string
	prefix := fmt.Sprintf("unit %q in relation %q: ", ru.unitName, ru.relation)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Before retrying the transaction, check the following
		// assertions:
		if attempt > 0 {
			if count, err := relationScopes.FindId(ruKey).Count(); err != nil {
				return nil, errors.Trace(err)
			} else if count != 0 {
				// The scope document exists, so we're actually already in scope.
				return nil, nil
			}

			// The relation or unit might no longer be Alive. (Note that there is no
			// need for additional checks if we're trying to create a subordinate
			// unit: this could fail due to the subordinate applications not being Alive,
			// but this case will always be caught by the check for the relation's
			// life (because a relation cannot be Alive if its applications are not).)
			relations, rCloser := db.GetCollection(relationsC)
			defer rCloser()
			if alive, err := isAliveWithSession(relations, relationDocID); err != nil {
				return nil, errors.Trace(err)
			} else if !alive {
				return nil, errors.Annotatef(stateerrors.ErrCannotEnterScope, "%srelation is no longer alive", prefix)
			}
			if ru.isLocalUnit {
				units, uCloser := db.GetCollection(unitsC)
				defer uCloser()
				if alive, err := isAliveWithSession(units, ru.unitName); err != nil {
					return nil, errors.Trace(err)
				} else if !alive {
					return nil, errors.Annotatef(stateerrors.ErrCannotEnterScope, "%sunit is no longer alive", prefix)

				}

				// Maybe a subordinate used to exist, but is no longer alive. If that is
				// case, we will be unable to enter scope until that unit is gone.
				if existingSubName != "" {
					if alive, err := isAliveWithSession(units, existingSubName); err != nil {
						return nil, errors.Trace(err)
					} else if !alive {
						return nil, errors.Annotatef(stateerrors.ErrCannotEnterScopeYet, "%ssubordinate %v is no longer alive", prefix, existingSubName)
					}
				}
			}

			// It's possible that there was a pre-existing settings doc whose version
			// has changed under our feet, preventing us from clearing it properly; if
			// that is the case, something is seriously wrong (nobody else should be
			// touching that doc under our feet) and we should bail out.
			if changed, err := settingsChanged(); err != nil {
				return nil, errors.Trace(err)
			} else if changed {
				return nil, fmt.Errorf("%s concurrent settings change detected", prefix)
			}
		}

		// Verify that the unit is not already in scope, and exit without error
		// if it is.
		if count, err := relationScopes.FindId(ruKey).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count != 0 {
			return nil, nil
		}

		// Collect the operations necessary to enter scope, as follows:
		// * Check unit and relation state, and incref the relation.
		// * TODO(fwereade): check unit status == params.StatusActive (this
		//   breaks a bunch of tests in a boring but noisy-to-fix way, and is
		//   being saved for a followup).
		var ops []txn.Op
		if ru.isLocalUnit {
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
		settingsColl, sCloser := db.GetCollection(settingsC)
		defer sCloser()
		if count, err := settingsColl.FindId(ruKey).Count(); err != nil {
			return nil, errors.Trace(err)
		} else if count == 0 {
			ops = append(ops, createSettingsOp(settingsC, ruKey, settings))
			settingsChanged = func() (bool, error) { return false, nil }
		} else {
			var rop txn.Op
			rop, settingsChanged, err = replaceSettingsOp(ru.st.db(), settingsC, ruKey, settings)
			if err != nil {
				return nil, errors.Trace(err)
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
		if subOps, subName, err := ru.subordinateOps(); err != nil {
			return nil, errors.Trace(err)
		} else {
			existingSubName = subName
			ops = append(ops, subOps...)
		}
		return ops, nil
	}

	// Now run the complete transaction.
	return ru.st.db().Run(buildTxn)
}

// CounterpartApplications returns the slice of application names that are the counterpart of this unit.
// (So for Peer relations, app is returned, for a Provider the apps on Requirer side is returned
func (ru *RelationUnit) CounterpartApplications() []string {
	counterApps := set.NewStrings()
	counterRole := relation.CounterpartRole(ru.endpoint.Role)
	for _, ep := range ru.relation.Endpoints() {
		if ep.Role == counterRole {
			counterApps.Add(ep.ApplicationName)
		}
	}
	return counterApps.SortedValues()
}

// counterpartApplicationSettingsKeys is the database keys of related applications.
func (ru *RelationUnit) counterpartApplicationSettingsKeys() []string {
	counterpartApps := ru.CounterpartApplications()
	out := make([]string, len(counterpartApps))
	for i, appName := range counterpartApps {
		out[i] = relationApplicationSettingsKey(ru.relation.Id(), appName)
	}
	return out
}

// subordinateOps returns any txn operations necessary to ensure sane
// subordinate state when entering scope. If a required subordinate unit
// exists and is Alive, its name will be returned as well; if one exists
// but is not Alive, ErrCannotEnterScopeYet is returned.
func (ru *RelationUnit) subordinateOps() ([]txn.Op, string, error) {
	units, closer := ru.st.db().GetCollection(unitsC)
	defer closer()

	if !ru.isPrincipal || ru.endpoint.Scope != charm.ScopeContainer {
		return nil, "", nil
	}
	related, err := ru.relation.RelatedEndpoints(ru.endpoint.ApplicationName)
	if err != nil {
		return nil, "", err
	}
	if len(related) != 1 {
		return nil, "", errors.Errorf("expected single related endpoint, got %v", related)
	}
	// Find the machine ID that the principal unit is deployed on, and use
	// that for the subordinate. It is worthwhile noting that if the unit is
	// in a CAAS model, there are no machines.
	principal, err := ru.st.Unit(ru.unitName)
	if err != nil {
		return nil, "", errors.Annotate(err, "unable to load principal unit")
	}
	var principalMachineID string
	if principal.ShouldBeAssigned() {
		// We don't care just now if the machine isn't assigned, as CAAS models
		// will return that error. For IAAS models, the machine *should* always
		// be assigned before it is able to enter scope.
		// We don't check the error here now because it'll cause *huge* test
		// fallout as many tests don't follow reality, particularly when
		// relations are being tested.
		principalMachineID, _ = principal.AssignedMachineId()
	}

	applicationname, unitName := related[0].ApplicationName, ru.unitName
	selSubordinate := bson.D{{"application", applicationname}, {"principal", unitName}}
	var lDoc lifeDoc
	if err := units.Find(selSubordinate).One(&lDoc); err == mgo.ErrNotFound {
		application, err := ru.st.Application(applicationname)
		if err != nil {
			return nil, "", err
		}
		_, ops, err := application.addUnitOps(unitName, AddUnitParams{
			machineID: principalMachineID,
		}, nil)
		return ops, "", err
	} else if err != nil {
		return nil, "", err
	} else if lDoc.Life != Alive {
		return nil, "", stateerrors.ErrCannotEnterScopeYet
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
	relationScopes, closer := ru.st.db().GetCollection(relationScopesC)
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
	return ru.st.db().RunTransaction(ops)
}

// LeaveScopeOperation returns a model operation that will allow relation to leave scope.
func (ru *RelationUnit) LeaveScopeOperation(force bool) *LeaveScopeOperation {
	return &LeaveScopeOperation{
		ru: &RelationUnit{
			st:          ru.st,
			relation:    ru.relation,
			unitName:    ru.unitName,
			isPrincipal: ru.isPrincipal,
			endpoint:    ru.endpoint,
			scope:       ru.scope,
			isLocalUnit: ru.isLocalUnit,
		},
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// LeaveScopeOperation is a model operation for relation to leave scope.
type LeaveScopeOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// ru holds the unit relation that wants to leave scope.
	ru *RelationUnit
}

// Build is part of the ModelOperation interface.
func (op *LeaveScopeOperation) Build(attempt int) ([]txn.Op, error) {
	rulogger.Tracef(context.TODO(), "%v attempt %d to leave scope", op.Description(), attempt+1)
	if attempt > 0 {
		if err := op.ru.relation.Refresh(); errors.Is(err, errors.NotFound) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.internalLeaveScope(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			rulogger.Warningf(context.TODO(), "forcing %v to leave scope despite error %v", op.Description(), err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

func (op *LeaveScopeOperation) Description() string {
	return fmt.Sprintf("unit %q in relation %q", op.ru.unitName, op.ru.relation)
}

// Done is part of the ModelOperation interface.
func (op *LeaveScopeOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "%v cannot leave scope", op.Description())
		}
		op.AddError(errors.Errorf("%v tried to forcefully leave scope but proceeded despite encountering ERROR %v", op.Description(), err))
	}
	return nil
}

// LeaveScopeWithForce in addition to doing what LeaveScope() does,
// when force is passed in as 'true', forces relation unit to leave scope,
// ignoring errors.
func (ru *RelationUnit) LeaveScopeWithForce(force bool, maxWait time.Duration) ([]error, error) {
	op := ru.LeaveScopeOperation(force)
	op.MaxWait = maxWait
	err := ru.st.ApplyOperation(op)
	return op.Errors, err
}

// LeaveScope signals that the unit has left its scope in the relation.
// After the unit has left its relation scope, it is no longer a member
// of the relation; if the relation is dying when its last member unit
// leaves, it is removed immediately. It is not an error to leave a scope
// that the unit is not, or never was, a member of.
func (ru *RelationUnit) LeaveScope() error {
	errs, err := ru.LeaveScopeWithForce(false, time.Duration(0))
	if len(errs) != 0 {
		rulogger.Warningf(context.TODO(), "operational errors leaving scope for unit %q in relation %q: %v", ru.unitName, ru.relation, errs)
	}
	return err
}

// leaveScopeForcedOps is an internal method used by other state objects when they just want
// to get database operations that are involved in leaving scope without
// the actual immediate act of leaving scope.
func (ru *RelationUnit) leaveScopeForcedOps(existingOperation *ForcedOperation) ([]txn.Op, error) {
	// It does not matter that we are say false to force here- we'll overwrite the whole ForcedOperation.
	leaveScopeOperation := ru.LeaveScopeOperation(false)
	leaveScopeOperation.ForcedOperation = *existingOperation
	return leaveScopeOperation.internalLeaveScope()
}

// When 'force' is set, this call will return needed operations
// and will accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be applied.
func (op *LeaveScopeOperation) internalLeaveScope() ([]txn.Op, error) {
	relationScopes, closer := op.ru.st.db().GetCollection(relationScopesC)
	defer closer()

	key := op.ru.key()
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
	rulogger.Debugf(context.TODO(), "%v leaving scope: unit count %d, life %v", op.Description(), op.ru.relation.doc.UnitCount, op.ru.relation.doc.Life)
	count, err := relationScopes.FindId(key).Count()
	if op.FatalError(errors.Annotatef(err, "cannot examine scope for %s", op.Description())) {
		return nil, err
	} else if count == 0 {
		return nil, jujutxn.ErrNoOperations
	}
	ops := []txn.Op{{
		C:      relationScopesC,
		Id:     key,
		Assert: txn.DocExists,
		Remove: true,
	}}
	if op.ru.relation.doc.Life == Alive {
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     op.ru.relation.doc.DocID,
			Assert: bson.D{{"life", Alive}},
			Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
		})
	} else if op.ru.relation.doc.UnitCount > 1 {
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     op.ru.relation.doc.DocID,
			Assert: bson.D{{"unitcount", bson.D{{"$gt", 1}}}},
			Update: bson.D{{"$inc", bson.D{{"unitcount", -1}}}},
		})
	} else {
		// When 'force' is set, this call will return needed operations
		// and accumulate all operational errors encountered in the operation.
		// If the 'force' is not set, any error will be fatal and no operations will be returned.
		relOps, err := op.ru.relation.removeOps("", op.ru.unitName, &op.ForcedOperation)
		if op.FatalError(err) {
			return nil, err
		}
		ops = append(ops, relOps...)
	}
	if rulogger.IsLevelEnabled(corelogger.TRACE) {
		rulogger.Tracef(context.TODO(), "leave scope ops for %s: %s", op.Description(), pretty.Sprint(ops))
	}
	return ops, nil
}

// Valid returns whether this RelationUnit is one that can actually
// exist in the relation. For container-scoped relations, RUs can be
// created for subordinate units whose principal unit isn't a member
// of the relation. There are too many places that rely on being able
// to construct a nonsensical RU to query InScope or Joined, so we
// allow them to be constructed but they will always return false for
// Valid.
// TODO(babbageclunk): unpick the reliance on creating invalid RUs.
func (ru *RelationUnit) Valid() (bool, error) {
	if ru.endpoint.Scope != charm.ScopeContainer || ru.isPrincipal {
		return true, nil
	}
	// A subordinate container-scoped relation unit is valid if:
	// the other end of the relation is also a subordinate charm
	// or its principal unit is also a member of the relation.
	appName, err := names.UnitApplication(ru.unitName)
	if err != nil {
		return false, errors.Trace(err)
	}
	var otherAppName string
	for _, ep := range ru.relation.Endpoints() {
		if ep.ApplicationName != appName {
			otherAppName = ep.ApplicationName
		}
	}
	if otherAppName == "" {
		return false, errors.Errorf("couldn't find other endpoint")
	}
	otherApp, err := ru.st.Application(otherAppName)
	if err != nil {
		return false, errors.Trace(err)
	}
	if !otherApp.IsPrincipal() {
		return true, nil
	}

	unit, err := ru.st.Unit(ru.unitName)
	if err != nil {
		return false, errors.Trace(err)
	}
	// No need to check the flag here - we know we're subordinate.
	pName, _ := unit.PrincipalName()
	principalAppName, err := names.UnitApplication(pName)
	if err != nil {
		return false, errors.Trace(err)
	}
	// If the other application is a principal, only allow it if it's in the relation.
	_, err = ru.relation.Endpoint(principalAppName)
	if errors.Is(err, errors.NotFound) {
		return false, nil
	} else if err != nil {
		return false, errors.Trace(err)
	}
	return true, nil
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
	relationScopes, closer := ru.st.db().GetCollection(relationScopesC)
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
	role := relation.CounterpartRole(ru.endpoint.Role)
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
	s, err := readSettings(ru.st.db(), settingsC, ru.key())
	if err != nil {
		return nil, errors.Annotatef(err, "unit %q", ru.unitName)
	}
	return s, nil
}

// ReadSettings returns a map holding the settings of the unit with the
// supplied name within this relation. An error will be returned if the
// relation no longer exists, or if the unit's application is not part of the
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
	node, err := readSettings(ru.st.db(), settingsC, key)
	if err != nil {
		return nil, errors.Annotatef(err, "unit %q", uname)
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
	Departing bool   `bson:"departing"`
}

func (d *relationScopeDoc) unitName() string {
	return unitNameFromScopeKey(d.Key)
}

func unitNameFromScopeKey(key string) string {
	parts := strings.Split(key, "#")
	return parts[len(parts)-1]
}

// unpackScopeKey returns the scope, role and unitname from the
// relation scope key.
func unpackScopeKey(key string) (string, string, string, error) {
	if _, localID, ok := splitDocID(key); ok {
		key = localID
	}
	parts := strings.Split(key, "#")
	if len(parts) < 4 {
		return "", "", "", errors.Errorf("%q has too few parts to be a relation scope key", key)
	}
	unitName := parts[len(parts)-1]
	role := parts[len(parts)-2]
	scope := strings.Join(parts[:len(parts)-2], "#")
	return scope, role, unitName, nil
}
