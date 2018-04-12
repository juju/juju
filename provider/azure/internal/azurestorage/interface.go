// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azurestorage

import (
	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/errors"
)

// Client is an interface providing access to Azure storage services.
type Client interface {
	// GetBlobService returns a BlobStorageClient which can operate
	// on the blob service of the storage account.
	GetBlobService() BlobStorageClient
}

// BlobStorageClient is an interface providing access to Azure blob storage.
//
// This interface the subet of functionality provided by
// https://godoc.org/github.com/Azure/azure-sdk-for-go/storage#BlobStorageClient
// that is required by Juju.
type BlobStorageClient interface {
	// GetContainerReference returns a Container object for the specified container name.
	GetContainerReference(name string) Container
}

// Container provides access to an Azure storage container.
type Container interface {
	// Blobs returns the blobs in the container.
	//
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/fileservices/List-Blobs
	Blobs() ([]Blob, error)

	// Blob returns a Blob object for the specified blob name.
	Blob(name string) Blob
}

// Blob provides access to an Azure storage blob.
type Blob interface {
	// Name returns the name of the blob.
	Name() string

	// Properties returns the properties of the blob.
	Properties() storage.BlobProperties

	// DeleteIfExists deletes the given blob from the specified container If the
	// blob is deleted with this call, returns true. Otherwise returns false.
	//
	// See https://docs.microsoft.com/en-us/rest/api/storageservices/fileservices/Delete-Blob
	DeleteIfExists(*storage.DeleteBlobOptions) (bool, error)
}

// NewClientFunc is the type of the NewClient function.
type NewClientFunc func(
	accountName, accountKey, blobServiceBaseURL, apiVersion string,
	useHTTPS bool,
) (Client, error)

// NewClient returns a Client that is backed by a storage.Client created with
// storage.NewClient
func NewClient(accountName, accountKey, blobServiceBaseURL, apiVersion string, useHTTPS bool) (Client, error) {
	client, err := storage.NewClient(accountName, accountKey, blobServiceBaseURL, apiVersion, useHTTPS)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return clientWrapper{client}, nil
}

type clientWrapper struct {
	storage.Client
}

// GetBlobService is part of the Client interface.
func (w clientWrapper) GetBlobService() BlobStorageClient {
	return &blobStorageClient{w.Client.GetBlobService()}
}

type blobStorageClient struct {
	storage.BlobStorageClient
}

// GetContainerReference is part of the BlobStorageClient interface.
func (c *blobStorageClient) GetContainerReference(name string) Container {
	return container{c.BlobStorageClient.GetContainerReference(name)}
}

type container struct {
	*storage.Container
}

// Blobs is part of the Container interface.
func (c container) Blobs() ([]Blob, error) {
	//TODO(axw) handle pagination.
	resp, err := c.Container.ListBlobs(storage.ListBlobsParameters{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	blobs := make([]Blob, len(resp.Blobs))
	for i := range blobs {
		blobs[i] = blob{&resp.Blobs[i]}
	}
	return blobs, nil
}

// Blob is part of the Container interface.
func (c container) Blob(name string) Blob {
	return blob{c.Container.GetBlobReference(name)}
}

type blob struct {
	*storage.Blob
}

// Name is part of the Blob interface.
func (b blob) Name() string {
	return b.Blob.Name
}

// Properties is part of the Blob interface.
func (b blob) Properties() storage.BlobProperties {
	return b.Blob.Properties
}
