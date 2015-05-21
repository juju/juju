// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

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

	// The machine on which this agent is running is being rebooted.
	// The juju-agent should move from rebooting to idle when the reboot is complete.
	StatusRebooting Status = "rebooting"

	// The agent is running a hook or action. The human-readable message should reflect
	// which hook or action is being run.
	StatusExecuting Status = "executing"

	// Once the agent is installed and running it will notify the Juju server and its state
	// becomes "idle". It will stay "idle" until some action (e.g. it needs to run a hook) or
	// error (e.g it loses contact with the Juju server) moves it to a different state.
	StatusIdle Status = "idle"

	// The unit agent has failed in some way,eg the agent ought to be signalling
	// activity, but it cannot be detected. It might also be that the unit agent
	// detected an unrecoverable condition and managed to tell the Juju server about it.
	StatusFailed Status = "failed"

	// The juju agent has has not communicated with the juju server for an unexpectedly long time;
	// the unit agent ought to be signalling activity, but none has been detected.
	StatusLost Status = "lost"

	// ---- Outdated ----
	// The unit agent is downloading the charm and running the install hook.
	StatusInstalling Status = "installing"

	// The unit is being destroyed; the agent will soon mark the unit as “dead”.
	// In Juju 2.x this will describe the state of the agent rather than a unit.
	StatusStopping Status = "stopping"
)

const (
	// Status values specific to services and units, reflecting the
	// state of the software itself.

	// The unit is not yet providing services, but is actively doing stuff
	// in preparation for providing those services.
	// This is a "spinning" state, not an error state.
	// It reflects activity on the unit itself, not on peers or related units.
	StatusMaintenance Status = "maintenance"

	// This unit used to exist, we have a record of it (perhaps because of storage
	// allocated for it that was flagged to survive it). Nonetheless, it is now gone.
	StatusTerminated Status = "terminated"

	// A unit-agent has finished calling install, config-changed, and start,
	// but the charm has not called status-set yet.
	StatusUnknown Status = "unknown"

	// The unit is unable to progress to an active state because a service to
	// which it is related is not running.
	StatusWaiting Status = "waiting"

	// The unit needs manual intervention to get back to the Running state.
	StatusBlocked Status = "blocked"

	// The unit believes it is correctly offering all the services it has
	// been asked to offer.
	StatusActive Status = "active"
)

type statusNotFoundError struct {
	error
}

// Cause implements errors.causer
func (e *statusNotFoundError) Cause() error {
	return e.error
}

func newStatusNotFound(key string) error {
	return &statusNotFoundError{errors.NotFoundf("status for key %q", key)}
}

// IsStatusNotFound returns true if the provided error is
// statusNotFoundError
func IsStatusNotFound(err error) bool {
	if _, ok := err.(*statusNotFoundError); ok {
		return true
	}
	err = errors.Cause(err)
	_, ok := err.(*statusNotFoundError)
	return ok
}

// ValidAgentStatus returns true if status has a known value for an agent.
// This is used by the status command to filter out
// unknown status values.
func (status Status) ValidAgentStatus() bool {
	switch status {
	case
		StatusAllocating,
		StatusError,
		StatusFailed,
		StatusRebooting,
		StatusExecuting,
		StatusIdle:
		return true
	case //Deprecated status vales
		StatusPending,
		StatusStarted,
		StatusStopped,
		StatusInstalling,
		StatusActive,
		StatusStopping,
		StatusDown:
		return true
	default:
		return false
	}
}

// ValidWorkloadStatus returns true if status has a known value for a workload.
// This is used by the status command to filter out
// unknown status values.
func (status Status) ValidWorkloadStatus() bool {
	switch status {
	case
		StatusBlocked,
		StatusMaintenance,
		StatusWaiting,
		StatusActive,
		StatusUnknown,
		StatusTerminated,
		StatusError: // include error so that we can filter on what the spec says is valid
		return true
	case // Deprecated statuses
		StatusPending,
		StatusInstalling,
		StatusStarted,
		StatusStopped,
		StatusDown:
		return true
	default:
		return false
	}
}

// WorkloadMatches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (status Status) WorkloadMatches(candidate Status) bool {
	switch candidate {
	case status: // We could be holding an old status ourselves
		return true
	case StatusDown, StatusStopped:
		candidate = StatusTerminated
	case StatusInstalling:
		candidate = StatusMaintenance
	case StatusStarted:
		candidate = StatusActive
	}
	return status == candidate
}

