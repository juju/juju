// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/featureflag"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/feature"
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
	cleanupMachinesForDyingModel         cleanupKind = "modelMachines"
	cleanupResourceBlob                  cleanupKind = "resourceBlob"
	cleanupStorageForDyingModel          cleanupKind = "modelStorage"
)

// cleanupDoc originally represented a set of documents that should be
// removed, but the Prefix field no longer means anything more than
// "what will be passed to the cleanup func".
type cleanupDoc struct {
	DocID  string      `bson:"_id"`
	Kind   cleanupKind `bson:"kind"`
	Prefix string      `bson:"prefix"`
}

// newCleanupOp returns a txn.Op that creates a cleanup document with a unique
// id and the supplied kind and prefix.
func newCleanupOp(kind cleanupKind, prefix string) txn.Op {
	doc := &cleanupDoc{
		DocID:  fmt.Sprint(bson.NewObjectId()),
		Kind:   kind,
		Prefix: prefix,
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
	iter := cleanups.Find(nil).Iter()
	defer closeIter(iter, &err, "reading cleanup document")
	for iter.Next(&doc) {
		var err error
		logger.Debugf("running %q cleanup: %q", doc.Kind, doc.Prefix)
		switch doc.Kind {
		case cleanupRelationSettings:
			err = st.cleanupRelationSettings(doc.Prefix)
		case cleanupCharm:
			err = st.cleanupCharm(doc.Prefix)
		case cleanupUnitsForDyingApplication:
			err = st.cleanupUnitsForDyingApplication(doc.Prefix)
		case cleanupDyingUnit:
			err = st.cleanupDyingUnit(doc.Prefix)
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
			err = st.cleanupModelsForDyingController()
		case cleanupMachinesForDyingModel:
			err = st.cleanupMachinesForDyingModel()
		case cleanupResourceBlob:
			err = st.cleanupResourceBlob(doc.Prefix)
		case cleanupStorageForDyingModel:
			err = st.cleanupStorageForDyingModel()
		default:
			err = errors.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Errorf("cleanup failed for %v(%q): %v", doc.Kind, doc.Prefix, err)
			continue
		}
		ops := []txn.Op{{
			C:      cleanupsC,
			Id:     doc.DocID,
			Remove: true,
		}}
		if err := st.runTransaction(ops); err != nil {
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
	return storage.Remove(storagePath)
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
func (st *State) cleanupModelsForDyingController() (err error) {
	models, err := st.AllModels()
	if err != nil {
		return errors.Trace(err)
	}
	for _, model := range models {
		if err := model.Destroy(); err != nil {
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
func (st *State) cleanupStorageForDyingModel() (err error) {
	storage, err := st.AllStorageInstances()
	if err != nil {
		return errors.Trace(err)
	}
	for _, s := range storage {
		err := st.DestroyStorageInstance(s.StorageTag())
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
	if featureflag.Enabled(feature.CrossModelRelations) {
		if err := st.removeRemoteApplicationsForDyingModel(); err != nil {
			return err
		}
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
		if err := application.Destroy(); err != nil {
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
func (st *State) cleanupUnitsForDyingApplication(applicationname string) (err error) {
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
		if err := unit.Destroy(); err != nil {
			return err
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
func (st *State) cleanupDyingUnit(name string) error {
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
	// Mark storage attachments as dying, so that they are detached
	// and removed from state, allowing the unit to terminate.
	return st.cleanupUnitStorageAttachments(unit.UnitTag(), false)
}

func (st *State) cleanupUnitStorageAttachments(unitTag names.UnitTag, remove bool) error {
	storageAttachments, err := st.UnitStorageAttachments(unitTag)
	if err != nil {
		return err
	}
	for _, storageAttachment := range storageAttachments {
		storageTag := storageAttachment.StorageInstance()
		err := st.DetachStorage(storageTag, unitTag)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}
		if !remove {
			continue
		}
		err = st.RemoveStorageAttachment(storageTag, unitTag)
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
		return err
	}
	// In an ideal world, we'd call machine.Destroy() here, and thus prevent
	// new dependencies being added while we clean up the ones we know about.
	// But machine destruction is unsophisticated, and doesn't allow for
	// destruction while dependencies exist; so we just have to deal with that
	// possibility below.
	if err := st.cleanupContainers(machine); err != nil {
		return err
	}
	for _, unitName := range machine.doc.Principals {
		if err := st.obliterateUnit(unitName); err != nil {
			return err
		}
	}
	if err := cleanupDyingMachineResources(machine); err != nil {
		return err
	}
	// We need to refresh the machine at this point, because the local copy
	// of the document will not reflect changes caused by the unit cleanups
	// above, and may thus fail immediately.
	if err := machine.Refresh(); errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	// TODO(fwereade): 2013-11-11 bug 1250104
	// If this fails, it's *probably* due to a race in which new dependencies
	// were added while we cleaned up the old ones. If the cleanup doesn't run
	// again -- which it *probably* will anyway -- the issue can be resolved by
	// force-destroying the machine again; that's better than adding layer
	// upon layer of complication here.
	if err := machine.EnsureDead(); err != nil {
		return err
	}
	removePortsOps, err := machine.removePortsOps()
	if err != nil {
		return err
	}
	return st.runTransaction(removePortsOps)

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
	volumeAttachments, err := m.st.MachineVolumeAttachments(m.MachineTag())
	if err != nil {
		return errors.Annotate(err, "getting machine volume attachments")
	}
	for _, va := range volumeAttachments {
		if detachable, err := isDetachableVolumeTag(m.st, va.Volume()); err != nil {
			return errors.Trace(err)
		} else if !detachable {
			// Non-detachable volumes will be removed along with the machine.
			continue
		}
		if err := m.st.DetachVolume(va.Machine(), va.Volume()); err != nil {
			if IsContainsFilesystem(err) {
				// The volume will be destroyed when the
				// contained filesystem is removed, whose
				// destruction is initiated below.
				continue
			}
			return errors.Trace(err)
		}
	}
	filesystemAttachments, err := m.st.MachineFilesystemAttachments(m.MachineTag())
	if err != nil {
		return errors.Annotate(err, "getting machine filesystem attachments")
	}
	for _, fsa := range filesystemAttachments {
		if detachable, err := isDetachableFilesystemTag(m.st, fsa.Filesystem()); err != nil {
			return errors.Trace(err)
		} else if !detachable {
			// Non-detachable filesystems will be removed along with the machine.
			continue
		}
		if err := m.st.DetachFilesystem(fsa.Machine(), fsa.Filesystem()); err != nil {
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
		if err := st.DetachStorage(storageTag, unitTag); err != nil {
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

	var doc volumeAttachmentDoc
	fields := bson.D{{"machineid", 1}}
	iter := coll.Find(bson.D{{"volumeid", volumeId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading volume attachment document")
	for iter.Next(&doc) {
		machineTag := names.NewMachineTag(doc.Machine)
		if err := st.DetachVolume(machineTag, volumeTag); err != nil {
			return errors.Annotate(err, "destroying volume attachment")
		}
	}
	return nil
}

// cleanupAttachmentsForDyingFilesystem sets all filesystem attachments related
// to the specified filesystem to Dying, if they are not already Dying or
// Dead. It's expected to be used when a filesystem is destroyed.
func (st *State) cleanupAttachmentsForDyingFilesystem(filesystemId string) (err error) {
	filesystemTag := names.NewFilesystemTag(filesystemId)

	// This won't miss attachments, because a Dying filesystem cannot have
	// attachments added to it. But we do have to remove the attachments
	// themselves via individual transactions, because they could be in
	// any state at all.
	coll, closer := st.db().GetCollection(filesystemAttachmentsC)
	defer closer()

	var doc filesystemAttachmentDoc
	fields := bson.D{{"machineid", 1}}
	iter := coll.Find(bson.D{{"filesystemid", filesystemId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading filesystem attachment document")
	for iter.Next(&doc) {
		machineTag := names.NewMachineTag(doc.Machine)
		if err := st.DetachFilesystem(machineTag, filesystemTag); err != nil {
			return errors.Annotate(err, "destroying filesystem attachment")
		}
	}
	return nil
}

func closeIter(iter *mgo.Iter, errOut *error, message string) {
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
