// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/network"
)

// RelationUnit holds information about a single unit in a relation, and
// allows clients to conveniently access unit-specific functionality.
type RelationUnit struct {
	st          *State
	relation    *Relation
	unitName    string
	isPrincipal bool
	endpoint    Endpoint
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
	settingsChanged := func() (bool, error) { return false, nil }
	settingsColl, closer := db.GetCollection(settingsC)
	defer closer()
	if count, err := settingsColl.FindId(ruKey).Count(); err != nil {
		return err
	} else if count == 0 {
		ops = append(ops, createSettingsOp(settingsC, ruKey, settings))
	} else {
		var rop txn.Op
		rop, settingsChanged, err = replaceSettingsOp(ru.st.db(), settingsC, ruKey, settings)
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
	if err := ru.st.db().RunTransaction(ops); err != txn.ErrAborted {
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
	// unit: this could fail due to the subordinate applications not being Alive,
	// but this case will always be caught by the check for the relation's
	// life (because a relation cannot be Alive if its applications are not).)
	relations, closer := db.GetCollection(relationsC)
	defer closer()
	if alive, err := isAliveWithSession(relations, relationDocID); err != nil {
		return err
	} else if !alive {
		return ErrCannotEnterScope
	}
	if ru.isLocalUnit {
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
		_, ops, err := application.addUnitOps(unitName, AddUnitParams{}, nil)
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
	if attempt > 0 {
		if err := op.ru.relation.Refresh(); errors.IsNotFound(err) {
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
			logger.Warningf("forcing %v to leave scope despite error %v", op.Description(), err)
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
		logger.Warningf("operational errors leaving scope for unit %q in relation %q: %v", ru.unitName, ru.relation, errs)
	}
	return err
}

// leaveScopeForcedOps is an internal method used by other state objects when they just want
// to get database operations that are involved in leaving scop without
// the actual immeiate act of leaving scope.
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
	logger.Debugf("%v leaving scope", op.Description())
	count, err := relationScopes.FindId(key).Count()
	if err != nil {
		err := fmt.Errorf("cannot examine scope for %s: %v", op.Description(), err)
		if !op.Force {
			return nil, err
		}
		op.AddError(err)
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
		if err != nil {
			if !op.Force {
				return nil, err
			}
			op.AddError(err)
		}
		ops = append(ops, relOps...)
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
	if errors.IsNotFound(err) {
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

// PreferredAddressRetryArgs returns the retry strategy for getting a unit's preferred address.
// Override for testing to use a different clock.
var PreferredAddressRetryArgs = func() retry.CallArgs {
	return retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       3 * time.Second,
		MaxDuration: 30 * time.Second,
	}
}

// NetworksForRelation returns the ingress and egress addresses for a relation and unit.
// The ingress addresses depend on if the relation is cross model and whether the
// relation endpoint is bound to a space.
func NetworksForRelation(
	binding string, unit *Unit, rel *Relation, defaultEgress []string, pollPublic bool,
) (boundSpace string, ingress []string, egress []string, _ error) {
	st := unit.st

	relEgress := NewRelationEgressNetworks(st)
	egressSubnets, err := relEgress.Networks(rel.Tag().Id())
	if err != nil && !errors.IsNotFound(err) {
		return "", nil, nil, errors.Trace(err)
	} else if err == nil {
		egress = egressSubnets.CIDRS()
	} else {
		egress = defaultEgress
	}

	boundSpace, err = unit.GetSpaceForBinding(binding)
	if err != nil && !errors.IsNotValid(err) {
		return "", nil, nil, errors.Trace(err)
	}

	fetchAddr := func(fetcher func() (network.Address, error)) (network.Address, error) {
		var address network.Address
		retryArg := PreferredAddressRetryArgs()
		retryArg.Func = func() error {
			var err error
			address, err = fetcher()
			return err
		}
		retryArg.IsFatalError = func(err error) bool {
			return !network.IsNoAddressError(err)
		}
		return address, retry.Call(retryArg)
	}

	fallbackIngressToPrivateAddr := func() error {
		// TODO(ycliuhw): lp-1830252 retry here once this is fixed.
		address, err := unit.PrivateAddress()
		if err != nil {
			logger.Warningf(
				"no private address for unit %q in relation %q",
				unit.Name(), rel)
		}
		if address.Value != "" {
			ingress = append(ingress, address.Value)
		}
		return nil
	}

	// If the endpoint for this relation is not bound to a space, or
	// is bound to the default space, we need to look up the ingress
	// address info which is aware of cross model relations.
	if boundSpace == corenetwork.DefaultSpaceName || err != nil {
		_, crossmodel, err := rel.RemoteApplication()
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
		if crossmodel && (unit.ShouldBeAssigned() || pollPublic) {
			address, err := fetchAddr(unit.PublicAddress)
			if err != nil {
				logger.Warningf(
					"no public address for unit %q in cross model relation %q, will use private address",
					unit.Name(), rel,
				)
			} else if address.Value != "" {
				ingress = append(ingress, address.Value)
			}
			if len(ingress) == 0 {
				if err := fallbackIngressToPrivateAddr(); err != nil {
					return "", nil, nil, errors.Trace(err)
				}
			}
		}
	}
	if len(ingress) == 0 {
		if unit.ShouldBeAssigned() {
			// We don't yet have an ingress address, so pick one from the space to
			// which the endpoint is bound.
			machineID, err := unit.AssignedMachineId()
			if err != nil {
				return "", nil, nil, errors.Trace(err)
			}
			machine, err := st.Machine(machineID)
			if err != nil {
				return "", nil, nil, errors.Trace(err)
			}
			networkInfos := machine.GetNetworkInfoForSpaces(set.NewStrings(boundSpace))
			// The binding address information based on link layer devices.
			for _, nwInfo := range networkInfos[boundSpace].NetworkInfos {
				for _, addr := range nwInfo.Addresses {
					ingress = append(ingress, addr.Address)
				}
			}
		} else {
			// Be be consistent with IAAS behaviour above, we'll return all addresses.
			addr, err := unit.AllAddresses()
			if err != nil {
				logger.Warningf(
					"no service address for unit %q in relation %q",
					unit.Name(), rel)
			} else {
				network.SortAddresses(addr)
				for _, a := range addr {
					ingress = append(ingress, a.Value)
				}
			}
		}
	}

	// If no egress subnets defined, We default to the ingress address.
	if len(egress) == 0 && len(ingress) > 0 {
		egress, err = network.FormatAsCIDR([]string{ingress[0]})
		if err != nil {
			return "", nil, nil, errors.Trace(err)
		}
	}
	return boundSpace, ingress, egress, nil
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
