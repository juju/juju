// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	statetxn "github.com/juju/juju/state/txn"
)

// ResourceHash contains hashes which are used to unambiguously
// identify stored data.
type ResourceHash struct {
	MD5Hash    string
	SHA256Hash string
}

// Resource is a catalog entry for stored data.
// It contains the path where the data is stored as well as
// hashes of the data which are used for de-duping.
type Resource struct {
	ResourceHash
	Path string
}

// resourceDoc is the persistent representation of a Resource.
type resourceDoc struct {
	Id         string `bson:"_id"`
	Path       string
	MD5Hash    string
	SHA256Hash string
	RefCount   int64
}

// resourceCatalog is a mongo backed ResourceCatalog instance.
type resourceCatalog struct {
	txnRunner  statetxn.TransactionRunner
	collection *mgo.Collection
}

var _ ResourceCatalog = (*resourceCatalog)(nil)

// newResource constructs a Resource from its attributes.
func newResource(path, md5hash, sha256hash string) *Resource {
	return &Resource{
		Path: path,
		ResourceHash: ResourceHash{
			MD5Hash:    md5hash,
			SHA256Hash: sha256hash},
	}
}

// newResourceDoc constructs a resourceDoc from a ResourceHash.
// This is used when writing new data to the resource store.
// Path is opaque and is generated using a bson object id.
func newResourceDoc(rh *ResourceHash) resourceDoc {
	return resourceDoc{
		Id:         rh.MD5Hash + rh.SHA256Hash,
		Path:       bson.NewObjectId().Hex(),
		MD5Hash:    rh.MD5Hash,
		SHA256Hash: rh.SHA256Hash,
		RefCount:   1,
	}
}

// newResourceCatalog creates a new ResourceCatalog using the transaction runner and
// storing resource entries in the mongo collection.
func newResourceCatalog(collection *mgo.Collection, txnRunner statetxn.TransactionRunner) ResourceCatalog {
	return &resourceCatalog{
		txnRunner:  txnRunner,
		collection: collection,
	}
}

// Get is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Get(id string) (*Resource, error) {
	var doc resourceDoc
	if err := rc.collection.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("resource with id %q", id)
	} else if err != nil {
		return nil, err
	}
	return &Resource{
		Path: doc.Path,
		ResourceHash: ResourceHash{
			MD5Hash:    doc.MD5Hash,
			SHA256Hash: doc.SHA256Hash},
	}, nil
}

// Put is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Put(rh *ResourceHash) (id, path string, err error) {
	txns := func(attempt int) (ops []txn.Op, err error) {
		id, path, ops, err = rc.resourceIncRefOps(rh)
		return ops, err
	}
	if err = rc.txnRunner.Run(txns); err != nil {
		return "", "", err
	}
	return id, path, nil
}

// Remove is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Remove(id string) error {
	txns := func(attempt int) (ops []txn.Op, err error) {
		if ops, err = rc.resourceDecRefOps(id); err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource with id %q", id)
		}
		return ops, err
	}
	return rc.txnRunner.Run(txns)
}

func checksumMatch(rh *ResourceHash) bson.D {
	return bson.D{{"md5hash", rh.MD5Hash}, {"sha256hash", rh.SHA256Hash}}
}

func (rc *resourceCatalog) resourceIncRefOps(rh *ResourceHash) (string, string, []txn.Op, error) {
	var doc resourceDoc
	exists := false
	checksumMatchTerm := checksumMatch(rh)
	err := rc.collection.Find(checksumMatchTerm).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return "", "", nil, err
	} else if err == nil {
		exists = true
	}
	if !exists {
		doc := newResourceDoc(rh)
		return doc.Id, doc.Path, []txn.Op{{
			C:      rc.collection.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	return doc.Id, doc.Path, []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: checksumMatchTerm,
		Update: bson.D{{"$inc", bson.D{{"refcount", 1}}}},
	}}, nil
}

func (rc *resourceCatalog) resourceDecRefOps(id string) ([]txn.Op, error) {
	var doc resourceDoc
	if err := rc.collection.FindId(id).One(&doc); err != nil {
		return nil, err
	}
	if doc.RefCount == 1 {
		return []txn.Op{{
			C:      rc.collection.Name,
			Id:     doc.Id,
			Assert: bson.D{{"refcount", 1}},
			Remove: true,
		}}, nil
	}
	return []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: bson.D{{"refcount", bson.D{{"$gt", 1}}}},
		Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
	}}, nil
}
