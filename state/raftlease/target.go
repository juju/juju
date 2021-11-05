// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"fmt"

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
	Infof(string, ...interface{})
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

// Expiries is part of raftlease.NotifyTarget.
func (t *notifyTarget) Expiries(keys []lease.Key) error {
	if len(keys) == 0 {
		return nil
	}

	coll, closer := t.mongo.GetCollection(t.collection)
	defer closer()

	// Cache all the document idents up front, incase we have to retry the
	// transaction again. Also this serves as a de-duping process.
	uniqueDocIds := make(map[lease.Key]string)
	for _, key := range keys {
		uniqueDocIds[key] = leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)
	}
	docIds := make([]string, 0, len(uniqueDocIds))
	for _, docId := range uniqueDocIds {
		docIds = append(docIds, docId)
	}
	t.logger.Infof("expiring leases %v", docIds)

	err := t.mongo.RunTransaction(func(_ int) ([]txn.Op, error) {
		// Bulk get the records, to prevent potato programming.
		existingDocs, err := getRecords(coll, docIds)
		if errors.Cause(err) == mgo.ErrNotFound {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}

		ops := make([]txn.Op, len(existingDocs))
		for k, doc := range existingDocs {
			ops[k] = txn.Op{
				C:  t.collection,
				Id: doc.Id,
				Assert: bson.M{
					fieldHolder: doc.Holder,
				},
				Remove: true,
			}
		}

		return ops, nil
	})

	return errors.Annotatef(err, "%v in db", docIds)
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
