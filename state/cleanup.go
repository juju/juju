// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

type cleanupKind string

const (
	// SCHEMACHANGE: the names are expressive, the values not so much.
	cleanupRelationSettings              cleanupKind = "settings"
	cleanupUnitsForDyingService          cleanupKind = "units"
	cleanupDyingUnit                     cleanupKind = "dyingUnit"
	cleanupRemovedUnit                   cleanupKind = "removedUnit"
	cleanupServicesForDyingEnvironment   cleanupKind = "services"
	cleanupDyingMachine                  cleanupKind = "dyingMachine"
	cleanupForceDestroyedMachine         cleanupKind = "machine"
	cleanupAttachmentsForDyingStorage    cleanupKind = "storageAttachments"
	cleanupAttachmentsForDyingVolume     cleanupKind = "volumeAttachments"
	cleanupAttachmentsForDyingFilesystem cleanupKind = "filesystemAttachments"
)

// cleanupDoc represents a potentially large set of documents that should be
// removed.
type cleanupDoc struct {
	DocID   string `bson:"_id"`
	EnvUUID string `bson:"env-uuid"`
	Kind    cleanupKind
	Prefix  string
}

// newCleanupOp returns a txn.Op that creates a cleanup document with a unique
// id and the supplied kind and prefix.
func (st *State) newCleanupOp(kind cleanupKind, prefix string) txn.Op {
	doc := &cleanupDoc{
		DocID:   st.docID(fmt.Sprint(bson.NewObjectId())),
		EnvUUID: st.EnvironUUID(),
		Kind:    kind,
		Prefix:  prefix,
	}
	return txn.Op{
		C:      cleanupsC,
		Id:     doc.DocID,
		Insert: doc,
	}
}

// NeedsCleanup returns true if documents previously marked for removal exist.
func (st *State) NeedsCleanup() (bool, error) {
	cleanups, closer := st.getCollection(cleanupsC)
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
	cleanups, closer := st.getCollection(cleanupsC)
	defer closer()
	iter := cleanups.Find(nil).Iter()
	defer closeIter(iter, &err, "reading cleanup document")
	for iter.Next(&doc) {
		var err error
		logger.Debugf("running %q cleanup: %q", doc.Kind, doc.Prefix)
		switch doc.Kind {
		case cleanupRelationSettings:
			err = st.cleanupRelationSettings(doc.Prefix)
		case cleanupUnitsForDyingService:
			err = st.cleanupUnitsForDyingService(doc.Prefix)
		case cleanupDyingUnit:
			err = st.cleanupDyingUnit(doc.Prefix)
		case cleanupRemovedUnit:
			err = st.cleanupRemovedUnit(doc.Prefix)
		case cleanupServicesForDyingEnvironment:
			err = st.cleanupServicesForDyingEnvironment()
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
		default:
			err = fmt.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Warningf("cleanup failed: %v", err)
			continue
		}
		ops := []txn.Op{{
			C:      cleanupsC,
			Id:     doc.DocID,
			Remove: true,
		}}
		if err := st.runTransaction(ops); err != nil {
			logger.Warningf("cannot remove empty cleanup document: %v", err)
		}
	}
	return nil
}

func (st *State) cleanupRelationSettings(prefix string) error {
	settings, closer := st.getCollection(settingsC)
	defer closer()
	// Documents marked for cleanup are not otherwise referenced in the
	// system, and will not be under watch, and are therefore safe to
	// delete directly.
	settingsW := settings.Writeable()

	sel := bson.D{{"_id", bson.D{{"$regex", "^" + st.docID(prefix)}}}}
	if count, err := settingsW.Find(sel).Count(); err != nil {
		return fmt.Errorf("cannot detect cleanup targets: %v", err)
	} else if count != 0 {
		if _, err := settingsW.RemoveAll(sel); err != nil {
			return fmt.Errorf("cannot remove documents marked for cleanup: %v", err)
		}
	}
	return nil
}

// cleanupServicesForDyingEnvironment sets all services to Dying, if they are
// not already Dying or Dead. It's expected to be used when an environment is
// destroyed.
func (st *State) cleanupServicesForDyingEnvironment() (err error) {
	// This won't miss services, because a Dying environment cannot have
	// services added to it. But we do have to remove the services themselves
	// via individual transactions, because they could be in any state at all.
	services, closer := st.getCollection(servicesC)
	defer closer()
	service := Service{st: st}
	sel := bson.D{{"life", Alive}}
	iter := services.Find(sel).Iter()
	defer closeIter(iter, &err, "reading service document")
	for iter.Next(&service.doc) {
		if err := service.Destroy(); err != nil {
			return err
		}
	}
	return nil
}

// cleanupUnitsForDyingService sets all units with the given prefix to Dying,
// if they are not already Dying or Dead. It's expected to be used when a
// service is destroyed.
func (st *State) cleanupUnitsForDyingService(serviceName string) (err error) {
	// This won't miss units, because a Dying service cannot have units added
	// to it. But we do have to remove the units themselves via individual
	// transactions, because they could be in any state at all.
	units, closer := st.getCollection(unitsC)
	defer closer()

	// TODO(mjs) - remove this post v1.21
	// Older versions of the code put a trailing forward slash on the
	// end of the service name. Remove it here in case a pre-upgrade
	// cleanup document is seen.
	serviceName = strings.TrimSuffix(serviceName, "/")

	unit := Unit{st: st}
	sel := bson.D{{"service", serviceName}, {"life", Alive}}
	iter := units.Find(sel).Iter()
	defer closeIter(iter, &err, "reading unit document")
	for iter.Next(&unit.doc) {
		if err := unit.Destroy(); err != nil {
			return err
		}
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
	storageAttachments, err := st.UnitStorageAttachments(unit.UnitTag())
	if err != nil {
		return err
	}
	for _, storageAttachment := range storageAttachments {
		err := st.DestroyStorageAttachment(
			storageAttachment.StorageInstance(), unit.UnitTag(),
		)
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
		return err
	}

	cancelled := ActionResults{Status: ActionCancelled, Message: "unit removed"}
	for _, action := range actions {
		if _, err = action.Finish(cancelled); err != nil {
			return err
		}
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
	if err := cleanupDyingMachineResources(machine); err != nil {
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
	coll, closer := st.getCollection(storageAttachmentsC)
	defer closer()

	var doc storageAttachmentDoc
	fields := bson.D{{"unitid", 1}}
	iter := coll.Find(bson.D{{"storageid", storageId}}).Select(fields).Iter()
	defer closeIter(iter, &err, "reading storage attachment document")
	for iter.Next(&doc) {
		unitTag := names.NewUnitTag(doc.Unit)
		if err := st.DestroyStorageAttachment(storageTag, unitTag); err != nil {
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
	coll, closer := st.getCollection(volumeAttachmentsC)
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
	coll, closer := st.getCollection(filesystemAttachmentsC)
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
