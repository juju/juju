// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	stateerrors "github.com/juju/juju/state/errors"
)

// relationKey returns a string describing the relation defined by
// endpoints, for use in various contexts (including error messages).
func relationKey(endpoints []Endpoint) string {
	eps := epSlice{}
	for _, ep := range endpoints {
		eps = append(eps, ep)
	}
	sort.Sort(eps)
	endpointNames := []string{}
	for _, ep := range eps {
		endpointNames = append(endpointNames, ep.String())
	}
	return strings.Join(endpointNames, " ")
}

// relationDoc is the internal representation of a Relation in MongoDB.
// Note the correspondence with RelationInfo in core/multiwatcher.
type relationDoc struct {
	DocID           string     `bson:"_id"`
	Key             string     `bson:"key"`
	ModelUUID       string     `bson:"model-uuid"`
	Id              int        `bson:"id"`
	Endpoints       []Endpoint `bson:"endpoints"`
	Life            Life       `bson:"life"`
	UnitCount       int        `bson:"unitcount"`
	Suspended       bool       `bson:"suspended"`
	SuspendedReason string     `bson:"suspended-reason"`
}

// Relation represents a relation between one or two application endpoints.
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

// Tag returns a name identifying the relation.
func (r *Relation) Tag() names.Tag {
	return names.NewRelationTag(r.doc.Key)
}

// UnitCount is the number of units still in relation scope.
func (r *Relation) UnitCount() int {
	return r.doc.UnitCount
}

// Suspended returns true if the relation is suspended.
func (r *Relation) Suspended() bool {
	return r.doc.Suspended
}

// SuspendedReason returns the reason why the relation is suspended.
func (r *Relation) SuspendedReason() string {
	return r.doc.SuspendedReason
}

