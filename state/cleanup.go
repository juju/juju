// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/mongo"
)

type cleanupKind string

var (
	// asap is the earliest possible time - cleanups scheduled at this
	// time will run now. Used instead of time.Now() (hard to test) or
	// some contextual clock now (requires that a clock or now value
	// be passed through layers of functions from state to
	// newCleanupOp).
	asap = time.Time{}
)

const (
	// SCHEMACHANGE: the names are expressive, the values not so much.
	cleanupRelationSettings              cleanupKind = "settings"
	cleanupForceDestroyedRelation        cleanupKind = "forceDestroyRelation"
	cleanupUnitsForDyingApplication      cleanupKind = "units"
	cleanupCharm                         cleanupKind = "charm"
	cleanupDyingUnit                     cleanupKind = "dyingUnit"
	cleanupForceDestroyedUnit            cleanupKind = "forceDestroyUnit"
	cleanupForceRemoveUnit               cleanupKind = "forceRemoveUnit"
	cleanupRemovedUnit                   cleanupKind = "removedUnit"
	cleanupApplication                   cleanupKind = "application"
	cleanupApplicationsForDyingModel     cleanupKind = "applications"
	cleanupDyingMachine                  cleanupKind = "dyingMachine"
	cleanupForceDestroyedMachine         cleanupKind = "machine"
	cleanupForceRemoveMachine            cleanupKind = "forceRemoveMachine"
	cleanupAttachmentsForDyingStorage    cleanupKind = "storageAttachments"
	cleanupAttachmentsForDyingVolume     cleanupKind = "volumeAttachments"
	cleanupAttachmentsForDyingFilesystem cleanupKind = "filesystemAttachments"
	cleanupModelsForDyingController      cleanupKind = "models"

	// IAAS models require machines to be cleaned up.
	cleanupMachinesForDyingModel cleanupKind = "modelMachines"

	// CAAS models require storage to be cleaned up.
	cleanupDyingUnitResources cleanupKind = "dyingUnitResources"

	cleanupResourceBlob          cleanupKind = "resourceBlob"
	cleanupStorageForDyingModel  cleanupKind = "modelStorage"
	cleanupBranchesForDyingModel cleanupKind = "branches"
)

// cleanupDoc originally represented a set of documents that should be
// removed, but the Prefix field no longer means anything more than
// "what will be passed to the cleanup func".
type cleanupDoc struct {
	DocID  string        `bson:"_id"`
	Kind   cleanupKind   `bson:"kind"`
	When   time.Time     `bson:"when"`
	Prefix string        `bson:"prefix"`
	Args   []*cleanupArg `bson:"args,omitempty"`
}

type cleanupArg struct {
	Value interface{}
}

// GetBSON is part of the bson.Getter interface.
func (a *cleanupArg) GetBSON() (interface{}, error) {
	return a.Value, nil
}

// SetBSON is part of the bson.Setter interface.
func (a *cleanupArg) SetBSON(raw bson.Raw) error {
	a.Value = raw
	return nil
}

// newCleanupOp returns a txn.Op that creates a cleanup document with a unique
// id and the supplied kind and prefix.
func newCleanupOp(kind cleanupKind, prefix string, args ...interface{}) txn.Op {
	return newCleanupAtOp(asap, kind, prefix, args...)
}

func newCleanupAtOp(when time.Time, kind cleanupKind, prefix string, args ...interface{}) txn.Op {
	var cleanupArgs []*cleanupArg
	if len(args) > 0 {
		cleanupArgs = make([]*cleanupArg, len(args))
		for i, arg := range args {
			cleanupArgs[i] = &cleanupArg{arg}
		}
	}
	doc := &cleanupDoc{
		DocID:  bson.NewObjectId().Hex(),
		Kind:   kind,
		When:   when,
		Prefix: prefix,
		Args:   cleanupArgs,
	}
	return txn.Op{
		C:      cleanupsC,
		Id:     doc.DocID,
		Insert: doc,
	}
}

