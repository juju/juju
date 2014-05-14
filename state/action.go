package state

import (
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
)

// ActionStatus represents the available status' for an Action
type ActionStatus string

const (
	// ActionPending is the default status of a new action.
	ActionPending ActionStatus = "pending"
	// ActionRunning indicates that the action has been picked up and is running.
	ActionRunning ActionStatus = "running"
)

type actionDoc struct {
	Id      string `bson:"_id"`
	Name    string
	Unit    string
	Payload interface{}
	Status  ActionStatus
}

// Action represents an instruction to do some "action" and is expected to match
// an action definition in a charm.
type Action struct {
	st  *State
	doc actionDoc
}

func newAction(st *State, adoc actionDoc) *Action {
	action := &Action{
		st:  st,
		doc: adoc,
	}
	return action
}

// Name returns the name of the Action
func (a *Action) Name() string {
	return a.doc.Name
}

// Id returns the mongo Id of the Action
func (a *Action) Id() string {
	return a.doc.Id
}

// Payload will contain a structure representing arguments or parameters to 
// an action, and is expected to be validated by the Unit using the Charm 
// definition of the Action
func (a *Action) Payload() interface{} {
	return a.doc.Payload
}

// Status shows the current status of the Action
func (a *Action) Status() ActionStatus {
	return a.doc.Status
}

func (a *Action) setRunning() error {
	ops := []txn.Op{{
		C:      a.st.actions.Name,
		Id:     a.doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"status", ActionRunning}}}},
	}}
	err := a.st.runTransaction(ops)
	if err != nil {
		return err
	}
	a.doc.Status = ActionRunning
	return nil
}