// Refresh refreshes the contents of the relation from the underlying
// state. It returns an error that satisfies errors.IsNotFound if the
// relation has been removed.
func (r *Relation) Refresh() error {
	relations, closer, err := r.st.db().GetCollection(relationsC)
	if err != nil {
		return errors.Trace(err)
	}
	defer closer()

	doc := relationDoc{}
	err = relations.FindId(r.doc.DocID).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("relation %v", r)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot refresh relation %v", r)
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

// Status returns the relation's current status data.
func (r *Relation) Status() (status.StatusInfo, error) {
	rStatus, err := getStatus(r.st.db(), r.globalScope(), "relation")
	if err != nil {
		return rStatus, err
	}
	return rStatus, nil
}

// SetStatus sets the status of the relation.
func (r *Relation) SetStatus(statusInfo status.StatusInfo) error {
	currentStatus, err := r.Status()
	if err != nil {
		return errors.Trace(err)
	}

	if currentStatus.Status != statusInfo.Status {
		validTransition := true
		switch statusInfo.Status {
		case status.Broken:
		case status.Suspending:
			validTransition = currentStatus.Status != status.Broken && currentStatus.Status != status.Suspended
		case status.Joining:
			validTransition = currentStatus.Status != status.Broken && currentStatus.Status != status.Joined
		case status.Joined, status.Suspended:
			validTransition = currentStatus.Status != status.Broken
		case status.Error:
			if statusInfo.Message == "" {
				return errors.Errorf("cannot set status %q without info", statusInfo.Status)
			}
		default:
			return errors.NewNotValid(nil, fmt.Sprintf("cannot set invalid status %q", statusInfo.Status))
		}
		if !validTransition {
			return errors.NewNotValid(nil, fmt.Sprintf(
				"cannot set status %q when relation has status %q", statusInfo.Status, currentStatus.Status))
		}
	}
	return setStatus(r.st.db(), setStatusParams{
		badge:     "relation",
		globalKey: r.globalScope(),
		status:    statusInfo.Status,
		message:   statusInfo.Message,
		rawData:   statusInfo.Data,
		updated:   timeOrNow(statusInfo.Since, r.st.clock()),
	})
}

// SetSuspended sets whether the relation is suspended.
func (r *Relation) SetSuspended(suspended bool, suspendedReason string) error {
	if r.doc.Suspended == suspended {
		return nil
	}
	if !suspended && suspendedReason != "" {
		return errors.New("cannot set suspended reason if not suspended")
	}

	var buildTxn jujutxn.TransactionSource = func(attempt int) ([]txn.Op, error) {

		if attempt > 1 {
			if err := r.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		return []txn.Op{{
			C:      relationsC,
			Id:     r.doc.DocID,
			Assert: bson.D{{"suspended", r.doc.Suspended}},
			Update: bson.D{
				{"$set", bson.D{{"suspended", suspended}}},
				{"$set", bson.D{{"suspended-reason", suspendedReason}}},
			},
		}}, nil
	}

	err := r.st.db().Run(buildTxn)
	if err == nil {
		r.doc.Suspended = suspended
	}
	return err
}

// DestroyOperation returns a model operation that will allow relation to leave scope.
func (r *Relation) DestroyOperation(force bool) *DestroyRelationOperation {
	return &DestroyRelationOperation{
		r:               &Relation{r.st, r.doc},
		ForcedOperation: ForcedOperation{Force: force},
	}
}

// DestroyRelationOperation is a model operation destroy relation.
type DestroyRelationOperation struct {
	// ForcedOperation stores needed information to force this operation.
	ForcedOperation

	// r holds the relation to destroy.
	r *Relation
}

// Build is part of the ModelOperation interface.
func (op *DestroyRelationOperation) Build(attempt int) ([]txn.Op, error) {
	if attempt > 0 {
		if err := op.r.Refresh(); errors.IsNotFound(err) {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, err
		}
	}
	// When 'force' is set on the operation, this call will return needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	switch ops, err := op.internalDestroy(); err {
	case errRefresh:
	case errAlreadyDying:
		return nil, jujutxn.ErrNoOperations
	case nil:
		return ops, nil
	default:
		if op.Force {
			logger.Warningf("force destroying relation %v despite error %v", op.r, err)
			return ops, nil
		}
		return nil, err
	}
	return nil, jujutxn.ErrNoOperations
}

// Done is part of the ModelOperation interface.
func (op *DestroyRelationOperation) Done(err error) error {
	if err != nil {
		if !op.Force {
			return errors.Annotatef(err, "cannot destroy relation %q", op.r)
		}
		op.AddError(errors.Errorf("forcefully destroying relation %v proceeded despite encountering ERROR %v", op.r, err))
	}
	return nil
}

// DestroyWithForce may force the destruction of the relation.
// In addition, this function also returns all non-fatal operational errors
// encountered.
func (r *Relation) DestroyWithForce(force bool, maxWait time.Duration) ([]error, error) {
	op := r.DestroyOperation(force)
	op.MaxWait = maxWait
	err := r.st.ApplyOperation(op)
	return op.Errors, err
}

// Destroy ensures that the relation will be removed at some point; if no units
// are currently in scope, it will be removed immediately.
func (r *Relation) Destroy() error {
	errs, err := r.DestroyWithForce(false, time.Duration(0))
	if len(errs) != 0 {
		logger.Warningf("operational errors removing relation %v: %v", r.Id(), errs)
	}
	return err
}

// destroyCrossModelRelationUnitsOps returns the operations necessary for the units
// of the specified remote application to leave scope.
func destroyCrossModelRelationUnitsOps(op *ForcedOperation, remoteApp *RemoteApplication, rel *Relation, onlyForTerminated bool) ([]txn.Op, error) {
	if onlyForTerminated {
		statusInfo, err := remoteApp.Status()
		if op.FatalError(err) && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if err != nil || statusInfo.Status != status.Terminated {
			return nil, jujutxn.ErrNoOperations
		}
	}
	logger.Debugf("forcing cleanup of units for %v", remoteApp.Name())
	remoteUnits, err := rel.AllRemoteUnits(remoteApp.Name())
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	} else if err != nil {
		logger.Warningf("could not get remote units for %q: %v", remoteApp.Name(), err)
	}

	var ops []txn.Op
	logger.Debugf("got %v relation units to clean", len(remoteUnits))
	failRemoteUnits := false
	for _, ru := range remoteUnits {
		leaveScopeOps, err := ru.leaveScopeForcedOps(op)
		if err != nil && err != jujutxn.ErrNoOperations {
			op.AddError(err)
			failRemoteUnits = true
		}
		ops = append(ops, leaveScopeOps...)
	}
	if !op.Force && failRemoteUnits {
		return nil, errors.Trace(op.LastError())
	}
	return ops, nil
}

