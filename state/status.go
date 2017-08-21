// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils/clock"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/status"
)

// ModelStatus holds all the current status values for a given model
// and offers accessors for the various parts of a model.
type ModelStatus struct {
	model *Model
	docs  map[string]statusDocWithID
}

// LoadModelStatus retrieves all the status documents for the model
// at once. Used to primarily speed up status.
func (m *Model) LoadModelStatus() (*ModelStatus, error) {
	db, closer := m.modelDatabase()
	defer closer()
	statuses, closer := db.GetCollection(statusesC)
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

func (m *ModelStatus) transform(doc statusDocWithID) status.StatusInfo {
	return status.StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    utils.UnescapeKeys(doc.StatusData),
		Since:   unixNanoToTime(doc.Updated),
	}
}

func (m *ModelStatus) getStatus(key, badge string) (status.StatusInfo, error) {
	doc, err := m.getDoc(key, badge)
	if err != nil {
		return status.StatusInfo{}, err
	}
	return m.transform(doc), nil
}

// Model returns the status of the model.
func (m *ModelStatus) Model() (status.StatusInfo, error) {
	return m.getStatus(m.model.globalKey(), "model")
}

// Application returns the status of the model.
// The unitNames are needed due to the current weird implementation of
// application status.
func (m *ModelStatus) Application(appName string, unitNames []string) (status.StatusInfo, error) {
	// This is kinda terrible, see notes in applcation.go for *Application.Status().
	doc, err := m.getDoc(applicationGlobalKey(appName), "application")
	if err != nil {
		return status.StatusInfo{}, err
	}
	if doc.NeverSet {
		// Get the status for the agents, and derive a status from that.
		var unitStatuses []status.StatusInfo
		for _, name := range unitNames {
			unitStatus, err := m.UnitWorkload(name)
			if err != nil {
				errors.Annotatef(err, "deriving application status from %q", name)
			}
			unitStatuses = append(unitStatuses, unitStatus)
		}
		if len(unitStatuses) > 0 {
			return deriveApplicationStatus(unitStatuses), nil
		}

	}
	return m.transform(doc), nil
}

// MachineAgent returns the status of the machine agent.
func (m *ModelStatus) MachineAgent(machineID string) (status.StatusInfo, error) {
	return m.getStatus(machineGlobalKey(machineID), "machine")
}

// MachineInstance returns the status of the machine instance.
func (m *ModelStatus) MachineInstance(machineID string) (status.StatusInfo, error) {
	return m.getStatus(machineGlobalInstanceKey(machineID), "instance")
}

// FullUnitWorkloadVersion returns the full status info for the workload
// version of a unit. This is used for selecting the workload version for
// an application.
func (m *ModelStatus) FullUnitWorkloadVersion(unitName string) (status.StatusInfo, error) {
	return m.getStatus(globalWorkloadVersionKey(unitName), "workload")
}

// UnitWorkload returns the status of the machine instance.
func (m *ModelStatus) UnitWorkloadVersion(unitName string) (string, error) {
	info, err := m.getStatus(globalWorkloadVersionKey(unitName), "workload")
	if err != nil {
		return "", err
	}
	return info.Message, nil
}

// UnitWorkload returns the status of the machine instance.
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

// UnitWorkload returns the status of the machine instance.
func (m *ModelStatus) UnitWorkload(unitName string) (status.StatusInfo, error) {
	// We do horrible things with unit status.
	// See notes in unit.go.
	info, err := m.getStatus(unitAgentGlobalKey(unitName), "unit")
	if err != nil {
		return info, err
	} else if info.Status == status.Error {
		return info, nil
	}

	return m.getStatus(unitGlobalKey(unitName), "workload")
}

type statusDocWithID struct {
	ID         string                 `bson:"_id"`
	ModelUUID  string                 `bson:"model-uuid"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`
	Updated    int64                  `bson:"updated"`
	NeverSet   bool                   `bson:"neverset"`
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

	// TODO(fwereade/wallyworld): lp:1479278
	// NeverSet is a short-term hack to work around a misfeature in service
	// status. To maintain current behaviour, we create service status docs
	// (and only service status documents) with NeverSet true; and then, when
	// reading them, if NeverSet is still true, we aggregate status from the
	// units instead.
	NeverSet bool `bson:"neverset"`
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

	return status.StatusInfo{
		Status:  doc.Status,
		Message: doc.StatusInfo,
		Data:    utils.UnescapeKeys(doc.StatusData),
		Since:   unixNanoToTime(doc.Updated),
	}, nil
}

// setStatusParams configures a setStatus call. All parameters are presumed to
// be set to valid values unless otherwise noted.
type setStatusParams struct {

	// badge is used to specialize any NotFound error emitted.
	badge string

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

	// udpated, the time the status was set.
	updated *time.Time
}

func timeOrNow(t *time.Time, clock clock.Clock) *time.Time {
	if t == nil {
		now := clock.Now()
		t = &now
	}
	return t
}

// setStatus inteprets the supplied params as documented on the type.
func setStatus(db Database, params setStatusParams) (err error) {
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
	probablyUpdateStatusHistory(db, params.globalKey, doc)

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
func createStatusOp(st *State, globalKey string, doc statusDoc) txn.Op {
	return txn.Op{
		C:      statusesC,
		Id:     st.docID(globalKey),
		Assert: txn.DocMissing,
		Insert: &doc,
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

func probablyUpdateStatusHistory(db Database, globalKey string, doc statusDoc) {
	historyDoc := &historicalStatusDoc{
		Status:     doc.Status,
		StatusInfo: doc.StatusInfo,
		StatusData: doc.StatusData, // coming from a statusDoc, already escaped
		Updated:    doc.Updated,
		GlobalKey:  globalKey,
	}
	history, closer := db.GetCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()
	if err := historyW.Insert(historyDoc); err != nil {
		logger.Errorf("failed to write status history: %v", err)
	}
}

func eraseStatusHistory(st *State, globalKey string) error {
	history, closer := st.db().GetCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()

	if _, err := historyW.RemoveAll(bson.D{{globalKeyField, globalKey}}); err != nil {
		return err
	}
	return nil
}

// statusHistoryArgs hold the arguments to call statusHistory.
type statusHistoryArgs struct {
	db        Database
	globalKey string
	filter    status.StatusHistoryFilter
}

// fetchNStatusResults will return status for the given key filtered with the
// given filter or error.
func fetchNStatusResults(col mongo.Collection, key string,
	filter status.StatusHistoryFilter) ([]historicalStatusDoc, error) {
	var (
		docs  []historicalStatusDoc
		query mongo.Query
	)
	baseQuery := bson.M{"globalkey": key}
	if filter.Delta != nil {
		delta := *filter.Delta
		// TODO(perrito666) 2016-10-06 lp:1558657
		updated := time.Now().Add(-delta)
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
	docs, err := fetchNStatusResults(statusHistory, args.globalKey, args.filter)
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

func PruneStatusHistory(st *State, maxHistoryTime time.Duration, maxHistoryMB int) error {
	err := pruneCollection(st, maxHistoryTime, maxHistoryMB, statusesHistoryC, "updated", NanoSeconds)
	return errors.Trace(err)
}
