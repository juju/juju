// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/leadership"
	"github.com/juju/juju/mongo"
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

const (
	// StorageReadyMessage is the message set to the agent status when all storage
	// attachments are properly done.
	StorageReadyMessage = "storage ready"

	// PreparingStorageMessage is the message set to the agent status before trying
	// to attach storages.
	PreparingStorageMessage = "preparing storage"
)

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
// This is used by the apiserver client facade to filter out completely-unknown
// status values.
func (status Status) ValidWorkloadStatus() bool {
	if validWorkloadStatus(status) {
		return true
	}
	switch status {
	case StatusError: // include error so that we can filter on what the spec says is valid
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

// validWorkloadStatus returns true if status has a known value for units or services.
func validWorkloadStatus(status Status) bool {
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
	EnvUUID    string                 `bson:"env-uuid"`
	Status     Status                 `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`
	Updated    *time.Time             `bson:"updated"`
}

// mapKeys returns a copy of the supplied map, with all nested map[string]interface{}
// keys transformed by f. All other types are ignored.
func mapKeys(f func(string) string, input map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range input {
		if submap, ok := value.(map[string]interface{}); ok {
			value = mapKeys(f, submap)
		}
		result[f(key)] = value
	}
	return result
}

func escapeKeys(input map[string]interface{}) map[string]interface{} {
	return mapKeys(escapeReplacer.Replace, input)
}

func unescapeKeys(input map[string]interface{}) map[string]interface{} {
	return mapKeys(unescapeReplacer.Replace, input)
}

// getStatus retrieves the status document associated with the given
// globalKey and converts it to a StatusInfo. If the status document
// is not found, a NotFoundError referencing badge will be returned.
func getStatus(st *State, globalKey, badge string) (_ StatusInfo, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get status")
	statuses, closer := st.getCollection(statusesC)
	defer closer()

	var doc statusDoc
	err = statuses.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return StatusInfo{}, errors.NotFoundf(badge)
	} else if err != nil {
		return StatusInfo{}, errors.Trace(err)
	}

	return StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    mapKeys(unescapeReplacer.Replace, doc.StatusData),
		Since:   doc.Updated,
	}, nil
}

// setStatusParams configures a setStatus call. All parameters are presumed to
// be set to valid values unless otherwise noted.
type setStatusParams struct {

	// badge is used to specialize any NotFound error emitted.
	badge string

	// globalKey uniquely identifies the entity to which the status belongs.
	globalKey string

	// status is the status value.
	status Status

	// message is an optional string elaborating upon the status.
	message string

	// rawData is a map of arbitrary data elaborating upon the status and
	// message. Its keys are assumed not to have been escaped.
	rawData map[string]interface{}

	// token, if present, must accept an *[]txn.Op passed to its Check method,
	// and will prevent any change if it becomes invalid.
	token leadership.Token
}

// createStatusOp returns the operation needed to create the given
// status document associated with the given globalKey *and* tries
// to write a corresponding historical status document. This is a
// hack but it's a small one compared to the unhacks in the CL that
// introduced it, and it's probably better to have the hack in one
// place here than in the N places that will create statuses.
func createStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	probablyUpdateStatusHistory(st, globalKey, doc)
	return txn.Op{
		C:      statusesC,
		Id:     st.docID(globalKey),
		Assert: txn.DocMissing,
		Insert: doc,
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

// setStatus inteprets the supplied params as documented on the type.
func setStatus(st *State, params setStatusParams) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set status")

	// TODO(fwereade): this can/should probably be recording the time the
	// status was *set*, not the time it happened to arrive in state.
	// And we shouldn't be throwing away accuracy here -- neither to the
	// second right here *or* by serializing into mongo as a time.Time,
	// which also discards precision.
	// We should almost certainly be accepting StatusInfo in the exposed
	// SetStatus methods, for symetry with the Status methods.
	now := nowToTheSecond()
	doc := statusDoc{
		Status:     params.status,
		StatusInfo: params.message,
		StatusData: escapeKeys(params.rawData),
		Updated:    &now,
	}
	probablyUpdateStatusHistory(st, params.globalKey, doc)

	// Set the authoritative status document, or fail trying.
	buildTxn := updateStatusSource(st, params.globalKey, doc)
	if params.token != nil {
		buildTxn = wrapSource(buildTxn, params.token)
	}
	err = st.run(buildTxn)
	if cause := errors.Cause(err); cause == mgo.ErrNotFound {
		return errors.NotFoundf(params.badge)
	}
	return errors.Trace(err)
}

