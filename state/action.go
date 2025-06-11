// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	jujutxn "github.com/juju/txn/v3"

	internallogger "github.com/juju/juju/internal/logger"
)

const (
	actionMarker = "_a_"
)

var actionLogger = internallogger.GetLogger("juju.state.action")

// ActionStatus represents the possible end states for an action.
type ActionStatus string

const (
	// ActionError signifies that the action did not get run due to an error.
	ActionError ActionStatus = "error"

	// ActionFailed signifies that the action did not complete successfully.
	ActionFailed ActionStatus = "failed"

	// ActionCompleted indicates that the action ran to completion as intended.
	ActionCompleted ActionStatus = "completed"

	// ActionCancelled means that the Action was cancelled before being run.
	ActionCancelled ActionStatus = "cancelled"

	// ActionPending is the default status when an Action is first queued.
	ActionPending ActionStatus = "pending"

	// ActionRunning indicates that the Action is currently running.
	ActionRunning ActionStatus = "running"

	// ActionAborting indicates that the Action is running but should be
	// aborted.
	ActionAborting ActionStatus = "aborting"

	// ActionAborted indicates the Action was aborted.
	ActionAborted ActionStatus = "aborted"
)

var activeStatus = set.NewStrings(
	string(ActionPending),
	string(ActionRunning),
	string(ActionAborting),
)

type actionNotificationDoc struct {
	// DocId is the composite _id that can be matched by an
	// idPrefixWatcher that is configured to watch for the
	// ActionReceiver Name() which makes up the first part of this
	// composite _id.
	DocId string `bson:"_id"`

	// ModelUUID is the model identifier.
	ModelUUID string `bson:"model-uuid"`

	// Receiver is the Name of the Unit or any other ActionReceiver for
	// which this notification is queued.
	Receiver string `bson:"receiver"`

	// ActionID is the unique identifier for the Action this notification
	// represents.
	ActionID string `bson:"actionid"`

	// Changed represents the time when the corresponding Action had a change
	// worthy of notifying on.
	// NOTE: changed should not be set on pending actions, see actionNotificationWatcher.
	Changed time.Time `bson:"changed,omitempty"`
}

type actionDoc struct {
	// DocId is the key for this document; it is a UUID.
	DocId string `bson:"_id"`

	// ModelUUID is the model identifier.
	ModelUUID string `bson:"model-uuid"`

	// Receiver is the Name of the Unit or any other ActionReceiver for
	// which this Action is queued.
	Receiver string `bson:"receiver"`

	// Name identifies the action that should be run; it should
	// match an action defined by the unit's charm.
	Name string `bson:"name"`

	// Parameters holds the action's parameters, if any; it should validate
	// against the schema defined by the named action in the unit's charm.
	Parameters map[string]interface{} `bson:"parameters"`

	// Parallel is true if this action can run in parallel with others
	// without requiring the mandatory acquisition of the machine lock.
	Parallel bool `bson:"parallel,omitempty"`

	// ExecutionGroup is used to group all actions which require the
	// same machine lock, ie actions in the same group cannot run in
	// in parallel with each other.
	ExecutionGroup string `bson:"execution-group,omitempty"`

	// Enqueued is the time the action was added.
	Enqueued time.Time `bson:"enqueued"`

	// Started reflects the time the action began running.
	Started time.Time `bson:"started"`

	// Completed reflects the time that the action was finished.
	Completed time.Time `bson:"completed"`

	// Operation is the parent operation of the action.
	Operation string `bson:"operation"`

	// Status represents the end state of the Action; ActionFailed for an
	// action that was removed prematurely, or that failed, and
	// ActionCompleted for an action that successfully completed.
	Status ActionStatus `bson:"status"`

	// Message captures any error returned by the action.
	Message string `bson:"message"`

	// Results are the structured results from the action.
	Results map[string]interface{} `bson:"results"`

	// Logs holds the progress messages logged by the action.
	Logs []ActionMessage `bson:"messages"`
}

// ActionMessage represents a progress message logged by an action.
type ActionMessage struct {
	MessageValue   string    `bson:"message"`
	TimestampValue time.Time `bson:"timestamp"`
}

