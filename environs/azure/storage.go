// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"io"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs"
)

type azureStorage struct {
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

func (context *environStorageContext) getContainer() string {
	return context.environ.getSnapshot().ecfg.StorageContainerName()
}

func (context *environStorageContext) getStorageContext() (*gwacl.StorageContext, error) {
	return context.environ.getStorageContext()
}

func NewStorage(env *azureEnviron) environs.Storage {
	context := &environStorageContext{environ: env}
	return &azureStorage{context}
}

// azureStorage implements Storage.
var _ environs.Storage = (*azureStorage)(nil)

// Get is specified in the StorageReader interface.
func (storage *azureStorage) Get(name string) (io.ReadCloser, error) {
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	return context.GetBlob(storage.getContainer(), name)
}

// List is specified in the StorageReader interface.
func (storage *azureStorage) List(prefix string) ([]string, error) {
	request := &gwacl.ListBlobsRequest{Container: storage.getContainer(), Prefix: prefix, Marker: ""}
	context, err := storage.getStorageContext()
	if err != nil {
		return nil, err
	}
	blobList, err := context.ListAllBlobs(request)
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
	panic("unimplemented")
}

// Put is specified in the StorageWriter interface.
func (storage *azureStorage) Put(name string, r io.Reader, length int64) error {
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
