// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/juju/errors"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/utils"
)

type azureStorage struct {
	mutex            sync.Mutex
	createdContainer bool
	storageContext
}

// storageContext is an abstraction that is there only to accommodate the need
// for using an azureStorage independently from an environ object in tests.
type storageContext interface {
	getContainer() string
	getStorageContext() (*gwacl.StorageContext, error)
}

// environStorageContext is a storageContext which gets its information from
// an azureEnviron object.
type environStorageContext struct {
	environ *azureEnviron
}

var _ storageContext = (*environStorageContext)(nil)

func (context *environStorageContext) getContainer() string {
	return context.environ.getContainerName()
}

func (context *environStorageContext) getStorageContext() (*gwacl.StorageContext, error) {
	return context.environ.getStorageContext()
}

// azureStorage implements Storage.
var _ storage.Storage = (*azureStorage)(nil)

// Get is specified in the StorageReader interface.
func (storage *azureStorage) Get(name string) (io.ReadCloser, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	reader, err := context.GetBlob(storage.getContainer(), name)
	if gwacl.IsNotFoundError(err) {
		return nil, errors.NotFoundf("file %q not found", name)
	}
	return reader, err
}

// List is specified in the StorageReader interface.
func (storage *azureStorage) List(prefix string) ([]string, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	request := &gwacl.ListBlobsRequest{Container: storage.getContainer(), Prefix: prefix, Marker: ""}
	blobList, err := context.ListAllBlobs(request)
	httpErr, isHTTPErr := err.(gwacl.HTTPError)
	if isHTTPErr && httpErr.StatusCode() == http.StatusNotFound {
		// A 404 means the container doesn't exist.  There are no files so
		// just return nothing.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, len(blobList.Blobs))
	for index, blob := range blobList.Blobs {
		names[index] = blob.Name
	}
	return names, nil
}

// URL is specified in the StorageReader interface.
func (storage *azureStorage) URL(name string) (string, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return "", err
	}
	if context.Key != "" {
		// 10 years should be good enough.
		expires := time.Now().AddDate(10, 0, 0)
		return context.GetAnonymousFileURL(storage.getContainer(), name, expires)
	}
	return context.GetFileURL(storage.getContainer(), name), nil
}

// DefaultConsistencyStrategy is specified in the StorageReader interface.
func (storage *azureStorage) DefaultConsistencyStrategy() utils.AttemptStrategy {
	// This storage backend has immediate consistency, so there's no
	// need to wait.  One attempt should do.
	return utils.AttemptStrategy{}
}

// ShouldRetry is specified in the StorageReader interface.
func (storage *azureStorage) ShouldRetry(err error) bool {
	return false
}

// Put is specified in the StorageWriter interface.
func (storage *azureStorage) Put(name string, r io.Reader, length int64) error {
	err := storage.createContainer(storage.getContainer())
	if err != nil {
		return err
	}
	limitedReader := io.LimitReader(r, length)
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.UploadBlockBlob(storage.getContainer(), name, limitedReader)
}

// Remove is specified in the StorageWriter interface.
func (storage *azureStorage) Remove(name string) error {
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.DeleteBlob(storage.getContainer(), name)
}

// RemoveAll is specified in the StorageWriter interface.
func (storage *azureStorage) RemoveAll() error {
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}
	return context.DeleteContainer(storage.getContainer())
}

// createContainer makes a private container in the storage account.
// It can be called when the container already exists and returns with no error
// if it does.  To avoid unnecessary HTTP requests, we do this only once for
// every PUT operation by using a mutex lock and boolean flag.
func (storage *azureStorage) createContainer(name string) error {
	// We must get our storage context before entering our critical
	// section, because this may lock the environment.
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}

	storage.mutex.Lock()
	defer storage.mutex.Unlock()

	if storage.createdContainer {
		return nil
	}
	_, err = context.GetContainerProperties(name)
	if err == nil {
		// No error means it's already there, just return now.
		return nil
	}
	httpErr, isHTTPErr := err.(gwacl.HTTPError)
	if !isHTTPErr || httpErr.StatusCode() != http.StatusNotFound {
		// We were hoping for a 404: Not Found.  That means we can go
		// ahead and create the container.  But we got some other
		// error.
		return err
	}
	err = context.CreateContainer(name)
	if err != nil {
		return err
	}
	storage.createdContainer = true
	return nil
}

// deleteContainer deletes the named comtainer from the storage account.
func (storage *azureStorage) deleteContainer(name string) error {
	context, err := storage.getStorageContext()
	if err != nil {
		return err
	}

	return context.DeleteContainer(name)
}
