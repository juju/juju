// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

// Operation represents a number of tasks resulting from running an action.
// The Operation provides both the justification for individual tasks
// to be performed and the grouping of them.
//
// As an example, if an action is run targeted to several units,
// the operation would reflect the request to run the actions,
// while the individual tasks would track the running of
// the individual actions on each unit.
type Operation interface {
	Entity

	// Id returns the local id of the Operation.
	Id() string

	// Enqueued returns the time the operation was added to state.
	Enqueued() time.Time

	// Started returns the time that the first Action execution began.
	Started() time.Time

	// Completed returns the completion time of the last Action.
	Completed() time.Time

	// Summary is the reason for running the operation.
	Summary() string

	// Status returns the final state of the operation.
	Status() ActionStatus

	// OperationTag returns the operation's tag.
	OperationTag() names.OperationTag

	// Refresh refreshes the contents of the operation.
	Refresh() error
}

type operationDoc struct {
	DocId     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	// Summary is the reason for running the operation.
	Summary string `bson:"summary"`

	// Enqueued is the time the operation was added.
	Enqueued time.Time `bson:"enqueued"`

	// Started reflects the time the first action began running.
	Started time.Time `bson:"started"`

	// Completed reflects the time that the last action was finished.
	Completed time.Time `bson:"completed"`

	// CompleteTaskCount is used internally for mgo asserts.
	// It is not exposed via the Operation interface.
	CompleteTaskCount int `bson:"complete-task-count"`

	// Status represents the end state of the Operation.
	// If not explicitly set, this is derived from the
	// status of the associated actions.
	Status ActionStatus `bson:"status"`
}

// operation represents a group of associated actions.
type operation struct {
	st         *State
	doc        operationDoc
	taskStatus []ActionStatus
}

// Id returns the local id of the Operation.
func (op *operation) Id() string {
	return op.st.localID(op.doc.DocId)
}

// Tag implements the Entity interface and returns a names.Tag that
// is a names.ActionTag.
func (op *operation) Tag() names.Tag {
	return op.OperationTag()
}

// OperationTag returns the operation's tag.
func (op *operation) OperationTag() names.OperationTag {
	return names.NewOperationTag(op.Id())
}

// Enqueued returns the time the action was added to state as a pending
// Action.
func (op *operation) Enqueued() time.Time {
	return op.doc.Enqueued
}

// Started returns the time that the Action execution began.
func (op *operation) Started() time.Time {
	return op.doc.Started
}

// Completed returns the completion time of the Operation.
func (op *operation) Completed() time.Time {
	return op.doc.Completed
}

// Summary is the reason for running the operation.
func (op *operation) Summary() string {
	return op.doc.Summary
}

// Status returns the final state of the operation.
// If not explicitly set, this is derived from the
// status of the associated actions/tasks.
func (op *operation) Status() ActionStatus {
	if op.doc.Status != ActionPending {
		return op.doc.Status
	}
	statusStats := set.NewStrings()
	for _, s := range op.taskStatus {
		statusStats.Add(string(s))
	}
	for _, s := range statusOrder {
		if statusStats.Contains(string(s)) {
			return s
		}
	}
	return op.doc.Status
}

// Refresh refreshes the contents of the operation.
func (op *operation) Refresh() error {
	doc, taskStatus, err := op.st.getOperationDoc(op.Id())
	if err != nil {
		if errors.IsNotFound(err) {
			return err
		}
		return errors.Annotatef(err, "cannot refresh operation %v", op.Id())
	}
	op.doc = *doc
	op.taskStatus = taskStatus
	return nil
}

var statusOrder = []ActionStatus{
	ActionRunning,
	ActionPending,
	ActionFailed,
	ActionCancelled,
	ActionCompleted,
}

var statusCompletedOrder = []ActionStatus{
	ActionFailed,
	ActionCancelled,
	ActionCompleted,
}

// newAction builds an Action for the given State and actionDoc.
func newOperation(st *State, doc operationDoc, taskStatus []ActionStatus) Operation {
	return &operation{
		st:         st,
		doc:        doc,
		taskStatus: taskStatus,
	}
}

// newOperationDoc builds a new operationDoc.
func newOperationDoc(mb modelBackend, summary string) (operationDoc, string, error) {
	id, err := sequenceWithMin(mb, "task", 1)
	if err != nil {
		return operationDoc{}, "", errors.Trace(err)
	}
	operationID := strconv.Itoa(id)
	modelUUID := mb.modelUUID()
	return operationDoc{
		DocId:     mb.docID(operationID),
		ModelUUID: modelUUID,
		Enqueued:  mb.nowToTheSecond(),
		Status:    ActionPending,
		Summary:   summary,
	}, operationID, nil
}

// EnqueueOperation records the start of an operation.
func (m *Model) EnqueueOperation(summary string) (string, error) {
	var operationID string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var doc operationDoc
		var err error
		doc, operationID, err = newOperationDoc(m.st, summary)
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{{
			C:      operationsC,
			Id:     doc.DocId,
			Assert: txn.DocMissing,
			Insert: doc,
		}}
		return ops, nil
	}
	err := m.st.db().Run(buildTxn)
	return operationID, errors.Trace(err)
}

// Operation returns an Operation by Id.
func (m *Model) Operation(id string) (Operation, error) {
	doc, taskStatus, err := m.st.getOperationDoc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newOperation(m.st, *doc, taskStatus), nil
}