// NeedsCleanup returns true if documents previously marked for removal exist.
func (st *State) NeedsCleanup() (bool, error) {
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()
	count, err := cleanups.Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Cleanup removes all documents that were previously marked for removal, if
// any such exist. It should be called periodically by at least one element
// of the system.
func (st *State) Cleanup() (err error) {
	var doc cleanupDoc
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()

	modelUUID := st.ModelUUID()
	modelId := modelUUID[:6]

	// Only look at cleanups that should be run now.
	query := bson.M{"$or": []bson.M{
		{"when": bson.M{"$lte": st.stateClock.Now()}},
		{"when": bson.M{"$exists": false}},
	}}
	// TODO(jam): 2019-05-01 We used to just query in any order, but that turned
	//  out to *normally* be in sorted order, and some cleanups ended up depending
	//  on that ordering. We shouldn't, but until we can fix the cleanups,
	//  enforce the sort ordering.
	iter := cleanups.Find(query).Sort("_id").Iter()
	defer closeIter(iter, &err, "reading cleanup document")
	for iter.Next(&doc) {
		var err error
		logger.Debugf("model %v cleanup: %v(%q)", modelId, doc.Kind, doc.Prefix)
		args := make([]bson.Raw, len(doc.Args))
		for i, arg := range doc.Args {
			args[i] = arg.Value.(bson.Raw)
		}
		switch doc.Kind {
		case cleanupRelationSettings:
			err = st.cleanupRelationSettings(doc.Prefix)
		case cleanupForceDestroyedRelation:
			err = st.cleanupForceDestroyedRelation(doc.Prefix)
		case cleanupCharm:
			err = st.cleanupCharm(doc.Prefix)
		case cleanupApplication:
			err = st.cleanupApplication(doc.Prefix, args)
		case cleanupUnitsForDyingApplication:
			err = st.cleanupUnitsForDyingApplication(doc.Prefix, args)
		case cleanupDyingUnit:
			err = st.cleanupDyingUnit(doc.Prefix, args)
		case cleanupForceDestroyedUnit:
			err = st.cleanupForceDestroyedUnit(doc.Prefix, args)
		case cleanupForceRemoveUnit:
			err = st.cleanupForceRemoveUnit(doc.Prefix, args)
		case cleanupDyingUnitResources:
			err = st.cleanupDyingUnitResources(doc.Prefix, args)
		case cleanupRemovedUnit:
			err = st.cleanupRemovedUnit(doc.Prefix, args)
		case cleanupApplicationsForDyingModel:
			err = st.cleanupApplicationsForDyingModel(args)
		case cleanupDyingMachine:
			err = st.cleanupDyingMachine(doc.Prefix, args)
		case cleanupForceDestroyedMachine:
			err = st.cleanupForceDestroyedMachine(doc.Prefix, args)
		case cleanupForceRemoveMachine:
			err = st.cleanupForceRemoveMachine(doc.Prefix, args)
		case cleanupAttachmentsForDyingStorage:
			err = st.cleanupAttachmentsForDyingStorage(doc.Prefix, args)
		case cleanupAttachmentsForDyingVolume:
			err = st.cleanupAttachmentsForDyingVolume(doc.Prefix)
		case cleanupAttachmentsForDyingFilesystem:
			err = st.cleanupAttachmentsForDyingFilesystem(doc.Prefix)
		case cleanupModelsForDyingController:
			err = st.cleanupModelsForDyingController(args)
		case cleanupMachinesForDyingModel: // IAAS models only
			err = st.cleanupMachinesForDyingModel(args)
		case cleanupResourceBlob:
			err = st.cleanupResourceBlob(doc.Prefix)
		case cleanupStorageForDyingModel:
			err = st.cleanupStorageForDyingModel(args)
		case cleanupBranchesForDyingModel:
			err = st.cleanupBranchesForDyingModel(args)
		default:
			err = errors.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Warningf(
				"cleanup failed in model %v for %v(%q): %v",
				modelUUID, doc.Kind, doc.Prefix, err,
			)
			continue
		}
		ops := []txn.Op{{
			C:      cleanupsC,
			Id:     doc.DocID,
			Remove: true,
		}}
		if err := st.db().RunTransaction(ops); err != nil {
			return errors.Annotate(err, "cannot remove empty cleanup document")
		}
	}
	return nil
}

func (st *State) cleanupResourceBlob(storagePath string) error {
	// Ignore attempts to clean up a placeholder resource.
	if storagePath == "" {
		return nil
	}

	persist := st.newPersistence()
	storage := persist.NewStorage()
	err := storage.Remove(storagePath)
	if errors.IsNotFound(err) {
		return nil
	}
	return errors.Trace(err)
}

func (st *State) cleanupRelationSettings(prefix string) error {
	change := relationSettingsCleanupChange{Prefix: st.docID(prefix)}
	if err := Apply(st.database, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) cleanupForceDestroyedRelation(prefix string) (err error) {
	relation, err := st.KeyRelation(prefix)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return errors.Annotatef(err, "getting relation %q", prefix)
	}
	scopes, closer := st.db().GetCollection(relationScopesC)
	defer closer()

	sel := bson.M{"_id": bson.M{
		"$regex": fmt.Sprintf("^%s#", st.docID(relation.globalScope())),
	}}
	iter := scopes.Find(sel).Iter()
	defer closeIter(iter, &err, "reading relation scopes")

	var doc struct {
		Key string `bson:"key"`
	}
	for iter.Next(&doc) {
		scope, role, unitName, err := unpackScopeKey(doc.Key)
		if err != nil {
			return errors.Annotatef(err, "unpacking scope key %q", doc.Key)
		}
		var matchingEp Endpoint
		for _, ep := range relation.Endpoints() {
			if string(ep.Role) == role {
				matchingEp = ep
			}
		}
		if matchingEp.Role == "" {
			return errors.NotFoundf("endpoint matching %q", doc.Key)
		}

		// This is nasty but I can't see any other way to do it - we
		// can't rely on the unit existing to determine the values of
		// isPrincipal and isLocalUnit, and we're only using the RU to
		// call LeaveScope on it.
		ru := RelationUnit{
			st:       st,
			relation: relation,
			unitName: unitName,
			endpoint: matchingEp,
			scope:    scope,
		}
		// Run the leave scope txn immediately rather than building
		// one big transaction because each one decrements the
		// relation's unitcount, and we need the last one to remove
		// the relation (which wouldn't work if the ops were combined
		// into one txn).

		// We know this should be forced, and we've already waited the
		// required time.
		errs, err := ru.LeaveScopeWithForce(true, 0)
		if len(errs) > 0 {
			logger.Warningf("operational errors leaving scope for unit %q in relation %q: %v", unitName, relation, errs)
		}
		if err != nil {
			return errors.Annotatef(err, "leaving scope for unit %q in relation %q", unitName, relation)
		}
	}

	return nil
}

// cleanupModelsForDyingController sets all models to dying, if
// they are not already Dying or Dead. It's expected to be used when a
// controller is destroyed.
func (st *State) cleanupModelsForDyingController(cleanupArgs []bson.Raw) (err error) {
	var args DestroyModelParams
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
		destroyStorage := true
		args.DestroyStorage = &destroyStorage
	case 1:
		if err := cleanupArgs[0].Unmarshal(&args); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}
	modelUUIDs, err := st.AllModelUUIDs()
	if err != nil {
		return errors.Trace(err)
	}
	for _, modelUUID := range modelUUIDs {
		newSt, err := st.newStateNoWorkers(modelUUID)
		// We explicitly don't start the workers.
		if err != nil {
			// This model could have been removed.
			if errors.IsNotFound(err) {
				continue
			}
			return errors.Trace(err)
		}
		defer newSt.Close()

		model, err := newSt.Model()
		if err != nil {
			return errors.Trace(err)
		}

		if err := model.Destroy(args); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupMachinesForDyingModel sets all non-manager machines to Dying,
// if they are not already Dying or Dead. It's expected to be used when
// a model is destroyed.
func (st *State) cleanupMachinesForDyingModel(cleanupArgs []bson.Raw) (err error) {
	var args DestroyModelParams
	switch n := len(cleanupArgs); n {
	case 0:
	// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&args); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup 'destroy model' args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}
	// This won't miss machines, because a Dying model cannot have
	// machines added to it. But we do have to remove the machines themselves
	// via individual transactions, because they could be in any state at all.
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	force := args.Force != nil && *args.Force
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		if err != nil {
			// TODO (force 2019-4-24) we should not break out here but continue with other machines.
			return errors.Trace(err)
		}
		if manual {
			// Manually added machines should never be force-
			// destroyed automatically. That should be a user-
			// driven decision, since it may leak applications
			// and resources on the machine. If something is
			// stuck, then the user can still force-destroy
			// the manual machines.
			if err := m.Destroy(); err != nil {
				// Since we cannot delete a manual machine, we cannot proceed with model destruction even if it is forced.
				// TODO (force 2019-4-24) However, we should not break out here but continue with other machines.
				return errors.Trace(errors.Annotatef(err, "could not destroy manual machine %v", m.Id()))
			}
			continue
		}
		if err := m.ForceDestroy(args.MaxWait); err != nil {
			err = errors.Annotatef(err, "while destroying machine %v is", m.Id())
			// TODO (force 2019-4-24) we should not break out here but continue with other machines.
			if !force {
				return errors.Trace(err)
			}
			logger.Warningf("%v", err)
		}
	}
	return nil
}

// cleanupStorageForDyingModel sets all storage to Dying, if they are not
// already Dying or Dead. It's expected to be used when a model is destroyed.
func (st *State) cleanupStorageForDyingModel(cleanupArgs []bson.Raw) (err error) {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	var args DestroyModelParams
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&args); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup 'destroy model' args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}

	destroyStorage := sb.DestroyStorageInstance
	if args.DestroyStorage == nil || !*args.DestroyStorage {
		destroyStorage = sb.ReleaseStorageInstance
	}

	storage, err := sb.AllStorageInstances()
	if err != nil {
		return errors.Trace(err)
	}
	force := args.Force != nil && *args.Force
	for _, s := range storage {
		const destroyAttached = true
		err := destroyStorage(s.StorageTag(), destroyAttached, force, args.MaxWait)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (st *State) cleanupBranchesForDyingModel(cleanupArgs []bson.Raw) (err error) {
	change := branchesCleanupChange{}
	if err := Apply(st.database, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// cleanupApplication checks if all references to a dying application have been removed,
// and if so, removes the application.
func (st *State) cleanupApplication(applicationname string, cleanupArgs []bson.Raw) (err error) {
	app, err := st.Application(applicationname)
	if err != nil {
		if errors.IsNotFound(err) {
			// Nothing to do, the application is already gone.
			logger.Tracef("cleanupApplication(%s): application already gone", applicationname)
			return nil
		}
		return errors.Trace(err)
	}
	if app.Life() == Alive {
		return errors.BadRequestf("cleanupApplication requested for an application (%s) that is still alive", applicationname)
	}
	// We know the app is at least Dying, so check if the unit/relation counts are no longer referencing this application.
	if app.UnitCount() > 0 || app.RelationCount() > 0 {
		// this is considered a no-op because whatever is currently referencing the application
		// should queue up a new cleanup once it stops
		logger.Tracef("cleanupApplication(%s) called, but it still has references: unitcount: %d relationcount: %d",
			applicationname, app.UnitCount(), app.RelationCount())
		return nil
	}
	destroyStorage := false
	force := false
	if n := len(cleanupArgs); n != 2 {
		return errors.Errorf("expected 2 arguments, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&destroyStorage); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup args")
	}
	if err := cleanupArgs[1].Unmarshal(&force); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
	}
	op := app.DestroyOperation()
	op.DestroyStorage = destroyStorage
	op.Force = force
	err = st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf("operational errors cleaning up application %v: %v", applicationname, op.Errors)
	}
	return err
}

// cleanupApplicationsForDyingModel sets all applications to Dying, if they are
// not already Dying or Dead. It's expected to be used when a model is
// destroyed.
func (st *State) cleanupApplicationsForDyingModel(cleanupArgs []bson.Raw) (err error) {
	var args DestroyModelParams
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&args); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup 'destroy model' args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}
	if err := st.removeRemoteApplicationsForDyingModel(args); err != nil {
		return err
	}
	return st.removeApplicationsForDyingModel(args)
}

