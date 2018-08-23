// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/collections/set"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
)

// upgradeSeriesLockDoc holds the attributes relevant to lock a machine during a
// series update of a machine
type upgradeSeriesLockDoc struct {
	Id            string                             `bson:"machine-id"`
	ToSeries      string                             `bson:"to-series"`
	FromSeries    string                             `bson:"from-series"`
	MachineStatus model.UpgradeSeriesStatus          `bson:"machine-status"`
	UnitStatuses  map[string]UpgradeSeriesUnitStatus `bson:"unit-statuses"`
}

type UpgradeSeriesUnitStatus struct {
	Status model.UpgradeSeriesStatus

	// The time that the status was set
	Timestamp time.Time
}

// CreateUpgradeSeriesLock create a prepare lock for series upgrade. If
// this item exists in the database for a given machine it indicates that a
// machine's operating system is being upgraded from one series to another;
// for example, from xenial to bionic.
func (m *Machine) CreateUpgradeSeriesLock(unitNames []string, toSeries string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		locked, err := m.IsLocked()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if locked {
			return nil, errors.AlreadyExistsf("upgrade series lock for machine %q", m)
		}
		if err = m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		// Exit early if the Machine series doesn't need to change.
		fromSeries := m.Series()
		if fromSeries == toSeries {
			return nil, errors.Trace(errors.Errorf("machine %s already at series %s", m.Id(), toSeries))
		}
		// If the units have changed, the verification is no longer valid.
		changed, err := m.unitsHaveChanged(unitNames)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if changed {
			return nil, errors.Errorf("Units have changed, please retry (%v)", unitNames)
		}
		data := m.prepareUpgradeSeriesLock(unitNames, toSeries)
		return createUpgradeSeriesLockTxnOps(m.doc.Id, data), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		logger.Errorf("cannot prepare series upgrade for machine %q: %v", m, err)
		return err
	}

	return nil
}

// IsLocked determines if a machine is locked for upgrade series.
func (m *Machine) IsLocked() (bool, error) {
	_, err := m.getUpgradeSeriesLock()
	if err == nil {
		return true, nil
	}
	if errors.IsBadRequest(err) {
		return false, nil
	}
	return false, errors.Trace(err)
}

func (m *Machine) unitsHaveChanged(unitNames []string) (bool, error) {
	curUnits, err := m.Units()
	if err != nil {
		return true, err
	}
	if len(curUnits) == 0 && len(unitNames) == 0 {
		return false, nil
	}
	if len(curUnits) != len(unitNames) {
		return true, nil
	}
	curUnitSet := set.NewStrings()
	for _, unit := range curUnits {
		curUnitSet.Add(unit.Name())
	}
	unitNameSet := set.NewStrings(unitNames...)
	return !unitNameSet.Difference(curUnitSet).IsEmpty(), nil
}

func (m *Machine) prepareUpgradeSeriesLock(unitNames []string, toSeries string) *upgradeSeriesLockDoc {
	unitStatuses := make(map[string]UpgradeSeriesUnitStatus, len(unitNames))
	for _, name := range unitNames {
		unitStatuses[name] = UpgradeSeriesUnitStatus{Status: model.UpgradeSeriesPrepareStarted, Timestamp: bson.Now()}
	}
	return &upgradeSeriesLockDoc{
		Id:            m.Id(),
		ToSeries:      toSeries,
		FromSeries:    m.Series(),
		MachineStatus: model.UpgradeSeriesPrepareStarted,
		UnitStatuses:  unitStatuses,
	}
}

func createUpgradeSeriesLockTxnOps(machineDocId string, data *upgradeSeriesLockDoc) []txn.Op {
	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocId,
			Assert: isAliveDoc,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     machineDocId,
			Assert: txn.DocMissing,
			Insert: data,
		},
	}
}

