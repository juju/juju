// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/mgo/v2/bson"
	"github.com/juju/mgo/v2/txn"
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/mongo"
)

const (
	// fieldNamespace identifies the namespace field in a leaseHolderDoc.
	fieldNamespace = "namespace"

	// fieldModelUUID identifies the model UUID field in a leaseHolderDoc.
	fieldModelUUID = "model-uuid"

	// fieldHolder identifies the holder field in a leaseHolderDoc.
	fieldHolder = "holder"
)

// logger is only used when we need to update the database from within
// a trapdoor function.
var logger = loggo.GetLogger("juju.state.raftlease")

// leaseHolderDoc is used to serialise lease holder info.
type leaseHolderDoc struct {
	Id        string `bson:"_id"`
	Namespace string `bson:"namespace"`
	ModelUUID string `bson:"model-uuid"`
	Lease     string `bson:"lease"`
	Holder    string `bson:"holder"`
}

// leaseHolderDocId returns the _id for the document holding details of the supplied
// namespace and lease.
func leaseHolderDocId(namespace, modelUUID, lease string) string {
	return fmt.Sprintf("%s:%s#%s#", modelUUID, namespace, lease)
}

// validate returns an error if any fields are invalid or inconsistent.
func (doc leaseHolderDoc) validate() error {
	if doc.Id != leaseHolderDocId(doc.Namespace, doc.ModelUUID, doc.Lease) {
		return errors.Errorf("inconsistent _id")
	}
	if err := lease.ValidateString(doc.Holder); err != nil {
		return errors.Annotatef(err, "invalid holder")
	}
	return nil
}

// newLeaseHolderDoc returns a valid lease document encoding the supplied lease and
// entry in the supplied namespace, or an error.
func newLeaseHolderDoc(namespace, modelUUID, name, holder string) (*leaseHolderDoc, error) {
	doc := &leaseHolderDoc{
		Id:        leaseHolderDocId(namespace, modelUUID, name),
		Namespace: namespace,
		ModelUUID: modelUUID,
		Lease:     name,
		Holder:    holder,
	}
	if err := doc.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return doc, nil
}

// Mongo exposes MongoDB operations for use by the lease package.
type Mongo interface {

	// RunTransaction should probably delegate to a jujutxn.Runner's Run method.
	RunTransaction(jujutxn.TransactionSource) error

	// GetCollection should probably call the mongo.CollectionFromName func.
	GetCollection(name string) (collection mongo.Collection, closer func())
}

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warningf(string, ...interface{})
}

// NewNotifyTarget returns something that can be used to report lease
// changes.
func NewNotifyTarget(mongo Mongo, collection string, logger Logger) raftlease.NotifyTarget {
	return &notifyTarget{
		mongo:      mongo,
		collection: collection,
		logger:     logger,
	}
}

// notifyTarget is a raftlease.NotifyTarget that updates the
// information in mongo, as well as logging the lease changes.  Since
// the callbacks it exposes aren't allowed to return errors, it takes
// a logger for write errors as well as a destination for tracing
// lease changes.
type notifyTarget struct {
	mongo      Mongo
	collection string
	logger     Logger
}