// Matches returns true if the candidate matches status,
// taking into account that the candidate may be a legacy
// status value which has been deprecated.
func (status Status) Matches(candidate Status) bool {
	switch candidate {
	case StatusDown:
		candidate = StatusLost
	case StatusStarted:
		candidate = StatusActive
	case StatusStopped:
		candidate = StatusStopping
	}
	return status == candidate
}

// StatusSetter represents a type whose status can be set.
type StatusSetter interface {
	SetStatus(status Status, info string, data map[string]interface{}) error
}

// StatusGetter represents a type whose status can be read.
type StatusGetter interface {
	Status() (StatusInfo, error)
}

// StatusInfo holds the status information for a machine, unit, service etc.
type StatusInfo struct {
	Status  Status
	Message string
	Data    map[string]interface{}
	Since   *time.Time
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
	Updated    *time.Time
}

type historicalStatusDoc struct {
	Id         int    `bson:"_id"`
	EnvUUID    string `bson:"env-uuid"`
	Status     Status
	StatusInfo string
	StatusData map[string]interface{}
	Updated    *time.Time
	EntityId   string
}

func newHistoricalStatusDoc(s statusDoc, entityId string) *historicalStatusDoc {
	return &historicalStatusDoc{
		EnvUUID:    s.EnvUUID,
		Status:     s.Status,
		StatusInfo: s.StatusInfo,
		StatusData: s.StatusData,
		Updated:    s.Updated,
		EntityId:   entityId,
	}
}

func updateStatusHistory(oldDoc statusDoc, globalKey string, st *State) error {
	id, err := st.sequence("statushistory")
	if err != nil {
		errors.Annotatef(err, "cannot make id updating status history of unit agent %q", globalKey)
	}
	hDoc := newHistoricalStatusDoc(oldDoc, globalKey)

	h := txn.Op{
		C:      statusesHistoryC,
		Id:     id,
		Insert: hDoc,
	}

	err = st.runTransaction([]txn.Op{h})
	return errors.Annotatef(err, "cannot update status history of unit agent %q", globalKey)
}

