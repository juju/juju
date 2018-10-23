// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// upgradeSeriesLockDoc holds the attributes relevant to lock a machine during a
// series update of a machine
type upgradeSeriesLockDoc struct {
	Id            string                             `bson:"machine-id"`
	ToSeries      string                             `bson:"to-series"`
	FromSeries    string                             `bson:"from-series"`
	MachineStatus model.UpgradeSeriesStatus          `bson:"machine-status"`
	Messages      []UpgradeSeriesMessage             `bson:"messages"`
	TimeStamp     time.Time                          `bson:"timestamp"`
	UnitStatuses  map[string]UpgradeSeriesUnitStatus `bson:"unit-statuses"`
}

type UpgradeSeriesUnitStatus struct {
	Status    model.UpgradeSeriesStatus
	Timestamp time.Time
}

// UpgradeSeriesMessage holds a message detailing why the upgrade series status
// was updated. This format of this message should be a single sentence similar
// to logging message. The string is accompanied by a timestamp and a boolean
// value indicating whether or not the message has been observed by a client.
type UpgradeSeriesMessage struct {
	Message   string    `bson:"message"`
	Timestamp time.Time `bson:"timestamp"`
	Seen      bool      `bson:"seen"`
}

func newUpgradeSeriesMessage(name string, message string, timestamp time.Time) UpgradeSeriesMessage {
	taggedMessage := fmt.Sprintf("%s %s", name, message)
	return UpgradeSeriesMessage{
		Message:   taggedMessage,
		Timestamp: timestamp,
		Seen:      false,
	}
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
	if errors.IsNotFound(err) {
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
	timestamp := bson.Now()
	message := fmt.Sprintf("started upgrade series from series %s to series %s", m.Series(), toSeries)
	updateMessage := newUpgradeSeriesMessage(m.Tag().String(), message, timestamp)
	return &upgradeSeriesLockDoc{
		Id:            m.Id(),
		ToSeries:      toSeries,
		FromSeries:    m.Series(),
		MachineStatus: model.UpgradeSeriesPrepareStarted,
		UnitStatuses:  unitStatuses,
		TimeStamp:     timestamp,
		Messages:      []UpgradeSeriesMessage{updateMessage},
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

// UpgradeSeriesTarget returns the series
// that the machine is being upgraded to.
func (m *Machine) UpgradeSeriesTarget() (string, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return "", errors.Trace(err)
	}
	return lock.ToSeries, nil
}

// StartUpgradeSeriesUnitCompletion notifies units that an upgrade-series
// workflow is ready for its "completion" phase.
func (m *Machine) StartUpgradeSeriesUnitCompletion(message string) error {
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
		timestamp := bson.Now()
		lock.Messages = append(lock.Messages, newUpgradeSeriesMessage(m.Tag().String(), message, timestamp))
		lock.TimeStamp = timestamp
		changeCount := 0
		for unitName, us := range lock.UnitStatuses {
			if us.Status == model.UpgradeSeriesCompleteStarted {
				continue
			}
			us.Status = model.UpgradeSeriesCompleteStarted
			us.Timestamp = timestamp
			lock.UnitStatuses[unitName] = us
			changeCount++
		}
		if changeCount == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		return startUpgradeSeriesUnitCompletionTxnOps(m.doc.Id, lock), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func startUpgradeSeriesUnitCompletionTxnOps(machineDocID string, lock *upgradeSeriesLockDoc) []txn.Op {
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
			Update: bson.D{{"$set", bson.D{
				{statusField, lock.UnitStatuses},
				{"timestamp", lock.TimeStamp},
				{"messages", lock.Messages}}}},
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
		timestamp := bson.Now()
		message := newUpgradeSeriesMessage(m.Tag().String(), "complete phase started", timestamp)
		return completeUpgradeSeriesTxnOps(m.doc.Id, timestamp, message), nil
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

func completeUpgradeSeriesTxnOps(machineDocID string, timestamp time.Time, message UpgradeSeriesMessage) []txn.Op {
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
			Update: bson.D{
				{"$set", bson.D{
					{"machine-status", model.UpgradeSeriesCompleteStarted},
					{"timestamp", timestamp},
				}},
				{"$push", bson.D{{"messages", message}}}},
		},
	}
}

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
			return nil, jujutxn.ErrNoOperations
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

func (m *Machine) UpgradeSeriesStatus() (model.UpgradeSeriesStatus, error) {
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

// UnitStatus returns the series upgrade status for the input unit.
func (m *Machine) UpgradeSeriesUnitStatus(unitName string) (model.UpgradeSeriesStatus, error) {
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

// UnitStatus returns the unit statuses from the upgrade-series lock
// for this machine.
func (m *Machine) UpgradeSeriesUnitStatuses() (map[string]UpgradeSeriesUnitStatus, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock.UnitStatuses, nil
}

// SetUpgradeSeriesUnitStatus sets the status of a series upgrade for a unit.
func (m *Machine) SetUpgradeSeriesUnitStatus(unitName string, status model.UpgradeSeriesStatus, message string) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		canUpdate, err := m.verifyUnitUpgradeSeriesStatus(unitName, status)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !canUpdate {
			return nil, jujutxn.ErrNoOperations
		}
		timestamp := bson.Now()
		updateMessage := newUpgradeSeriesMessage(unitName, message, timestamp)
		return setUpgradeSeriesTxnOps(m.doc.Id, unitName, status, timestamp, updateMessage)
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

// verifyUnitUpgradeSeriesStatus returns a boolean indicating whether or not it
// is safe to update the UpgradeSeriesStatus of a lock.
func (m *Machine) verifyUnitUpgradeSeriesStatus(unitName string, status model.UpgradeSeriesStatus) (bool, error) {
	lock, err := m.getUpgradeSeriesLock()
	if err != nil {
		return false, err
	}
	us, ok := lock.UnitStatuses[unitName]
	if !ok {
		return false, errors.NotFoundf(unitName)
	}

	comp, err := model.CompareUpgradeSeriesStatus(us.Status, status)
	if err != nil {
		return false, err
	}
	return comp == -1, nil
}

// [TODO](externalreality): move some/all of these parameters into an argument structure.
func setUpgradeSeriesTxnOps(
	machineDocID,
	unitName string,
	status model.UpgradeSeriesStatus,
	timestamp time.Time,
	message UpgradeSeriesMessage,
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
				{"$set", bson.D{
					{unitStatusField, status},
					{"timestamp", timestamp},
					{unitTimestampField, timestamp}}},
				{"$push", bson.D{{"messages", message}}},
			},
		},
	}, nil
}

// SetUpgradeSeriesStatus sets the status of the machine in
// the upgrade-series lock.
func (m *Machine) SetUpgradeSeriesStatus(status model.UpgradeSeriesStatus, message string) error {
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
		timestamp := bson.Now()
		upgradeSeriesMessage := newUpgradeSeriesMessage(m.Tag().String(), message, timestamp)
		return setMachineUpgradeSeriesTxnOps(m.doc.Id, status, upgradeSeriesMessage, timestamp), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

// GetUpgradeSeriesMessages returns all 'unseen' upgrade series
// notifications sorted by timestamp.
func (m *Machine) GetUpgradeSeriesMessages() ([]string, bool, error) {
	lock, err := m.getUpgradeSeriesLock()
	if errors.IsNotFound(err) {
		// If the lock is not found here then there are no more messages
		return nil, true, nil
	}
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	// finished means that a subsequent call to this method, while the
	// Machine Lock is of a similar Machine Status, would return no
	// additional messages (notifications). Since the value of this variable
	// is returned, callers may choose to close streams or stop watchers
	// based on this information.
	finished := lock.MachineStatus == model.UpgradeSeriesCompleted ||
		lock.MachineStatus == model.UpgradeSeriesPrepareCompleted
	// Filter seen messages
	unseenMessages := make([]UpgradeSeriesMessage, 0)
	for _, upgradeSeriesMessage := range lock.Messages {
		if !upgradeSeriesMessage.Seen {
			unseenMessages = append(unseenMessages, upgradeSeriesMessage)
		}
	}
	if len(unseenMessages) == 0 {
		return []string{}, finished, nil
	}
	sort.Slice(unseenMessages, func(i, j int) bool {
		return unseenMessages[i].Timestamp.Before(unseenMessages[j].Timestamp)
	})
	messages := make([]string, 0)
	for _, unseenMessage := range unseenMessages {
		messages = append(messages, unseenMessage.Message)
	}
	err = m.SetUpgradeSeriesMessagesAsSeen(lock.Messages)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return messages, finished, nil
}

// SetUpgradeSeriesMessagesAsSeen marks a given upgrade series messages as
// having been seen by a client of the API.
func (m *Machine) SetUpgradeSeriesMessagesAsSeen(messages []UpgradeSeriesMessage) error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			if err := m.Refresh(); err != nil {
				return nil, errors.Trace(err)
			}
		}
		if len(messages) == 0 {
			return nil, jujutxn.ErrNoOperations
		}
		if err := m.isStillAlive(); err != nil {
			return nil, errors.Trace(err)
		}
		return setUpgradeSeriesMessageTxnOps(m.doc.Id, messages, true), nil
	}
	err := m.st.db().Run(buildTxn)
	if err != nil {
		err = onAbort(err, ErrDead)
		return err
	}
	return nil
}

