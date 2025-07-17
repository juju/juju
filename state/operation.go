// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/names/v6"
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

	// Fail is why the operation failed.
	Fail() string

	// Status returns the final state of the operation.
	Status() ActionStatus

	// OperationTag returns the operation's tag.
	OperationTag() names.OperationTag

	// Refresh refreshes the contents of the operation.
	Refresh() error

	// SpawnedTaskCount returns the number of spawned actions.
	SpawnedTaskCount() int
}

type operationDoc struct {
	DocId     string `bson:"_id"`
	ModelUUID string `bson:"model-uuid"`

	// Summary is the reason for running the operation.
	Summary string `bson:"summary"`

	// Fail is why the operation failed.
	Fail string `bson:"fail"`

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

	// SpawnedTaskCount is the number of tasks to be completed in
	// this operation. It is used internally for mgo asserts and
	// not exposed via the Operation interface.
	SpawnedTaskCount int `bson:"spawned-task-count"`
}

// operation represents a group of associated actions.
type operation struct {
	doc operationDoc
}

// Id returns the local id of the Operation.
func (op *operation) Id() string {
	return ""
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

// Fail is why the operation failed.
func (op *operation) Fail() string {
	return op.doc.Fail
}

// SpawnedTaskCount is the number of spawned actions.
func (op *operation) SpawnedTaskCount() int {
	return op.doc.SpawnedTaskCount
}

// Status returns the final state of the operation.
// If not explicitly set, this is derived from the
// status of the associated actions/tasks.
func (op *operation) Status() ActionStatus {
	return ActionCancelled
}

// Refresh refreshes the contents of the operation.
func (op *operation) Refresh() error {
	return nil
}

// Operation returns an Operation by Id.
func (m *Model) Operation(id string) (Operation, error) {
	return &operation{}, nil
}

// OperationWithActions returns an OperationInfo by Id.
func (m *Model) OperationWithActions(id string) (*OperationInfo, error) {
	return &OperationInfo{}, nil
}

// AllOperations returns all Operations.
func (m *Model) AllOperations() ([]Operation, error) {
	results := []Operation{}
	return results, nil
}

// OperationInfo encapsulates an operation and summary
// information about some of its actions.
type OperationInfo struct {
	Operation Operation
	Actions   []Action
}
