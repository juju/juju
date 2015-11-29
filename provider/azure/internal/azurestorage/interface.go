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
	// ListBlobs returns an object that contains list of blobs in the
	// container, pagination token and other information in the response
	// of List Blobs call.
	//
	// See https://godoc.org/github.com/Azure/azure-sdk-for-go/storage#BlobStorageClient.ListBlobs
	ListBlobs(container string, params storage.ListBlobsParameters) (storage.BlobListResponse, error)

	// DeleteBlobIfExists deletes the given blob from the specified
	// container If the blob is deleted with this call, returns true.
	// Otherwise returns false.
	//
	// See https://godoc.org/github.com/Azure/azure-sdk-for-go/storage#BlobStorageClient.DeleteBlobIfExists
	DeleteBlobIfExists(container, name string) (bool, error)
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

func (w clientWrapper) GetBlobService() BlobStorageClient {
	return w.Client.GetBlobService()
}
