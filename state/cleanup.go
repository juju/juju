// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/mongo"
	stateerrors "github.com/juju/juju/state/errors"
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
	cleanupUnitsForDyingApplication      cleanupKind = "units"
	cleanupDyingUnit                     cleanupKind = "dyingUnit"
	cleanupForceDestroyedUnit            cleanupKind = "forceDestroyUnit"
	cleanupForceRemoveUnit               cleanupKind = "forceRemoveUnit"
	cleanupRemovedUnit                   cleanupKind = "removedUnit"
	cleanupApplication                   cleanupKind = "application"
	cleanupForceApplication              cleanupKind = "forceApplication"
	cleanupApplicationsForDyingModel     cleanupKind = "applications"
	cleanupAttachmentsForDyingStorage    cleanupKind = "storageAttachments"
	cleanupAttachmentsForDyingVolume     cleanupKind = "volumeAttachments"
	cleanupAttachmentsForDyingFilesystem cleanupKind = "filesystemAttachments"
	cleanupModelsForDyingController      cleanupKind = "models"

	// CAAS models require storage to be cleaned up.
	// TODO: should be renamed to something like deadCAASUnitResources.
	cleanupDyingUnitResources cleanupKind = "dyingUnitResources"

	cleanupResourceBlob         cleanupKind = "resourceBlob"
	cleanupStorageForDyingModel cleanupKind = "modelStorage"
	cleanupForceStorage         cleanupKind = "forceStorage"
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

type cancelCleanupOpsArg struct {
	kind    cleanupKind
	pattern bson.DocElem
}

