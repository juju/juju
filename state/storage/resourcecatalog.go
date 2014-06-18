// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	statetxn "github.com/juju/juju/state/txn"
)

// ErrUploadPending is used to indicate that the underlying resource for a catalog entry
// is not yet fully uploaded.
var ErrUploadPending = fmt.Errorf("Resource not available because upload is not yet complete")

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
	// Pending is true while the underlying resource is uploaded.
	Pending bool
}

// resourceCatalog is a mongo backed ResourceCatalog instance.
type resourceCatalog struct {
	txnRunner  statetxn.Runner
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
		Pending:    true,
	}
}

// newResourceCatalog creates a new ResourceCatalog using the transaction runner and
// storing resource entries in the mongo collection.
func newResourceCatalog(collection *mgo.Collection, txnRunner statetxn.Runner) ResourceCatalog {
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
	if doc.Pending {
		return nil, ErrUploadPending
	}
	return &Resource{
		Path: doc.Path,
		ResourceHash: ResourceHash{
			MD5Hash:    doc.MD5Hash,
			SHA256Hash: doc.SHA256Hash},
	}, nil
}

// Put is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Put(rh *ResourceHash) (id, path string, isNew bool, err error) {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		id, path, isNew, ops, err = rc.resourceIncRefOps(rh)
		return ops, err
	}
	if err = rc.txnRunner.Run(buildTxn); err != nil {
		return "", "", false, err
	}

	return id, path, isNew, nil
}

// UploadComplete is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) UploadComplete(id string) error {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		if ops, err = rc.uploadCompleteOps(id); err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource with id %q", id)
		}
		return ops, err
	}
	return rc.txnRunner.Run(buildTxn)
}

// Remove is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Remove(id string) error {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		if ops, err = rc.resourceDecRefOps(id); err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource with id %q", id)
		}
		return ops, err
	}
	return rc.txnRunner.Run(buildTxn)
}

func checksumMatch(rh *ResourceHash) bson.D {
	return bson.D{{"md5hash", rh.MD5Hash}, {"sha256hash", rh.SHA256Hash}}
}

func (rc *resourceCatalog) resourceIncRefOps(rh *ResourceHash) (id, path string, isNew bool, ops []txn.Op, err error) {
	var doc resourceDoc
	exists := false
	checksumMatchTerm := checksumMatch(rh)
	err = rc.collection.Find(checksumMatchTerm).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return "", "", false, nil, err
	} else if err == nil {
		exists = true
	}
	if !exists {
		doc := newResourceDoc(rh)
		return doc.Id, doc.Path, true, []txn.Op{{
			C:      rc.collection.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	return doc.Id, doc.Path, false, []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: checksumMatchTerm,
		Update: bson.D{{"$inc", bson.D{{"refcount", 1}}}},
	}}, nil
}

func (rc *resourceCatalog) uploadCompleteOps(id string) ([]txn.Op, error) {
	var doc resourceDoc
	if err := rc.collection.FindId(id).One(&doc); err != nil {
		return nil, err
	}
	if !doc.Pending {
		return nil, statetxn.ErrNoOperations
	}
	return []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"pending", false}}}},
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
