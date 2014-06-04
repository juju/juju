// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"labix.org/v2/mgo/txn"
)

// ActionResultState represents the possible end states for an action.
type ActionResultState string

const (
	// Fail signifies that the action did not complete successfully.
	ActionFailed ActionResultState = "fail"

	// Complete indicates that the action ran to completion as intended.
	ActionCompleted ActionResultState = "complete"
)

type actionResultDoc struct {
	Id string `bson:"_id"`

	// Name identifies the action that was run.
	Name string

	// Payload describes the parameters passed in for the action
	// when it was run.
	Payload map[string]interface{}

	// Result represents the end state of the Action; ActionFailed for an
	// action that was removed prematurely, or that failed, and
	// ActionCompleted for an action that successfully completed.
	// TODO(jcw4) better name? EndResult? ExitState? Status? blech.
	Result ActionResultState

	// Output captures any text emitted by the action.
	Output string
}

// ActionResult represents an instruction to do some "action" and is
// expected to match an action definition in a charm.
type ActionResult struct {
	st  *State
	doc actionResultDoc
}

// newActionResult builds an ActionResult from the supplied state and
// actionResultDoc.
func newActionResult(st *State, adoc actionResultDoc) *ActionResult {
	return &ActionResult{
		st:  st,
		doc: adoc,
	}
}

// actionResultMarker is the token used to separate the action id prefix
// from the unique actionResult suffix
const actionResultMarker = "#ar#"

// generateActionResultIdPrefix returns a suitable prefix for an action given the
// globalKey of a containing item.
func generateActionResultIdPrefix(globalKey string) string {
	return globalKey + actionResultMarker
}

// newActionResultId generates a new unique key from another globalKey as
// a prefix, and a generated unique number.
func newActionResultId(st *State, globalKey string) (string, error) {
	prefix := generateActionResultIdPrefix(globalKey)
	suffix, err := st.sequence(prefix)
	if err != nil {
		return "", fmt.Errorf("cannot assign new sequence for prefix '%s': %v", prefix, err)
	}
	return fmt.Sprintf("%s%d", prefix, suffix), nil
}

// newActionResultDoc builds a new doc
func newActionResultDoc(action *Action, result ActionResultState, output string) (*actionResultDoc, error) {
	id, err := newActionResultId(action.st, action.Id())
	if err != nil {
		return nil, err
	}
	return &actionResultDoc{
		Id:      id,
		Name:    action.Name(),
		Payload: action.Payload(),
		Result:  result,
		Output:  output,
	}, nil
}

// addActionResultOps builds the []txn.Op used to add an action result
func addActionResultOps(st *State, doc *actionResultDoc) []txn.Op {
	return []txn.Op{
		{
			C:      st.actionresults.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
}

// getActionResultIdPrefix takes an ActionResult.Id() and returns the prefix
// used to build it.  Useful for filtering.
func getActionResultIdPrefix(actionResultId string) string {
	return strings.Split(actionResultId, actionResultMarker)[0]
}

// Id returns the id of the ActionResult.
func (a *ActionResult) Id() string {
	return a.doc.Id
}

// Name returns the name of the ActionResult.
func (a *ActionResult) Name() string {
	return a.doc.Name
}

// Payload will contain a structure representing arguments or parameters
// that were passed to the action.
func (a *ActionResult) Payload() map[string]interface{} {
	return a.doc.Payload
}

// Result returns the final state of the action.
func (a *ActionResult) Result() ActionResultState {
	return a.doc.Result
}

// Output returns the text caputured from the action as it was executed.
func (a *ActionResult) Output() string {
	return a.doc.Output
}