func (st *State) removeApplicationsForDyingModel(args DestroyModelParams) (err error) {
	// This won't miss applications, because a Dying model cannot have
	// applications added to it. But we do have to remove the applications
	// themselves via individual transactions, because they could be in any
	// state at all.
	applications, closer := st.db().GetCollection(applicationsC)
	defer closer()
	// Note(jam): 2019-04-25 This will only try to shut down Alive applications,
	//  it doesn't cause applications that are Dying to finish progressing to Dead.
	application := Application{st: st}
	sel := bson.D{{"life", Alive}}
	force := args.Force != nil && *args.Force
	if force {
		// If we're forcing, propagate down to even dying
		// applications, just in case they weren't originally forced.
		sel = nil
	}
	iter := applications.Find(sel).Iter()
	defer closeIter(iter, &err, "reading application document")
	for iter.Next(&application.doc) {
		op := application.DestroyOperation()
		op.RemoveOffers = true
		op.Force = force
		op.MaxWait = args.MaxWait
		err := st.ApplyOperation(op)
		if len(op.Errors) != 0 {
			logger.Warningf("operational errors removing application %v for dying model %v: %v", application.Name(), st.ModelUUID(), op.Errors)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (st *State) removeRemoteApplicationsForDyingModel(args DestroyModelParams) (err error) {
	// This won't miss remote applications, because a Dying model cannot have
	// applications added to it. But we do have to remove the applications themselves
	// via individual transactions, because they could be in any state at all.
	remoteApps, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()
	remoteApp := RemoteApplication{st: st}
	sel := bson.D{{"life", Alive}}
	iter := remoteApps.Find(sel).Iter()
	defer closeIter(iter, &err, "reading remote application document")

	force := args.Force != nil && *args.Force
	for iter.Next(&remoteApp.doc) {
		errs, err := remoteApp.DestroyWithForce(force, args.MaxWait)
		if len(errs) != 0 {
			logger.Warningf("operational errors removing remote application %v for dying model %v: %v", remoteApp.Name(), st.ModelUUID(), errs)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupUnitsForDyingApplication sets all units with the given prefix to Dying,
// if they are not already Dying or Dead. It's expected to be used when a
// application is destroyed.
func (st *State) cleanupUnitsForDyingApplication(applicationname string, cleanupArgs []bson.Raw) (err error) {
	var destroyStorage bool
	destroyStorageArg := func() error {
		err := cleanupArgs[0].Unmarshal(&destroyStorage)
		return errors.Annotate(err, "unmarshalling cleanup arg 'destroyStorage'")
	}
	var force bool
	var maxWait time.Duration
	switch n := len(cleanupArgs); n {
	case 0:
	// It's valid to have no args: old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := destroyStorageArg(); err != nil {
			return err
		}
	case 3:
		if err := destroyStorageArg(); err != nil {
			return err
		}
		if err := cleanupArgs[1].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
		if err := cleanupArgs[2].Unmarshal(&maxWait); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
		}
	default:
		return errors.Errorf("expected 0, 1 or 3 arguments, got %d", n)
	}

	// This won't miss units, because a Dying application cannot have units
	// added to it. But we do have to remove the units themselves via
	// individual transactions, because they could be in any state at all.
	units, closer := st.db().GetCollection(unitsC)
	defer closer()

	unit := Unit{st: st}
	sel := bson.D{{"application", applicationname}}
	// If we're forcing then include dying and dead units, since we
	// still want the opportunity to schedule fallback cleanups if the
	// unit or machine agents aren't doing their jobs.
	if !force {
		sel = append(sel, bson.DocElem{"life", Alive})
	}
	iter := units.Find(sel).Iter()
	defer closeIter(iter, &err, "reading unit document")
	for iter.Next(&unit.doc) {
		op := unit.DestroyOperation()
		op.DestroyStorage = destroyStorage
		op.Force = force
		op.MaxWait = maxWait
		err := st.ApplyOperation(op)
		if len(op.Errors) != 0 {
			logger.Warningf("operational errors destroying unit %v for dying application %v: %v", unit.Name(), applicationname, op.Errors)
		}

		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupCharm is speculative: it can abort without error for many
// reasons, because it's triggered somewhat over-enthusiastically for
// simplicity's sake.
func (st *State) cleanupCharm(charmURL string) error {
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return errors.Annotatef(err, "invalid charm URL %v", charmURL)
	}

	ch, err := st.Charm(curl)
	if errors.IsNotFound(err) {
		// Charm already removed.
		logger.Tracef("cleanup charm(%s) no-op, charm already gone", charmURL)
		return nil
	} else if err != nil {
		return errors.Annotate(err, "reading charm")
	}

	logger.Tracef("cleanup charm(%s): Destroy", charmURL)
	err = ch.Destroy()
	switch errors.Cause(err) {
	case nil:
	case errCharmInUse:
		// No cleanup necessary at this time.
		logger.Tracef("cleanup charm(%s): charm still in use", charmURL)
		return nil
	default:
		return errors.Annotate(err, "destroying charm")
	}

	logger.Tracef("cleanup charm(%s): Remove", charmURL)
	if err := ch.Remove(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// cleanupDyingUnit marks resources owned by the unit as dying, to ensure
// they are cleaned up as well.
func (st *State) cleanupDyingUnit(name string, cleanupArgs []bson.Raw) error {
	var destroyStorage bool
	destroyStorageArg := func() error {
		err := cleanupArgs[0].Unmarshal(&destroyStorage)
		return errors.Annotate(err, "unmarshalling cleanup arg 'destroyStorage'")
	}
	var force bool
	var maxWait time.Duration
	switch n := len(cleanupArgs); n {
	case 0:
	// It's valid to have no args: old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := destroyStorageArg(); err != nil {
			return err
		}
	case 3:
		if err := destroyStorageArg(); err != nil {
			return err
		}
		if err := cleanupArgs[1].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
		if err := cleanupArgs[2].Unmarshal(&maxWait); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
		}
	default:
		return errors.Errorf("expected 0, 1 or 3 arguments, got %d", n)
	}

	unit, err := st.Unit(name)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	// Mark the unit as departing from its joined relations, allowing
	// related units to start converging to a state in which that unit
	// is gone as quickly as possible.
	relations, err := unit.RelationsJoined()
	if err != nil {
		if !force {
			return err
		}
		logger.Warningf("could not get joined relations for unit %v during dying unit cleanup: %v", unit.Name(), err)
	}
	for _, relation := range relations {
		relationUnit, err := relation.Unit(unit)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			if !force {
				return err
			}
			logger.Warningf("could not get unit relation for unit %v during dying unit cleanup: %v", unit.Name(), err)
		} else {
			if err := relationUnit.PrepareLeaveScope(); err != nil {
				if !force {
					return err
				}
				logger.Warningf("could not prepare to leave scope for relation %v for unit %v during dying unit cleanup: %v", relation, unit.Name(), err)
			}
		}
	}

	// If we're forcing, set up a backstop cleanup to really remove
	// the unit in the case that the unit and machine agents don't for
	// some reason.
	if force {
		st.scheduleForceCleanup(cleanupForceDestroyedUnit, name, maxWait)
	}

	if destroyStorage {
		// Detach and mark storage instances as dying, allowing the
		// unit to terminate.
		return st.cleanupUnitStorageInstances(unit.UnitTag(), force, maxWait)
	} else {
		// Mark storage attachments as dying, so that they are detached
		// and removed from state, allowing the unit to terminate.
		return st.cleanupUnitStorageAttachments(unit.UnitTag(), false, force, maxWait)
	}
}

func (st *State) scheduleForceCleanup(kind cleanupKind, name string, maxWait time.Duration) {
	deadline := st.stateClock.Now().Add(maxWait)
	op := newCleanupAtOp(deadline, kind, name, maxWait)
	err := st.db().Run(func(int) ([]txn.Op, error) {
		return []txn.Op{op}, nil
	})
	if err != nil {
		logger.Warningf("couldn't schedule %s cleanup: %v", kind, err)
	}
}

func (st *State) cleanupForceDestroyedUnit(unitId string, cleanupArgs []bson.Raw) error {
	var maxWait time.Duration
	if n := len(cleanupArgs); n != 1 {
		return errors.Errorf("expected 1 argument, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
	}

	unit, err := st.Unit(unitId)
	if errors.IsNotFound(err) {
		logger.Debugf("no need to force unit to dead %q", unitId)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// If we're here then the usual unit cleanup hasn't happened but
	// since force was specified we still want the machine to go to
	// dead.

	// Destroy all subordinates.
	for _, subName := range unit.SubordinateNames() {
		subUnit, err := st.Unit(subName)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			logger.Warningf("couldn't get subordinate %q to force destroy: %v", subName, err)
		}
		opErrs, err := subUnit.DestroyWithForce(true, maxWait)
		if len(opErrs) != 0 || err != nil {
			logger.Warningf("errors while destroying subordinate %q: %v, %v", subName, err, opErrs)
		}
	}

	// LeaveScope on all of the unit's relations.
	relations, err := unit.RelationsInScope()
	if err == nil {
		for _, relation := range relations {
			ru, err := relation.Unit(unit)
			if err != nil {
				logger.Warningf("couldn't get relation unit for %q in %q: %v", unit, relation, err)
				continue
			}
			errs, err := ru.LeaveScopeWithForce(true, maxWait)
			if len(errs) != 0 {
				logger.Warningf("operational errors cleaning up force destroyed unit %v in relation %v: %v", unit, relation, errs)
			}
			if err != nil {
				logger.Warningf("unit %q couldn't leave scope of relation %q: %v", unitId, relation, err)
			}
		}
	} else {
		logger.Warningf("couldn't get in-scope relations for unit %q: %v", unitId, err)
	}

	// Detach all storage.
	err = st.forceRemoveUnitStorageAttachments(unit)
	if err != nil {
		logger.Warningf("couldn't remove storage attachments for %q: %v", unitId, err)
	}

	// Mark the unit dead.
	err = unit.EnsureDead()
	if err == ErrUnitHasSubordinates || err == ErrUnitHasStorageAttachments {
		// In this case we do want to die and try again - we can't set
		// the unit to dead until the subordinates and storage are
		// gone, so we should give them time to be removed.
		return err
	} else if err != nil {
		logger.Warningf("couldn't set unit %q dead: %v", unitId, err)
	}

	// Set up another cleanup to remove the unit in a minute if the
	// deployer doesn't do it.
	st.scheduleForceCleanup(cleanupForceRemoveUnit, unitId, maxWait)
	return nil
}

func (st *State) forceRemoveUnitStorageAttachments(unit *Unit) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Annotate(err, "couldn't get storage backend")
	}
	err = sb.DestroyUnitStorageAttachments(unit.UnitTag())
	if err != nil {
		return errors.Annotatef(err, "destroying storage attachments for %q", unit.Tag().Id())
	}
	attachments, err := sb.UnitStorageAttachments(unit.UnitTag())
	if err != nil {
		return errors.Annotatef(err, "getting storage attachments for %q", unit.Tag().Id())
	}
	for _, attachment := range attachments {
		err := sb.RemoveStorageAttachment(
			attachment.StorageInstance(), unit.UnitTag(), true)
		if err != nil {
			logger.Warningf("couldn't remove storage attachment %q for %q: %v", attachment.StorageInstance(), unit, err)
		}
	}
	return nil
}

func (st *State) cleanupForceRemoveUnit(unitId string, cleanupArgs []bson.Raw) error {
	var maxWait time.Duration
	if n := len(cleanupArgs); n != 1 {
		return errors.Errorf("expected 1 argument, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
	}
	unit, err := st.Unit(unitId)
	if errors.IsNotFound(err) {
		logger.Debugf("no need to force remove unit %q", unitId)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	opErrs, err := unit.RemoveWithForce(true, maxWait)
	if len(opErrs) != 0 {
		logger.Warningf("errors encountered force-removing unit %q: %v", unitId, opErrs)
	}
	return errors.Trace(err)
}

func (st *State) cleanupDyingUnitResources(unitId string, cleanupArgs []bson.Raw) error {
	var force bool
	var maxWait time.Duration
	switch n := len(cleanupArgs); n {
	case 0:
	// It's valid to have no args: old cleanups have no args, so follow the old behaviour.
	case 2:
		if err := cleanupArgs[0].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
		if err := cleanupArgs[1].Unmarshal(&maxWait); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
		}
	default:
		return errors.Errorf("expected 0 or 2 arguments, got %d", n)
	}
	unitTag := names.NewUnitTag(unitId)
	sb, err := NewStorageBackend(st)
	if err != nil {
		return err
	}
	filesystemAttachments, err := sb.UnitFilesystemAttachments(unitTag)
	if err != nil {
		err := errors.Annotate(err, "getting unit filesystem attachments")
		if !force {
			return err
		}
		logger.Warningf("%v", err)
	}
	volumeAttachments, err := sb.UnitVolumeAttachments(unitTag)
	if err != nil {
		err := errors.Annotate(err, "getting unit volume attachments")
		if !force {
			return err
		}
		logger.Warningf("%v", err)
	}

	return cleanupDyingEntityStorage(sb, unitTag, false, filesystemAttachments, volumeAttachments, force)
}

func (st *State) cleanupUnitStorageAttachments(unitTag names.UnitTag, remove bool, force bool, maxWait time.Duration) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return err
	}
	storageAttachments, err := sb.UnitStorageAttachments(unitTag)
	if err != nil {
		return err
	}
	for _, storageAttachment := range storageAttachments {
		storageTag := storageAttachment.StorageInstance()
		err := sb.DetachStorage(storageTag, unitTag, force, maxWait)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			if !force {
				return err
			}
			logger.Warningf("could not detach storage %v for unit %v: %v", storageTag.Id(), unitTag.Id(), err)
		}
		if !remove {
			continue
		}
		err = sb.RemoveStorageAttachment(storageTag, unitTag, force)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			if !force {
				return err
			}
			logger.Warningf("could not remove storage attachment for storage %v for unit %v: %v", storageTag.Id(), unitTag.Id(), err)
		}
	}
	return nil
}

