// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strconv"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"
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
	operationId := strconv.Itoa(id)
	modelUUID := mb.modelUUID()
	return operationDoc{
		DocId:     mb.docID(operationId),
		ModelUUID: modelUUID,
		Enqueued:  mb.nowToTheSecond(),
		Status:    ActionPending,
		Summary:   summary,
	}, operationId, nil
}

// EnqueueOperation records the start of an operation.
func (m *Model) EnqueueOperation(summary string) (string, error) {
	doc, id, err := newOperationDoc(m.st, summary)
	if err != nil {
		return "", errors.Trace(err)
	}

	ops := []txn.Op{{
		C:      operationsC,
		Id:     doc.DocId,
		Assert: txn.DocMissing,
		Insert: doc,
	}}

	err = m.st.db().RunTransaction(ops)
	return id, errors.Trace(err)
}

// Operation returns an Operation by Id.
func (m *Model) Operation(id string) (Operation, error) {
	doc, taskStatus, err := m.st.getOperationDoc(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newOperation(m.st, *doc, taskStatus), nil
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
