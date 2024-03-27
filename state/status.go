// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"reflect"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/mongo"
	"github.com/juju/juju/internal/mongo/utils"
)

var status_logger = loggo.GetLogger("juju.status")

type displayStatusFunc func(unitStatus status.StatusInfo, containerStatus status.StatusInfo) status.StatusInfo

// ModelStatus holds all the current status values for a given model
// and offers accessors for the various parts of a model.
type ModelStatus struct {
	model *Model
	docs  map[string]statusDocWithID
}

// LoadModelStatus retrieves all the status documents for the model
// at once. Used to primarily speed up status.
func (m *Model) LoadModelStatus() (*ModelStatus, error) {
	statuses, closer := m.st.db().GetCollection(statusesC)
	defer closer()

	var docs []statusDocWithID
	err := statuses.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read status collection")
	}

	result := &ModelStatus{
		model: m,
		docs:  make(map[string]statusDocWithID),
	}
	for _, doc := range docs {
		id := m.localID(doc.ID)
		result.docs[id] = doc
	}

	return result, nil
}

func (m *ModelStatus) getDoc(key, badge string) (statusDocWithID, error) {
	doc, found := m.docs[key]
	if !found {
		return statusDocWithID{}, errors.Annotate(errors.NotFoundf(badge), "cannot get status")
	}
	return doc, nil
}

func (m *ModelStatus) getStatus(key, badge string) (status.StatusInfo, error) {
	doc, err := m.getDoc(key, badge)
	if err != nil {
		return status.StatusInfo{}, err
	}
	return doc.asStatusInfo(), nil
}

// Model returns the status of the model.
func (m *ModelStatus) Model() (status.StatusInfo, error) {
	return m.getStatus(m.model.globalKey(), "model")
}

// MachineAgent returns the status of the machine agent.
func (m *ModelStatus) MachineAgent(machineID string) (status.StatusInfo, error) {
	return m.getStatus(machineGlobalKey(machineID), "machine")
}

// MachineInstance returns the status of the machine instance.
func (m *ModelStatus) MachineInstance(machineID string) (status.StatusInfo, error) {
	return m.getStatus(machineGlobalInstanceKey(machineID), "instance")
}

// MachineModification returns the status of the machine modification
func (m *ModelStatus) MachineModification(machineID string) (status.StatusInfo, error) {
	return m.getStatus(machineGlobalModificationKey(machineID), "modification")
}

// FullUnitWorkloadVersion returns the full status info for the workload
// version of a unit. This is used for selecting the workload version for
// an application.
func (m *ModelStatus) FullUnitWorkloadVersion(unitName string) (status.StatusInfo, error) {
	return m.getStatus(globalWorkloadVersionKey(unitName), "workload")
}

// UnitWorkloadVersion returns workload version for the unit
func (m *ModelStatus) UnitWorkloadVersion(unitName string) (string, error) {
	info, err := m.getStatus(globalWorkloadVersionKey(unitName), "workload")
	if err != nil {
		return "", err
	}
	return info.Message, nil
}

// UnitAgent returns the status of the Unit's agent.
func (m *ModelStatus) UnitAgent(unitName string) (status.StatusInfo, error) {
	// We do horrible things with unit status.
	// See notes in unitagent.go.
	info, err := m.getStatus(unitAgentGlobalKey(unitName), "agent")
	if err != nil {
		return info, err
	}
	if info.Status == status.Error {
		return status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Data:    map[string]interface{}{},
			Since:   info.Since,
		}, nil
	}
	return info, nil
}

// UnitWorkload returns the status of the unit's workload.
func (m *ModelStatus) UnitWorkload(unitName string) (status.StatusInfo, error) {
	// We do horrible things with unit status.
	// See notes in unit.go.
	info, err := m.getStatus(unitAgentGlobalKey(unitName), "unit")
	if err != nil {
		return info, err
	} else if info.Status == status.Error {
		return info, nil
	}

	// (for CAAS models) Use cloud container status over unit if the cloud
	// container status is error or active or the unit status hasn't shifted
	// from 'allocating'
	info, err = m.getStatus(unitGlobalKey(unitName), "workload")
	if err != nil {
		return info, errors.Trace(err)
	}

	if m.model.Type() == ModelTypeIAAS {
		return info, nil
	}

	containerInfo, err := m.getStatus(globalCloudContainerKey(unitName), "cloud container")
	if err != nil && !errors.Is(err, errors.NotFound) {
		return info, err
	}
	return status.UnitDisplayStatus(info, containerInfo), nil
}