func (st *State) cleanupUnitStorageInstances(unitTag names.UnitTag, force bool, maxWait time.Duration) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return err
	}
	storageAttachments, err := sb.UnitStorageAttachments(unitTag)
	if err != nil {
		return err
	}
	for _, storageAttachment := range storageAttachments {
		storageTag := storageAttachment.StorageInstance()
		err := sb.DestroyStorageInstance(storageTag, true, force, maxWait)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

// cleanupRemovedUnit takes care of all the final cleanup required when
// a unit is removed.
func (st *State) cleanupRemovedUnit(unitId string, cleanupArgs []bson.Raw) error {
	var force bool
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}

	actions, err := st.matchingActionsByReceiverId(unitId)
	if err != nil {
		if !force {
			return errors.Trace(err)
		}
		logger.Warningf("could not get unit actions for unit %v during cleanup of removed unit: %v", unitId, err)
	}
	cancelled := ActionResults{
		Status:  ActionCancelled,
		Message: "unit removed",
	}
	for _, action := range actions {
		switch action.Status() {
		case ActionCompleted, ActionCancelled, ActionFailed:
			// nothing to do here
		default:
			if _, err = action.Finish(cancelled); err != nil {
				if !force {
					return errors.Trace(err)
				}
				logger.Warningf("could not finish action %v for unit %v during cleanup of removed unit: %v", action.Name(), unitId, err)
			}
		}
	}

	change := payloadCleanupChange{
		Unit: unitId,
	}
	if err := Apply(st.database, change); err != nil {
		if !force {
			return errors.Trace(err)
		}
		logger.Warningf("could not cleanup payload for unit %v during cleanup of removed unit: %v", unitId, err)
	}
	return nil
}