// StartUpgradeSeriesUnitCompletion notifies units and machines that an
// upgrade-series workflow is ready for its "completion" phase.
func (m *Machine) StartUpgradeSeriesUnitCompletion() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		lock, err := m.getUpgradeSeriesLock()
		if err != nil {
			return nil, err
		}
		if lock.MachineStatus != model.UpgradeSeriesCompleteStarted {
			return nil, fmt.Errorf("machine %q can not complete its unit, the machine has not yet been marked as completed", m.Id())
		}
		for unitName, us := range lock.UnitStatuses {
			us.Status = model.UpgradeSeriesCompleteStarted
			lock.UnitStatuses[unitName] = us
		}
		return startUpgradeSeriesUnitCompletionTxnOps(m.doc.Id, lock.UnitStatuses), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func startUpgradeSeriesUnitCompletionTxnOps(machineDocID string, units map[string]UpgradeSeriesUnitStatus) []txn.Op {
	statusField := "unit-statuses"
	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     machineDocID,
			Assert: bson.D{{"machine-status", model.UpgradeSeriesCompleteStarted}},
			Update: bson.D{{"$set", bson.D{{statusField, units}}}},
		},
	}
}

// CompleteUpgradeSeries notifies units and machines that an upgrade series is
// ready for its "completion" phase.
func (m *Machine) CompleteUpgradeSeries() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		readyForCompletion, err := m.isReadyForCompletion()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !readyForCompletion {
			return nil, fmt.Errorf("machine %q can not complete, it is either not prepared or already completed", m.Id())
		}
		return completeUpgradeSeriesTxnOps(m.doc.Id), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func (m *Machine) isReadyForCompletion() (bool, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return false, err
	}
	return lock.MachineStatus == model.UpgradeSeriesPrepareCompleted, nil
}

func completeUpgradeSeriesTxnOps(machineDocID string) []txn.Op {
	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     machineDocID,
			Assert: bson.D{{"machine-status", model.UpgradeSeriesPrepareCompleted}},
			Update: bson.D{{"$set", bson.D{{"machine-status", model.UpgradeSeriesCompleteStarted}}}},
		},
	}
}

// [TODO](externalreality) We still need this, eventually the lock is going to cleaned up
// RemoveUpgradeSeriesLock removes a series upgrade prepare lock for a
// given machine.
func (m *Machine) RemoveUpgradeSeriesLock() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		locked, err := m.IsLocked()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !locked {
			return nil, errors.NotFoundf("upgrade series lock for machine %q", m)
		}
		return removeUpgradeSeriesLockTxnOps(m.doc.Id), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}

	return nil
}

func removeUpgradeSeriesLockTxnOps(machineDocId string) []txn.Op {
	return []txn.Op{
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     machineDocId,
			Assert: txn.DocExists,
			Remove: true,
		},
	}
}

// MachineUpgradeSeriesStatus returns the upgrade-series status of a machine.
// TODO (manadart 2018-08-07) This should be renamed to UpgradeSeriesStatus,
// and the unit-based methods renamed to indicate their context.
// The translation code can be removed once the old->new bootstrap is no
// longer required.
func (m *Machine) MachineUpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
	coll, closer := m.st.db().GetCollection(machineUpgradeSeriesLocksC)
	defer closer()

	var lock upgradeSeriesLockDoc
	err := coll.FindId(m.Id()).One(&lock)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("upgrade series lock for machine %q", m.Id())
	}
	if err != nil {
		return "", errors.Trace(err)
	}

	return lock.MachineStatus, errors.Trace(err)
}

// UpgradeSeriesStatus returns the series upgrade status for the input unit.
func (m *Machine) UpgradeSeriesStatus(unitName string) (model.UpgradeSeriesStatus, error) {
	coll, closer := m.st.db().GetCollection(machineUpgradeSeriesLocksC)
	defer closer()

	var lock upgradeSeriesLockDoc
	err := coll.FindId(m.Id()).One(&lock)
	if err == mgo.ErrNotFound {
		return "", errors.NotFoundf("upgrade series lock for machine %q", m.Id())
	}
	if err != nil {
		return "", errors.Trace(err)
	}
	if _, ok := lock.UnitStatuses[unitName]; !ok {
		return "", errors.NotFoundf("unit %q of machine %q", unitName, m.Id())
	}

	return lock.UnitStatuses[unitName].Status, nil
}