// caasHistoryRewriteDoc determines which status should be stored as history.
func caasHistoryRewriteDoc(jujuStatus, caasStatus status.StatusInfo, displayStatus displayStatusFunc, clock clock.Clock) (*statusDoc, error) {
	modifiedStatus := displayStatus(jujuStatus, caasStatus)
	if modifiedStatus.Status == jujuStatus.Status && modifiedStatus.Message == jujuStatus.Message {
		return nil, nil
	}
	return &statusDoc{
		Status:     modifiedStatus.Status,
		StatusInfo: modifiedStatus.Message,
		StatusData: utils.EscapeKeys(modifiedStatus.Data),
		Updated:    timeOrNow(modifiedStatus.Since, clock).UnixNano(),
	}, nil
}

type statusDocWithID struct {
	ID         string                 `bson:"_id"`
	ModelUUID  string                 `bson:"model-uuid"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`
	Updated    int64                  `bson:"updated"`
}

func (doc *statusDocWithID) asStatusInfo() status.StatusInfo {
	return status.StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    utils.UnescapeKeys(doc.StatusData),
		Since:   unixNanoToTime(doc.Updated),
	}
}

// statusDoc represents a entity status in Mongodb.  The implicit
// _id field is explicitly set to the global key of the associated
// entity in the document's creation transaction, but omitted to allow
// direct use of the document in both create and update transactions.
type statusDoc struct {
	ModelUUID  string                 `bson:"model-uuid"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`

	// Updated used to be a *time.Time that was not present on statuses dating
	// from older versions of juju so this might be 0 for those cases.
	Updated int64 `bson:"updated"`
}

func (doc *statusDoc) asStatusInfo() status.StatusInfo {
	return status.StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    utils.UnescapeKeys(doc.StatusData),
		Since:   unixNanoToTime(doc.Updated),
	}
}

func unixNanoToTime(i int64) *time.Time {
	t := time.Unix(0, i)
	return &t
}

// getStatus retrieves the status document associated with the given
// globalKey and converts it to a StatusInfo. If the status document
// is not found, a NotFoundError referencing badge will be returned.
func getStatus(db Database, globalKey, badge string) (_ status.StatusInfo, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get status")
	statuses, closer := db.GetCollection(statusesC)
	defer closer()

	var doc statusDoc
	err = statuses.FindId(globalKey).One(&doc)
	if err == mgo.ErrNotFound {
		return status.StatusInfo{}, errors.NotFoundf(badge)
	} else if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	return doc.asStatusInfo(), nil
}

func getEntityKeysForStatus(mb modelBackend, keyType string, status status.Status) ([]string, error) {
	statuses, closer := mb.db().GetCollection(statusesC)
	defer closer()

	var ids []bson.M
	query := bson.D{
		{"_id", bson.D{{"$regex", fmt.Sprintf(".+\\:%s#.+", keyType)}}},
		{"status", status},
	}
	err := statuses.Find(query).Select(bson.D{{"_id", 1}}).All(&ids)
	if err != nil {
		return nil, errors.Trace(err)
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = mb.localID(id["_id"].(string))
	}
	return keys, nil
}

// setStatusParams configures a setStatus call. All parameters are presumed to
// be set to valid values unless otherwise noted.
type setStatusParams struct {

	// badge is used to specialize any NotFound error emitted.
	badge string

	// statusKind is the kind of the entity for which the status is being set.
	statusKind string

	// statusId is the id of the entity for which the status is being set. It's
	// not necessarily the same as the entity's Id().
	statusId string

	// globalKey uniquely identifies the entity to which the
	globalKey string

	// status is the status value.
	status status.Status

	// message is an optional string elaborating upon the status.
	message string

	// rawData is a map of arbitrary data elaborating upon the status and
	// message. Its keys are assumed not to have been escaped.
	rawData map[string]interface{}

	// token, if present, must accept an *[]txn.Op passed to its Check method,
	// and will prevent any change if it becomes invalid.
	token leadership.Token

	// updated, the time the status was set.
	updated *time.Time

	// historyOverwrite provides an optional ability to write a different
	// version of status as history (vs. what status actually gets set.)
	// Used only with caas models as there is currently no way for a charm
	// to query its' workload and the cloud container status might contradict
	// what it thinks it is.
	historyOverwrite *statusDoc
}

func timeOrNow(t *time.Time, clock clock.Clock) *time.Time {
	if t == nil {
		now := clock.Now()
		t = &now
	}
	return t
}