// Timestamp returns the message timestamp.
func (m ActionMessage) Timestamp() time.Time {
	return m.TimestampValue
}

// Message returns the message string.
func (m ActionMessage) Message() string {
	return m.MessageValue
}

// action represents an instruction to do some "action" and is expected
// to match an action definition in a charm.
type action struct {
	st  *State
	doc actionDoc
}

// Refresh the contents of the action.
func (a *action) Refresh() error {
	actions, closer := a.st.db().GetCollection(actionsC)
	defer closer()
	id := a.Id()
	doc := actionDoc{}
	err := actions.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("action %q", id)
	}
	if err != nil {
		return errors.Annotatef(err, "cannot get action %q", id)
	}
	a.doc = doc
	return nil
}

// Id returns the local id of the Action.
func (a *action) Id() string {
	return a.st.localID(a.doc.DocId)
}

// Receiver returns the Name of the ActionReceiver for which this action
// is enqueued.  Usually this is a Unit Name().
func (a *action) Receiver() string {
	return a.doc.Receiver
}

// Name returns the name of the action, as defined in the charm.
func (a *action) Name() string {
	return a.doc.Name
}

// Parameters will contain a structure representing arguments or parameters to
// an action, and is expected to be validated by the Unit using the Charm
// definition of the Action.
func (a *action) Parameters() map[string]interface{} {
	return a.doc.Parameters
}

// Parallel returns true if the action can run without
// needed to acquire the machine lock.
func (a *action) Parallel() bool {
	return a.doc.Parallel
}

// ExecutionGroup is the group of actions which cannot
// execute in parallel with each other.
func (a *action) ExecutionGroup() string {
	return a.doc.ExecutionGroup
}

// Enqueued returns the time the action was added to state as a pending
// Action.
func (a *action) Enqueued() time.Time {
	return a.doc.Enqueued
}

// Started returns the time that the Action execution began.
func (a *action) Started() time.Time {
	return a.doc.Started
}

// Completed returns the completion time of the Action.
func (a *action) Completed() time.Time {
	return a.doc.Completed
}

// Status returns the final state of the action.
func (a *action) Status() ActionStatus {
	return a.doc.Status
}

// Results returns the structured output of the action and any error.
func (a *action) Results() (map[string]interface{}, string) {
	return a.doc.Results, a.doc.Message
}

// Tag implements the Entity interface and returns a names.Tag that
// is a names.ActionTag.
func (a *action) Tag() names.Tag {
	return a.ActionTag()
}

// ActionTag returns an ActionTag constructed from this action's
// Prefix and Sequence.
func (a *action) ActionTag() names.ActionTag {
	return names.NewActionTag(a.Id())
}

// Model returns the model associated with the action
func (a *action) Model() (*Model, error) {
	return a.st.Model()
}

// ActionResults is a data transfer object that holds the key Action
// output and results information.
type ActionResults struct {
	Status  ActionStatus           `json:"status"`
	Results map[string]interface{} `json:"results"`
	Message string                 `json:"message"`
}

// Begin marks an action as running, and logs the time it was started.
// It asserts that the action is currently pending.
func (a *action) Begin() (Action, error) {
	m, err := a.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	parentOperation, err := m.Operation(a.doc.Operation)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}
	startedTime := a.st.nowToTheSecond()
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If this is the first action to be marked as running
		// for the parent operation, the operation itself is
		// marked as running also.
		var updateOperationOp *txn.Op
		var err error
		if parentOperation != nil {
			if attempt > 0 {
				err = parentOperation.Refresh()
				if err != nil && !errors.Is(err, errors.NotFound) {
					return nil, errors.Trace(err)
				}
			}
			parentOpDetails, ok := parentOperation.(*operation)
			if !ok {
				return nil, errors.Errorf("parentOperation must be of type operation")
			}
			if err == nil && parentOpDetails.doc.Status == ActionPending {
				updateOperationOp = &txn.Op{
					C:      operationsC,
					Id:     a.st.docID(parentOperation.Id()),
					Assert: bson.D{{"status", ActionPending}},
					Update: bson.D{{"$set", bson.D{
						{"status", ActionRunning},
						{"started", startedTime},
					}}},
				}
			}
		}
		ops := []txn.Op{
			{
				C:      actionsC,
				Id:     a.doc.DocId,
				Assert: bson.D{{"status", ActionPending}},
				Update: bson.D{{"$set", bson.D{
					{"status", ActionRunning},
					{"started", startedTime},
				}}},
			}}
		if updateOperationOp != nil {
			ops = append(ops, *updateOperationOp)
		}
		return ops, nil
	}
	if err = m.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return m.Action(a.Id())
}