// OperationWithActions returns an OperationInfo by Id.
func (m *Model) OperationWithActions(id string) (*OperationInfo, error) {
	// First gather the matching actions and record the parent operation ids we need.
	actionsCollection, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	var actions []actionDoc
	err := actionsCollection.Find(bson.D{{"operation", id}}).
		Sort("_id").All(&actions)
	if err != nil {
		return nil, errors.Trace(err)
	}

	operationCollection, closer := m.st.db().GetCollection(operationsC)
	defer closer()

	var docs []operationDoc
	err = operationCollection.FindId(id).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(docs) == 0 {
		return nil, errors.NotFoundf("operation %v", id)
	}

	var result OperationInfo
	taskStatus := make([]ActionStatus, len(actions))
	result.Actions = make([]Action, len(actions))
	for i, action := range actions {
		result.Actions[i] = newAction(m.st, action)
		taskStatus[i] = action.Status
	}
	operation := newOperation(m.st, docs[0], taskStatus)
	result.Operation = operation
	return &result, nil
}

func (st *State) getOperationDoc(id string) (*operationDoc, []ActionStatus, error) {
	operations, closer := st.db().GetCollection(operationsC)
	defer closer()
	actions, closer := st.db().GetCollection(actionsC)
	defer closer()

	doc := operationDoc{}
	err := operations.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, nil, errors.NotFoundf("operation %q", id)
	}
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot get operation %q", id)
	}
	var actionDocs []actionDoc
	err = actions.Find(bson.D{{"operation", id}}).All(&actionDocs)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot get tasks for operation %q", id)
	}
	taskStatus := make([]ActionStatus, len(actionDocs))
	for i, a := range actionDocs {
		taskStatus[i] = a.Status
	}
	return &doc, taskStatus, nil
}

// AllOperations returns all Operations.
func (m *Model) AllOperations() ([]Operation, error) {
	operations, closer := m.st.db().GetCollection(operationsC)
	defer closer()

	results := []Operation{}
	docs := []operationDoc{}
	err := operations.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all operations")
	}
	for _, doc := range docs {
		// This is for gathering operations for migration - task status values are not relevant
		results = append(results, newOperation(m.st, doc, nil))
	}
	return results, nil
}

// defaultMaxOperationsLimit is the default maximum number of operations to
// return when performing an operations query.
const defaultMaxOperationsLimit = 500

// OperationInfo encapsulates an operation and summary
// information about some of its actions.
type OperationInfo struct {
	Operation Operation
	Actions   []Action
}

// ListOperations returns operations that match the specified criteria.
func (m *Model) ListOperations(
	actionNames []string, actionReceivers []names.Tag, operationStatus []ActionStatus,
	offset, limit int,
) ([]OperationInfo, bool, error) {
	// First gather the matching actions and record the parent operation ids we need.
	actionsCollection, closer := m.st.db().GetCollection(actionsC)
	defer closer()

	var receiverIDs []string
	for _, tag := range actionReceivers {
		receiverIDs = append(receiverIDs, tag.Id())
	}
	var receiverTerm, namesTerm bson.D
	if len(receiverIDs) > 0 {
		receiverTerm = bson.D{{"receiver", bson.D{{"$in", receiverIDs}}}}
	}
	if len(actionNames) > 0 {
		namesTerm = bson.D{{"name", bson.D{{"$in", actionNames}}}}
	}
	actionsQuery := append(receiverTerm, namesTerm...)
	var actions []actionDoc
	err := actionsCollection.Find(actionsQuery).
		// For now we'll limit what we return to the caller as action results
		// can be large and show-task ca be used to get more detail as needed.
		Select(bson.D{
			{"model-uuid", 0},
			{"messages", 0},
			{"results", 0}}).
		Sort("_id").All(&actions)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	if len(actions) == 0 {
		return nil, false, nil
	}

	// We have the operation ids which are parent to any actions matching the criteria.
	// Combine these with additional operation filtering criteria to do the final query.
	operationIds := make([]string, len(actions))
	operationActions := make(map[string][]actionDoc)
	for i, action := range actions {
		operationIds[i] = m.st.docID(action.Operation)
		actions := operationActions[action.Operation]
		actions = append(actions, action)
		operationActions[action.Operation] = actions
	}

	idsTerm := bson.D{{"_id", bson.D{{"$in", operationIds}}}}
	var statusTerm bson.D
	if len(operationStatus) > 0 {
		statusTerm = bson.D{{"status", bson.D{{"$in", operationStatus}}}}
	}
	operationsQuery := append(idsTerm, statusTerm...)

	operationCollection, closer := m.st.db().GetCollection(operationsC)
	defer closer()

	var docs []operationDoc
	query := operationCollection.Find(operationsQuery)
	nominalCount, err := query.Count()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	if offset != 0 {
		query = query.Skip(offset)
	}
	// Don't let the user shoot themselves in the foot.
	if limit <= 0 {
		limit = defaultMaxOperationsLimit
	}
	query = query.Limit(limit)
	err = query.Sort("_id").All(&docs)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	truncated := nominalCount > len(docs)

	result := make([]OperationInfo, len(docs))
	for i, doc := range docs {
		actions := operationActions[m.st.localID(doc.DocId)]
		taskStatus := make([]ActionStatus, len(actions))
		result[i].Actions = make([]Action, len(actions))
		for j, action := range actions {
			result[i].Actions[j] = newAction(m.st, action)
			taskStatus[j] = action.Status
		}
		operation := newOperation(m.st, doc, taskStatus)
		result[i].Operation = operation
	}
	return result, truncated, nil
}
