// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"
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
	err := a.st.runTransaction([]txn.Op{
		{
			C:      actionsC,
			Id:     a.doc.DocId,
			Assert: bson.D{{"status", ActionPending}},
			Update: bson.D{{"$set", bson.D{
				{"status", ActionRunning},
				{"started", nowToTheSecond()},
			}}},
		}})
	if err != nil {
		return nil, err
	}
	return a.st.Action(a.Id())
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
	err := a.st.runTransaction([]txn.Op{
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
				{"completed", nowToTheSecond()},
			}}},
		}, {
			C:      actionNotificationsC,
			Id:     a.st.docID(ensureActionMarker(a.Receiver()) + a.Id()),
			Remove: true,
		}})
	if err != nil {
		return nil, err
	}
	return a.st.Action(a.Id())
}

// newAction builds an Action for the given State and actionDoc.
func newAction(st *State, adoc actionDoc) Action {
	return &action{
		st:  st,
		doc: adoc,
	}
}

// newActionDoc builds the actionDoc with the given name and parameters.
func newActionDoc(st *State, receiverTag names.Tag, actionName string, parameters map[string]interface{}) (actionDoc, actionNotificationDoc, error) {
	prefix := ensureActionMarker(receiverTag.Id())
	actionId, err := NewUUID()
	if err != nil {
		return actionDoc{}, actionNotificationDoc{}, err
	}
	actionLogger.Debugf("newActionDoc name: '%s', receiver: '%s', actionId: '%s'", actionName, receiverTag, actionId)
	modelUUID := st.ModelUUID()
	return actionDoc{
			DocId:      st.docID(actionId.String()),
			ModelUUID:  modelUUID,
			Receiver:   receiverTag.Id(),
			Name:       actionName,
			Parameters: parameters,
			Enqueued:   nowToTheSecond(),
			Status:     ActionPending,
		}, actionNotificationDoc{
			DocId:     st.docID(prefix + actionId.String()),
			ModelUUID: modelUUID,
			Receiver:  receiverTag.Id(),
			ActionID:  actionId.String(),
		}, nil
}

var ensureActionMarker = ensureSuffixFn(actionMarker)

// Action returns an Action by Id, which is a UUID.
func (st *State) Action(id string) (Action, error) {
	actionLogger.Tracef("Action() %q", id)
	actions, closer := st.getCollection(actionsC)
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
func (st *State) AllActions() ([]Action, error) {
	actionLogger.Tracef("AllActions()")
	actions, closer := st.getCollection(actionsC)
	defer closer()

	results := []Action{}
	docs := []actionDoc{}
	err := actions.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all actions")
	}
	for _, doc := range docs {
		results = append(results, newAction(st, doc))
	}
	return results, nil
}

// ActionByTag returns an Action given an ActionTag.
func (st *State) ActionByTag(tag names.ActionTag) (Action, error) {
	return st.Action(tag.Id())
}

// FindActionTagsByPrefix finds Actions with ids that share the supplied prefix, and
// returns a list of corresponding ActionTags.
func (st *State) FindActionTagsByPrefix(prefix string) []names.ActionTag {
	actionLogger.Tracef("FindActionTagsByPrefix() %q", prefix)
	var results []names.ActionTag
	var doc struct {
		Id string `bson:"_id"`
	}

	actions, closer := st.getCollection(actionsC)
	defer closer()

	iter := actions.Find(bson.D{{"_id", bson.D{{"$regex", "^" + st.docID(prefix)}}}}).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		actionLogger.Tracef("FindActionTagsByPrefix() iter doc %+v", doc)
		localID := st.localID(doc.Id)
		if names.IsValidAction(localID) {
			results = append(results, names.NewActionTag(localID))
		}
	}
	actionLogger.Tracef("FindActionTagsByPrefix() %q found %+v", prefix, results)
	return results
}

// FindActionsByName finds Actions with the given name.
func (st *State) FindActionsByName(name string) ([]Action, error) {
	var results []Action
	var doc actionDoc

	actions, closer := st.getCollection(actionsC)
	defer closer()

	iter := actions.Find(bson.D{{"name", name}}).Iter()
	for iter.Next(&doc) {
		results = append(results, newAction(st, doc))
	}
	return results, errors.Trace(iter.Close())
}

// EnqueueAction
func (st *State) EnqueueAction(receiver names.Tag, actionName string, payload map[string]interface{}) (Action, error) {
	if len(actionName) == 0 {
		return nil, errors.New("action name required")
	}

	receiverCollectionName, receiverId, err := st.tagToCollectionAndId(receiver)
	if err != nil {
		return nil, errors.Trace(err)
	}

	doc, ndoc, err := newActionDoc(st, receiver, actionName, payload)
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
		if notDead, err := isNotDead(st, receiverCollectionName, receiverId); err != nil {
			return nil, err
		} else if !notDead {
			return nil, ErrDead
		} else if attempt != 0 {
			return nil, errors.Errorf("unexpected attempt number '%d'", attempt)
		}
		return ops, nil
	}
	if err = st.run(buildTxn); err == nil {
		return newAction(st, doc), nil
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

	actionsCollection, closer := st.getCollection(actionsC)
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

	actionsCollection, closer := st.getCollection(actionsC)
	defer closer()

	sel := append(bson.D{{"receiver", tag.Id()}}, statusCondition...)
	iter := actionsCollection.Find(sel).Iter()

	for iter.Next(&doc) {
		actions = append(actions, newAction(st, doc))
	}
	return actions, errors.Trace(iter.Close())
}
