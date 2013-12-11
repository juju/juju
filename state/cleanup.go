package state

import (
	"fmt"

	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
)

// cleanupDoc represents a potentially large set of documents that should be
// removed.
type cleanupDoc struct {
	Id     bson.ObjectId `bson:"_id"`
	Kind   string
	Prefix string
}

// newCleanupOp returns a txn.Op that creates a cleanup document with a unique
// id and the supplied kind and prefix.
func (st *State) newCleanupOp(kind, prefix string) txn.Op {
	doc := &cleanupDoc{
		Id:     bson.NewObjectId(),
		Kind:   kind,
		Prefix: prefix,
	}
	return txn.Op{
		C:      st.cleanups.Name,
		Id:     doc.Id,
		Insert: doc,
	}
}

// NeedsCleanup returns true if documents previously marked for removal exist.
func (st *State) NeedsCleanup() (bool, error) {
	count, err := st.cleanups.Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Cleanup removes all documents that were previously marked for removal, if
// any such exist. It should be called periodically by at least one element
// of the system.
func (st *State) Cleanup() error {
	doc := cleanupDoc{}
	iter := st.cleanups.Find(nil).Iter()
	for iter.Next(&doc) {
		var err error
		logger.Debugf("running %q cleanup: %q", doc.Kind, doc.Prefix)
		switch doc.Kind {
		case "settings":
			err = st.cleanupSettings(doc.Prefix)
		case "units":
			err = st.cleanupUnits(doc.Prefix)
		case "services":
			err = st.cleanupServices()
		case "machine":
			err = st.cleanupMachine(doc.Prefix)
		default:
			err = fmt.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			logger.Warningf("cleanup failed: %v", err)
			continue
		}
		ops := []txn.Op{{
			C:      st.cleanups.Name,
			Id:     doc.Id,
			Remove: true,
		}}
		if err := st.runTransaction(ops); err != nil {
			logger.Warningf("cannot remove empty cleanup document: %v", err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read cleanup document: %v", err)
	}
	return nil
}

func (st *State) cleanupSettings(prefix string) error {
	// Documents marked for cleanup are not otherwise referenced in the
	// system, and will not be under watch, and are therefore safe to
	// delete directly.
	sel := D{{"_id", D{{"$regex", "^" + prefix}}}}
	if count, err := st.settings.Find(sel).Count(); err != nil {
		return fmt.Errorf("cannot detect cleanup targets: %v", err)
	} else if count != 0 {
		if _, err := st.settings.RemoveAll(sel); err != nil {
			return fmt.Errorf("cannot remove documents marked for cleanup: %v", err)
		}
	}
	return nil
}

// cleanupServices sets all services to Dying, if they are not already Dying
// or Dead. It's expected to be used when an environment is destroyed.
func (st *State) cleanupServices() error {
	// This won't miss services, because a Dying environment cannot have
	// services added to it. But we do have to remove the services themselves
	// via individual transactions, because they could be in any state at all.
	service := &Service{st: st}
	sel := D{{"life", Alive}}
	iter := st.services.Find(sel).Iter()
	for iter.Next(&service.doc) {
		if err := service.Destroy(); err != nil {
			return err
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read service document: %v", err)
	}
	return nil
}

// cleanupUnits sets all units with the given prefix to Dying, if they are not
// already Dying or Dead. It's expected to be used when a service is destroyed.
func (st *State) cleanupUnits(prefix string) error {
	// This won't miss units, because a Dying service cannot have units added
	// to it. But we do have to remove the units themselves via individual
	// transactions, because they could be in any state at all.
	unit := &Unit{st: st}
	sel := D{{"_id", D{{"$regex", "^" + prefix}}}, {"life", Alive}}
	iter := st.units.Find(sel).Iter()
	for iter.Next(&unit.doc) {
		if err := unit.Destroy(); err != nil {
			return err
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read unit document: %v", err)
	}
	return nil
}

// cleanupMachine systematically destroys and removes all entities that
// depend upon the supplied machine, and removes the machine from state. It's
// expected to be used in response to destroy-machine --force.
func (st *State) cleanupMachine(machineId string) error {
	machine, err := st.Machine(machineId)
	if errors.IsNotFoundError(err) {
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
	// We need to refresh the machine at this point, because the local copy
	// of the document will not reflect changes caused by the unit cleanups
	// above, and may thus fail immediately.
	if err := machine.Refresh(); errors.IsNotFoundError(err) {
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
	return machine.EnsureDead()

	// Note that we do *not* remove the machine entirely: we leave it for the
	// provisioner to clean up, so that we don't end up with an unreferenced
	// instance that would otherwise be ignored when in provisioner-safe-mode.
}

// cleanupContainers recursively calls cleanupMachine on the supplied
// machine's containers, and removes them from state entirely.
func (st *State) cleanupContainers(machine *Machine) error {
	containerIds, err := machine.Containers()
	if errors.IsNotFoundError(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, containerId := range containerIds {
		if err := st.cleanupMachine(containerId); err != nil {
			return err
		}
		container, err := st.Machine(containerId)
		if errors.IsNotFoundError(err) {
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

// obliterateUnit removes a unit from state completely. It is not safe or
// sane to obliterate any unit in isolation; its only reasonable use is in
// the context of machine obliteration, in which we can be sure that unclean
// shutdown of units is not going to leave a machine in a difficult state.
func (st *State) obliterateUnit(unitName string) error {
	unit, err := st.Unit(unitName)
	if errors.IsNotFoundError(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Unlike the machine, we *can* always destroy the unit, and (at least)
	// prevent further dependencies being added. If we're really lucky, the
	// unit will be removed immediately.
	if err := unit.Destroy(); err != nil {
		return err
	}
	if err := unit.Refresh(); errors.IsNotFoundError(err) {
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
