// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"

	statetxn "github.com/juju/juju/state/txn"
)

// ManagedResource is a catalog entry for stored data.
// The data may be associated with a specified environment and/or user.
// The data is logically considered to be stored at the specified path.
type ManagedResource struct {
	EnvUUID string
	User    string
	Path    string
}

// managedResourceDoc is the persistent representation of a ManagedResource.
type managedResourceDoc struct {
	Id         string `bson:"_id"`
	EnvUUID    string
	User       string
	Path       string
	ResourceId string
}

// managedStorage is a mongo backed ManagedResource instance.
type managedStorage struct {
	resourceStore             ResourceStorage
	resourceCatalog           ResourceCatalog
	managedResourceCollection *mgo.Collection
	txnRunner                 statetxn.Runner
}

var _ ManagedStorage = (*managedStorage)(nil)

// newManagedResourceDoc constructs a managedResourceDoc from a ManagedResource and resource id.
// This is used when writing new data to the managed storage catalog.
func newManagedResourceDoc(r ManagedResource, resourceId string) managedResourceDoc {
	return managedResourceDoc{
		Id:         r.Path,
		ResourceId: resourceId,
		Path:       r.Path,
		EnvUUID:    r.EnvUUID,
		User:       r.User,
	}
}

const (
	resourceCatalogCollection = "storedResources"
	managedResourceCollection = "managedStoredResources"
)

// NewManagedStorage creates a new ManagedStorage using the transaction runner,
// storing resource entries in the specified database, and resource data in the
// specified resource storage.
func NewManagedStorage(db *mgo.Database, txnRunner statetxn.Runner, rs ResourceStorage) ManagedStorage {
	ms := &managedStorage{
		resourceStore:   rs,
		resourceCatalog: newResourceCatalog(db.C(resourceCatalogCollection), txnRunner),
		txnRunner:       txnRunner,
	}
	ms.managedResourceCollection = db.C(managedResourceCollection)
	return ms
}

// resourceStoragePath returns the full path used to store a resource with resourcePath
// in the specified environment for the specified user.
func (ms *managedStorage) resourceStoragePath(envUUID, user, resourcePath string) (string, error) {
	// No envUUID or user should contain "/" but we perform a sanity check just in case.
	if strings.Index(envUUID, "/") >= 0 {
		return "", errors.Errorf("environment UUID %q cannot contain %q", envUUID, "/")
	}
	if strings.Index(user, "/") >= 0 {
		return "", errors.Errorf("user %q cannot contain %q", user, "/")
	}
	storagePath := resourcePath
	if user != "" {
		storagePath = path.Join("users", user, storagePath)
	}
	if envUUID != "" {
		storagePath = path.Join("environs", envUUID, storagePath)
	}
	if user == "" && envUUID == "" {
		storagePath = path.Join("global", storagePath)
	}
	return storagePath, nil
}

// preprocessUpload pulls in all the data from the reader, storing it in a temp file and
// calculating the md5 and sha256 checksums.
// The caller is expected to remove the temporary file if and only if we return a nil error.
func (ms *managedStorage) preprocessUpload(r io.Reader, length int64) (
	f *os.File, md5hashHex, sha256hashHex string, err error,
) {
	sha256hash := sha256.New()
	md5hash := md5.New()
	// Set up a chain of readers to pull in the data and calculate the checksums.
	rdr := io.TeeReader(io.TeeReader(r, sha256hash), md5hash)
	f, err = ioutil.TempFile(os.TempDir(), "juju-resource")
	if err != nil {
		return nil, "", "", err
	}
	// Add a cleanup function to remove the data file if we exit with an error.
	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()
	// Write the data to a temp file.
	_, err = io.CopyN(f, rdr, length)
	if err != nil {
		return nil, "", "", err
	}
	// Reset the file so when we return it, it can be read from to get the data.
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, "", "", err
	}
	md5hashHex = fmt.Sprintf("%x", md5hash.Sum(nil))
	sha256hashHex = fmt.Sprintf("%x", sha256hash.Sum(nil))
	return f, md5hashHex, sha256hashHex, nil
}