// updateStatusSource returns a transaction source that builds the operations
// necessary to set the supplied status (and to fail safely if leaked and
// executed late, so as not to overwrite more recent documents).
func updateStatusSource(st *State, globalKey string, doc statusDoc) jujutxn.TransactionSource {
	update := bson.D{{"$set", &doc}}
	return func(_ int) ([]txn.Op, error) {
		txnRevno, err := st.readTxnRevno(statusesC, globalKey)
		if err != nil {
			return nil, errors.Trace(err)
		}
		assert := bson.D{{"txn-revno", txnRevno}}
		return []txn.Op{{
			C:      statusesC,
			Id:     globalKey,
			Assert: assert,
			Update: update,
		}}, nil
	}
}

type historicalStatusDoc struct {
	Id         int                    `bson:"_id"`
	EnvUUID    string                 `bson:"env-uuid"`
	Status     Status                 `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`
	Updated    *time.Time             `bson:"updated"`
	EntityId   string                 `bson:"entityid"`
}

func probablyUpdateStatusHistory(st *State, globalKey string, doc statusDoc) {
	// TODO(fwereade): we do NOT need every single status-history operation
	// to write to the same document in mongodb. If you need to order them,
	// use a time representation that does not discard precision, like an
	// int64 holding the time's UnixNanoseconds.
	id, err := st.sequence("statushistory")
	if err != nil {
		logger.Errorf("failed to generate id for status history: %v", err)
		return
	}
	historyDoc := &historicalStatusDoc{
		Id: id,
		// We can't guarantee that the statusDoc we're dealing with has the
		// env-uuid filled in; and envStateCollection does not trap inserts.
		// Good to be explicit; better to fix leaky abstraction.
		EnvUUID:    st.EnvironUUID(),
		Status:     doc.Status,
		StatusInfo: doc.StatusInfo,
		StatusData: doc.StatusData, // coming from a statusDoc, already escaped
		Updated:    doc.Updated,
		EntityId:   globalKey,
	}
	history, closer := st.getCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()
	if err := historyW.Insert(historyDoc); err != nil {
		logger.Errorf("failed to write status history: %v", err)
	}
}

func statusHistory(st *State, globalKey string, size int) ([]StatusInfo, error) {
	statusHistory, closer := st.getCollection(statusesHistoryC)
	defer closer()

	var docs []historicalStatusDoc
	query := statusHistory.Find(bson.D{{"entityid", globalKey}})
	err := query.Sort("-_id").Limit(size).All(&docs)
	if err == mgo.ErrNotFound {
		return []StatusInfo{}, errors.NotFoundf("status history")
	} else if err != nil {
		return []StatusInfo{}, errors.Annotatef(err, "cannot get status history")
	}

	results := make([]StatusInfo, len(docs))
	for i, doc := range docs {
		results[i] = StatusInfo{
			Status:  doc.Status,
			Message: doc.StatusInfo,
			Data:    unescapeKeys(doc.StatusData),
			Since:   doc.Updated,
		}
	}
	return results, nil
}

// PruneStatusHistory removes status history entries until
// only the maxLogsPerEntity newest records per unit remain.
func PruneStatusHistory(st *State, maxLogsPerEntity int) error {
	history, closer := st.getCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()

	// TODO(fwereade): This is a very strange implementation. Is it specced
	// that we should keep different spans of history for different entities?
	// It would seem normal to either keep entries for a fixed time (say 24h),
	// or to prune down to a target total data size by discarding the oldest
	// entries. This renders useless -- but is careful to keep -- every status
	// older than the oldest status of the most frequently updated entity...
	//
	// ...and it's really doing a *lot* of work to subtly corrupt the data.
	// If you want to break status history like this you can do it *much*
	// more efficiently.
	globalKeys, err := getEntitiesWithStatuses(historyW)
	if err != nil {
		return errors.Trace(err)
	}
	for _, globalKey := range globalKeys {
		keepUpTo, ok, err := getOldestTimeToKeep(historyW, globalKey, maxLogsPerEntity)
		if err != nil {
			return errors.Trace(err)
		}
		if !ok {
			continue
		}
		_, err = historyW.RemoveAll(bson.D{
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
func getOldestTimeToKeep(coll mongo.Collection, globalKey string, size int) (int, bool, error) {
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
func getEntitiesWithStatuses(coll mongo.Collection) ([]string, error) {
	var entityKeys []string
	err := coll.Find(nil).Distinct("entityid", &entityKeys)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return entityKeys, nil
}

const MessageInstalling = "installing charm software"

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
	isInstalled := workloadStatus != StatusMaintenance || workloadMessage != MessageInstalling

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