// Finish removes action from the pending queue and captures the output
// and end state of the action.
func (a *action) Finish(results ActionResults) (Action, error) {
	return a.removeAndLog(results.Status, results.Results, results.Message)
}

// Cancel or Abort the action.
func (a *action) Cancel() (Action, error) {
	m, err := a.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	parentOperation, err := m.Operation(a.doc.Operation)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	cancelTime := a.st.nowToTheSecond()
	removeAndLog := a.removeAndLogBuildTxn(ActionCancelled, nil, "action cancelled via the API",
		m, parentOperation, cancelTime)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		err := a.Refresh()
		if err != nil {
			return nil, errors.Trace(err)
		}
		switch a.Status() {
		case ActionRunning:
			ops := []txn.Op{
				{
					C:      actionsC,
					Id:     a.doc.DocId,
					Assert: bson.D{{"status", ActionRunning}},
					Update: bson.D{{"$set", bson.D{
						{"status", ActionAborting},
					}}},
				}, {
					C:  actionNotificationsC,
					Id: m.st.docID(ensureActionMarker(a.Receiver()) + a.Id()),
					Update: bson.D{{"$set", bson.D{
						{"changed", cancelTime},
					}}},
				},
			}
			return ops, nil
		case ActionPending:
			return removeAndLog(attempt)
		default:
			// Already done.
			return nil, nil
		}
	}
	if err = m.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return m.Action(a.Id())
}

// removeAndLog takes the action off of the pending queue, and creates
// an actionresult to capture the outcome of the action. It asserts that
// the action is not already completed.
func (a *action) removeAndLog(finalStatus ActionStatus, results map[string]interface{}, message string) (Action, error) {
	m, err := a.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	parentOperation, err := m.Operation(a.doc.Operation)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	completedTime := a.st.nowToTheSecond()
	buildTxn := a.removeAndLogBuildTxn(finalStatus, results, message, m, parentOperation, completedTime)
	if err = m.st.db().Run(buildTxn); err != nil {
		return nil, errors.Trace(err)
	}
	return m.Action(a.Id())
}

