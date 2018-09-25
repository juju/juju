// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/mongo"
)

type cleanupKind string

const (
	// SCHEMACHANGE: the names are expressive, the values not so much.
	cleanupRelationSettings              cleanupKind = "settings"
	cleanupUnitsForDyingApplication      cleanupKind = "units"
	cleanupCharm                         cleanupKind = "charm"
	cleanupDyingUnit                     cleanupKind = "dyingUnit"
	cleanupRemovedUnit                   cleanupKind = "removedUnit"
	cleanupApplicationsForDyingModel     cleanupKind = "applications"
	cleanupDyingMachine                  cleanupKind = "dyingMachine"
	cleanupForceDestroyedMachine         cleanupKind = "machine"
	cleanupAttachmentsForDyingStorage    cleanupKind = "storageAttachments"
	cleanupAttachmentsForDyingVolume     cleanupKind = "volumeAttachments"
	cleanupAttachmentsForDyingFilesystem cleanupKind = "filesystemAttachments"
	cleanupModelsForDyingController      cleanupKind = "models"

	// IAAS models require machines to be cleaned up.
	cleanupMachinesForDyingModel cleanupKind = "modelMachines"

	// CAAS models require storage to be cleaned up.
	cleanupDyingUnitResources cleanupKind = "dyingUnitResources"

	cleanupResourceBlob         cleanupKind = "resourceBlob"
	cleanupStorageForDyingModel cleanupKind = "modelStorage"
)

