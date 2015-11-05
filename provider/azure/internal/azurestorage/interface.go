// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azurestorage

import (
	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/errors"
)

// Client is an interface providing access to Azure storage services.
type Client interface {
	GetBlobService() BlobStorageClient
}

// BlobStorageClient is an interface providing access to Azure blob storage.
type BlobStorageClient interface {
	ListBlobs(container string, params storage.ListBlobsParameters) (storage.BlobListResponse, error)
	//GetBlobProperties(container, name string) (*storage.BlobProperties, error)
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
