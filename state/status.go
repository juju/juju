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

	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/status"
)

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
func getStatus(st *State, globalKey, badge string) (_ status.StatusInfo, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot get status")
	statuses, closer := st.getCollection(statusesC)
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

// setStatus inteprets the supplied params as documented on the type.
func setStatus(st *State, params setStatusParams) (err error) {
	defer errors.DeferredAnnotatef(&err, "cannot set status")

	doc := statusDoc{
		Status:     params.status,
		StatusInfo: params.message,
		StatusData: utils.EscapeKeys(params.rawData),
		Updated:    params.updated.UnixNano(),
	}
	probablyUpdateStatusHistory(st, params.globalKey, doc)

	// Set the authoritative status document, or fail trying.
	buildTxn := updateStatusSource(st, params.globalKey, doc)
	if params.token != nil {
		buildTxn = buildTxnWithLeadership(buildTxn, params.token)
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

type historicalStatusDoc struct {
	ModelUUID  string                 `bson:"model-uuid"`
	GlobalKey  string                 `bson:"globalkey"`
	Status     status.Status          `bson:"status"`
	StatusInfo string                 `bson:"statusinfo"`
	StatusData map[string]interface{} `bson:"statusdata"`

	// Updated might not be present on statuses copied by old versions of juju
	// from yet older versions of juju. Do not dereference without checking.
	// Updated *time.Time `bson:"updated"`
	Updated int64 `bson:"updated"`
}

func probablyUpdateStatusHistory(st *State, globalKey string, doc statusDoc) {
	historyDoc := &historicalStatusDoc{
		Status:     doc.Status,
		StatusInfo: doc.StatusInfo,
		StatusData: doc.StatusData, // coming from a statusDoc, already escaped
		Updated:    doc.Updated,
		GlobalKey:  globalKey,
	}
	history, closer := st.getCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()
	if err := historyW.Insert(historyDoc); err != nil {
		logger.Errorf("failed to write status history: %v", err)
	}
}

// statusHistoryArgs hold the arguments to call statusHistory.
type statusHistoryArgs struct {
	st        *State
	globalKey string
	filter    status.StatusHistoryFilter
}

func statusHistory(args *statusHistoryArgs) ([]status.StatusInfo, error) {
	filter := args.filter
	if err := args.filter.Validate(); err != nil {
		return nil, errors.Annotate(err, "validating arguments")
	}
	statusHistory, closer := args.st.getCollection(statusesHistoryC)
	defer closer()

	var (
		docs  []historicalStatusDoc
		query mongo.Query
	)
	baseQuery := bson.M{"globalkey": args.globalKey}
	if filter.Delta != nil {
		delta := *filter.Delta
		// TODO(perrito666) 2016-05-02 lp:1558657
		updated := time.Now().Add(-delta)
		baseQuery = bson.M{"updated": bson.M{"$gt": updated.UnixNano()}, "globalkey": args.globalKey}
	}
	if filter.Date != nil {
		baseQuery = bson.M{"updated": bson.M{"$gt": filter.Date.UnixNano()}, "globalkey": args.globalKey}
	}
	query = statusHistory.Find(baseQuery).Sort("-updated")
	if filter.Size > 0 {
		query = query.Limit(filter.Size)
	}
	err := query.All(&docs)

	if err == mgo.ErrNotFound {
		return []status.StatusInfo{}, errors.NotFoundf("status history")
	} else if err != nil {
		return []status.StatusInfo{}, errors.Annotatef(err, "cannot get status history")
	}

	results := make([]status.StatusInfo, len(docs))
	for i, doc := range docs {
		results[i] = status.StatusInfo{
			Status:  doc.Status,
			Message: doc.StatusInfo,
			Data:    utils.UnescapeKeys(doc.StatusData),
			Since:   unixNanoToTime(doc.Updated),
		}
	}
	return results, nil
}

// PruneStatusHistory removes status history entries until
// only logs newer than <maxLogTime> remain and also ensures
// that the collection is smaller than <maxLogsMB> after the
// deletion.
func PruneStatusHistory(st *State, maxHistoryTime time.Duration, maxHistoryMB int) error {
	if maxHistoryMB < 0 {
		return errors.NotValidf("non-positive maxHistoryMB")
	}
	if maxHistoryTime < 0 {
		return errors.NotValidf("non-positive maxHistoryTime")
	}
	if maxHistoryMB == 0 && maxHistoryTime == 0 {
		return errors.NotValidf("backlog size and time constraints are both 0")
	}
	history, closer := st.getRawCollection(statusesHistoryC)
	defer closer()

	// Status Record Age
	if maxHistoryTime > 0 {
		t := st.clock.Now().Add(-maxHistoryTime)
		_, err := history.RemoveAll(bson.D{
			{"updated", bson.M{"$lt": t.UnixNano()}},
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	if maxHistoryMB == 0 {
		return nil
	}
	// Collection Size
	collMB, err := getCollectionMB(history)
	if err != nil {
		return errors.Annotate(err, "retrieving status history collection size")
	}
	if collMB <= maxHistoryMB {
		return nil
	}
	// TODO(perrito666) explore if there would be any beneffit from having the
	// size limit be per model
	count, err := history.Count()
	if err == mgo.ErrNotFound || count <= 0 {
		return nil
	}
	if err != nil {
		return errors.Annotate(err, "counting status history records")
	}
	// We are making the assumption that status sizes can be averaged for
	// large numbers and we will get a reasonable approach on the size.
	// Note: Capped collections are not used for this because they, currently
	// at least, lack a way to be resized and the size is expected to change
	// as real life data of the history usage is gathered.
	sizePerStatus := float64(collMB) / float64(count)
	if sizePerStatus == 0 {
		return errors.New("unexpected result calculating status history entry size")
	}
	deleteStatuses := count - int(float64(collMB-maxHistoryMB)/sizePerStatus)
	result := historicalStatusDoc{}
	err = history.Find(nil).Sort("-updated").Skip(deleteStatuses).One(&result)
	if err != nil {
		return errors.Trace(err)
	}
	_, err = history.RemoveAll(bson.D{
		{"updated", bson.M{"$lt": result.Updated}},
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
