// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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
	err = m.st.db().RunTransaction([]txn.Op{
		{
			C:      actionsC,
			Id:     a.doc.DocId,
			Assert: bson.D{{"status", ActionPending}},
			Update: bson.D{{"$set", bson.D{
				{"status", ActionRunning},
				{"started", a.st.nowToTheSecond()},
			}}},
		}})
	if err != nil {
		return nil, err
	}
	return m.Action(a.Id())
}

// Finish removes action from the pending queue and captures the output
// and end state of the action.
func (a *action) Finish(results ActionResults) (Action, error) {
	return a.removeAndLog(results.Status, results.Results, results.Message)
}

// removeAndLog takes the action off of the pending queue, and creates
// an actionresult to capture the outcome of the action. It asserts that
// the action is not already completed.
func (a *action) removeAndLog(finalStatus ActionStatus, results map[string]interface{}, message string) (Action, error) {
	m, err := a.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	err = m.st.db().RunTransaction([]txn.Op{
		{
			C:  actionsC,
			Id: a.doc.DocId,
			Assert: bson.D{{"status", bson.D{
				{"$nin", []interface{}{
					ActionCompleted,
					ActionCancelled,
					ActionFailed,
				}}}}},
			Update: bson.D{{"$set", bson.D{
				{"status", finalStatus},
				{"message", message},
				{"results", results},
				{"completed", a.st.nowToTheSecond()},
			}}},
		}, {
			C:      actionNotificationsC,
			Id:     m.st.docID(ensureActionMarker(a.Receiver()) + a.Id()),
			Remove: true,
		}})
	if err != nil {
		return nil, err
	}
	return m.Action(a.Id())
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
		if s := a.Status(); s != ActionRunning {
			return nil, errors.Errorf("cannot log message to task %q with status %v", a.Id(), s)
		}
		ops := []txn.Op{
			{
				C:      actionsC,
				Id:     a.doc.DocId,
				Assert: bson.D{{"status", ActionRunning}},
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

var newActionIDFirstSupportedVersion = version.MustParse("2.7.0")

func newActionIDSupported(ver version.Number) bool {
	return ver.Compare(newActionIDFirstSupportedVersion) >= 0
}

// newActionDoc builds the actionDoc with the given name and parameters.
func newActionDoc(mb modelBackend, receiverTag names.Tag, actionName string, parameters map[string]interface{}, modelAgentVersion version.Number) (actionDoc, actionNotificationDoc, error) {
	prefix := ensureActionMarker(receiverTag.Id())
	// For actions run on units, we want to use a user friendly action id.
	// Theoretically, an action receiver could also be a machine, but for
	// now we'll continue to use a UUID for that case, since I don't think

	// we support machine actions anymore.
	var actionId string
	if receiverTag.Kind() == names.UnitTagKind && newActionIDSupported(modelAgentVersion) {
		id, err := sequence(mb, "task")
		if err != nil {
			return actionDoc{}, actionNotificationDoc{}, err
		}
		// Start numbering from 1 not 0.
		actionId = strconv.Itoa(id + 1)
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
			Status:     ActionPending,
		}, actionNotificationDoc{
			DocId:     mb.docID(prefix + actionId),
			ModelUUID: modelUUID,
			Receiver:  receiverTag.Id(),
			ActionID:  actionId,
		}, nil
}

var ensureActionMarker = ensureSuffixFn(actionMarker)

// Action returns an Action by Id, which is a UUID.
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
// the supplied id (for newer id integers).
// It returns a list of corresponding ActionTags.
func (m *Model) FindActionTagsById(idValue string) []names.ActionTag {
	actionLogger.Tracef("FindActionTagsById() %q", idValue)
	var results []names.ActionTag
	var doc struct {
		Id string `bson:"_id"`
	}

	actions, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	matchValue := m.st.docID(idValue)
	filter := bson.D{{"$or", []bson.D{
		{{"_id", bson.D{{"$regex", "^" + matchValue}}}},
		{{"_id", matchValue}},
	}}}
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
	return results
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
func (m *Model) EnqueueAction(receiver names.Tag, actionName string, payload map[string]interface{}) (Action, error) {
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
	doc, ndoc, err := newActionDoc(m.st, receiver, actionName, payload, agentVersion)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      receiverCollectionName,
		Id:     receiverId,
		Assert: notDeadDoc,
	}, {
		C:      actionsC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}, {
		C:      actionNotificationsC,
		Id:     ndoc.DocId,
		Assert: txn.DocMissing,
		Insert: ndoc,
	}}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		if notDead, err := isNotDead(m.st, receiverCollectionName, receiverId); err != nil {
			return nil, err
		} else if !notDead {
			return nil, ErrDead
		} else if attempt != 0 {
			return nil, errors.Errorf("unexpected attempt number '%d'", attempt)
		}
		return ops, nil
	}
	if err = m.st.db().Run(buildTxn); err == nil {
		return newAction(m.st, doc), nil
	}
	return nil, err
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
	completed := bson.D{{"status", ActionPending}}
	return st.matchingActionsByReceiverAndStatus(ar.Tag(), completed)
}

// matchingActionsRunning finds actions that match ActionReceiver and
// that are running.
func (st *State) matchingActionsRunning(ar ActionReceiver) ([]Action, error) {
	completed := bson.D{{"status", ActionRunning}}
	return st.matchingActionsByReceiverAndStatus(ar.Tag(), completed)
}

// matchingActionsCompleted finds actions that match ActionReceiver and
// that are complete.
func (st *State) matchingActionsCompleted(ar ActionReceiver) ([]Action, error) {
	completed := bson.D{{"$or", []bson.D{
		{{"status", ActionCompleted}},
		{{"status", ActionCancelled}},
		{{"status", ActionFailed}},
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

// PruneActions removes action entries until
// only logs newer than <maxLogTime> remain and also ensures
// that the collection is smaller than <maxLogsMB> after the
// deletion.
func PruneActions(st *State, maxHistoryTime time.Duration, maxHistoryMB int) error {
	err := pruneCollection(st, maxHistoryTime, maxHistoryMB, actionsC, "completed", GoTime)
	return errors.Trace(err)
}