// removeAndLogBuildTxn is shared by Cancel and removeAndLog to correctly finalise an action and it's parent op.
func (a *action) removeAndLogBuildTxn(finalStatus ActionStatus, results map[string]interface{}, message string,
	m *Model, parentOperation Operation, completedTime time.Time) jujutxn.TransactionSource {
	return func(attempt int) ([]txn.Op, error) {
		// If the action is already finished, return an error.
		if a.Status() == finalStatus {
			return nil, errors.NewAlreadyExists(nil, fmt.Sprintf("action %v already has status %q", a.Id(), finalStatus))
		}
		if attempt > 0 {
			err := a.Refresh()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !activeStatus.Contains(string(a.Status())) {
				return nil, errors.NewAlreadyExists(nil, fmt.Sprintf("action %v is already finished with status %q", a.Id(), finalStatus))
			}
		}
		assertNotComplete := bson.D{{"status", bson.D{
			{"$nin", []interface{}{
				ActionCompleted,
				ActionCancelled,
				ActionFailed,
				ActionAborted,
				ActionError,
			}}}}}
		// If this is the last action to be marked as completed
		// for the parent operation, the operation itself is also
		// marked as complete.
		var updateOperationOp *txn.Op
		var err error
		if parentOperation != nil {
			parentOpDetails, ok := parentOperation.(*operation)
			if !ok {
				return nil, errors.Errorf("parentOperation must be of type operation")
			}
			if attempt > 0 {
				err = parentOperation.Refresh()
				if err != nil && !errors.Is(err, errors.NotFound) {
					return nil, errors.Trace(err)
				}
			}
			tasks := parentOpDetails.taskStatus
			statusStats := set.NewStrings(string(finalStatus))
			var numComplete int
			for _, status := range tasks {
				statusStats.Add(string(status))
				if !activeStatus.Contains(string(status)) {
					numComplete++
				}
			}
			spawnedTaskCount := parentOpDetails.doc.SpawnedTaskCount
			if numComplete == spawnedTaskCount-1 {
				// Set the operation status based on the individual
				// task status values. eg if any task is failed,
				// the entire operation is considered failed.
				finalOperationStatus := finalStatus
				for _, s := range statusCompletedOrder {
					if statusStats.Contains(string(s)) {
						finalOperationStatus = s
						break
					}
				}
				if finalOperationStatus == ActionCompleted && parentOpDetails.Fail() != "" {
					// If an action fails enqueuing, there may not be a doc
					// to reference. It will only be noted in the operation
					// fail string.
					finalOperationStatus = ActionError
				}
				updateOperationOp = &txn.Op{
					C:  operationsC,
					Id: a.st.docID(parentOperation.Id()),
					Assert: append(assertNotComplete,
						bson.DocElem{"complete-task-count", bson.D{{"$eq", spawnedTaskCount - 1}}}),
					Update: bson.D{{"$set", bson.D{
						{"status", finalOperationStatus},
						{"completed", completedTime},
						{"complete-task-count", spawnedTaskCount},
					}}},
				}
			} else {
				updateOperationOp = &txn.Op{
					C:  operationsC,
					Id: a.st.docID(parentOperation.Id()),
					Assert: append(assertNotComplete,
						bson.DocElem{"complete-task-count",
							bson.D{{"$lt", spawnedTaskCount - 1}}}),
					Update: bson.D{{"$inc", bson.D{
						{"complete-task-count", 1},
					}}},
				}
			}
		}
		ops := []txn.Op{
			{
				C:      actionsC,
				Id:     a.doc.DocId,
				Assert: assertNotComplete,
				Update: bson.D{{"$set", bson.D{
					{"status", finalStatus},
					{"message", message},
					{"results", results},
					{"completed", completedTime},
				}}},
			}, {
				C:      actionNotificationsC,
				Id:     m.st.docID(ensureActionMarker(a.Receiver()) + a.Id()),
				Remove: true,
			}}
		if updateOperationOp != nil {
			ops = append(ops, *updateOperationOp)
		}
		return ops, nil
	}
}

// Messages returns the action's progress messages.
func (a *action) Messages() []ActionMessage {
	// Timestamps are not decoded as UTC, so we need to convert :-(
	result := make([]ActionMessage, len(a.doc.Logs))
	for i, m := range a.doc.Logs {
		result[i] = ActionMessage{
			MessageValue:   m.MessageValue,
			TimestampValue: m.TimestampValue.UTC(),
		}
	}
	return result
}

// Log adds message to the action's progress message array.
func (a *action) Log(message string) error {
	// Just to ensure we do not allow bad actions to fill up disk.
	// 1000 messages should be enough for anyone.
	if len(a.doc.Logs) > 1000 {
		logger.Warningf(context.TODO(), "exceeded 1000 log messages, action may be stuck")
		return nil
	}
	m, err := a.st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	buildTxn := func(attempt int) ([]txn.Op, error) {
		if attempt > 0 {
			anAction, err := m.Action(a.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			a = anAction.(*action)
		}
		if s := a.Status(); s != ActionRunning && s != ActionAborting {
			return nil, errors.Errorf("cannot log message to task %q with status %v", a.Id(), s)
		}
		ops := []txn.Op{
			{
				C:  actionsC,
				Id: a.doc.DocId,
				Assert: bson.D{{"$or", []bson.D{
					{{"status", ActionRunning}},
					{{"status", ActionAborting}},
				}}},
				Update: bson.D{{"$push", bson.D{
					{"messages", ActionMessage{MessageValue: message, TimestampValue: a.st.nowToTheSecond().UTC()}},
				}}},
			}}
		return ops, nil
	}
	err = a.st.db().Run(buildTxn)
	return errors.Trace(err)
}

// newAction builds an Action for the given State and actionDoc.
func newAction(st *State, adoc actionDoc) Action {
	return &action{
		st:  st,
		doc: adoc,
	}
}

var ensureActionMarker = ensureSuffixFn(actionMarker)

// Action returns an Action by Id.
func (m *Model) Action(id string) (Action, error) {
	actionLogger.Tracef(context.TODO(), "Action() %q", id)
	st := m.st
	actions, closer := st.db().GetCollection(actionsC)
	defer closer()

	doc := actionDoc{}
	err := actions.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("action %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get action %q", id)
	}
	actionLogger.Tracef(context.TODO(), "Action() %q found %+v", id, doc)
	return newAction(st, doc), nil
}

