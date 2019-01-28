// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"fmt"
	"io"
	"log"
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

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

// Logger allows us to report errors if we can't write to the database
// for some reason.
type Logger interface {
	Errorf(string, ...interface{})
}

// NewNotifyTarget returns something that can be used to report lease
// changes.
func NewNotifyTarget(mongo Mongo, collection string, logDest io.Writer, errorLogger Logger) raftlease.NotifyTarget {
	return &notifyTarget{
		mongo:       mongo,
		collection:  collection,
		logger:      log.New(logDest, "", log.LstdFlags|log.Lmicroseconds|log.LUTC),
		errorLogger: errorLogger,
	}
}

// notifyTarget is a raftlease.NotifyTarget that updates the
// information in mongo, as well as logging the lease changes.  Since
// the callbacks it exposes aren't allowed to return errors, it takes
// a logger for write errors as well as a destination for tracing
// lease changes.
type notifyTarget struct {
	mongo       Mongo
	collection  string
	logger      *log.Logger
	errorLogger Logger
}

func (t *notifyTarget) log(message string, args ...interface{}) {
	err := t.logger.Output(2, fmt.Sprintf(message, args...))
	if err != nil {
		t.errorLogger.Errorf("couldn't write to lease log: %s", err.Error())
	}
}

// Claimed is part of raftlease.NotifyTarget.
func (t *notifyTarget) Claimed(key lease.Key, holder string) {
	coll, closer := t.mongo.GetCollection(t.collection)
	defer closer()
	docId := leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)
	err := t.mongo.RunTransaction(func(_ int) ([]txn.Op, error) {
		existingDoc, err := getRecord(coll, docId)
		switch {
		case err == mgo.ErrNotFound:
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
				C:      t.collection,
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
				C:  t.collection,
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
	})
	if err != nil {
		t.errorLogger.Errorf("couldn't claim lease %q for %q: %s", docId, holder, err.Error())
		return
	}
	t.log("claimed %q for %q", docId, holder)
}

// Expired is part of raftlease.NotifyTarget.
func (t *notifyTarget) Expired(key lease.Key) {
	coll, closer := t.mongo.GetCollection(t.collection)
	defer closer()
	docId := leaseHolderDocId(key.Namespace, key.ModelUUID, key.Lease)
	err := t.mongo.RunTransaction(func(_ int) ([]txn.Op, error) {
		existingDoc, err := getRecord(coll, docId)
		if err == mgo.ErrNotFound {
			return nil, jujutxn.ErrNoOperations
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []txn.Op{{
			C:  t.collection,
			Id: docId,
			Assert: bson.M{
				fieldHolder: existingDoc.Holder,
			},
			Remove: true,
		}}, nil
	})

	if err != nil {
		t.errorLogger.Errorf("couldn't expire lease %q: %s", docId, err.Error())
		return
	}
	t.log("expired %q", docId)
}

// MakeTrapdoorFunc returns a raftlease.TrapdoorFunc for the specified
// collection.
func MakeTrapdoorFunc(collection string) raftlease.TrapdoorFunc {
	return func(key lease.Key, holder string) lease.Trapdoor {
		op := txn.Op{
			C: collection,
			Id: leaseHolderDocId(
				key.Namespace,
				key.ModelUUID,
				key.Lease,
			),
			Assert: bson.M{
				fieldHolder: holder,
			},
		}
		return func(out interface{}, store lease.Store) error {
			outPtr, ok := out.(*[]txn.Op)
			if !ok {
				return errors.NotValidf("expected *[]txn.Op; %T", out)
			}

			// If being asked to sync, re-claim the lease for the specifies
			// holder so that the ensuing notifyTarget.Claimed() call updates
			// state. If the lease is not held by the input hold according to
			// Raft, an error will result.
			if store != nil {
				if err := store.ExtendLease(key, lease.Request{Holder: holder, Duration: time.Minute}); err != nil {
					return errors.Trace(err)
				}
			}

			*outPtr = []txn.Op{op}
			return nil
		}
	}
}

func getRecord(coll mongo.Collection, docId string) (leaseHolderDoc, error) {
	var doc leaseHolderDoc
	err := coll.FindId(docId).One(&doc)
	if err != nil {
		return leaseHolderDoc{}, err
	}
	return doc, nil
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
