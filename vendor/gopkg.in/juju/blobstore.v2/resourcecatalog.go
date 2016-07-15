// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package blobstore

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
)

var (
	// ErrUploadPending is used to indicate that the underlying resource for a catalog entry
	// is not yet fully uploaded.
	ErrUploadPending = errors.New("Resource not available because upload is not yet complete")

	// errUploadedConcurrently is used to indicate that another client uploaded the
	// resource already.
	errUploadedConcurrently = errors.AlreadyExistsf("resource")
)

// Resource is a catalog entry for stored data.
// It contains the path where the data is stored as well as
// a hash of the data which are used for de-duping.
type Resource struct {
	SHA384Hash string
	Path       string
	Length     int64
}

// resourceDoc is the persistent representation of a Resource.
type resourceDoc struct {
	Id string `bson:"_id"`
	// Path is the storage path of the resource, which will be
	// the empty string until the upload has been completed.
	Path       string `bson:"path"`
	SHA384Hash string `bson:"sha384hash"`
	Length     int64  `bson:"length"`
	RefCount   int64  `bson:"refcount"`
}

// resourceCatalog is a mongo backed ResourceCatalog instance.
type resourceCatalog struct {
	collection *mgo.Collection
}

var _ ResourceCatalog = (*resourceCatalog)(nil)

// newResource constructs a Resource from its attributes.
func newResource(path, sha384hash string, length int64) *Resource {
	return &Resource{
		Path:       path,
		Length:     length,
		SHA384Hash: sha384hash,
	}
}

// newResourceDoc constructs a resourceDoc from a sha384 hash.
// This is used when writing new data to the resource store.
// Path is opaque and is generated using a bson object id.
func newResourceDoc(sha384Hash string, length int64) resourceDoc {
	return resourceDoc{
		Id:         sha384Hash,
		SHA384Hash: sha384Hash,
		RefCount:   1,
		Length:     length,
	}
}

const (
	// resourceCatalogCollection is the name of the collection
	// which stores the resourceDoc records.
	resourceCatalogCollection = "storedResources"
)

// newResourceCatalog creates a new ResourceCatalog
// storing resource entries in the mongo database.
func newResourceCatalog(db *mgo.Database) ResourceCatalog {
	return &resourceCatalog{
		collection: db.C(resourceCatalogCollection),
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
	if doc.Path == "" {
		return nil, ErrUploadPending
	}
	return newResource(doc.Path, doc.SHA384Hash, doc.Length), nil
}

// Find is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Find(hash string) (string, error) {
	var doc resourceDoc
	if err := rc.collection.Find(checksumMatch(hash)).One(&doc); err == mgo.ErrNotFound {
		return "", errors.NotFoundf("resource with sha384=%q", hash)
	} else if err != nil {
		return "", err
	}
	if doc.Path == "" {
		return "", ErrUploadPending
	}
	return doc.Id, nil
}

// Put is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Put(hash string, length int64) (id, path string, err error) {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		id, path, ops, err = rc.resourceIncRefOps(hash, length)
		return ops, err
	}
	txnRunner := txnRunner(rc.collection.Database)
	if err = txnRunner.Run(buildTxn); err != nil {
		return "", "", err
	}
	return id, path, nil
}

// UploadComplete is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) UploadComplete(id, path string) error {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		if ops, err = rc.uploadCompleteOps(id, path); err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource with id %q", id)
		}
		return ops, err
	}
	txnRunner := txnRunner(rc.collection.Database)
	return txnRunner.Run(buildTxn)
}

// Remove is defined on the ResourceCatalog interface.
func (rc *resourceCatalog) Remove(id string) (wasDeleted bool, path string, err error) {
	buildTxn := func(attempt int) (ops []txn.Op, err error) {
		if wasDeleted, path, ops, err = rc.resourceDecRefOps(id); err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource with id %q", id)
		}
		return ops, err
	}
	txnRunner := txnRunner(rc.collection.Database)
	return wasDeleted, path, txnRunner.Run(buildTxn)
}

func checksumMatch(hash string) bson.D {
	return bson.D{{"sha384hash", hash}}
}

func (rc *resourceCatalog) resourceIncRefOps(hash string, length int64) (
	id, path string, ops []txn.Op, err error,
) {
	var doc resourceDoc
	exists := false
	checksumMatchTerm := checksumMatch(hash)
	err = rc.collection.Find(checksumMatchTerm).One(&doc)
	if err != nil && err != mgo.ErrNotFound {
		return "", "", nil, err
	} else if err == nil {
		exists = true
	}
	if !exists {
		doc := newResourceDoc(hash, length)
		return doc.Id, "", []txn.Op{{
			C:      rc.collection.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	if doc.Length != length {
		return "", "", nil, errors.Errorf("length mismatch in resource document %d != %d", doc.Length, length)
	}
	return doc.Id, doc.Path, []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: checksumMatchTerm,
		Update: bson.D{{"$inc", bson.D{{"refcount", 1}}}},
	}}, nil
}

func (rc *resourceCatalog) uploadCompleteOps(id, path string) ([]txn.Op, error) {
	var doc resourceDoc
	if err := rc.collection.FindId(id).One(&doc); err != nil {
		return nil, err
	}
	if doc.Path != "" {
		return nil, errUploadedConcurrently
	}
	return []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: bson.D{{"path", ""}}, // doc exists, path is unset
		Update: bson.D{{"$set", bson.D{{"path", path}}}},
	}}, nil
}

func (rc *resourceCatalog) resourceDecRefOps(id string) (wasDeleted bool, path string, ops []txn.Op, err error) {
	var doc resourceDoc
	if err = rc.collection.FindId(id).One(&doc); err != nil {
		return false, "", nil, err
	}
	if doc.RefCount == 1 {
		return true, doc.Path, []txn.Op{{
			C:      rc.collection.Name,
			Id:     doc.Id,
			Assert: bson.D{{"refcount", 1}},
			Remove: true,
		}}, nil
	}
	return false, doc.Path, []txn.Op{{
		C:      rc.collection.Name,
		Id:     doc.Id,
		Assert: bson.D{{"refcount", bson.D{{"$gt", 1}}}},
		Update: bson.D{{"$inc", bson.D{{"refcount", -1}}}},
	}}, nil
}