// AllActions returns all Actions.
func (m *Model) AllActions() ([]Action, error) {
	actionLogger.Tracef(context.TODO(), "AllActions()")
	actions, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	results := []Action{}
	docs := []actionDoc{}
	err := actions.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all actions")
	}
	for _, doc := range docs {
		results = append(results, newAction(m.st, doc))
	}
	return results, nil
}

// ActionByTag returns an Action given an ActionTag.
func (st *State) ActionByTag(tag names.ActionTag) (Action, error) {
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return m.Action(tag.Id())
}

// FindActionsByName finds Actions with the given name.
func (m *Model) FindActionsByName(name string) ([]Action, error) {
	var results []Action
	var doc actionDoc

	actions, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	iter := actions.Find(bson.D{{"name", name}}).Iter()
	for iter.Next(&doc) {
		results = append(results, newAction(m.st, doc))
	}
	return results, errors.Trace(iter.Close())
}

// matchingActions finds actions that match ActionReceiver.
func (st *State) matchingActions(ar ActionReceiver) ([]Action, error) {
	return st.matchingActionsByReceiverId(ar.Tag().Id())
}

// matchingActionsByReceiverId finds actions that match ActionReceiver name.
func (st *State) matchingActionsByReceiverId(id string) ([]Action, error) {
	var doc actionDoc
	var actions []Action

	actionsCollection, closer := st.db().GetCollection(actionsC)
	defer closer()

	iter := actionsCollection.Find(bson.D{{"receiver", id}}).Iter()
	for iter.Next(&doc) {
		actions = append(actions, newAction(st, doc))
	}
	return actions, errors.Trace(iter.Close())
}

// matchingActionsPending finds actions that match ActionReceiver and
// that are pending.
func (st *State) matchingActionsPending(ar ActionReceiver) ([]Action, error) {
	pending := bson.D{{"status", ActionPending}}
	return st.matchingActionsByReceiverAndStatus(ar.Tag(), pending)
}

// matchingActionsRunning finds actions that match ActionReceiver and
// that are running.
func (st *State) matchingActionsRunning(ar ActionReceiver) ([]Action, error) {
	running := bson.D{{"$or", []bson.D{
		{{"status", ActionRunning}},
		{{"status", ActionAborting}},
	}}}
	return st.matchingActionsByReceiverAndStatus(ar.Tag(), running)
}

// matchingActionsCompleted finds actions that match ActionReceiver and
// that are complete.
func (st *State) matchingActionsCompleted(ar ActionReceiver) ([]Action, error) {
	completed := bson.D{{"$or", []bson.D{
		{{"status", ActionCompleted}},
		{{"status", ActionCancelled}},
		{{"status", ActionFailed}},
		{{"status", ActionAborted}},
	}}}
	return st.matchingActionsByReceiverAndStatus(ar.Tag(), completed)
}

// matchingActionsByReceiverAndStatus finds actionNotifications that
// match ActionReceiver.
func (st *State) matchingActionsByReceiverAndStatus(tag names.Tag, statusCondition bson.D) ([]Action, error) {
	var doc actionDoc
	var actions []Action

	actionsCollection, closer := st.db().GetCollection(actionsC)
	defer closer()

	sel := append(bson.D{{"receiver", tag.Id()}}, statusCondition...)
	iter := actionsCollection.Find(sel).Iter()

	for iter.Next(&doc) {
		actions = append(actions, newAction(st, doc))
	}
	return actions, errors.Trace(iter.Close())
}