// When 'force' is set, this call will construct and apply needed operations
// as well as accumulate all operational errors encountered.
// If the 'force' is not set, any error will be fatal and no operations will be applied.
func (op *DestroyRelationOperation) internalDestroy() (ops []txn.Op, err error) {
	if len(op.r.doc.Endpoints) == 1 && op.r.doc.Endpoints[0].Role == charm.RolePeer {
		return nil, errors.Errorf("is a peer relation")
	}
	defer func() {
		if err == nil {
			// This is a white lie; the document might actually be removed.
			op.r.doc.Life = Dying
		}
	}()
	rel := &Relation{op.r.st, op.r.doc}

	remoteApp, isCrossModel, err := op.r.RemoteApplication()
	if op.FatalError(err) {
		return nil, errors.Trace(err)
	} else if isCrossModel {
		// If the status of the consumed app is terminated, we will never
		// get an orderly exit of units from scope so force the issue.
		// TODO(wallyworld) - this should be in a force cleanup job after giving things a change to complete normally.
		destroyOps, err := destroyCrossModelRelationUnitsOps(&op.ForcedOperation, remoteApp, op.r, !op.Force)
		if err != nil && err != jujutxn.ErrNoOperations {
			return nil, errors.Trace(err)
		}
		ops = append(ops, destroyOps...)
		// On the offering side, if this is the last relation to the consumer proxy, we may need to remove it also,
		// but only if the unit count is already 0. This is just a backstop to allow force to fully clean up; normally
		// unit count is not 0 so this doesn't get run.
		// NB: the decision to use this to figure out if it's the last relation
		// uses a query on the relations collection rather than the relationcount,
		// so a corrupt count can never prematurely remove a remote app proxy
		// that other relations still reference.
		if remoteApp.IsConsumerProxy() && (op.Force || rel.doc.UnitCount == 0) {
			refCount, err := countRelationsForApplication(op.r.st, remoteApp.Name())
			if err != nil {
				return nil, errors.Trace(err)
			}
			if refCount <= 1 {
				logger.Debugf("removing cross model consumer proxy and last relation %d: %v", op.r.doc.Id, op.r.doc.Key)
				// The remote app is removed by removeAppOps below; skip the remote
				// endpoint here so removeRemoteEndpointOps does not also try
				// to decrement (or double-remove) it.
				removeRelOps, err := rel.removeOps(remoteApp.Name(), "", &op.ForcedOperation)
				if err != nil && err != jujutxn.ErrNoOperations {
					return nil, errors.Trace(err)
				}
				ops = append(ops, removeRelOps...)
				// Assert on txn-revno rather than the possibly stale
				// relationcount so a concurrent change (e.g. a new
				// relation being added) aborts the remote app removal.
				removeAppOps, err := remoteApp.removeOps(bson.D{{"txn-revno", remoteApp.doc.TxnRevno}})
				if err != nil {
					return nil, errors.Trace(err)
				}
				return append(ops, removeAppOps...), nil
			}
		}
	}

	// In this context, aborted transactions indicate that the number of units
	// in scope have changed between 0 and not-0. The chances of 5 successive
	// attempts each hitting this change -- which is itself an unlikely one --
	// are considered to be extremely small.
	// When 'force' is set, this call will return  needed operations
	// and accumulate all operational errors encountered in the operation.
	// If the 'force' is not set, any error will be fatal and no operations will be returned.
	destroyOps, _, err := rel.destroyOps("", &op.ForcedOperation)
	if err == errAlreadyDying {
		return nil, jujutxn.ErrNoOperations
	} else if op.FatalError(err) {
		return nil, err
	}

	ops = append(ops, destroyOps...)
	sortRemovalOpsLast(ops)
	return ops, nil
}

