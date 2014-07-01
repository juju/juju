// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo/txn"
)

// ActionStatus represents the possible end states for an action.
type ActionStatus string

const (
	// Fail signifies that the action did not complete successfully.
	ActionFailed ActionStatus = "fail"

	// Complete indicates that the action ran to completion as intended.
	ActionCompleted ActionStatus = "complete"
)

type actionResultDoc struct {
	// Id is the key for this document.  The format of the id encodes
	// the id of the Action that was used to produce this ActionResult.
	// The format is: <action id> + actionResultMarker + <generated sequence>
	Id string `bson:"_id"`

	// UnitName is the name of the unit from which this action result came
	UnitName string

	// Sequence is the unique instance of this action result, derived
	// from the action sequence number
	Sequence int

	// ActionName identifies the action that was run.
	ActionName string

	// Payload describes the parameters passed in for the action
	// when it was run.
	Payload map[string]interface{}

	// Status represents the end state of the Action; ActionFailed for an
	// action that was removed prematurely, or that failed, and
	// ActionCompleted for an action that successfully completed.
	Status ActionStatus

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

// newActionResultDoc converts an Action into an actionResultDoc given
// the finalStatus and the output of the action
func newActionResultDoc(a *Action, finalStatus ActionStatus, output string) actionResultDoc {
	prefix := actionResultPrefix(a.doc.UnitName)
	id := fmt.Sprintf("%s%d", prefix, a.doc.Sequence)
	return actionResultDoc{
		Id:         id,
		UnitName:   a.doc.UnitName,
		Sequence:   a.doc.Sequence,
		ActionName: a.doc.Name,
		Payload:    a.doc.Payload,
		Status:     finalStatus,
		Output:     output,
	}
}

// actionResultPrefix returns a well formed _id prefix for an
// ActionResult from a given unit
func actionResultPrefix(unitName string) string {
	return "ar#" + unitName + "#a#"
}

// addActionResultOp builds the txn.Op used to add an actionresult
func addActionResultOp(st *State, doc *actionResultDoc) txn.Op {
	return txn.Op{
		C:      st.actionresults.Name,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// actionIdToActionResultId converts the actionId to the id of it's
// result.  This is somewhat temporary until the decision to merge
// action and actionresult is made.
func actionIdToActionResultId(actionId string) string {
	if len(actionId) < 2 {
		return ""
	}
	if actionId[:2] != "a#" {
		return ""
	}
	return "ar#" + actionId[2:]
}

// Id returns the id of the ActionResult.
func (a *ActionResult) Id() string {
	return a.doc.Id
}

// ActionName returns the name of the Action.
func (a *ActionResult) ActionName() string {
	return a.doc.ActionName
}

// Payload will contain a structure representing arguments or parameters
// that were passed to the action.
func (a *ActionResult) Payload() map[string]interface{} {
	return a.doc.Payload
}

// Status returns the final state of the action.
func (a *ActionResult) Status() ActionStatus {
	return a.doc.Status
}

// Output returns the text caputured from the action as it was executed.
func (a *ActionResult) Output() string {
	return a.doc.Output
}
