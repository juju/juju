package state

import ()

type actionDoc struct {
	Id      string `bson:"_id"`
	Name    string
	Unit    string
	Payload interface{}
}

// Action represents an instruction to do some "action" and is expected to match
// an action definition in a charm.
type Action struct {
	st  *State
	doc actionDoc
}

func newAction(st *State, adoc actionDoc) *Action {
	return &Action{
		st:  st,
		doc: adoc,
	}
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
