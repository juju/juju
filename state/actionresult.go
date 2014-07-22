// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"labix.org/v2/mgo/txn"
)

// ActionStatus represents the possible end states for an action.
type ActionStatus string

const (
	// ActionFailed signifies that the action did not complete successfully.
	ActionFailed ActionStatus = "fail"

	// ActionCompleted indicates that the action ran to completion as intended.
	ActionCompleted ActionStatus = "complete"
)

const actionResultMarker string = "_ar_"

type actionResultDoc struct {
	// Id is the key for this document.  The format of the id encodes
	// the id of the Action that was used to produce this ActionResult.
	// The format is: <action id> + actionResultMarker + <generated sequence>
	Id string `bson:"_id"`

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

// globalKey returns the global database key for the action.
func (a *ActionResult) globalKey() string {
	return actionResultGlobalKey(a.doc.Id)
}

// actionResultGlobalKey returns the global database key for the named action.
func actionResultGlobalKey(name string) string {
	return "ar#" + name
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
	actionId := a.Id()
	id, ok := convertActionIdToActionResultId(actionId)
	if !ok {
		panic(fmt.Sprintf("cannot convert actionId to actionResultId: %v", actionId))
	}
	return actionResultDoc{
		Id:         id,
		ActionName: a.doc.Name,
		Payload:    a.doc.Payload,
		Status:     finalStatus,
		Output:     output,
	}
}

// convertActionIdToActionResultId builds an actionResultId from an actionId
func convertActionIdToActionResultId(actionId string) (string, bool) {
	parts := strings.Split(actionId, actionMarker)
	if len(parts) != 2 {
		return "", false
	}
	actionResultId := strings.Join(parts, actionResultMarker)
	return actionResultId, true
}

// actionResultPrefix returns a string prefix for matching action results for
// the given ActionReceiver
func actionResultPrefix(ar ActionReceiver) string {
	return ar.Name() + actionResultMarker
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
