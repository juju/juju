// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
)

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
	return nil
}

// Id returns the local id of the Action.
func (a *action) Id() string {
	return ""
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
	return nil
}

// newAction builds an Action for the given State and actionDoc.
func newAction(st *State, adoc actionDoc) Action {
	return &action{
		st:  st,
		doc: adoc,
	}
}

// Action returns an Action by Id.
func (m *Model) Action(id string) (Action, error) {
	return newAction(m.st, actionDoc{}), nil
}

// AllActions returns all Actions.
func (m *Model) AllActions() ([]Action, error) {
	results := []Action{}
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
	return results, nil
}
