// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var (
	_ StatusSetter = (*Machine)(nil)
	_ StatusSetter = (*Unit)(nil)
	_ StatusGetter = (*Machine)(nil)
	_ StatusGetter = (*Unit)(nil)
)

// Status represents the status of an entity.
// It could be a unit, machine or its agent.
type Status string

const (
	// The entity is not yet participating in the environment.
	StatusPending Status = "pending"

	// The unit has performed initial setup and is adapting itself to
	// the environment. Not applicable to machines.
	StatusInstalled Status = "installed"

	// The entity is actively participating in the environment.
	StatusStarted Status = "started"

	// The entity's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The entity requires human intervention in order to operate
	// correctly.
	StatusError Status = "error"

	// The entity ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)

// Valid returns true if status has a known value.
func (status Status) Valid() bool {
	switch status {
	case
		StatusPending,
		StatusInstalled,
		StatusStarted,
		StatusStopped,
		StatusError,
		StatusDown:
	default:
		return false
	}
	return true
}

type StatusSetter interface {
	SetStatus(status Status, info string, data map[string]interface{}) error
}

type StatusGetter interface {
	Status() (status Status, info string, data map[string]interface{}, err error)
}

// statusDoc represents a entity status in Mongodb.  The implicit
// _id field is explicitly set to the global key of the associated
// entity in the document's creation transaction, but omitted to allow
// direct use of the document in both create and update transactions.
type statusDoc struct {
	Status     Status
	StatusInfo string
	StatusData map[string]interface{}
}

// validateSet returns an error if the statusDoc does not represent a sane
// SetStatus operation.
func (doc statusDoc) validateSet(allowPending bool) error {
	if !doc.Status.Valid() {
		return fmt.Errorf("cannot set invalid status %q", doc.Status)
	}
	switch doc.Status {
	case StatusPending:
		if !allowPending {
			return fmt.Errorf("cannot set status %q", doc.Status)
		}
	case StatusDown:
		return fmt.Errorf("cannot set status %q", doc.Status)
	case StatusError:
		if doc.StatusInfo == "" {
			return fmt.Errorf("cannot set status %q without info", doc.Status)
		}
	}
	if doc.StatusData != nil && doc.Status != StatusError {
		return fmt.Errorf("cannot set status data when status is %q", doc.Status)
	}
	return nil
}

// getStatus retrieves the status document associated with the given
// globalKey and copies it to outStatusDoc, which needs to be created
// by the caller before.
func getStatus(st *State, globalKey string) (statusDoc, error) {
	statuses, closer := st.getCollection(statusesC)
	defer closer()

	var doc statusDoc
	err := statuses.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return statusDoc{}, errors.NotFoundf("status")
	}
	if err != nil {
		return statusDoc{}, fmt.Errorf("cannot get status %q: %v", globalKey, err)
	}
	return doc, nil
}

// createStatusOp returns the operation needed to create the given
// status document associated with the given globalKey.
func createStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     globalKey,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// updateStatusOp returns the operations needed to update the given
// status document associated with the given globalKey.
func updateStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     globalKey,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", doc}},
	}
}

// removeStatusOp returns the operation needed to remove the status
// document associated with the given globalKey.
func removeStatusOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     globalKey,
		Remove: true,
	}
}
