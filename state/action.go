// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/names"
	"labix.org/v2/mgo/txn"
)

type actionDoc struct {
	// Id is the key for this document. The structure of the key is
	// a composite of UnitName and a unique Sequence, to facilitate
	// indexing and prefix filtering
	Id string `bson:"_id"`

	// UnitName is the name of the unit for which this action is
	// queued.
	UnitName string

	// Sequence is the unique segment in the composite key, and is
	// used to disambiguate multiple queued actions on the same unit
	Sequence int

	// Name identifies the action that should be run; it should
	// match an action defined by the unit's charm.
	Name string

	// Payload holds the action's parameters, if any; it should validate
	// against the schema defined by the named action in the unit's charm
	Payload map[string]interface{}
}

// Action represents an instruction to do some "action" and is expected
// to match an action definition in a charm.
type Action struct {
	st  *State
	doc actionDoc
}

// newAction builds an Action for the given State and actionDoc
func newAction(st *State, adoc actionDoc) *Action {
	return &Action{
		st:  st,
		doc: adoc,
	}
}

// newActionDoc builds the actionDoc with the given name and parameters
func newActionDoc(u *Unit, actionName string, parameters map[string]interface{}) (actionDoc, error) {
	prefix := actionPrefix(u.Name())
	seq, err := u.st.sequence(prefix)
	if err != nil {
		return actionDoc{}, err
	}
	return actionDoc{Id: actionId(prefix, seq), UnitName: u.Name(), Sequence: seq, Name: actionName, Payload: parameters}, nil
}

// actionPrefix returns the expected action prefix for a given unit name
func actionPrefix(unitName string) string {
	return "a#" + unitName + "#a#"
}

// actionId builds the id from the prefix and suffix
func actionId(prefix string, suffix int) string {
	return fmt.Sprintf("%s%d", prefix, suffix)
}

// Id returns the id of the Action
func (a *Action) Id() string {
	return a.doc.Id
}

// Tag implements the Entity interface and returns a names.Tag that
// is a names.ActionTag
func (a *Action) Tag() names.Tag {
	return a.ActionTag()
}

// ActionTag returns an ActionTag constructed from this action's
// UnitName and Sequence
func (a *Action) ActionTag() names.ActionTag {
	unitTag := names.NewUnitTag(a.doc.UnitName)
	actionTag := names.NewActionTag(unitTag, a.doc.Sequence)
	return actionTag
}

// Name returns the name of the action, as defined in the charm
func (a *Action) Name() string {
	return a.doc.Name
}

// Payload will contain a structure representing arguments or parameters to
// an action, and is expected to be validated by the Unit using the Charm
// definition of the Action
func (a *Action) Payload() map[string]interface{} {
	return a.doc.Payload
}

// Complete removes action from the pending queue and creates an ActionResult
// to capture the output and end state of the action.
func (a *Action) Complete(output string) error {
	return a.removeAndLog(ActionCompleted, output)
}

// Fail removes an Action from the queue, and creates an ActionResult that
// will capture the reason for the failure.
func (a *Action) Fail(reason string) error {
	return a.removeAndLog(ActionFailed, reason)
}

// removeAndLog takes the action off of the pending queue, and creates an
// actionresult to capture the outcome of the action.
func (a *Action) removeAndLog(finalStatus ActionStatus, output string) error {
	doc := newActionResultDoc(a, finalStatus, output)
	return a.st.runTransaction([]txn.Op{
		addActionResultOp(a.st, &doc),
		{
			C:      a.st.actions.Name,
			Id:     a.doc.Id,
			Remove: true,
		},
	})
}