// UpgradeSeriesStatus returns the unit statuses from the upgrade-series lock
// for this machine.
func (m *Machine) UpgradeSeriesUnitStatuses() (map[string]UpgradeSeriesUnitStatus, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock.UnitStatuses, nil
}

// SetUpgradeSeriesStatus sets the status of a series upgrade for a unit.
func (m *Machine) SetUpgradeSeriesStatus(unitName string, status model.UpgradeSeriesStatus) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		statusSet, err := m.isUnitUpgradeSeriesStatusSet(unitName, status)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if statusSet {
			return nil, jujutxn.ErrNoOperations
		}
		return setUpgradeSeriesTxnOps(m.doc.Id, unitName, status, bson.Now())
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func (m *Machine) isUnitUpgradeSeriesStatusSet(unitName string, status model.UpgradeSeriesStatus) (bool, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return false, err
	}
	us, ok := lock.UnitStatuses[unitName]
	if !ok {
		return false, errors.NotFoundf(unitName)
	}
	return us.Status == status, nil
}

// [TODO](externalreality): move some/all of these parameters into an argument structure.
func setUpgradeSeriesTxnOps(
	machineDocID, unitName string, status model.UpgradeSeriesStatus, timestamp time.Time,
) ([]txn.Op, error) {
	statusField := "unit-statuses"
	unitStatusField := fmt.Sprintf("%s.%s.status", statusField, unitName)
	unitTimestampField := fmt.Sprintf("%s.%s.timestamp", statusField, unitName)
	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
		{
			C:  machineUpgradeSeriesLocksC,
			Id: machineDocID,
			Assert: bson.D{{"$and", []bson.D{
				{{statusField, bson.D{{"$exists", true}}}}, // if it doesn't exist something is wrong
				{{unitStatusField, bson.D{{"$ne", status}}}}}}},
			Update: bson.D{
				{"$set", bson.D{{unitStatusField, status}, {unitTimestampField, timestamp}}}},
		},
	}, nil
}

// SetUpgradeSeriesStatus sets the machine status of a series upgrade.
// TODO (manadart 2018-08-07) This should be renamed to UpgradeSeriesStatus,
// and the unit-based methods renamed to indicate their context.
// The translation code can be removed once the old->new bootstrap is no
// longer required.
func (m *Machine) SetMachineUpgradeSeriesStatus(status model.UpgradeSeriesStatus) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		statusSet, err := m.isMachineUpgradeSeriesStatusSet(status)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if statusSet {
			return nil, jujutxn.ErrNoOperations
		}
		return setMachineUpgradeSeriesTxnOps(m.doc.Id, status), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func (m *Machine) isMachineUpgradeSeriesStatusSet(status model.UpgradeSeriesStatus) (bool, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return false, err
	}

	return lock.MachineStatus == status, nil
}

func (m *Machine) getUpgradeSeriesLock() (*upgradeSeriesLockDoc, error) {
	coll, closer := m.st.db().GetCollection(machineUpgradeSeriesLocksC)
	defer closer()

	var lock upgradeSeriesLockDoc
	err := coll.FindId(m.Id()).One(&lock)
	if err == mgo.ErrNotFound {
		return nil, errors.BadRequestf("machine %q is not locked for upgrade", m)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving upgrade series lock for machine %v: %v", m.Id())
	}
	return &lock, nil
}

func setMachineUpgradeSeriesTxnOps(machineDocID string, status model.UpgradeSeriesStatus) []txn.Op {
	field := "machine-status"

	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
		{
			C:      machineUpgradeSeriesLocksC,
			Id:     machineDocID,
			Update: bson.D{{"$set", bson.D{{field, status}}}},
		},
	}
}
