// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
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
// It could be a service, unit, machine or its agent.
type Status string

const (
	// Status values common to machine and unit agents.

	// The entity requires human intervention in order to operate
	// correctly.
	StatusError Status = "error"

	// The entity is actively participating in the environment.
	// For unit agents, this is a state we preserve for backwards
	// compatibility with scripts during the life of Juju 1.x.
	// In Juju 2.x, the agent-state will remain “active” and scripts
	// will watch the unit-state instead for signals of service readiness.
	StatusStarted Status = "started"
)

const (
	// Status values specific to machine agents.

	// The machine is not yet participating in the environment.
	StatusPending Status = "pending"

	// The machine's agent will perform no further action, other than
	// to set the unit to Dead at a suitable moment.
	StatusStopped Status = "stopped"

	// The machine ought to be signalling activity, but it cannot be
	// detected.
	StatusDown Status = "down"
)

const (
	// Status values specific to unit agents.

	// The machine on which a unit is to be hosted is still being
	// spun up in the cloud.
	StatusAllocating Status = "allocating"

	// The unit agent is downloading the charm and running the install hook.
	StatusInstalling Status = "installing"

	// The agent is actively participating in the environment.
	StatusActive Status = "active"

	// The unit is being destroyed; the agent will soon mark the unit as “dead”.
	// In Juju 2.x this will describe the state of the agent rather than a unit.
	StatusStopping Status = "stopping"

	// The unit agent has failed in some way,eg the agent ought to be signalling
	// activity, but it cannot be detected. It might also be that the unit agent
	// detected an unrecoverable condition and managed to tell the Juju server about it.
	StatusFailed Status = "failed"
)

const (
	// Status values specific to services and units, reflecting the
	// state of the software itself.

	// The unit is installed and has no problems but is busy getting itself
	// ready to provide services.
	StatusBusy Status = "busy"

	// The unit is unable to offer services because it needs another
	// service to be up.
	StatusWaiting Status = "waiting"

	// The unit needs manual intervention to get back to the Running state.
	StatusBlocked Status = "blocked"

	// The unit believes it is correctly offering all the services it has
	// been asked to offer.
	StatusRunning Status = "running"
)

// ValidAgentStatus returns true if status has a known value for an agent.
// This is used by the status command to filter out
// unknown status values.
func (status Status) ValidAgentStatus() bool {
	switch status {
	case
		StatusPending,
		StatusStarted,
		StatusStopped,
		StatusError,
		StatusDown,
		StatusAllocating,
		StatusInstalling,
		StatusFailed,
		StatusActive,
		StatusStopping:
		return true
	default:
		return false
	}
}

// Matches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (status Status) Matches(candidate Status) bool {
	switch candidate {
	case StatusDown:
		candidate = StatusFailed
	case StatusStarted:
		candidate = StatusActive
	case StatusStopped:
		candidate = StatusStopping
	}
	return status == candidate
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
	EnvUUID    string `bson:"env-uuid"`
	Status     Status
	StatusInfo string
	StatusData map[string]interface{}
}

type machineStatusDoc struct {
	statusDoc
}

// newMachineStatusDoc creates a new machineAgentStatusDoc with the given status and other data.
func newMachineStatusDoc(status Status, info string, data map[string]interface{},
	allowPending bool,
) (*machineStatusDoc, error) {
	doc := &machineStatusDoc{statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}}
	if err := doc.validateSet(allowPending); err != nil {
		return nil, err
	}
	return doc, nil
}

// machineStatusValid returns true if status has a known value for machines.
func machineStatusValid(status Status) bool {
	switch status {
	case
		StatusPending,
		StatusStarted,
		StatusStopped,
		StatusError,
		StatusDown:
		return true
	default:
		return false
	}
}

// validateSet returns an error if the machineStatusDoc does not represent a sane
// SetStatus operation.
func (doc machineStatusDoc) validateSet(allowPending bool) error {
	if !machineStatusValid(doc.Status) {
		return errors.Errorf("cannot set invalid status %q", doc.Status)
	}
	switch doc.Status {
	case StatusPending:
		if !allowPending {
			return errors.Errorf("cannot set status %q", doc.Status)
		}
	case StatusDown:
		return errors.Errorf("cannot set status %q", doc.Status)
	case StatusError:
		if doc.StatusInfo == "" {
			return errors.Errorf("cannot set status %q without info", doc.Status)
		}
	}
	if doc.StatusData != nil && doc.Status != StatusError {
		return errors.Errorf("cannot set status data when status is %q", doc.Status)
	}
	return nil
}

type unitAgentStatusDoc struct {
	statusDoc
}

// newUnitAgentStatusDoc creates a new unitAgentStatusDoc with the given status and other data.
func newUnitAgentStatusDoc(status Status, info string, data map[string]interface{}) (*unitAgentStatusDoc, error) {
	doc := &unitAgentStatusDoc{statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}}
	if err := doc.validateSet(); err != nil {
		return nil, err
	}
	return doc, nil
}

// unitAgentStatusValid returns true if status has a known value for unit agents.
func unitAgentStatusValid(status Status) bool {
	switch status {
	case
		StatusAllocating,
		StatusInstalling,
		StatusActive,
		StatusStopping,
		StatusFailed,
		StatusError:
		return true
	default:
		return false
	}
}

// validateSet returns an error if the unitAgentStatusDoc does not represent a sane
// SetStatus operation for a unit agent.
func (doc *unitAgentStatusDoc) validateSet() error {
	if !unitAgentStatusValid(doc.Status) {
		return errors.Errorf("cannot set invalid status %q", doc.Status)
	}
	switch doc.Status {
	// For safety; no code will use these deprecated values.
	case StatusPending, StatusDown, StatusStarted, StatusStopped:
		return errors.Errorf("status %q is deprecated and invalid", doc.Status)
	case StatusAllocating, StatusFailed:
		return errors.Errorf("cannot set status %q", doc.Status)
	case StatusError:
		if doc.StatusInfo == "" {
			return errors.Errorf("cannot set status %q without info", doc.Status)
		}
	}
	if doc.StatusData != nil && doc.Status != StatusError {
		return errors.Errorf("cannot set status data when status is %q", doc.Status)
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
		return statusDoc{}, errors.Annotatef(err, "cannot get status %q", globalKey)
	}
	return doc, nil
}

// createStatusOp returns the operation needed to create the given
// status document associated with the given globalKey.
func createStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     st.docID(globalKey),
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// updateStatusOp returns the operations needed to update the given
// status document associated with the given globalKey.
func updateStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     st.docID(globalKey),
		Assert: txn.DocExists,
		Update: bson.D{{"$set", doc}},
	}
}

// removeStatusOp returns the operation needed to remove the status
// document associated with the given globalKey.
func removeStatusOp(st *State, globalKey string) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     st.docID(globalKey),
		Remove: true,
	}
}