// setStatus inteprets the supplied params as documented on the type.
func setStatus(db Database, params setStatusParams, recorder status.StatusHistoryRecorder) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set status")
	if params.updated == nil {
		return errors.NotValidf("nil updated time")
	}

	doc := statusDoc{
		Status:     params.status,
		StatusInfo: params.message,
		StatusData: utils.EscapeKeys(params.rawData),
		Updated:    params.updated.UnixNano(),
	}

	historyDoc := &doc
	if params.historyOverwrite != nil {
		historyDoc = params.historyOverwrite
	}

	newStatus, historyErr := probablyUpdateStatusHistory(db,
		params.statusKind, params.statusId, params.globalKey, *historyDoc, recorder)
	if params.historyOverwrite == nil && (!newStatus && historyErr == nil) {
		// If this status is not new (i.e. it is exactly the same as
		// our last status), there is no need to update the record.
		// Update here will only reset the 'Since' field.
		return nil
	}

	// Set the authoritative status document, or fail trying.
	var buildTxn jujutxn.TransactionSource = func(int) ([]txn.Op, error) {
		return statusSetOps(db, doc, params.globalKey)
	}
	if params.token != nil {
		buildTxn = buildTxnWithLeadership(buildTxn, params.token)
	}
	err = db.Run(buildTxn)
	if cause := errors.Cause(err); cause == mgo.ErrNotFound {
		return errors.NotFoundf(params.badge)
	}
	return errors.Trace(err)
}

func statusSetOps(db Database, doc statusDoc, globalKey string) ([]txn.Op, error) {
	update := bson.D{{"$set", &doc}}
	txnRevno, err := readTxnRevno(db, statusesC, globalKey)
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

// createStatusOp returns the operation needed to create the given status
// document associated with the given globalKey.
func createStatusOp(mb modelBackend, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     mb.docID(globalKey),
		Assert: txn.DocMissing,
		Insert: &doc,
	}
}

// removeStatusOp returns the operation needed to remove the status
// document associated with the given globalKey.
func removeStatusOp(mb modelBackend, globalKey string) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     mb.docID(globalKey),
		Remove: true,
	}
}

// globalKeyField must have the same value as the tag for
// historicalStatusDoc.GlobalKey.
const globalKeyField = "globalkey"

type historicalStatusDoc struct {
	ModelUUID  string                 `bson:"model-uuid"`
	GlobalKey  string                 `bson:"globalkey"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`

	// Updated might not be present on statuses copied by old
	// versions of juju from yet older versions of juju.
	Updated int64 `bson:"updated"`
}

type recordedHistoricalStatusDoc struct {
	ID         bson.ObjectId          `bson:"_id"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`
}

// probablyUpdateStatusHistory inspects existing status-history
// and determines if this status is new or the same as the last recorded.
// If this is a new status, a new status history record will be added.
// If this status is the same as the last status we've received,
// we update that record to have a new timestamp.
// Status messages are considered to be the same if they only differ in their timestamps.
// The call returns true if a new status history record has been created.
func probablyUpdateStatusHistory(db Database,
	statusKind string, statusId string, globalKey string, doc statusDoc, logRecorder status.StatusHistoryRecorder) (bool, error) {
	historyDoc := &historicalStatusDoc{
		Status:     doc.Status,
		StatusInfo: doc.StatusInfo,
		StatusData: doc.StatusData, // coming from a statusDoc, already escaped
		Updated:    doc.Updated,
		GlobalKey:  globalKey,
	}

	logRecorder(statusKind, statusId, doc.Status, doc.StatusInfo)

	history, closer := db.GetCollection(statusesHistoryC)
	defer closer()

	exists, currentID := statusHistoryExists(db, historyDoc)
	if exists {
		// If the status values have not changed since the last run,
		// update history record with this timestamp
		// to keep correct track of when SetStatus ran.
		historyW := history.Writeable()
		err := historyW.Update(
			bson.D{{"_id", currentID}},
			bson.D{{"$set", bson.D{{"updated", doc.Updated}}}})
		if err != nil {
			logger.Errorf("failed to update status history: %v", err)
			return false, err
		}
		return false, nil
	}

	historyW := history.Writeable()
	err := historyW.Insert(historyDoc)
	if err != nil {
		logger.Errorf("failed to write status history: %v", err)
		return false, err
	}
	return true, nil
}