// cleanupDyingMachine marks resources owned by the machine as dying, to ensure
// they are cleaned up as well.
func (st *State) cleanupDyingMachine(machineID string, cleanupArgs []bson.Raw) error {
	var (
		force   bool
		maxWait time.Duration
	)
	argCount := len(cleanupArgs)
	if argCount > 2 {
		return errors.Errorf("expected 0-1 arguments, got %d", argCount)
	}
	// Old cleanups have no args, so use the default values.
	if argCount >= 1 {
		if err := cleanupArgs[0].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
	}
	if argCount >= 2 {
		if err := cleanupArgs[1].Unmarshal(&maxWait); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
		}
	}

	machine, err := st.Machine(machineID)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	err = cleanupDyingMachineResources(machine, force)
	if err != nil {
		return errors.Trace(err)
	}
	// If we're forcing, schedule a fallback cleanup to remove the
	// machine if the provisioner has gone AWOL - the main case here
	// is if the cloud credential is invalid so the provisioner is
	// stopped.
	if force {
		st.scheduleForceCleanup(cleanupForceRemoveMachine, machineID, maxWait)
	}
	return nil
}

// cleanupForceDestroyedMachine systematically destroys and removes all entities
// that depend upon the supplied machine, and removes the machine from state. It's
// expected to be used in response to destroy-machine --force.
func (st *State) cleanupForceDestroyedMachine(machineId string, cleanupArgs []bson.Raw) error {
	var maxWait time.Duration
	// It's valid to have no args: old cleanups have no args, so follow the old behaviour.
	if n := len(cleanupArgs); n > 0 {
		if n > 1 {
			return errors.Errorf("expected 0-1 arguments, got %d", n)
		}
		if n >= 1 {
			if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
				return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
			}
		}
	}
	return st.cleanupForceDestroyedMachineInternal(machineId, maxWait)
}