func (st *State) cancelCleanupOps(args ...cancelCleanupOpsArg) ([]txn.Op, error) {
	col, closer := st.db().GetCollection(cleanupsC)
	defer closer()
	patterns := make([]bson.D, len(args))
	for i, arg := range args {
		patterns[i] = bson.D{
			arg.pattern,
			{Name: "kind", Value: arg.kind},
		}
	}
	var docs []cleanupDoc
	if err := col.Find(bson.D{{Name: "$or", Value: patterns}}).All(&docs); err != nil {
		return nil, errors.Annotate(err, "cannot get cleanups docs")
	}
	var ops []txn.Op
	for _, doc := range docs {
		ops = append(ops, txn.Op{
			C:      cleanupsC,
			Id:     doc.DocID,
			Remove: true,
		})
	}
	return ops, nil
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

// MachineRemover deletes a machine from the dqlite database.
// This allows us to initially weave some dqlite support into the cleanup workflow.
type MachineRemover interface {
	DeleteMachine(context.Context, machine.Name) error
}

// Cleanup removes all documents that were previously marked for removal, if
// any such exist. It should be called periodically by at least one element
// of the system.
func (st *State) Cleanup(
	ctx context.Context, store objectstore.ObjectStore,
	machineRemover MachineRemover,
) (err error) {
	var doc cleanupDoc
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()

	modelUUID := st.ModelUUID()
	modelId := names.NewModelTag(modelUUID).ShortId()

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
		logger.Debugf(context.TODO(), "model %v cleanup: %v(%q)", modelId, doc.Kind, doc.Prefix)
		args := make([]bson.Raw, len(doc.Args))
		for i, arg := range doc.Args {
			args[i] = arg.Value.(bson.Raw)
		}
		switch doc.Kind {
		case cleanupApplication:
			err = st.cleanupApplication(ctx, store, doc.Prefix, args)
		case cleanupForceApplication:
			err = st.cleanupForceApplication(ctx, store, doc.Prefix, args)
		case cleanupUnitsForDyingApplication:
			err = st.cleanupUnitsForDyingApplication(ctx, store, doc.Prefix, args)
		case cleanupDyingUnit:
			err = st.cleanupDyingUnit(doc.Prefix, args)
		case cleanupForceDestroyedUnit:
			err = st.cleanupForceDestroyedUnit(ctx, store, doc.Prefix, args)
		case cleanupForceRemoveUnit:
			err = st.cleanupForceRemoveUnit(ctx, store, doc.Prefix, args)
		case cleanupDyingUnitResources:
			err = st.cleanupDyingUnitResources(doc.Prefix, args)
		case cleanupRemovedUnit:
			err = st.cleanupRemovedUnit(doc.Prefix, args)
		case cleanupApplicationsForDyingModel:
			err = st.cleanupApplicationsForDyingModel(ctx, store, args)
		case cleanupAttachmentsForDyingStorage:
			err = st.cleanupAttachmentsForDyingStorage(doc.Prefix, args)
		case cleanupAttachmentsForDyingVolume:
			err = st.cleanupAttachmentsForDyingVolume(doc.Prefix)
		case cleanupAttachmentsForDyingFilesystem:
			err = st.cleanupAttachmentsForDyingFilesystem(doc.Prefix)
		case cleanupModelsForDyingController:
			err = st.cleanupModelsForDyingController(args)
		case cleanupResourceBlob:
			err = st.cleanupResourceBlob(ctx, store, doc.Prefix)
		case cleanupStorageForDyingModel:
			err = st.cleanupStorageForDyingModel(doc.Prefix, args)
		case cleanupForceStorage:
			err = st.cleanupForceStorage(args)
		default:
			err = errors.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Warningf(context.TODO(),
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

func (st *State) cleanupResourceBlob(ctx context.Context, store objectstore.WriteObjectStore, storagePath string) error {
	// Ignore attempts to clean up a placeholder resource.
	if storagePath == "" {
		return nil
	}

	err := store.Remove(ctx, storagePath)
	if errors.Is(err, errors.NotFound) {
		return nil
	}
	return errors.Trace(err)
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
			if errors.Is(err, errors.NotFound) {
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

// cleanupStorageForDyingModel sets all storage to Dying, if they are not
// already Dying or Dead. It's expected to be used when a model is destroyed.
func (st *State) cleanupStorageForDyingModel(modelUUID string, cleanupArgs []bson.Raw) (err error) {
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
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	if force {
		st.scheduleForceCleanup(cleanupForceStorage, modelUUID, args.MaxWait)
	}
	return nil
}

// cleanupForceStorage forcibly removes any remaining storage records from a dying model.
func (st *State) cleanupForceStorage(cleanupArgs []bson.Raw) (err error) {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	// There may be unattached filesystems left over that need to be deleted.
	filesystems, err := sb.AllFilesystems()
	if err != nil {
		return errors.Trace(err)
	}
	for _, fs := range filesystems {
		if err := sb.DestroyFilesystem(fs.FilesystemTag(), true); err != nil {
			return errors.Trace(err)
		}
		if err := sb.RemoveFilesystem(fs.FilesystemTag()); err != nil {
			return errors.Trace(err)
		}
	}

	// There may be unattached volumes left over that need to be deleted.
	volumes, err := sb.AllVolumes()
	if err != nil {
		return errors.Trace(err)
	}
	for _, v := range volumes {
		if err := sb.DestroyVolume(v.VolumeTag(), true); err != nil {
			return errors.Trace(err)
		}
		if err := sb.RemoveVolume(v.VolumeTag()); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupApplication checks if all references to a dying application have been removed,
// and if so, removes the application.
func (st *State) cleanupApplication(ctx context.Context, store objectstore.ObjectStore, appName string, cleanupArgs []bson.Raw) (err error) {
	app, err := st.Application(appName)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// Nothing to do, the application is already gone.
			logger.Tracef(context.TODO(), "cleanupApplication(%s): application already gone", appName)
			return nil
		}
		return errors.Trace(err)
	}
	if app.life() == Alive {
		return errors.BadRequestf("cleanupApplication requested for an application (%s) that is still alive", appName)
	}
	// We know the app is at least Dying, so check if the unit/relation counts are no longer referencing this application.
	if app.unitCount() > 0 {
		// this is considered a no-op because whatever is currently referencing the application
		// should queue up a new cleanup once it stops
		logger.Tracef(context.TODO(), "cleanupApplication(%s) called, but it still has references: unitcount: %d",
			appName, app.unitCount())
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
	// Minimally initiate destroy in dqlite.
	// It's sufficient for now just to advance the life to dying.
	op := app.destroyOperation(store)
	op.destroyStorage = destroyStorage
	op.Force = force
	err = st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf(context.TODO(), "operational errors cleaning up application %v: %v", appName, op.Errors)
	}
	return err
}

// cleanupForceApplication forcibly removes the application.
func (st *State) cleanupForceApplication(ctx context.Context, store objectstore.ObjectStore, appName string, cleanupArgs []bson.Raw) (err error) {
	logger.Debugf(context.TODO(), "force destroy application: %v", appName)
	app, err := st.Application(appName)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// Nothing to do, the application is already gone.
			logger.Tracef(context.TODO(), "forceCleanupApplication(%s): application already gone", appName)
			return nil
		}
		return errors.Trace(err)
	}

	var maxWait time.Duration
	if n := len(cleanupArgs); n != 1 {
		return errors.Errorf("expected 1 argument, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
	}

	op := app.destroyOperation(store)
	op.Force = true
	op.cleanupIgnoringResources = true
	op.MaxWait = maxWait
	err = st.ApplyOperation(op)
	if len(op.Errors) != 0 {
		logger.Warningf(context.TODO(), "operational errors cleaning up application %v: %v", appName, op.Errors)
	}
	return err
}

// cleanupApplicationsForDyingModel sets all applications to Dying, if they are
// not already Dying or Dead. It's expected to be used when a model is
// destroyed.
func (st *State) cleanupApplicationsForDyingModel(ctx context.Context, store objectstore.ObjectStore, cleanupArgs []bson.Raw) (err error) {
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
	return st.removeApplicationsForDyingModel(ctx, store, args)
}

func (st *State) removeApplicationsForDyingModel(ctx context.Context, store objectstore.ObjectStore, args DestroyModelParams) (err error) {
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
		// Minimally initiate destroy in dqlite.
		// It's sufficient for now just to advance the life to dying.
		op := application.destroyOperation(store)
		op.Force = force
		op.MaxWait = args.MaxWait
		err := st.ApplyOperation(op)
		if len(op.Errors) != 0 {
			logger.Warningf(context.TODO(), "operational errors removing application %v for dying model %v: %v", application.name(), st.ModelUUID(), op.Errors)
		}
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupUnitsForDyingApplication sets all units with the given prefix to Dying,
// if they are not already Dying or Dead. It's expected to be used when an
// application is destroyed.
func (st *State) cleanupUnitsForDyingApplication(
	ctx context.Context, store objectstore.ObjectStore,
	applicationName string, cleanupArgs []bson.Raw,
) (err error) {
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

	sel := bson.D{{"application", applicationName}}
	// If we're forcing then include dying and dead units, since we
	// still want the opportunity to schedule fallback cleanups if the
	// unit or machine agents aren't doing their jobs.
	if !force {
		sel = append(sel, bson.DocElem{"life", Alive})
	}
	iter := units.Find(sel).Iter()
	defer closeIter(iter, &err, "reading unit document")

	m, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	unitsToDestroy := set.NewStrings()
	var unitDoc unitDoc
	for iter.Next(&unitDoc) {
		unit := newUnit(st, m.Type(), &unitDoc)
		op := unit.destroyOperation(store)
		op.destroyStorage = destroyStorage
		op.Force = force
		op.MaxWait = maxWait
		err := st.ApplyOperation(op)
		if err == nil {
			unitsToDestroy.Add(unit.name())
		}
		if len(op.Errors) != 0 {
			logger.Warningf(context.TODO(), "operational errors destroying unit %v for dying application %v: %v", unit.name(), applicationName, op.Errors)
		}
		if err != nil {
			return errors.Trace(err)
		}
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
	if errors.Is(err, errors.NotFound) {
		return nil
	} else if err != nil {
		return err
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
		return st.cleanupUnitStorageInstances(unit.unitTag(), force, maxWait)
	} else {
		// Mark storage attachments as dying, so that they are detached
		// and removed from state, allowing the unit to terminate.
		return st.cleanupUnitStorageAttachments(unit.unitTag(), false, force, maxWait)
	}
}

func (st *State) scheduleForceCleanup(kind cleanupKind, name string, maxWait time.Duration) {
	deadline := st.stateClock.Now().Add(maxWait)
	op := newCleanupAtOp(deadline, kind, name, maxWait)
	err := st.db().Run(func(int) ([]txn.Op, error) {
		return []txn.Op{op}, nil
	})
	if err != nil {
		logger.Warningf(context.TODO(), "couldn't schedule %s cleanup: %v", kind, err)
	}
}

func (st *State) cleanupForceDestroyedUnit(ctx context.Context, store objectstore.ObjectStore, unitNameString string, cleanupArgs []bson.Raw) error {
	unitName, err := coreunit.NewName(unitNameString)
	if err != nil {
		return errors.Annotate(err, "parsing unit name")
	}
	var maxWait time.Duration
	if n := len(cleanupArgs); n != 1 {
		return errors.Errorf("expected 1 argument, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
	}

	unit, err := st.Unit(unitName.String())
	if errors.Is(err, errors.NotFound) {
		logger.Debugf(context.TODO(), "no need to force unit to dead %q", unitName)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// If we're here then the usual unit cleanup hasn't happened but
	// since force was specified we still want the machine to go to
	// dead.

	// Destroy all subordinates.
	for _, subNameString := range unit.subordinateNames() {
		opErrs := []error{}
		subName, err := coreunit.NewName(subNameString)
		if err != nil {
			opErrs = []error{err}
		}
		subUnit, err := st.Unit(subName.String())
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			logger.Warningf(context.TODO(), "couldn't get subordinate %q to force destroy: %v", subName, err)
		}
		_, destroyOpErrs, err := subUnit.destroyWithForce(store, true, maxWait)
		if len(destroyOpErrs) != 0 {
			opErrs = append(opErrs, destroyOpErrs...)
		}
		if len(opErrs) != 0 || err != nil {
			logger.Warningf(context.TODO(), "errors while destroying subordinate %q: %v, %v", subName, err, opErrs)
		}
	}

	// Detach all storage.
	err = st.forceRemoveUnitStorageAttachments(unit)
	if err != nil {
		logger.Warningf(context.TODO(), "couldn't remove storage attachments for %q: %v", unitName, err)
	}

	// TODO(units) - remove me
	// Dual write to state.
	err = unit.EnsureDead()
	if err == stateerrors.ErrUnitHasSubordinates || err == stateerrors.ErrUnitHasStorageAttachments {
		// In this case we do want to die and try again - we can't set
		// the unit to dead until the subordinates and storage are
		// gone, so we should give them time to be removed.
		return err
	} else if err != nil {
		logger.Warningf(context.TODO(), "couldn't set unit %q dead: %v", unitName, err)
	}

	// Set up another cleanup to remove the unit in a minute if the
	// deployer doesn't do it.
	st.scheduleForceCleanup(cleanupForceRemoveUnit, unitName.String(), maxWait)
	return nil
}

func (st *State) forceRemoveUnitStorageAttachments(unit *Unit) error {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Annotate(err, "couldn't get storage backend")
	}
	err = sb.DestroyUnitStorageAttachments(unit.unitTag())
	if err != nil {
		return errors.Annotatef(err, "destroying storage attachments for %q", unit.Tag().Id())
	}
	attachments, err := sb.UnitStorageAttachments(unit.unitTag())
	if err != nil {
		return errors.Annotatef(err, "getting storage attachments for %q", unit.Tag().Id())
	}
	for _, attachment := range attachments {
		err := sb.RemoveStorageAttachment(
			attachment.StorageInstance(), unit.unitTag(), true)
		if err != nil {
			logger.Warningf(context.TODO(), "couldn't remove storage attachment %q for %q: %v", attachment.StorageInstance(), unit, err)
		}
	}
	return nil
}

func (st *State) cleanupForceRemoveUnit(ctx context.Context, store objectstore.ObjectStore, unitNameString string, cleanupArgs []bson.Raw) error {
	unitName, err := coreunit.NewName(unitNameString)
	if err != nil {
		return errors.Annotate(err, "parsing unit name")
	}
	var maxWait time.Duration
	if n := len(cleanupArgs); n != 1 {
		return errors.Errorf("expected 1 argument, got %d", n)
	}
	if err := cleanupArgs[0].Unmarshal(&maxWait); err != nil {
		return errors.Annotate(err, "unmarshalling cleanup arg 'maxWait'")
	}
	unit, err := st.Unit(unitName.String())
	if errors.Is(err, errors.NotFound) {
		logger.Debugf(context.TODO(), "no need to force remove unit %q", unitName)
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	opErrs, err := unit.RemoveWithForce(store, true, maxWait)
	if len(opErrs) != 0 {
		logger.Warningf(context.TODO(), "errors encountered force-removing unit %q: %v", unitName, opErrs)
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
		logger.Warningf(context.TODO(), "%v", err)
	}
	volumeAttachments, err := sb.UnitVolumeAttachments(unitTag)
	if err != nil {
		err := errors.Annotate(err, "getting unit volume attachments")
		if !force {
			return err
		}
		logger.Warningf(context.TODO(), "%v", err)
	}

	cleaner := newDyingEntityStorageCleaner(sb, unitTag, false, force)
	return errors.Trace(cleaner.cleanupStorage(filesystemAttachments, volumeAttachments))
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
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			if !force {
				return err
			}
			logger.Warningf(context.TODO(), "could not detach storage %v for unit %v: %v", storageTag.Id(), unitTag.Id(), err)
		}
		if !remove {
			continue
		}
		err = sb.RemoveStorageAttachment(storageTag, unitTag, force)
		if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			if !force {
				return err
			}
			logger.Warningf(context.TODO(), "could not remove storage attachment for storage %v for unit %v: %v", storageTag.Id(), unitTag.Id(), err)
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
		if errors.Is(err, errors.NotFound) {
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
		logger.Warningf(context.TODO(), "could not get unit actions for unit %v during cleanup of removed unit: %v", unitId, err)
	}
	cancelled := ActionResults{
		Status:  ActionCancelled,
		Message: "unit removed",
	}
	for _, action := range actions {
		switch action.Status() {
		case ActionCompleted, ActionCancelled, ActionFailed, ActionAborted, ActionError:
			// nothing to do here
		default:
			if _, err = action.Finish(cancelled); err != nil {
				if !force {
					return errors.Trace(err)
				}
				logger.Warningf(context.TODO(), "could not finish action %v for unit %v during cleanup of removed unit: %v", action.Name(), unitId, err)
			}
		}
	}

	return nil
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
			logger.Warningf(context.TODO(), "%v", detachErr)
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
		if err := sb.DetachVolume(hostTag, volumeTag, false); err != nil {
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
	logger.Errorf(context.TODO(), "%v", err)
}