// destroyOps returns the operations necessary to destroy the relation, and
// whether those operations will lead to the relation's removal. These
// operations may include changes to the relation's applications; however, if
// ignoreApplication is not empty, no operations modifying that application will
// be generated.
// When 'force' is set, this call will return both operations to remove this
// relation as well as all operational errors encountered.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (r *Relation) destroyOps(ignoreApplication string, op *ForcedOperation) (ops []txn.Op, isRemove bool, err error) {
	if r.doc.Life != Alive {
		if !op.Force {
			return nil, false, errAlreadyDying
		}
	}
	scopes, closer, err := r.st.db().GetCollection(relationScopesC)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	defer closer()
	sel := bson.M{"_id": bson.M{
		"$regex": fmt.Sprintf("^%s#", r.st.docID(r.globalScope())),
	}}
	unitsInScope, err := scopes.Find(sel).Count()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	if unitsInScope != r.doc.UnitCount {
		if op.Force {
			logger.Warningf("ignoring unit count mismatch on relation %v: expected %d units in scope but got %d", r, r.doc.UnitCount, unitsInScope)
		} else {
			return nil, false, errors.Errorf("unit count mismatch on relation %v: expected %d units in scope but got %d", r, r.doc.UnitCount, unitsInScope)
		}
	}

	if r.doc.UnitCount == 0 || unitsInScope == 0 {
		// When 'force' is set, this call will return both needed operations
		// as well as all operational errors encountered.
		// If the 'force' is not set, any error will be fatal and no operations will be returned.
		removeOps, err := r.removeOps(ignoreApplication, "", op)
		return forceRelationRemoveOps(r, removeOps, err, op)
	}

	lifeAssert := isAliveDoc
	if op.Force {
		// Since we are force destroying, life assert should be current relation's life.
		lifeAssert = bson.D{{"life", r.doc.Life}}
		deadline := r.st.stateClock.Now().Add(op.MaxWait)
		ops = append(ops, newCleanupAtOp(deadline, cleanupForceDestroyedRelation, strconv.Itoa(r.Id())))
	}

	ops = append(ops, txn.Op{
		C:      relationsC,
		Id:     r.doc.DocID,
		Assert: append(bson.D{{"unitcount", bson.D{{"$gt", 0}}}}, lifeAssert...),
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
	})
	return ops, false, nil
}

// forceTeardownCleanupOps returns ops queueing an asynchronous force
// teardown of the relation. When the relation is Alive it is marked
// Dying in the same transaction. The numeric ID prevents a stale request
// from fencing a recreated relation that has the same key.
func forceTeardownCleanupOps(r *Relation) []txn.Op {
	var ops []txn.Op
	if r.doc.Life == Alive {
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     r.doc.DocID,
			Assert: bson.D{{"life", Alive}, {"id", r.Id()}},
			Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
		})
	}
	return append(ops, newCleanupOp(
		cleanupForceDestroyedRelation,
		strconv.Itoa(r.Id()),
	))
}

// forceRelationRemoveOps decides the destroy ops for a relation whose
// removal ops failed to build. Without force the error is fatal.
// With force, the removal ops are discarded (an error means they are
// an inconsistent set anyway) and an asynchronous force teardown is
// queued instead so the relation is always cleaned up; the teardown
// is idempotent, so a re-run after partial progress is safe.
func forceRelationRemoveOps(r *Relation, removeOps []txn.Op, err error, op *ForcedOperation) ([]txn.Op, bool, error) {
	if err == nil {
		return removeOps, true, nil
	}
	if !op.Force {
		return nil, false, err
	}
	logger.Warningf("ignoring error (%v) while constructing relation %v destroy operations since force is used", err, r)
	return forceTeardownCleanupOps(r), true, nil
}

// removeOps returns the operations necessary to remove the relation. If
// ignoreApplication is not empty, no operations affecting that application will be
// included; if departingUnitName is non-empty, this implies that the
// relation's applications may be Dying and otherwise unreferenced, and may thus
// require removal themselves.
// When 'force' is set, this call will return needed operations
// and accumulate all operational errors encountered in the operation.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (r *Relation) removeOps(ignoreApplication string, departingUnitName string, op *ForcedOperation) ([]txn.Op, error) {
	relOp := txn.Op{
		C:      relationsC,
		Id:     r.doc.DocID,
		Remove: true,
	}
	if op.Force {
		// There can be a mismatch between the unit count recorded on a relation and the actual number
		// of units in scope if a multi-controller cross model relation is removed and the controllers
		// can't talk to coordinate cleanup.
		relOp.Assert = bson.D{{"life", r.doc.Life}, {"unitcount", r.doc.UnitCount}}
	} else {
		if departingUnitName != "" {
			relOp.Assert = bson.D{{"life", Dying}, {"unitcount", 1}}
		} else {
			relOp.Assert = bson.D{{"life", Alive}, {"unitcount", 0}}
		}
	}
	ops := []txn.Op{relOp}
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == ignoreApplication {
			continue
		}
		app, err := applicationByName(r.st, ep.ApplicationName)
		if err != nil {
			op.AddError(err)
		} else {
			if app.IsRemote() {
				epOps, err := r.removeRemoteEndpointOps(ep, departingUnitName != "")
				if err != nil {
					op.AddError(err)
				}
				ops = append(ops, epOps...)
			} else {
				// When 'force' is set, this call will return both needed operations
				// as well as all operational errors encountered.
				// If the 'force' is not set, any error will be fatal and no operations will be returned.
				epOps, err := r.removeLocalEndpointOps(ep, departingUnitName, op)
				if err != nil {
					op.AddError(err)
				}
				ops = append(ops, epOps...)
			}
		}
	}
	removeChildRecordOps, err := removeRelationChildRecordOps(r.st, r.doc.Id, r.doc.Key)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, removeChildRecordOps...)
	// This cleanup does not need to be forced. Settings and scopes are
	// removed asynchronously by prefix. The cleanups are independent and
	// safe in any order: the relation document is already gone by the
	// time either runs, so no ordering dependency (and no reliance on the
	// cleanups worker's document iteration order) is introduced. A
	// relation state cleanup is also queued: unit agents may have
	// persisted relation state referencing this relation, which would
	// otherwise wedge their hooks until a restart.
	cleanupOps := []txn.Op{
		newCleanupOp(cleanupRelationScopes, fmt.Sprintf("r#%d#", r.Id())),
		newCleanupOp(cleanupRelationSettings, fmt.Sprintf("r#%d#", r.Id())),
		newCleanupOp(cleanupRelationState, strconv.Itoa(r.Id())),
	}
	return append(ops, cleanupOps...), nil
}