// GetForEnvironment is defined on the ManagedStorage interface.
func (ms *managedStorage) GetForEnvironment(envUUID, path string) (io.ReadCloser, error) {
	managedPath, err := ms.resourceStoragePath(envUUID, "", path)
	if err != nil {
		return nil, err
	}
	var doc managedResourceDoc
	if err := ms.managedResourceCollection.Find(bson.D{{"path", managedPath}}).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return nil, errors.NotFoundf("resource at path %q", managedPath)
		}
		return nil, errors.Annotatef(err, "cannot load record for resource with path %q", managedPath)
	}
	r, err := ms.resourceCatalog.Get(doc.ResourceId)
	if err == ErrUploadPending {
		return nil, err
	} else if err != nil {
		return nil, errors.Annotatef(err, "cannot load catalog entry for resource with path %q", managedPath)
	}
	return ms.resourceStore.Get(r.Path)
}

// cleanupResourceCatalog is used to delete a resource catalog record if a put operation fails.
func cleanupResourceCatalog(rc ResourceCatalog, id string, err *error) {
	if *err == nil {
		return
	}
	_, _, removeErr := rc.Remove(id)
	if removeErr != nil {
		finalErr := errors.Annotatef(*err, "cannot clean up after failed storage operation because: %v", removeErr)
		*err = finalErr
	}
}

// cleanupResource is usd to delete a resource blob if a put operation fails.
func cleanupResource(rs ResourceStorage, resourcePath string, err *error) {
	if *err == nil {
		return
	}
	removeErr := rs.Remove(resourcePath)
	if removeErr != nil {
		finalErr := errors.Annotatef(*err, "cannot clean up after failed storage operation because: %v", removeErr)
		*err = finalErr
	}
}

// PutForEnvironment is defined on the ManagedStorage interface.
func (ms *managedStorage) PutForEnvironment(envUUID, path string, r io.Reader, length int64) (putError error) {
	dataFile, md5hash, sha256hash, err := ms.preprocessUpload(r, length)
	if err != nil {
		return errors.Annotate(err, "cannot calculate data checksums")
	}
	// Remove the data file when we're done.
	defer os.Remove(dataFile.Name())
	rh := &ResourceHash{
		MD5Hash:    md5hash,
		SHA256Hash: sha256hash,
	}
	resourceId, resourcePath, isNew, err := ms.resourceCatalog.Put(rh)
	if err != nil {
		return errors.Annotate(err, "cannot update resource catalog")
	}

	logger.Debugf("resource catalog entry created with id %q", resourceId)
	// If there's an error saving the resource data, ensure the resource catalog is cleaned up.
	defer cleanupResourceCatalog(ms.resourceCatalog, resourceId, &putError)

	managedPath, err := ms.resourceStoragePath(envUUID, "", path)
	if err != nil {
		return err
	}

	// Newly added resource data needs to be saved to the storage.
	if isNew {
		if _, err := ms.resourceStore.Put(resourcePath, dataFile, length); err != nil {
			return errors.Annotatef(err, "cannot add resource %q to store at storage path %q", managedPath, resourcePath)
		}
		// If there's an error from here on, we need to ensure the saved resource data is cleaned up.
		defer cleanupResource(ms.resourceStore, resourcePath, &putError)
		if err := ms.resourceCatalog.UploadComplete(resourceId); err != nil {
			return errors.Annotatef(err, "cannot mark resource %q as upload complete", managedPath)
		}
	}

	// Resource data is saved, resource catalog entry is created/updated, now write the
	// managed storage entry.
	managedResource := ManagedResource{
		EnvUUID: envUUID,
		Path:    managedPath,
	}
	existingResourceId, err := ms.putManagedResource(managedResource, resourceId)
	if err != nil {
		return err
	}
	logger.Debugf("managed resource entry created with path %q", managedPath)
	// If we are overwriting an existing resource with the same path, the managed resource
	// entry will no longer reference the same resource catalog entry, so we need to remove
	// the reference.
	if existingResourceId != "" {
		if _, _, err = ms.resourceCatalog.Remove(existingResourceId); err != nil {
			return errors.Annotatef(err, "cannot remove old resource catalog entry with id %q", existingResourceId)
		}
	}
	return nil
}

