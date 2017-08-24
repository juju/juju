// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/provider/azure/internal/azurestorage"
)

type MockStorageClient struct {
	testing.Stub
	Containers map[string]azurestorage.Container
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

func (c *MockStorageClient) GetContainerReference(name string) azurestorage.Container {
	c.MethodCall(c, "GetContainerReference", name)
	container := c.Containers[name]
	if container == nil {
		container = notFoundContainer{name}
	}
	return container
}

type MockStorageContainer struct {
	testing.Stub
	Blobs_ []azurestorage.Blob
}

func (c *MockStorageContainer) Blobs() ([]azurestorage.Blob, error) {
	c.MethodCall(c, "Blobs")
	return c.Blobs_, c.NextErr()
}

func (c *MockStorageContainer) Blob(name string) azurestorage.Blob {
	c.MethodCall(c, "Blob", name)
	for _, blob := range c.Blobs_ {
		if blob.Name() == name {
			return blob
		}
	}
	return notFoundBlob{name: name}
}

type MockStorageBlob struct {
	testing.Stub
	Name_       string
	Properties_ storage.BlobProperties
}

func (c *MockStorageBlob) Name() string {
	return c.Name_
}

func (c *MockStorageBlob) Properties() storage.BlobProperties {
	return c.Properties_
}

func (c *MockStorageBlob) DeleteIfExists(opts *storage.DeleteBlobOptions) (bool, error) {
	c.MethodCall(c, "DeleteIfExists", opts)
	return true, c.NextErr()
}

type notFoundContainer struct {
	name string
}

func (c notFoundContainer) Blobs() ([]azurestorage.Blob, error) {
	return nil, errors.NotFoundf("container %q", c.name)
}

func (c notFoundContainer) Blob(name string) azurestorage.Blob {
	return notFoundBlob{
		name:      name,
		deleteErr: errors.NotFoundf("container %q", c.name),
	}
}

type notFoundBlob struct {
	name      string
	deleteErr error
}

func (b notFoundBlob) Name() string {
	return b.name
}

func (notFoundBlob) Properties() storage.BlobProperties {
	return storage.BlobProperties{}
}

func (b notFoundBlob) DeleteIfExists(opts *storage.DeleteBlobOptions) (bool, error) {
	// TODO(axw) should this return an error if the container doesn't exist?
	return false, b.deleteErr
}