func (st *State) cleanupForceDestroyedMachineInternal(machineID string, maxWait time.Duration) error {
	// The first thing we want to do is remove any series upgrade machine
	// locks that might prevent other resources from being removed.
	// We don't tie the lock cleanup to existence of the machine.
	// Just always delete it if it exists.
	if err := st.cleanupUpgradeSeriesLock(machineID); err != nil {
		return errors.Trace(err)
	}

	machine, err := st.Machine(machineID)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// In an ideal world, we'd call machine.Destroy() here, and thus prevent
	// new dependencies being added while we clean up the ones we know about.
	// But machine destruction is unsophisticated, and doesn't allow for
	// destruction while dependencies exist; so we just have to deal with that
	// possibility below.
	if err := st.cleanupContainers(machine, maxWait); err != nil {
		return errors.Trace(err)
	}
	for _, unitName := range machine.doc.Principals {
		opErrs, err := st.obliterateUnit(unitName, true, maxWait)
		if len(opErrs) != 0 {
			logger.Warningf("while obliterating unit %v: %v", unitName, opErrs)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	if err := cleanupDyingMachineResources(machine, true); err != nil {
		return errors.Trace(err)
	}
	if machine.IsManager() {
		node, err := st.ControllerNode(machineID)
		if err != nil {
			return errors.Annotatef(err, "cannot get controller node for machine %v", machineID)
		}
		if node.HasVote() {
			// we remove the vote from the controller so that it can be torn down cleanly. Note that this isn't reflected
			// in the actual replicaset, so users using --force should be careful.
			if err := node.SetHasVote(false); err != nil {
				return errors.Trace(err)
			}
		}
		if err := st.RemoveControllerReference(node); err != nil {
			return errors.Trace(err)
		}
	}

	// We need to refresh the machine at this point, because the local copy
	// of the document will not reflect changes caused by the unit cleanups
	// above, and may thus fail immediately.
	if err := machine.Refresh(); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	// TODO(fwereade): 2013-11-11 bug 1250104
	// If this fails, it's *probably* due to a race in which new dependencies
	// were added while we cleaned up the old ones. If the cleanup doesn't run
	// again -- which it *probably* will anyway -- the issue can be resolved by
	// force-destroying the machine again; that's better than adding layer
	// upon layer of complication here.
	if err := machine.advanceLifecycle(Dead, true, maxWait); err != nil {
		return errors.Trace(err)
	}
	removePortsOps, err := machine.removePortsOps()
	if len(removePortsOps) == 0 || err != nil {
		return errors.Trace(err)
	}
	if err := st.db().RunTransaction(removePortsOps); err != nil {
		return errors.Trace(err)
	}

	// Note that we do *not* remove the machine immediately: we leave
	// it for the provisioner to clean up, so that we don't end up
	// with an unreferenced instance that would otherwise be ignored
	// when in provisioner-safe-mode.
	return nil
}

// cleanupForceRemoveMachine is a backstop to remove a force-destroyed
// machine after a certain amount of time if it hasn't gone away
// already.
func (st *State) cleanupForceRemoveMachine(machineId string, cleanupArgs []bson.Raw) error {
	machine, err := st.Machine(machineId)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	return machine.Remove()
}

// cleanupContainers recursively calls cleanupForceDestroyedMachine on the supplied
// machine's containers, and removes them from state entirely.
func (st *State) cleanupContainers(machine *Machine, maxWait time.Duration) error {
	containerIds, err := machine.Containers()
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, containerId := range containerIds {
		if err := st.cleanupForceDestroyedMachineInternal(containerId, maxWait); err != nil {
			return err
		}
		container, err := st.Machine(containerId)
		if errors.IsNotFound(err) {
			return nil
		} else if err != nil {
			return err
		}
		if err := container.Remove(); err != nil {
			return err
		}
	}
	return nil
}

func cleanupDyingMachineResources(m *Machine, force bool) error {
	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}

	filesystemAttachments, err := sb.MachineFilesystemAttachments(m.MachineTag())
	if err != nil {
		err = errors.Annotate(err, "getting machine filesystem attachments")
		if !force {
			return err
		}
		logger.Warningf("%v", err)
	}
	volumeAttachments, err := sb.MachineVolumeAttachments(m.MachineTag())
	if err != nil {
		err = errors.Annotate(err, "getting machine volume attachments")
		if !force {
			return err
		}
		logger.Warningf("%v", err)
	}

	// Check if the machine is manual, to decide whether or not to
	// short circuit the removal of non-detachable filesystems.
	manual, err := m.IsManual()
	if err != nil {
		if !force {
			return errors.Trace(err)
		}
		logger.Warningf("could not determine if machine %v is manual: %v", m.MachineTag().Id(), err)
	}
	return cleanupDyingEntityStorage(sb, m.Tag(), manual, filesystemAttachments, volumeAttachments, force)
}

func cleanupDyingEntityStorage(sb *storageBackend, hostTag names.Tag, manual bool, filesystemAttachments []FilesystemAttachment, volumeAttachments []VolumeAttachment, force bool) error {
	// Destroy non-detachable machine/unit filesystems first.
	filesystems, err := sb.filesystems(bson.D{{"hostid", hostTag.Id()}})
	if err != nil && !errors.IsNotFound(err) {
		err = errors.Annotate(err, "getting host filesystems")
		if !force {
			logger.Warningf("%v", err)
		}
	}
	for _, f := range filesystems {
		if err := sb.DestroyFilesystem(f.FilesystemTag()); err != nil && !errors.IsNotFound(err) {
			if !force {
				return errors.Trace(err)
			}
			logger.Warningf("could not destroy filesystem %v for %v: %v", f.FilesystemTag().Id(), hostTag, err)
		}
	}

	// Detach all filesystems from the machine/unit.
	for _, fsa := range filesystemAttachments {
		detachable, err := isDetachableFilesystemTag(sb.mb.db(), fsa.Filesystem())
		if err != nil && !errors.IsNotFound(err) {
			if !force {
				return errors.Trace(err)
			}
			logger.Warningf("could not determine if filesystem %v for %v is detachable: %v", fsa.Filesystem().Id(), hostTag, err)
		}
		if detachable {
			if err := sb.DetachFilesystem(fsa.Host(), fsa.Filesystem()); err != nil && !errors.IsNotFound(err) {
				if !force {
					return errors.Trace(err)
				}
				logger.Warningf("could not detach filesystem %v for %v: %v", fsa.Filesystem().Id(), hostTag, err)
			}
		}
		if !manual {
			// For non-manual machines we immediately remove the attachments
			// for non-detachable or volume-backed filesystems, which should
			// have been set to Dying by the destruction of the machine
			// filesystems, or filesystem detachment, above.
			machineTag := fsa.Host()
			var remove bool
			var volumeTag names.VolumeTag
			var updateStatus func() error
			if !detachable {
				remove = true
				updateStatus = func() error { return nil }
			} else {
				f, err := sb.Filesystem(fsa.Filesystem())
				if err != nil && !errors.IsNotFound(err) {
					if !force {
						return errors.Trace(err)
					}
					logger.Warningf("could not get filesystem %v for %v: %v", fsa.Filesystem().Id(), hostTag, err)
				}
				if err == nil {
					if v, err := f.Volume(); err == nil && !errors.IsNotFound(err) {
						// Filesystem is volume-backed.
						volumeTag = v
						remove = true
					}
					updateStatus = func() error {
						return f.SetStatus(status.StatusInfo{
							Status: status.Detached,
						})
					}
				}
			}
			if remove {
				if err := sb.RemoveFilesystemAttachment(fsa.Host(), fsa.Filesystem()); err != nil && !errors.IsNotFound(err) {
					if !force {
						return errors.Trace(err)
					}
					logger.Warningf("could not remove attachment for filesystem %v for %v: %v", fsa.Filesystem().Id(), hostTag, err)
				}
				if volumeTag != (names.VolumeTag{}) {
					if err := sb.RemoveVolumeAttachmentPlan(machineTag, volumeTag); err != nil && !errors.IsNotFound(err) {
						if !force {
							return errors.Trace(err)
						}
						logger.Warningf("could not remove attachment plan for volume %v for %v: %v", volumeTag.Id(), hostTag, err)
					}
				}
				if err := updateStatus(); err != nil && !errors.IsNotFound(err) {
					if !force {
						return errors.Trace(err)
					}
					logger.Warningf("could not update status while cleaning up storage for dying %v: %v", hostTag, err)
				}
			}
		}
	}

	// For non-manual machines we immediately remove the non-detachable
	// filesystems, which should have been detached above. Short circuiting
	// the removal of machine filesystems means we can avoid stuck
	// filesystems preventing any model-scoped backing volumes from being
	// detached and destroyed. For non-manual machines this is safe, because
	// the machine is about to be terminated. For manual machines, stuck
	// filesystems will have to be fixed manually.
	if !manual {
		for _, f := range filesystems {
			if err := sb.RemoveFilesystem(f.FilesystemTag()); err != nil && !errors.IsNotFound(err) {
				if !force {
					return errors.Trace(err)
				}
				logger.Warningf("could not remove filesystem %v for dying %v: %v", f.FilesystemTag().Id(), hostTag, err)
			}
		}
	}

	// Detach all remaining volumes from the machine.
	for _, va := range volumeAttachments {
		if detachable, err := isDetachableVolumeTag(sb.mb.db(), va.Volume()); err != nil && !errors.IsNotFound(err) {
			if !force {
				return errors.Trace(err)
			}
			logger.Warningf("could not determine if volume %v for dying %v is detachable: %v", va.Volume().Id(), hostTag, err)
		} else if !detachable {
			// Non-detachable volumes will be removed along with the machine.
			continue
		}
		if err := sb.DetachVolume(va.Host(), va.Volume()); err != nil && !errors.IsNotFound(err) {
			if IsContainsFilesystem(err) {
				// The volume will be destroyed when the
				// contained filesystem is removed, whose
				// destruction is initiated below.
				continue
			}
			if !force {
				return errors.Trace(err)
			}
			logger.Warningf("could not detach volume %v for dying %v: %v", va.Volume().Id(), hostTag, err)
		}
	}
	return nil
}

// obliterateUnit removes a unit from state completely. It is not safe or
// sane to obliterate any unit in isolation; its only reasonable use is in
// the context of machine obliteration, in which we can be sure that unclean
// shutdown of units is not going to leave a machine in a difficult state.
func (st *State) obliterateUnit(unitName string, force bool, maxWait time.Duration) ([]error, error) {
	var opErrs []error
	unit, err := st.Unit(unitName)
	if errors.IsNotFound(err) {
		return opErrs, nil
	} else if err != nil {
		return opErrs, err
	}
	// Unlike the machine, we *can* always destroy the unit, and (at least)
	// prevent further dependencies being added. If we're really lucky, the
	// unit will be removed immediately.
	errs, err := unit.DestroyWithForce(force, maxWait)
	opErrs = append(opErrs, errs...)
	if err != nil {
		if !force {
			return opErrs, errors.Annotatef(err, "cannot destroy unit %q", unitName)
		}
		opErrs = append(opErrs, err)
	}
	if err := unit.Refresh(); errors.IsNotFound(err) {
		return opErrs, nil
	} else if err != nil {
		if !force {
			return opErrs, err
		}
		opErrs = append(opErrs, err)
	}
	// Destroy and remove all storage attachments for the unit.
	if err := st.cleanupUnitStorageAttachments(unit.UnitTag(), true, force, maxWait); err != nil {
		err := errors.Annotatef(err, "cannot destroy storage for unit %q", unitName)
		if !force {
			return opErrs, err
		}
		opErrs = append(opErrs, err)
	}
	for _, subName := range unit.SubordinateNames() {
		errs, err := st.obliterateUnit(subName, force, maxWait)
		opErrs = append(opErrs, errs...)
		if err != nil {
			if !force {
				return opErrs, err
			}
			opErrs = append(opErrs, err)
		}
	}
	if err := unit.EnsureDead(); err != nil {
		if !force {
			return opErrs, err
		}
		opErrs = append(opErrs, err)
	}
	errs, err = unit.RemoveWithForce(force, maxWait)
	opErrs = append(opErrs, errs...)
	return opErrs, err
}

// cleanupAttachmentsForDyingStorage sets all storage attachments related
// to the specified storage instance to Dying, if they are not already Dying
// or Dead. It's expected to be used when a storage instance is destroyed.
func (st *State) cleanupAttachmentsForDyingStorage(storageId string, cleanupArgs []bson.Raw) (err error) {
	var force bool
	var maxWait time.Duration
	switch n := len(cleanupArgs); n {
	case 0:
	// It's valid to have no args: old cleanups have no args, so follow the old behaviour.
	case 2:
		if err := cleanupArgs[0].Unmarshal(&force); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'force'")
		}
		if err := cleanupArgs[1].Unmarshal(&maxWait); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
		}
	default:
		return errors.Errorf("expected 0 or 2 arguments, got %d", n)
	}

	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	storageTag := names.NewStorageTag(storageId)

	// This won't miss attachments, because a Dying storage instance cannot
	// have attachments added to it. But we do have to remove the attachments
	// themselves via individual transactions, because they could be in
	// any state at all.
	coll, closer := st.db().GetCollection(storageAttachmentsC)
	defer closer()

	var doc storageAttachmentDoc
	fields := bson.D{{"unitid", 1}}
	iter := coll.Find(bson.D{{"storageid", storageId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading storage attachment document")
	var detachErr error
	for iter.Next(&doc) {
		unitTag := names.NewUnitTag(doc.Unit)
		if err := sb.DetachStorage(storageTag, unitTag, force, maxWait); err != nil {
			detachErr = errors.Annotate(err, "destroying storage attachment")
			logger.Warningf("%v", detachErr)
		}
	}
	if !force && detachErr != nil {
		return detachErr
	}
	return nil
}

// cleanupAttachmentsForDyingVolume sets all volume attachments related
// to the specified volume to Dying, if they are not already Dying or
// Dead. It's expected to be used when a volume is destroyed.
func (st *State) cleanupAttachmentsForDyingVolume(volumeId string) (err error) {
	volumeTag := names.NewVolumeTag(volumeId)

	// This won't miss attachments, because a Dying volume cannot have
	// attachments added to it. But we do have to remove the attachments
	// themselves via individual transactions, because they could be in
	// any state at all.
	coll, closer := st.db().GetCollection(volumeAttachmentsC)
	defer closer()

	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}

	var doc volumeAttachmentDoc
	fields := bson.D{{"hostid", 1}}
	iter := coll.Find(bson.D{{"volumeid", volumeId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading volume attachment document")
	for iter.Next(&doc) {
		hostTag := storageAttachmentHost(doc.Host)
		if err := sb.DetachVolume(hostTag, volumeTag); err != nil {
			return errors.Annotate(err, "destroying volume attachment")
		}
	}
	return nil
}

// cleanupAttachmentsForDyingFilesystem sets all filesystem attachments related
// to the specified filesystem to Dying, if they are not already Dying or
// Dead. It's expected to be used when a filesystem is destroyed.
func (st *State) cleanupAttachmentsForDyingFilesystem(filesystemId string) (err error) {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}

	filesystemTag := names.NewFilesystemTag(filesystemId)

	// This won't miss attachments, because a Dying filesystem cannot have
	// attachments added to it. But we do have to remove the attachments
	// themselves via individual transactions, because they could be in
	// any state at all.
	coll, closer := sb.mb.db().GetCollection(filesystemAttachmentsC)
	defer closer()

	var doc filesystemAttachmentDoc
	fields := bson.D{{"hostid", 1}}
	iter := coll.Find(bson.D{{"filesystemid", filesystemId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading filesystem attachment document")
	for iter.Next(&doc) {
		hostTag := storageAttachmentHost(doc.Host)
		if err := sb.DetachFilesystem(hostTag, filesystemTag); err != nil {
			return errors.Annotate(err, "destroying filesystem attachment")
		}
	}
	return nil
}

// cleanupUpgradeSeriesLock removes a series upgrade lock
// for the input machine ID if one exists.
// The
func (st *State) cleanupUpgradeSeriesLock(machineID string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if _, err := st.getUpgradeSeriesLock(machineID); err != nil {
			if errors.IsNotFound(err) {
				return nil, jujutxn.ErrNoOperations
			}
			return nil, errors.Trace(err)
		}

		return removeUpgradeSeriesLockTxnOps(machineID), nil
	}
	return errors.Trace(st.db().Run(buildTxn))
}

func closeIter(iter mongo.Iterator, errOut *error, message string) {
	err := iter.Close()
	if err == nil {
		return
	}
	err = errors.Annotate(err, message)
	if *errOut == nil {
		*errOut = err
		return
	}
	logger.Errorf("%v", err)
}