func buildClaimedOps(coll mongo.Collection, docId string, key lease.Key, holder string) ([]txn.Op, error) {
	existingDoc, err := getRecord(coll, docId)
	switch {
	case errors.Cause(err) == mgo.ErrNotFound:
		doc, err := newLeaseHolderDoc(
			key.Namespace,
			key.ModelUUID,
			key.Lease,
			holder,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{{
			C:      coll.Name(),
			Id:     docId,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil

	case err != nil:
		return nil, errors.Trace(err)

	case existingDoc.Holder == holder:
		return nil, jujutxn.ErrNoOperations

	default:
		return []txn.Op{{
			C:  coll.Name(),
			Id: docId,
			Assert: bson.M{
				fieldHolder: existingDoc.Holder,
			},
			Update: bson.M{
				"$set": bson.M{
					fieldHolder: holder,
				},
			},
		}}, nil
	}
}

func applyClaimed(mongo Mongo, collection string, docId string, key lease.Key, holder string) (bool, error) {
	coll, closer := mongo.GetCollection(collection)
	defer closer()

	var writeNeeded bool
	err := mongo.RunTransaction(func(int) ([]txn.Op, error) {
		ops, err := buildClaimedOps(coll, docId, key, holder)
		writeNeeded = len(ops) != 0
		return ops, err
	})
	return writeNeeded, errors.Trace(err)
}

// Claimed is part of raftlease.NotifyTarget.
func (t *notifyTarget) Claimed(key lease.Key, holder string) error {
	docId := leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)
	t.logger.Infof("claiming lease %q for %q", docId, holder)

	_, err := applyClaimed(t.mongo, t.collection, docId, key, holder)
	return errors.Annotatef(err, "%q for %q in db", docId, holder)
}

type leaseDoc struct {
	Key    lease.Key
	DocId  string
	Holder string
}

// Expiries is part of raftlease.NotifyTarget.
func (t *notifyTarget) Expiries(expiries []raftlease.Expired) error {
	if len(expiries) == 0 {
		return nil
	}

	coll, closer := t.mongo.GetCollection(t.collection)
	defer closer()

	// Cache all the document idents up front, incase we have to retry the
	// transaction again. Also this serves as a de-duping process.
	leaseDocs := make(map[string]leaseDoc)
	for _, expired := range expiries {
		key := expired.Key
		docId := leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)

		if doc, ok := leaseDocs[docId]; ok && doc.Holder != expired.Holder {
			// We have an existing lease.Key already in the documents that
			// doesn't have the same holder.
			t.logger.Warningf("ignoring key %q, has existing lease key but different holders %q and %q", key, doc.Holder, expired.Holder)
			continue
		}

		leaseDocs[docId] = leaseDoc{
			Key:    key,
			DocId:  docId,
			Holder: expired.Holder,
		}
	}
	docIds := set.NewStrings()
	for _, leaseDoc := range leaseDocs {
		docIds.Add(leaseDoc.DocId)
	}
	sortedDocIds := docIds.SortedValues()
	t.logger.Debugf("expiring leases %v", sortedDocIds)

	err := t.mongo.RunTransaction(func(_ int) ([]txn.Op, error) {
		// Bulk get the records, to prevent potato programming.
		existingDocs, err := getRecords(coll, sortedDocIds)
		if errors.Cause(err) == mgo.ErrNotFound {
			t.logger.Warningf("unable to expire %v, no documents found", sortedDocIds)
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Ensure that we have all the docIds represented in the existingDocs
		// and that we haven't missed any.
		existingDocsSet := set.NewStrings()
		for _, doc := range existingDocs {
			existingDocsSet.Add(doc.Id)
		}
		if missing := docIds.Difference(existingDocsSet); len(missing) > 0 {
			// Not all documents are represented by the returned records query
			// from mongo. Report a warning, but continue and expire as much as
			// possible.
			t.logger.Warningf("we were requested to expire leases that we did not find: %v, continuing to expire remaining documents", missing)
		}

		ops := make([]txn.Op, 0)
		for _, doc := range existingDocs {
			leaseDoc, ok := leaseDocs[doc.Id]
			if !ok {
				// This should never happen, we have an existing document, that
				// doesn't have a leaseDoc.
				t.logger.Warningf("missing lease document for existing document %q", doc.Id)
				continue
			}

			if leaseDoc.Holder != doc.Holder {
				// The holder for the document has changed. We shouldn't attempt
				// to remove this document.
				t.logger.Infof("lease %q is currently held by %q but we were asked to expire it for %q, skipping", leaseDoc.DocId, leaseDoc.Holder, doc.Holder)
				continue
			}

			ops = append(ops, txn.Op{
				C:  t.collection,
				Id: doc.Id,
				Assert: bson.M{
					// Ensure that the lease holder is the same holder as the
					// one we where told to expire. This should prevent the
					// race of removing a lease that might have been changed to
					// another holder, before the batch remove.
					fieldHolder: leaseDoc.Holder,
				},
				Remove: true,
			})
		}

		return ops, nil
	})
	if err == nil {
		return nil
	}

	return errors.Annotatef(err, "%v in db", docIds.SortedValues())
}

// MakeTrapdoorFunc returns a raftlease.TrapdoorFunc for the specified
// collection.
func MakeTrapdoorFunc(mongo Mongo, collection string) raftlease.TrapdoorFunc {
	return func(key lease.Key, holder string) lease.Trapdoor {
		return func(attempt int, out interface{}) error {
			outPtr, ok := out.(*[]txn.Op)
			if !ok {
				return errors.NotValidf("expected *[]txn.Op; %T", out)
			}
			if attempt != 0 {
				// If the assertion failed it may be because a claim
				// notify failed in the past due to the DB not being
				// available. Sync the lease holder - this is safe to
				// do because raft is the arbiter of who really holds
				// the lease, and we check that the lease is held in
				// buildTxnWithLeadership each time before collecting
				// the assertion ops.
				docId := leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)
				writeNeeded, err := applyClaimed(mongo, collection, docId, key, holder)
				if err != nil {
					return errors.Trace(err)
				}
				if writeNeeded {
					logger.Infof("trapdoor claimed lease %q for %q", docId, holder)
				}
			}
			*outPtr = []txn.Op{{
				C: collection,
				Id: leaseHolderDocId(
					key.Namespace,
					key.ModelUUID,
					key.Lease,
				),
				Assert: bson.M{
					fieldHolder: holder,
				},
			}}
			return nil
		}
	}
}

func getRecord(coll mongo.Collection, docId string) (leaseHolderDoc, error) {
	var doc leaseHolderDoc
	if err := coll.FindId(docId).One(&doc); err != nil {
		return leaseHolderDoc{}, errors.Trace(err)
	}
	return doc, nil
}

func getRecords(coll mongo.Collection, docIds []string) ([]leaseHolderDoc, error) {
	var docs []leaseHolderDoc
	if err := coll.Find(bson.M{
		"_id": bson.M{
			"$in": docIds,
		},
	}).Sort("_id").All(&docs); err != nil {
		return nil, errors.Trace(err)
	}
	return docs, nil
}

// LeaseHolders returns a map of each lease and the holder in the
// specified namespace and model.
func LeaseHolders(mongo Mongo, collection, namespace, modelUUID string) (map[string]string, error) {
	coll, closer := mongo.GetCollection(collection)
	defer closer()

	iter := coll.Find(bson.M{
		fieldNamespace: namespace,
		fieldModelUUID: modelUUID,
	}).Iter()
	results := make(map[string]string)
	var doc leaseHolderDoc
	for iter.Next(&doc) {
		results[doc.Lease] = doc.Holder
	}

	if err := iter.Close(); err != nil {
		return nil, errors.Trace(err)
	}
	return results, nil
}