// cleanupDoc originally represented a set of documents that should be
// removed, but the Prefix field no longer means anything more than
// "what will be passed to the cleanup func".
type cleanupDoc struct {
	DocID  string        `bson:"_id"`
	Kind   cleanupKind   `bson:"kind"`
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

	iter := cleanups.Find(nil).Iter()
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
		case cleanupCharm:
			err = st.cleanupCharm(doc.Prefix)
		case cleanupUnitsForDyingApplication:
			err = st.cleanupUnitsForDyingApplication(doc.Prefix, args)
		case cleanupDyingUnit:
			err = st.cleanupDyingUnit(doc.Prefix, args)
		case cleanupDyingUnitResources:
			err = st.cleanupDyingUnitResources(doc.Prefix)
		case cleanupRemovedUnit:
			err = st.cleanupRemovedUnit(doc.Prefix)
		case cleanupApplicationsForDyingModel:
			err = st.cleanupApplicationsForDyingModel()
		case cleanupDyingMachine:
			err = st.cleanupDyingMachine(doc.Prefix)
		case cleanupForceDestroyedMachine:
			err = st.cleanupForceDestroyedMachine(doc.Prefix)
		case cleanupAttachmentsForDyingStorage:
			err = st.cleanupAttachmentsForDyingStorage(doc.Prefix)
		case cleanupAttachmentsForDyingVolume:
			err = st.cleanupAttachmentsForDyingVolume(doc.Prefix)
		case cleanupAttachmentsForDyingFilesystem:
			err = st.cleanupAttachmentsForDyingFilesystem(doc.Prefix)
		case cleanupModelsForDyingController:
			err = st.cleanupModelsForDyingController(args)
		case cleanupMachinesForDyingModel: // IAAS models only
			err = st.cleanupMachinesForDyingModel()
		case cleanupResourceBlob:
			err = st.cleanupResourceBlob(doc.Prefix)
		case cleanupStorageForDyingModel:
			err = st.cleanupStorageForDyingModel(args)
		default:
			err = errors.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Errorf(
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
func (st *State) cleanupMachinesForDyingModel() (err error) {
	// This won't miss machines, because a Dying model cannot have
	// machines added to it. But we do have to remove the machines themselves
	// via individual transactions, because they could be in any state at all.
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		if _, isContainer := m.ParentId(); isContainer {
			continue
		}
		manual, err := m.IsManual()
		if err != nil {
			return errors.Trace(err)
		}
		destroy := m.ForceDestroy
		if manual {
			// Manually added machines should never be force-
			// destroyed automatically. That should be a user-
			// driven decision, since it may leak applications
			// and resources on the machine. If something is
			// stuck, then the user can still force-destroy
			// the manual machines.
			destroy = m.Destroy
		}
		if err := destroy(); err != nil {
			return errors.Trace(err)
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
	destroyStorage := sb.DestroyStorageInstance
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old
		// behaviour: destroy the storage.
	case 1:
		var destroyStorageFlag bool
		if err := cleanupArgs[0].Unmarshal(&destroyStorageFlag); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup args")
		}
		if !destroyStorageFlag {
			destroyStorage = sb.ReleaseStorageInstance
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}

	storage, err := sb.AllStorageInstances()
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range storage {
		const destroyAttached = true
		err := destroyStorage(s.StorageTag(), destroyAttached)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupApplicationsForDyingModel sets all applications to Dying, if they are
// not already Dying or Dead. It's expected to be used when a model is
// destroyed.
func (st *State) cleanupApplicationsForDyingModel() (err error) {
	if err := st.removeRemoteApplicationsForDyingModel(); err != nil {
		return err
	}
	return st.removeApplicationsForDyingModel()
}

func (st *State) removeApplicationsForDyingModel() (err error) {
	// This won't miss applications, because a Dying model cannot have
	// applications added to it. But we do have to remove the applications
	// themselves via individual transactions, because they could be in any
	// state at all.
	applications, closer := st.db().GetCollection(applicationsC)
	defer closer()
	application := Application{st: st}
	sel := bson.D{{"life", Alive}}
	iter := applications.Find(sel).Iter()
	defer closeIter(iter, &err, "reading application document")
	for iter.Next(&application.doc) {
		op := application.DestroyOperation()
		op.RemoveOffers = true
		if err := st.ApplyOperation(op); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (st *State) removeRemoteApplicationsForDyingModel() (err error) {
	// This won't miss remote applications, because a Dying model cannot have
	// applications added to it. But we do have to remove the applications themselves
	// via individual transactions, because they could be in any state at all.
	remoteApps, closer := st.db().GetCollection(remoteApplicationsC)
	defer closer()
	remoteApp := RemoteApplication{st: st}
	sel := bson.D{{"life", Alive}}
	iter := remoteApps.Find(sel).Iter()
	defer closeIter(iter, &err, "reading remote application document")
	for iter.Next(&remoteApp.doc) {
		if err := remoteApp.Destroy(); err != nil {
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
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&destroyStorage); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
	}

	// This won't miss units, because a Dying application cannot have units
	// added to it. But we do have to remove the units themselves via
	// individual transactions, because they could be in any state at all.
	units, closer := st.db().GetCollection(unitsC)
	defer closer()

	unit := Unit{st: st}
	sel := bson.D{{"application", applicationname}, {"life", Alive}}
	iter := units.Find(sel).Iter()
	defer closeIter(iter, &err, "reading unit document")
	for iter.Next(&unit.doc) {
		op := unit.DestroyOperation()
		op.DestroyStorage = destroyStorage
		if err := st.ApplyOperation(op); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// cleanupCharm is speculative: it can abort without error for many
// reasons, because it's triggered somewhat overenthusiastically for
// simplicity's sake.
func (st *State) cleanupCharm(charmURL string) error {
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return errors.Annotatef(err, "invalid charm URL %v", charmURL)
	}

	ch, err := st.Charm(curl)
	if errors.IsNotFound(err) {
		// Charm already removed.
		return nil
	} else if err != nil {
		return errors.Annotate(err, "reading charm")
	}

	err = ch.Destroy()
	switch errors.Cause(err) {
	case nil:
	case errCharmInUse:
		// No cleanup necessary at this time.
		return nil
	default:
		return errors.Annotate(err, "destroying charm")
	}

	if err := ch.Remove(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// cleanupDyingUnit marks resources owned by the unit as dying, to ensure
// they are cleaned up as well.
func (st *State) cleanupDyingUnit(name string, cleanupArgs []bson.Raw) error {
	var destroyStorage bool
	switch n := len(cleanupArgs); n {
	case 0:
		// Old cleanups have no args, so follow the old behaviour.
	case 1:
		if err := cleanupArgs[0].Unmarshal(&destroyStorage); err != nil {
			return errors.Annotate(err, "unmarshalling cleanup args")
		}
	default:
		return errors.Errorf("expected 0-1 arguments, got %d", n)
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
		return err
	}
	for _, relation := range relations {
		relationUnit, err := relation.Unit(unit)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := relationUnit.PrepareLeaveScope(); err != nil {
			return err
		}
	}

	if destroyStorage {
		// Detach and mark storage instances as dying, allowing the
		// unit to terminate.
		return st.cleanupUnitStorageInstances(unit.UnitTag())
	} else {
		// Mark storage attachments as dying, so that they are detached
		// and removed from state, allowing the unit to terminate.
		return st.cleanupUnitStorageAttachments(unit.UnitTag(), false)
	}
}

func (st *State) cleanupDyingUnitResources(unitId string) error {
	unitTag := names.NewUnitTag(unitId)
	sb, err := NewStorageBackend(st)
	if err != nil {
		return err
	}
	filesystemAttachments, err := sb.UnitFilesystemAttachments(unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit filesystem attachments")
	}
	volumeAttachments, err := sb.UnitVolumeAttachments(unitTag)
	if err != nil {
		return errors.Annotate(err, "getting unit volume attachments")
	}

	return cleanupDyingEntityStorage(sb, unitTag, false, filesystemAttachments, volumeAttachments)
}

func (st *State) cleanupUnitStorageAttachments(unitTag names.UnitTag, remove bool) error {
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
		err := sb.DetachStorage(storageTag, unitTag)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
		if !remove {
			continue
		}
		err = sb.RemoveStorageAttachment(storageTag, unitTag)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (st *State) cleanupUnitStorageInstances(unitTag names.UnitTag) error {
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
		err := sb.DestroyStorageInstance(storageTag, true)
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
func (st *State) cleanupRemovedUnit(unitId string) error {
	actions, err := st.matchingActionsByReceiverId(unitId)
	if err != nil {
		return errors.Trace(err)
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
				return errors.Trace(err)
			}
		}
	}

	change := payloadCleanupChange{
		Unit: unitId,
	}
	if err := Apply(st.database, change); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// cleanupDyingMachine marks resources owned by the machine as dying, to ensure
// they are cleaned up as well.
func (st *State) cleanupDyingMachine(machineId string) error {
	machine, err := st.Machine(machineId)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	return cleanupDyingMachineResources(machine)
}

// cleanupForceDestroyedMachine systematically destroys and removes all entities
// that depend upon the supplied machine, and removes the machine from state. It's
// expected to be used in response to destroy-machine --force.
func (st *State) cleanupForceDestroyedMachine(machineId string) error {
	machine, err := st.Machine(machineId)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}

	// The first thing we want to do is remove any series upgrade machine
	// locks that might prevent other resources from being removed.
	if err := cleanupUpgradeSeriesLock(machine); err != nil {
		return errors.Trace(err)
	}

	// In an ideal world, we'd call machine.Destroy() here, and thus prevent
	// new dependencies being added while we clean up the ones we know about.
	// But machine destruction is unsophisticated, and doesn't allow for
	// destruction while dependencies exist; so we just have to deal with that
	// possibility below.
	if err := st.cleanupContainers(machine); err != nil {
		return errors.Trace(err)
	}
	for _, unitName := range machine.doc.Principals {
		if err := st.obliterateUnit(unitName); err != nil {
			return errors.Trace(err)
		}
	}
	if err := cleanupDyingMachineResources(machine); err != nil {
		return errors.Trace(err)
	}
	if machine.IsManager() {
		if machine.HasVote() {
			// we remove the vote from the machine so that it can be torn down cleanly. Note that this isn't reflected
			// in the actual replicaset, so users using --force should be careful.
			hasVoteTxn := func(attempt int) ([]txn.Op, error) {
				if attempt != 0 {
					if err := machine.Refresh(); err != nil {
						return nil, errors.Trace(err)
					}
					if !machine.HasVote() {
						return nil, jujutxn.ErrNoOperations
					}
				}
				return []txn.Op{{
					C:      machinesC,
					Id:     machine.doc.Id,
					Update: bson.D{{"$set", bson.D{{"hasvote", false}}}},
				}}, nil
			}
			if err := st.db().Run(hasVoteTxn); err != nil {
				return errors.Trace(err)
			}
		}
		if err := st.RemoveControllerMachine(machine); err != nil {
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
	if err := machine.EnsureDead(); err != nil {
		return errors.Trace(err)
	}
	removePortsOps, err := machine.removePortsOps()
	if len(removePortsOps) == 0 || err != nil {
		return errors.Trace(err)
	}
	if err := st.db().RunTransaction(removePortsOps); err != nil {
		return errors.Trace(err)
	}
	return nil

	// Note that we do *not* remove the machine entirely: we leave it for the
	// provisioner to clean up, so that we don't end up with an unreferenced
	// instance that would otherwise be ignored when in provisioner-safe-mode.
}

// cleanupContainers recursively calls cleanupForceDestroyedMachine on the supplied
// machine's containers, and removes them from state entirely.
func (st *State) cleanupContainers(machine *Machine) error {
	containerIds, err := machine.Containers()
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, containerId := range containerIds {
		if err := st.cleanupForceDestroyedMachine(containerId); err != nil {
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

func cleanupDyingMachineResources(m *Machine) error {
	sb, err := NewStorageBackend(m.st)
	if err != nil {
		return errors.Trace(err)
	}

	filesystemAttachments, err := sb.MachineFilesystemAttachments(m.MachineTag())
	if err != nil {
		return errors.Annotate(err, "getting machine filesystem attachments")
	}
	volumeAttachments, err := sb.MachineVolumeAttachments(m.MachineTag())
	if err != nil {
		return errors.Annotate(err, "getting machine volume attachments")
	}

	// Check if the machine is manual, to decide whether or not to
	// short circuit the removal of non-detachable filesystems.
	manual, err := m.IsManual()
	if err != nil {
		return errors.Trace(err)
	}
	return cleanupDyingEntityStorage(sb, m.Tag(), manual, filesystemAttachments, volumeAttachments)
}

func cleanupDyingEntityStorage(sb *storageBackend, hostTag names.Tag, manual bool, filesystemAttachments []FilesystemAttachment, volumeAttachments []VolumeAttachment) error {
	// Destroy non-detachable machine/unit filesystems first.
	filesystems, err := sb.filesystems(bson.D{{"hostid", hostTag.Id()}})
	if err != nil {
		return errors.Annotate(err, "getting host filesystems")
	}
	for _, f := range filesystems {
		if err := sb.DestroyFilesystem(f.FilesystemTag()); err != nil {
			return errors.Trace(err)
		}
	}

	// Detach all filesystems from the machine/unit.
	for _, fsa := range filesystemAttachments {
		detachable, err := isDetachableFilesystemTag(sb.mb.db(), fsa.Filesystem())
		if err != nil {
			return errors.Trace(err)
		}
		if detachable {
			if err := sb.DetachFilesystem(fsa.Host(), fsa.Filesystem()); err != nil {
				return errors.Trace(err)
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
				if err != nil {
					return errors.Trace(err)
				}
				if v, err := f.Volume(); err == nil {
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
			if remove {
				if err := sb.RemoveFilesystemAttachment(
					fsa.Host(), fsa.Filesystem(),
				); err != nil {
					return errors.Trace(err)
				}
				if volumeTag != (names.VolumeTag{}) {
					if err := sb.RemoveVolumeAttachmentPlan(
						machineTag, volumeTag,
					); err != nil {
						return errors.Trace(err)
					}
				}
				if err := updateStatus(); err != nil && !errors.IsNotFound(err) {
					return errors.Trace(err)
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
			if err := sb.RemoveFilesystem(f.FilesystemTag()); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Detach all remaining volumes from the machine.
	for _, va := range volumeAttachments {
		if detachable, err := isDetachableVolumeTag(sb.mb.db(), va.Volume()); err != nil {
			return errors.Trace(err)
		} else if !detachable {
			// Non-detachable volumes will be removed along with the machine.
			continue
		}
		if err := sb.DetachVolume(va.Host(), va.Volume()); err != nil {
			if IsContainsFilesystem(err) {
				// The volume will be destroyed when the
				// contained filesystem is removed, whose
				// destruction is initiated below.
				continue
			}
			return errors.Trace(err)
		}
	}
	return nil
}

// obliterateUnit removes a unit from state completely. It is not safe or
// sane to obliterate any unit in isolation; its only reasonable use is in
// the context of machine obliteration, in which we can be sure that unclean
// shutdown of units is not going to leave a machine in a difficult state.
func (st *State) obliterateUnit(unitName string) error {
	unit, err := st.Unit(unitName)
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Unlike the machine, we *can* always destroy the unit, and (at least)
	// prevent further dependencies being added. If we're really lucky, the
	// unit will be removed immediately.
	if err := unit.Destroy(); err != nil {
		return errors.Annotatef(err, "cannot destroy unit %q", unitName)
	}
	if err := unit.Refresh(); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Destroy and remove all storage attachments for the unit.
	if err := st.cleanupUnitStorageAttachments(unit.UnitTag(), true); err != nil {
		return errors.Annotatef(err, "cannot destroy storage for unit %q", unitName)
	}
	for _, subName := range unit.SubordinateNames() {
		if err := st.obliterateUnit(subName); err != nil {
			return err
		}
	}
	if err := unit.EnsureDead(); err != nil {
		return err
	}
	return unit.Remove()
}

// cleanupAttachmentsForDyingStorage sets all storage attachments related
// to the specified storage instance to Dying, if they are not already Dying
// or Dead. It's expected to be used when a storage instance is destroyed.
func (st *State) cleanupAttachmentsForDyingStorage(storageId string) (err error) {
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
	for iter.Next(&doc) {
		unitTag := names.NewUnitTag(doc.Unit)
		if err := sb.DetachStorage(storageTag, unitTag); err != nil {
			return errors.Annotate(err, "destroying storage attachment")
		}
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

func cleanupUpgradeSeriesLock(machine *Machine) error {
	logger.Infof("removing any upgrade series locks for machine, %s", machine)
	err := machine.RemoveUpgradeSeriesLock()
	// Do not return an error
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}