// removeRelationChildRecordOps returns the operations that remove the
// child records of a relation: status, relation networks, offer
// connection, remote entity, and scoped secret permissions.
func removeRelationChildRecordOps(st *State, relID int, relationKey string) ([]txn.Op, error) {
	ops := []txn.Op{removeStatusOp(st, relationGlobalScope(relID))}
	ops = append(ops, removeRelationNetworksOps(st, relationKey)...)
	tag := names.NewRelationTag(relationKey)
	ops = append(ops, st.RemoteEntities().removeRemoteEntityOps(tag)...)
	ops = append(ops, removeOfferConnectionsForRelationOps(st, relID)...)
	secretPermissionsOps, err := st.removeScopedSecretPermissionOps(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return append(ops, secretPermissionsOps...), nil
}

// When 'force' is set, this call will return both needed operations
// as well as all operational errors encountered.
// If the 'force' is not set, any error will be fatal and no operations will be returned.
func (r *Relation) removeLocalEndpointOps(ep Endpoint, departingUnitName string, op *ForcedOperation) ([]txn.Op, error) {
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	departingUnitApplicationMatchesEndpoint := func() bool {
		s, err := names.UnitApplication(departingUnitName)
		return err == nil && s == ep.ApplicationName
	}
	var cleanupOps []txn.Op
	if departingUnitName == "" {
		// We're constructing a destroy operation, either of the relation
		// or one of its applications, and can therefore be assured that both
		// applications are Alive.
		asserts = append(hasRelation, isAliveDoc...)
	} else if departingUnitApplicationMatchesEndpoint() {
		// This application must have at least one unit -- the one that's
		// departing the relation -- so it cannot be ready for removal.
		cannotDieYet := bson.D{{"unitcount", bson.D{{"$gt", 0}}}}
		asserts = append(hasRelation, cannotDieYet...)
	} else {
		// This application may require immediate removal.
		// Check if the application is Dying, and if so, queue up a potential
		// cleanup in case this was the last reference.
		applications, closer, err := r.st.db().GetCollection(applicationsC)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer closer()

		asserts = append(bson.D{}, hasRelation...)
		var appDoc applicationDoc
		if err := applications.FindId(ep.ApplicationName).One(&appDoc); err == nil {
			if appDoc.Life != Alive {
				cleanupOps = append(cleanupOps, newCleanupOp(
					cleanupApplication,
					ep.ApplicationName,
					false, // destroyStorage
					op.Force,
				))
			}
		} else if !op.Force {
			return nil, errors.Trace(err)
		}
	}
	return append([]txn.Op{{
		C:      applicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: asserts,
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, cleanupOps...), nil
}

func (r *Relation) removeRemoteEndpointOps(ep Endpoint, unitDying bool) ([]txn.Op, error) {
	applications, closer, err := r.st.db().GetCollection(remoteApplicationsC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()

	app := &RemoteApplication{st: r.st}
	if err := applications.FindId(ep.ApplicationName).One(&app.doc); err != nil {
		if err == mgo.ErrNotFound {
			// The remote application is already gone; nothing to do.
			return nil, jujutxn.ErrNoOperations
		}
		return nil, err
	}
	// The relation currently being removed (or destroyed) still counts
	// towards the count below; see countRelationsForApplication.
	refCount, err := countRelationsForApplication(r.st, app.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if unitDying && refCount == 1 && (app.doc.Life != Alive || app.doc.IsConsumerProxy) {
		// Remove the proxy together with its final relation. Assert on
		// txn-revno rather than the relationcount so a concurrent change
		// aborts the removal.
		removeOps, err := app.removeOps(bson.D{{"txn-revno", app.doc.TxnRevno}})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return removeOps, nil
	}
	// The stored count may be out of date. An application destroy can
	// remove several relations referencing the same remote application
	// in one transaction, so each removal must decrement the count by
	// exactly one. Pre-existing corruption may have left the count
	// inflated or low (stale), so decide from the actual relations: the
	// recorded value is trusted only when it agrees with the relations
	// that really reference the application (the relation being removed
	// still counts towards both; see countRelationsForApplication).
	var asserts bson.D
	hasRelation := bson.D{{"relationcount", bson.D{{"$gt", 0}}}}
	if app.doc.RelationCount != refCount {
		logger.Warningf("relation count for remote application %q is corrupt: recorded %d, actual relations %d",
			app.Name(), app.doc.RelationCount, refCount)
		if refCount == 1 {
			// This is the final reference, count will be 0. Setting
			// the count instead of decrementing it heals an inflated
			// or low (stale) recorded value.
			return []txn.Op{{
				C:      remoteApplicationsC,
				Id:     r.st.docID(ep.ApplicationName),
				Assert: bson.D{{"txn-revno", app.doc.TxnRevno}},
				Update: bson.D{{"$set", bson.D{{"relationcount", 0}}}},
			}}, nil
		}
		// This removal might not be the final reference. Refuse to
		// make things worse: decrementing could drive a low (stale)
		// count negative and would leave an inflated count inflated,
		// so the relation is removed without touching the count and
		// the recorded value converges as the remaining references
		// are removed (the final reference heals it above).
		// Keep the revision assertion even when leaving the count alone.
		// If a concurrent add caused the mismatch between the two reads,
		// the removal must abort and retry with the updated application.
		return []txn.Op{{
				C:      remoteApplicationsC,
				Id:     app.doc.DocID,
				Assert: bson.D{{"txn-revno", app.doc.TxnRevno}},
			}}, stateerrors.NewRelationCountCorruptError(
				app.Name(), app.doc.RelationCount, refCount)
	}
	if !unitDying {
		// We're constructing a destroy operation, either of the relation
		// or one of its application, and can therefore be assured that the
		// remote application is Alive.
		asserts = append(bson.D{}, hasRelation...)
		asserts = append(asserts, isAliveDoc...)
	} else {
		asserts = append(bson.D{}, hasRelation...)
	}
	return []txn.Op{{
		C:      remoteApplicationsC,
		Id:     r.st.docID(ep.ApplicationName),
		Assert: append(asserts, bson.DocElem{Name: "txn-revno", Value: app.doc.TxnRevno}),
		Update: bson.D{{"$inc", bson.D{{"relationcount", -1}}}},
	}}, nil
}

// countRelationsForApplication returns the number of relations whose endpoints
// reference the named application. The relation currently being removed or
// destroyed still counts towards the result, so callers treat a count of 1
// as the final reference.
func countRelationsForApplication(st *State, appName string) (int, error) {
	relations, closer, err := st.db().GetCollection(relationsC)
	if err != nil {
		return 0, errors.Trace(err)
	}
	defer closer()
	return relations.Find(bson.M{"endpoints.applicationname": appName}).Count()
}

// Id returns the integer internal relation key. This is exposed
// because the unit agent needs to expose a value derived from this
// (as JUJU_RELATION_ID) to allow relation hooks to differentiate
// between relations with different applications.
func (r *Relation) Id() int {
	return r.doc.Id
}

// Endpoint returns the endpoint of the relation for the named application.
// If the application is not part of the relation, an error will be returned.
func (r *Relation) Endpoint(applicationname string) (Endpoint, error) {
	for _, ep := range r.doc.Endpoints {
		if ep.ApplicationName == applicationname {
			return ep, nil
		}
	}
	msg := fmt.Sprintf("application %q is not a member of %q", applicationname, r)
	return Endpoint{}, errors.NewNotFound(nil, msg)
}

// ModelUUID returns the model UUID for the relation.
func (r *Relation) ModelUUID() string {
	return r.doc.ModelUUID
}

// Endpoints returns the endpoints for the relation.
func (r *Relation) Endpoints() []Endpoint {
	return r.doc.Endpoints
}

// RelatedEndpoints returns the endpoints of the relation r with which
// units of the named application will establish relations. If the application
// is not part of the relation r, an error will be returned.
func (r *Relation) RelatedEndpoints(applicationname string) ([]Endpoint, error) {
	local, err := r.Endpoint(applicationname)
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
		return nil, errors.Errorf("no endpoints of %q relate to application %q", r, applicationname)
	}
	return eps, nil
}

// Unit returns a RelationUnit for the supplied unit.
func (r *Relation) Unit(u *Unit) (*RelationUnit, error) {
	const isLocalUnit = true
	return r.unit(u.Name(), u.doc.Principal, u.IsPrincipal(), isLocalUnit)
}

// RemoteUnit returns a RelationUnit for the supplied unit
// of a remote application.
func (r *Relation) RemoteUnit(unitName string) (*RelationUnit, error) {
	// Verify that the unit belongs to a remote application.
	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if _, err := r.st.RemoteApplication(appName); err != nil {
		return nil, errors.Trace(err)
	}
	// Only non-subordinate applications may be offered for remote
	// relation, so all remote units are principals.
	const principal = ""
	const isPrincipal = true
	const isLocalUnit = false
	return r.unit(unitName, principal, isPrincipal, isLocalUnit)
}

// AllRemoteUnits returns all the RelationUnits for the remote
// application units for a given application.
func (r *Relation) AllRemoteUnits(appName string) ([]*RelationUnit, error) {
	// Verify that the unit belongs to a remote application.
	if _, err := r.st.RemoteApplication(appName); err != nil {
		return nil, errors.Trace(err)
	}

	relationScopes, closer, err := r.st.db().GetCollection(relationScopesC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()

	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, err
	}
	scope := r.globalScope()
	parts := []string{"^" + scope, string(ep.Role), appName + "/"}
	ruRegex := strings.Join(parts, "#")

	var docs []relationScopeDoc
	if err := relationScopes.Find(bson.D{{"key", bson.D{{"$regex", ruRegex}}}}).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*RelationUnit, len(docs))
	for i, doc := range docs {
		result[i] = &RelationUnit{
			st:          r.st,
			relation:    r,
			unitName:    doc.unitName(),
			isPrincipal: true,
			isLocalUnit: false,
			endpoint:    ep,
			scope:       scope,
		}
	}
	return result, nil
}

// RemoteApplication returns the remote application if
// this relation is a cross-model relation, and a bool
// indicating if it cross-model or not.
func (r *Relation) RemoteApplication() (*RemoteApplication, bool, error) {
	for _, ep := range r.Endpoints() {
		app, err := r.st.RemoteApplication(ep.ApplicationName)
		if err == nil {
			return app, true, nil
		} else if !errors.IsNotFound(err) {
			return nil, false, errors.Trace(err)
		}
	}
	return nil, false, nil
}

func (r *Relation) unit(
	unitName string,
	principal string,
	isPrincipal bool,
	isLocalUnit bool,
) (*RelationUnit, error) {
	appName, err := names.UnitApplication(unitName)
	if err != nil {
		return nil, err
	}
	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, err
	}
	scope := r.globalScope()
	if ep.Scope == charm.ScopeContainer {
		container := principal
		if container == "" {
			container = unitName
		}
		scope = fmt.Sprintf("%s#%s", scope, container)
	}
	return &RelationUnit{
		st:          r.st,
		relation:    r,
		unitName:    unitName,
		isPrincipal: isPrincipal,
		isLocalUnit: isLocalUnit,
		endpoint:    ep,
		scope:       scope,
	}, nil
}

// globalScope returns the scope prefix for relation scope document keys
// in the global scope.
func (r *Relation) globalScope() string {
	return relationGlobalScope(r.doc.Id)
}

func relationGlobalScope(id int) string {
	return fmt.Sprintf("r#%d", id)
}

// relationSettingsCleanupChange removes the settings doc.
type relationSettingsCleanupChange struct {
	Prefix string
}

// Prepare is part of the Change interface.
func (change relationSettingsCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	settings, closer, err := db.GetCollection(settingsC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	sel := bson.D{{"_id", bson.D{{"$regex", "^" + change.Prefix}}}}
	var docs []struct {
		DocID string `bson:"_id"`
	}
	err = settings.Find(sel).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      settingsC,
			Id:     doc.DocID,
			Remove: true,
		}
	}
	return ops, nil

}

// relationScopesCleanupChange removes the relation scope docs.
type relationScopesCleanupChange struct {
	Prefix string
}

// Prepare is part of the Change interface.
func (change relationScopesCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	scopes, closer, err := db.GetCollection(relationScopesC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	sel := bson.D{{"_id", bson.D{{"$regex", "^" + change.Prefix}}}}
	var docs []struct {
		DocID string `bson:"_id"`
	}
	err = scopes.Find(sel).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		ops[i] = txn.Op{
			C:      relationScopesC,
			Id:     doc.DocID,
			Remove: true,
		}
	}
	return ops, nil
}

// relationStateCleanupChange cleans up stale relation state from unitstates
// documents after a relation is removed. Otherwise, when a relation is removed
// by force (or while the unit agent was down), the persisted relation state
// could still reference it.
type relationStateCleanupChange struct {
	Prefix string
}

// Prepare is part of the Change interface.
func (change relationStateCleanupChange) Prepare(db Database) ([]txn.Op, error) {
	states, closer, err := db.GetCollection(unitStatesC)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closer()
	relState := fmt.Sprintf("relation-state.%s", change.Prefix)
	sel := bson.D{{relState, bson.D{{"$exists", true}}}}
	var docs []struct {
		DocID         string `bson:"_id"`
		TxnRevno      int64  `bson:"txn-revno"`
		RelationState bson.M `bson:"relation-state"`
	}
	err = states.Find(sel).Select(bson.D{{"_id", 1}, {"txn-revno", 1}, {"relation-state", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, ErrChangeComplete
	}

	ops := make([]txn.Op, len(docs))
	for i, doc := range docs {
		// Unsetting the embedded key alone would leave an empty
		// relation-state map behind.
		unset := relState
		if len(doc.RelationState) == 1 {
			unset = "relation-state"
		}
		ops[i] = txn.Op{
			C:      unitStatesC,
			Id:     doc.DocID,
			Assert: bson.D{{"txn-revno", doc.TxnRevno}},
			Update: bson.D{{"$unset", bson.D{{unset, 1}}}},
		}
	}
	return ops, nil
}

func relationApplicationSettingsKey(id int, application string) string {
	return fmt.Sprintf("%s#%s", relationGlobalScope(id), application)
}

// ApplicationSettings returns the application-level settings for the
// specified application in this relation.
func (r *Relation) ApplicationSettings(appName string) (map[string]interface{}, error) {
	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	applicationKey := relationApplicationSettingsKey(r.Id(), ep.ApplicationName)
	s, err := readSettings(r.st.db(), settingsC, applicationKey)
	if err != nil {
		return nil, errors.Annotatef(err, "relation %q application %q", r.String(), appName)
	}
	return s.Map(), nil
}

// UpdateApplicationSettings updates the given application's settings
// in this relation. It requires a current leadership token.
func (r *Relation) UpdateApplicationSettings(appName string, token leadership.Token, updates map[string]interface{}) error {
	modelOp, err := r.UpdateApplicationSettingsOperation(appName, token, updates)
	if err != nil {
		return errors.Trace(err)
	}

	err = r.st.ApplyOperation(modelOp)
	if errors.IsNotFound(err) {
		return errors.NotFoundf("relation %q application %q", r, appName)
	} else if err != nil {
		return errors.Annotatef(err, "relation %q application %q", r, appName)
	}
	return nil
}

// UpdateApplicationSettingsOperation returns a ModelOperation for updating the
// given application's settings in this relation. It requires a current
// leadership token.
func (r *Relation) UpdateApplicationSettingsOperation(appName string, token leadership.Token, updates map[string]interface{}) (ModelOperation, error) {
	// We can calculate the actual update ahead of time; it's not dependent
	// upon the current state of the document. (*Writing* it should depend
	// on document state, but that's handled below.)
	ep, err := r.Endpoint(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	key := relationApplicationSettingsKey(r.Id(), ep.ApplicationName)
	return newUpdateLeaderSettingsOperation(r.st.db(), token, key, updates), nil
}

// WatchApplicationSettings returns a notify watcher that will signal
// whenever the specified application's relation settings are changed.
func (r *Relation) WatchApplicationSettings(app *Application) (NotifyWatcher, error) {
	ep, err := r.Endpoint(app.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	key := relationApplicationSettingsKey(r.Id(), ep.ApplicationName)
	watcher := newEntityWatcher(r.st, settingsC, r.st.docID(key))
	return watcher, nil
}