func setUpgradeSeriesMessageTxnOps(machineDocID string, messages []UpgradeSeriesMessage, seen bool) []txn.Op {
	ops := []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
	}
	fields := bson.D{}
	for i := range messages {
		field := fmt.Sprintf("messages.%d.seen", i)
		fields = append(fields, bson.DocElem{field, seen})
	}
	ops = append(ops, txn.Op{
		C:      machineUpgradeSeriesLocksC,
		Id:     machineDocID,
		Update: bson.D{{"$set", fields}},
	})
	return ops
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
		return nil, errors.NotFoundf("upgrade lock for machine %q", m)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "retrieving upgrade series lock for machine %v", m.Id())
	}
	return &lock, nil
}

func setMachineUpgradeSeriesTxnOps(machineDocID string, status model.UpgradeSeriesStatus, message UpgradeSeriesMessage, timestamp time.Time) []txn.Op {
	field := "machine-status"

	return []txn.Op{
		{
			C:      machinesC,
			Id:     machineDocID,
			Assert: isAliveDoc,
		},
		{
			C:  machineUpgradeSeriesLocksC,
			Id: machineDocID,
			Update: bson.D{
				{"$set", bson.D{{field, status}, {"timestamp", timestamp}}},
				{"$push", bson.D{{"messages", message}}},
			},
		},
	}
}