// putManagedResource saves the managed resource record and returns the resource id of any
// existing record with the same path.
func (ms *managedStorage) putManagedResource(managedResource ManagedResource, resourceId string) (
	existingResourceId string, err error,
) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var addManagedResourceOps []txn.Op
		existingResourceId, addManagedResourceOps, err = ms.putResourceTxn(managedResource, resourceId)
		return addManagedResourceOps, err
	}

	if err = ms.txnRunner.Run(buildTxn); err != nil {
		return "", errors.Annotate(err, "cannot update managed resource catalog")
	}
	return existingResourceId, nil
}

// RemoveForEnvironment is defined on the ManagedStorage interface.
func (ms *managedStorage) RemoveForEnvironment(envUUID, path string) (err error) {
	// This operation may leave the db in an inconsistent state if any of the
	// latter steps fail, but not in a way that will impact external users.
	// eg if the managed resource record is removed, but the subsequent call to
	// remove the resource catalog entry fails, the resource at the path will
	// not be visible anymore, but the data will still be stored.

	managedPath, err := ms.resourceStoragePath(envUUID, "", path)
	if err != nil {
		return err
	}

	// First remove the managed resource catalog entry.
	var resourceId string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		var removeManagedResourceOps []txn.Op
		resourceId, removeManagedResourceOps, err = ms.removeResourceTxn(managedPath)
		return removeManagedResourceOps, err
	}
	if err := ms.txnRunner.Run(buildTxn); err != nil {
		if err == mgo.ErrNotFound {
			return errors.NotFoundf("resource at path %q", managedPath)
		}
		return errors.Annotate(err, "cannot update managed resource catalog")
	}

	// Now remove the resource catalog entry.
	wasDeleted, resourcePath, err := ms.resourceCatalog.Remove(resourceId)
	if err != nil {
		return errors.Annotatef(err, "cannot delete resource %q from resource catalog", resourceId)
	}
	// If the there are no more references to the data, delete from the resource store.
	if wasDeleted {
		if err := ms.resourceStore.Remove(resourcePath); err != nil {
			return errors.Annotatef(err, "cannot delete resource %q at storage path %q", managedPath, resourcePath)
		}
	}
	return nil
}

func (ms *managedStorage) putResourceTxn(managedResource ManagedResource, resourceId string) (string, []txn.Op, error) {
	return putResourceTxn(ms.managedResourceCollection, managedResource, resourceId)
}

// putResourceTxn is split out so it can be overridden for testing.
var putResourceTxn = func(coll *mgo.Collection, managedResource ManagedResource, resourceId string) (string, []txn.Op, error) {
	doc := newManagedResourceDoc(managedResource, resourceId)
	var existingDoc managedResourceDoc
	err := coll.FindId(doc.Id).One(&existingDoc)
	if err != nil && err != mgo.ErrNotFound {
		return "", nil, err
	}
	if err == mgo.ErrNotFound {
		return "", []txn.Op{{
			C:      coll.Name,
			Id:     doc.Id,
			Assert: txn.DocMissing,
			Insert: doc,
		}}, nil
	}
	return existingDoc.ResourceId, []txn.Op{{
		C:      coll.Name,
		Id:     doc.Id,
		Assert: txn.DocExists,
		Update: bson.D{{"$set",
			bson.D{{"path", doc.Path}, {"resourceid", resourceId}},
		}},
	}}, nil
}

func (ms *managedStorage) removeResourceTxn(managedPath string) (string, []txn.Op, error) {
	var existingDoc managedResourceDoc
	if err := ms.managedResourceCollection.FindId(managedPath).One(&existingDoc); err != nil {
		return "", nil, err
	}
	return existingDoc.ResourceId, []txn.Op{{
		C:      ms.managedResourceCollection.Name,
		Id:     existingDoc.Id,
		Assert: txn.DocExists,
		Remove: true,
	}}, nil
}
