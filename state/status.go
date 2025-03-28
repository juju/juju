// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/mongo/utils"
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

	// updated, the time the status was set.
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

	// Set the authoritative status document, or fail trying.
	var buildTxn jujutxn.TransactionSource = func(int) ([]txn.Op, error) {
		return statusSetOps(db, doc, params.globalKey)
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
