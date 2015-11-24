// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/testing"

	"github.com/juju/juju/provider/azure/internal/azurestorage"
)

type MockStorageClient struct {
	testing.Stub

	ListBlobsFunc          func(container string, _ storage.ListBlobsParameters) (storage.BlobListResponse, error)
	DeleteBlobIfExistsFunc func(container, name string) (bool, error)
}

// NewClient exists to satisfy users who want a NewClientFunc.
func (c *MockStorageClient) NewClient(
	accountName, accountKey, blobServiceBaseURL, apiVersion string,
	useHTTPS bool,
) (azurestorage.Client, error) {
	c.AddCall("NewClient", accountName, accountKey, blobServiceBaseURL, apiVersion, useHTTPS)
	return c, c.NextErr()
}

func (c *MockStorageClient) GetBlobService() azurestorage.BlobStorageClient {
	return c
}

func (c *MockStorageClient) ListBlobs(
	container string,
	params storage.ListBlobsParameters,
) (storage.BlobListResponse, error) {
	c.MethodCall(c, "ListBlobs", container, params)
	if c.ListBlobsFunc != nil {
		return c.ListBlobsFunc(container, params)
	}
	return storage.BlobListResponse{}, c.NextErr()
}

func (c *MockStorageClient) DeleteBlobIfExists(container, name string) (bool, error) {
	c.MethodCall(c, "DeleteBlobIfExists", container, name)
	if c.DeleteBlobIfExistsFunc != nil {
		return c.DeleteBlobIfExistsFunc(container, name)
	}
	return false, c.NextErr()
}