func statusHistory(size int, globalKey string, st *State) ([]StatusInfo, error) {
	statusHistory, closer := st.getCollection(statusesHistoryC)
	defer closer()

	sInfo := []StatusInfo{}
	results := []historicalStatusDoc{}
	err := statusHistory.Find(bson.D{{"entityid", globalKey}}).Sort("-_id").Limit(size).All(&results)
	if err == mgo.ErrNotFound {
		return []StatusInfo{}, errors.NotFoundf("statusHistory")
	}
	if err != nil {
		return []StatusInfo{}, errors.Annotatef(err, "cannot get status history for %q", globalKey)
	}
	for _, s := range results {
		sInfo = append(sInfo, StatusInfo{
			Status:  s.Status,
			Message: s.StatusInfo,
			Data:    s.StatusData,
			Since:   s.Updated,
		})
	}
	return sInfo, nil
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
	timestamp := nowToTheSecond()
	doc.Updated = &timestamp
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
	timestamp := nowToTheSecond()
	doc.Updated = &timestamp
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
		StatusRebooting,
		StatusExecuting,
		StatusIdle,
		StatusFailed,
		StatusLost,
		// The current health spec says an agent should not be in error
		// but this needs discussion so we'll retain it for now.
		StatusError:
		return true
	case // TODO(perrito666) Deprecate in 2.x
		StatusPending,
		StatusStarted,
		StatusStopped:
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
	case StatusAllocating, StatusLost:
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

type unitStatusDoc struct {
	statusDoc
}

// newUnitStatusDoc creates a new unitStatusDoc with the given status and other data.
func newUnitStatusDoc(status Status, info string, data map[string]interface{}) (*unitStatusDoc, error) {
	doc := &unitStatusDoc{statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}}
	timestamp := nowToTheSecond()
	doc.Updated = &timestamp
	if err := doc.validateSet(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}

// unitStatusValid returns true if status has a known value for units.
func unitStatusValid(status Status) bool {
	switch status {
	case
		StatusBlocked,
		StatusMaintenance,
		StatusWaiting,
		StatusActive,
		StatusUnknown,
		StatusTerminated:
		return true
	default:
		return false
	}
}

// validateSet returns an error if the unitStatusDoc does not represent a sane
// SetStatus operation for a unit.
func (doc *unitStatusDoc) validateSet() error {
	if !unitStatusValid(doc.Status) {
		return errors.Errorf("cannot set invalid status %q", doc.Status)
	}
	return nil
}

type serviceStatusDoc struct {
	statusDoc
}

// newServiceStatusDoc creates a new serviceStatusDoc with the given status and other data.
func newServiceStatusDoc(status Status, info string, data map[string]interface{}) (*serviceStatusDoc, error) {
	doc := &serviceStatusDoc{statusDoc{
		Status:     status,
		StatusInfo: info,
		StatusData: data,
	}}
	timestamp := nowToTheSecond()
	doc.Updated = &timestamp
	if err := doc.validateSet(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}

// validateSet returns an error if the serviceStatusDoc does not represent a sane
// SetStatus operation for a service.
func (doc *serviceStatusDoc) validateSet() error {
	// Valid service status values are the same as those for the unit.
	if !unitStatusValid(doc.Status) {
		return errors.Errorf("cannot set invalid status %q", doc.Status)
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
		return statusDoc{}, newStatusNotFound(globalKey)
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

// PruneStatusHistory removes status history entries until
// only the maxLogsPerEntity newest records per unit remain.
func PruneStatusHistory(st *State, maxLogsPerEntity int) error {
	historyColl, closer := st.getCollection(statusesHistoryC)
	defer closer()
	globalKeys, err := getEntitiesWithStatuses(historyColl)
	if err != nil {
		return errors.Trace(err)
	}
	for _, globalKey := range globalKeys {
		keepUpTo, ok, err := getOldestTimeToKeep(historyColl, globalKey, maxLogsPerEntity)
		if err != nil {
			return errors.Trace(err)
		}
		if !ok {
			continue
		}
		_, err = historyColl.RemoveAll(bson.D{
			{"entityid", globalKey},
			{"_id", bson.M{"$lt": keepUpTo}},
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// getOldestTimeToKeep returns the create time for the oldest
// status log to be kept.
func getOldestTimeToKeep(coll stateCollection, globalKey string, size int) (int, bool, error) {
	result := historicalStatusDoc{}
	err := coll.Find(bson.D{{"entityid", globalKey}}).Sort("-_id").Skip(size - 1).One(&result)
	if err == mgo.ErrNotFound {
		return -1, false, nil
	}
	if err != nil {
		return -1, false, errors.Trace(err)
	}
	return result.Id, true, nil

}

// getEntitiesWithStatuses returns the ids for all entities that
// have history entries
func getEntitiesWithStatuses(coll stateCollection) ([]string, error) {
	var entityKeys []string
	err := coll.Find(nil).Distinct("entityid", &entityKeys)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return entityKeys, nil
}

// TranslateLegacyAgentStatus returns the status value clients expect to see for
// agent-state in versions prior to 1.24
func TranslateToLegacyAgentState(agentStatus, workloadStatus Status, workloadMessage string) (Status, bool) {
	// Originally AgentState (a member of api.UnitStatus) could hold one of:
	// StatusPending
	// StatusInstalled
	// StatusStarted
	// StatusStopped
	// StatusError
	// StatusDown
	// For compatibility reasons we convert modern states (from V2 uniter) into
	// four of the old ones: StatusPending, StatusStarted, StatusStopped, or StatusError.

	// For the purposes of deriving the legacy status, there's currently no better
	// way to determine if a unit is installed.
	// TODO(wallyworld) - use status history to see if start hook has run.
	isInstalled := workloadStatus != StatusMaintenance || workloadMessage != "installing charm software"

	switch agentStatus {
	case StatusAllocating:
		return StatusPending, true
	case StatusError:
		return StatusError, true
	case StatusRebooting, StatusExecuting, StatusIdle, StatusLost, StatusFailed:
		switch workloadStatus {
		case StatusError:
			return StatusError, true
		case StatusTerminated:
			return StatusStopped, true
		case StatusMaintenance:
			if isInstalled {
				return StatusStarted, true
			} else {
				return StatusPending, true
			}
		default:
			return StatusStarted, true
		}
	}
	return "", false
}