func statusHistoryExists(db Database, historyDoc *historicalStatusDoc) (bool, bson.ObjectId) {
	// Find the current value to see if it is worthwhile adding the new
	// status value.
	history, closer := db.GetCollection(statusesHistoryC)
	defer closer()

	var latest []recordedHistoricalStatusDoc
	query := history.Find(bson.D{{globalKeyField, historyDoc.GlobalKey}})
	query = query.Sort("-updated").Limit(1)
	err := query.All(&latest)
	if err == nil && len(latest) == 1 {
		current := latest[0]
		// Short circuit the writing to the DB if the status, message,
		// and data match.
		dataSame := func(left, right map[string]interface{}) bool {
			// If they are both empty, then it is the same.
			if len(left) == 0 && len(right) == 0 {
				return true
			}
			// If either are now empty, they aren't the same.
			if len(left) == 0 || len(right) == 0 {
				return false
			}
			// Failing that, use reflect.
			return reflect.DeepEqual(left, right)
		}
		// Check the data last as the short circuit evaluation may mean
		// we rarely need to drop down into the reflect library.
		if current.Status == historyDoc.Status &&
			current.StatusInfo == historyDoc.StatusInfo &&
			dataSame(current.StatusData, historyDoc.StatusData) {
			return true, current.ID
		}
	}
	return false, ""
}

// eraseStatusHistory removes all status history documents for
// the given global key. The documents are removed in batches
// to avoid locking the status history collection for extended
// periods of time, preventing status history being recorded
// for other entities.
func eraseStatusHistory(stop <-chan struct{}, mb modelBackend, globalKey string) error {
	// TODO(axw) restructure status history so we have one
	// document per global key, and sub-documents per status
	// recording. This method would then become a single
	// Remove operation.

	history, closer := mb.db().GetCollection(statusesHistoryC)
	defer closer()

	iter := history.Find(bson.D{{
		globalKeyField, globalKey,
	}}).Select(bson.M{"_id": 1}).Iter()
	defer iter.Close()

	logFormat := "deleted %d status history documents for " + fmt.Sprintf("%q", globalKey)
	deleted, err := deleteInBatches(
		stop,
		history.Writeable().Underlying(), nil, "", iter,
		logFormat, loggo.DEBUG,
		noEarlyFinish,
	)
	if err != nil {
		return errors.Trace(err)
	}
	if deleted > 0 {
		logger.Debugf(logFormat, deleted)
	}
	return nil
}

// statusHistoryArgs hold the arguments to call statusHistory.
type statusHistoryArgs struct {
	db        Database
	globalKey string
	filter    status.StatusHistoryFilter
	clock     clock.Clock
}

// fetchNStatusResults will return status for the given key filtered with the
// given filter or error.
func fetchNStatusResults(col mongo.Collection, clock clock.Clock,
	key string, filter status.StatusHistoryFilter) ([]historicalStatusDoc, error) {
	var (
		docs  []historicalStatusDoc
		query mongo.Query
	)
	baseQuery := bson.M{"globalkey": key}
	if filter.Delta != nil {
		delta := *filter.Delta
		updated := clock.Now().Add(-delta)
		baseQuery["updated"] = bson.M{"$gt": updated.UnixNano()}
	}
	if filter.FromDate != nil {
		baseQuery["updated"] = bson.M{"$gt": filter.FromDate.UnixNano()}
	}
	excludes := []string{}
	excludes = append(excludes, filter.Exclude.Values()...)
	if len(excludes) > 0 {
		baseQuery["statusinfo"] = bson.M{"$nin": excludes}
	}

	query = col.Find(baseQuery).Sort("-updated")
	if filter.Size > 0 {
		query = query.Limit(filter.Size)
	}
	err := query.All(&docs)

	if err == mgo.ErrNotFound {
		return []historicalStatusDoc{}, errors.NotFoundf("status history")
	} else if err != nil {
		return []historicalStatusDoc{}, errors.Annotatef(err, "cannot get status history")
	}
	return docs, nil

}

func statusHistory(args *statusHistoryArgs) ([]status.StatusInfo, error) {
	if err := args.filter.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating arguments")
	}
	statusHistory, closer := args.db.GetCollection(statusesHistoryC)
	defer closer()

	var results []status.StatusInfo
	docs, err := fetchNStatusResults(statusHistory, args.clock, args.globalKey, args.filter)
	partial := []status.StatusInfo{}
	if err != nil {
		return []status.StatusInfo{}, errors.Trace(err)
	}
	for _, doc := range docs {
		partial = append(partial, status.StatusInfo{
			Status:  doc.Status,
			Message: doc.StatusInfo,
			Data:    utils.UnescapeKeys(doc.StatusData),
			Since:   unixNanoToTime(doc.Updated),
		})
	}
	results = partial
	return results, nil
}

// PruneStatusHistory prunes the status history collection.
func PruneStatusHistory(stop <-chan struct{}, st *State, maxHistoryTime time.Duration, maxHistoryMB int) error {
	coll, closer := st.db().GetRawCollection(statusesHistoryC)
	defer closer()

	err := pruneCollection(stop, st, maxHistoryTime, maxHistoryMB, coll, "updated", nil, NanoSeconds)
	return errors.Trace(err)
}
