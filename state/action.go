// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn/v2"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	stateerrors "github.com/juju/juju/state/errors"
)

const (
	actionMarker = "_a_"
)

var (
	actionLogger = loggo.GetLogger("juju.state.action")

	// NewUUID wraps the utils.NewUUID() call, and exposes it as a var to
	// facilitate patching.
	NewUUID = func() (utils.UUID, error) { return utils.NewUUID() }
)

// ActionStatus represents the possible end states for an action.
type ActionStatus string

const (
	// ActionError signifies that the action did get run due to an error.
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
	if err != nil && !errors.IsNotFound(err) {
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
				if err != nil && !errors.IsNotFound(err) {
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
	if err != nil && !errors.IsNotFound(err) {
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
	if err != nil && !errors.IsNotFound(err) {
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
				if err != nil && !errors.IsNotFound(err) {
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
		logger.Warningf("exceeded 1000 log messages, action may be stuck")
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

// MinVersionSupportNewActionID should be un-exposed after 2.7 released.
// TODO(action): un-expose MinVersionSupportNewActionID and IsNewActionIDSupported and remove those helper functions using these two vars in tests from 2.7.0.
var MinVersionSupportNewActionID = version.MustParse("2.6.999")

// IsNewActionIDSupported checks if new action ID is supported for the specified version.
func IsNewActionIDSupported(ver version.Number) bool {
	return ver.Compare(MinVersionSupportNewActionID) >= 0
}

// newActionDoc builds the actionDoc with the given name and parameters.
func newActionDoc(mb modelBackend, operationID string, receiverTag names.Tag, actionName string, parameters map[string]interface{}, modelAgentVersion version.Number) (actionDoc, actionNotificationDoc, error) {
	prefix := ensureActionMarker(receiverTag.Id())
	var actionId string
	if IsNewActionIDSupported(modelAgentVersion) {
		id, err := sequenceWithMin(mb, "task", 1)
		if err != nil {
			return actionDoc{}, actionNotificationDoc{}, err
		}
		actionId = strconv.Itoa(id)
	} else {
		actionUUID, err := NewUUID()
		if err != nil {
			return actionDoc{}, actionNotificationDoc{}, err
		}
		actionId = actionUUID.String()
	}
	actionLogger.Debugf("newActionDoc name: '%s', receiver: '%s', actionId: '%s'", actionName, receiverTag, actionId)
	modelUUID := mb.modelUUID()
	return actionDoc{
			DocId:      mb.docID(actionId),
			ModelUUID:  modelUUID,
			Receiver:   receiverTag.Id(),
			Name:       actionName,
			Parameters: parameters,
			Enqueued:   mb.nowToTheSecond(),
			Operation:  operationID,
			Status:     ActionPending,
		}, actionNotificationDoc{
			DocId:     mb.docID(prefix + actionId),
			ModelUUID: modelUUID,
			Receiver:  receiverTag.Id(),
			ActionID:  actionId,
		}, nil
}

var ensureActionMarker = ensureSuffixFn(actionMarker)

// Action returns an Action by Id.
func (m *Model) Action(id string) (Action, error) {
	actionLogger.Tracef("Action() %q", id)
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
	actionLogger.Tracef("Action() %q found %+v", id, doc)
	return newAction(st, doc), nil
}

// AllActions returns all Actions.
func (m *Model) AllActions() ([]Action, error) {
	actionLogger.Tracef("AllActions()")
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
func (m *Model) ActionByTag(tag names.ActionTag) (Action, error) {
	return m.Action(tag.Id())
}

// FindActionTagsById finds Actions with ids that either
// share the supplied prefix (for deprecated UUIDs), or match
// the supplied id (for newer id integers). If passed an empty string
// match all Actions.
// It returns a list of corresponding ActionTags.
func (m *Model) FindActionTagsById(idValue string) ([]names.ActionTag, error) {
	actionLogger.Tracef("FindActionTagsById() %q", idValue)
	var results []names.ActionTag
	var doc struct {
		Id string `bson:"_id"`
	}

	actions, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	matchValue := m.st.docID(idValue)
	agentVersion, err := m.AgentVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newIdsSupported := IsNewActionIDSupported(agentVersion)
	maybeOldId := strings.ContainsAny(idValue, "-abcdef")
	var filter bson.D
	if idValue == "" {
		// Match all when passed an empty id prefix for
		// legacy behaviour.
		filter = nil
	} else if !newIdsSupported || maybeOldId {
		filter = bson.D{{
			"_id", bson.D{{"$regex", "^" + matchValue}},
		}}
	} else {
		filter = bson.D{{
			"_id", matchValue,
		}}
	}
	iter := actions.Find(filter).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		actionLogger.Tracef("FindActionTagsById() iter doc %+v", doc)
		localID := m.st.localID(doc.Id)
		if names.IsValidAction(localID) {
			results = append(results, names.NewActionTag(localID))
		}
	}
	actionLogger.Tracef("FindActionTagsById() %q found %+v", idValue, results)
	return results, nil
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

// EnqueueAction caches the action doc to the database.
func (m *Model) EnqueueAction(operationID string, receiver names.Tag, actionName string, payload map[string]interface{}, actionError error) (Action, error) {
	if len(actionName) == 0 {
		return nil, errors.New("action name required")
	}

	receiverCollectionName, receiverId, err := m.st.tagToCollectionAndId(receiver)
	if err != nil {
		return nil, errors.Trace(err)
	}
	agentVersion, err := m.AgentVersion()
	if err != nil {
		return nil, errors.Trace(err)
	}
	doc, ndoc, err := newActionDoc(m.st, operationID, receiver, actionName, payload, agentVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if actionError != nil {
		doc.Status = ActionError
		doc.Message = actionError.Error()
	}
	ops := []txn.Op{{
		C:      receiverCollectionName,
		Id:     receiverId,
		Assert: notDeadDoc,
	}, {
		C:      operationsC,
		Id:     m.st.docID(operationID),
		Assert: txn.DocExists,
	}, {
		C:      actionsC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	if actionError == nil {
		ops = append(ops, txn.Op{
			C:      actionNotificationsC,
			Id:     ndoc.DocId,
			Assert: txn.DocMissing,
			Insert: ndoc,
		})
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if notDead, err := isNotDead(m.st, receiverCollectionName, receiverId); err != nil {
			return nil, err
		} else if !notDead {
			return nil, stateerrors.ErrDead
		} else if attempt != 0 {
			_, err := m.Operation(operationID)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return nil, errors.Errorf("unexpected attempt number '%d'", attempt)
		}
		return ops, nil
	}
	if err = m.st.db().Run(buildTxn); err == nil {
		return newAction(m.st, doc), nil
	}
	return nil, err
}

// AddAction adds a new Action of type name and using arguments payload to
// the receiver, and returns its ID.
func (m *Model) AddAction(receiver ActionReceiver, operationID, name string, payload map[string]interface{}) (Action, error) {
	payload, err := receiver.PrepareActionPayload(name, payload)
	if err != nil {
		_, err2 := m.EnqueueAction(operationID, receiver.Tag(), name, payload, err)
		if err2 != nil {
			err = err2
		}
		return nil, errors.Trace(err)
	}
	return m.EnqueueAction(operationID, receiver.Tag(), name, payload, nil)
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

// PruneOperations removes operation entries and their sub-tasks until
// only logs newer than <maxLogTime> remain and also ensures
// that the actions collection is smaller than <maxLogsMB> after the deletion.
func PruneOperations(st *State, maxHistoryTime time.Duration, maxHistoryMB int) error {
	// There may be older actions without parent operations so try those first.
	hasNoOperation := bson.D{{"$or", []bson.D{
		{{"operation", ""}},
		{{"operation", bson.D{{"$exists", false}}}},
	}}}
	err := pruneCollection(st, maxHistoryTime, maxHistoryMB, actionsC, "completed", hasNoOperation, GoTime)
	if err != nil {
		return errors.Trace(err)
	}
	// First calculate the average ratio of tasks to operations. Since deletion is
	// done at the operation level, and any associated tasks are then deleted, but
	// the actions collection is where the disk space goes, we approximate the
	// number of operations to delete to achieve a given size deduction based on
	// the average ratio of number of operations to tasks.
	operationsColl, closer := st.db().GetRawCollection(operationsC)
	defer closer()
	operationsCount, err := operationsColl.Count()
	if err != nil {
		return errors.Annotate(err, "retrieving operations collection count")
	}
	actionsColl, closer := st.db().GetRawCollection(actionsC)
	defer closer()
	actionsCount, err := actionsColl.Count()
	if err != nil {
		return errors.Annotate(err, "retrieving actions collection count")
	}
	sizeFactor := float64(actionsCount) / float64(operationsCount)

	err = pruneCollectionAndChildren(st, maxHistoryTime, maxHistoryMB, operationsC, "completed", actionsC, "operation", nil, sizeFactor, GoTime)
	return errors.Trace(err)
}
