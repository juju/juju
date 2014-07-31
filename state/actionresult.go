// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"
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

// actionResultMarker is the token used to separate the action id prefix
// from the unique actionResult suffix
const actionResultMarker = "_ar_"

// newActionResultId generates a new unique key from an action id
func newActionResultId(st *State, actionId string) (string, error) {
	prefix := actionId + actionResultMarker
	suffix, err := st.sequence(prefix)
	if err != nil {
		return "", errors.Errorf("cannot assign new sequence for prefix '%s': %v", prefix, err)
	}
	return fmt.Sprintf("%s%d", prefix, suffix), nil
}

// newActionResultDoc builds a new doc
func newActionResultDoc(action *Action, status ActionStatus, output string) (*actionResultDoc, error) {
	id, err := newActionResultId(action.st, action.Id())
	if err != nil {
		return nil, err
	}
	return &actionResultDoc{
		Id:         id,
		ActionName: action.Name(),
		Payload:    action.Payload(),
		Status:     status,
		Output:     output,
	}, nil
}

// addActionResultOp builds the txn.Op used to add an actionresult
func addActionResultOp(st *State, doc *actionResultDoc) txn.Op {
	return txn.Op{
		C:      actionresultsC,
		Id:     doc.Id,
		Assert: txn.DocMissing,
		Insert: doc,
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
