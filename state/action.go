// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"labix.org/v2/mgo/txn"
)

type actionDoc struct {
	Id string `bson:"_id"`

	// Name identifies the action; it should match an action defined by
	// the unit's charm.
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

// newAction builds an Action from the supplied state and actionDoc
func newAction(st *State, adoc actionDoc) *Action {
	return &Action{
		st:  st,
		doc: adoc,
	}
}

// actionPrefix returns a suitable prefix for an action given the
// globalKey of a containing item
func actionPrefix(globalKey string) string {
	return globalKey + "#a#"
}

// newActionId generates a new unique key from another globalKey as
// a prefix, and a generated unique number
func newActionId(st *State, globalKey string) (string, error) {
	prefix := actionPrefix(globalKey)
	suffix, err := st.sequence(prefix)
	if err != nil {
		return "", fmt.Errorf("cannot assign new sequence for prefix '%s': %v", prefix, err)
	}
	return fmt.Sprintf("%s%d", prefix, suffix), nil
}

// Name returns the name of the Action
func (a *Action) Name() string {
	return a.doc.Name
}

// Id returns the id of the Action
func (a *Action) Id() string {
	return a.doc.Id
}

// Payload will contain a structure representing arguments or parameters to
// an action, and is expected to be validated by the Unit using the Charm
// definition of the Action
func (a *Action) Payload() map[string]interface{} {
	return a.doc.Payload
}

// Fail removes an Action from the queue, and documents the reason for the
// failure.
func (a *Action) Fail(reason string) error {
	// TODO(jcw4) replace with code to generate a result that records this failure
	logger.Warningf("action '%s' failed because '%s'", a.doc.Name, reason)
	return a.st.runTransaction([]txn.Op{{
		C:      a.st.actions.Name,
		Id:     a.doc.Id,
		Remove: true,
	}})
}
