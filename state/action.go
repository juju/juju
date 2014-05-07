package state

import (
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
)

type ActionStatus string

const (
	ActionPending ActionStatus = "pending"
	ActionRunning ActionStatus = "running"
)

type actionDoc struct {
	Id      string `bson:"_id"`
	Name    string
	Unit    string
	Payload string
	Status  ActionStatus
}

type Action struct {
	st  *State
	doc actionDoc
}

func newAction(st *State, adoc *actionDoc) *Action {
	action := &Action{
		st:  st,
		doc: *adoc,
	}
	return action
}

func (a *Action) Name() string {
	return a.doc.Name
}

func (a *Action) Id() string {
	return a.doc.Id
}

func (a *Action) Payload() string {
	return a.doc.Payload
}

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

func (a *Action) setPending() error {
	ops := []txn.Op{{
		C:      a.st.actions.Name,
		Id:     a.doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"status", ActionPending}}}},
	}}
	err := a.st.runTransaction(ops)
	if err != nil {
		return err
	}
	a.doc.Status = ActionPending
	return nil
}
