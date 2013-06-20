// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/environs/config"
	"net/http"
	"strings"
)

type StorageSuite struct {
	ProviderSuite
}

var _ = Suite(new(StorageSuite))

func (StorageSuite) TestNewStorage(c *C) {
	attrs := makeAzureConfigMap(c)
	container := "test container name"
	accountName := "test account name"
	accountKey := "test account key"
	attrs["storage-container-name"] = container
	attrs["storage-account-name"] = accountName
	attrs["storage-account-key"] = accountKey
	provider := azureEnvironProvider{}
	config, err := config.New(attrs)
	c.Assert(err, IsNil)
	azureConfig, err := provider.newConfig(config)
	c.Assert(err, IsNil)
	environ := &azureEnviron{name: "azure", ecfg: azureConfig}
	storage := NewStorage(environ).(*azureStorage)

	c.Check(storage.storageContext.getContainer(), Equals, container)
	context, err := storage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(context.Key, Equals, accountKey)
	c.Check(context.Account, Equals, accountName)
}

// TestTransport is used as an http.Client.Transport for testing.  It records
// the latest request, and returns a predetermined Response and error.
type TestTransport struct {
	Request  *http.Request
	Response *http.Response
	Error    error
}

func (t *TestTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	t.Request = req
	return t.Response, t.Error
}

func makeResponse(content string, status int) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d", status),
		StatusCode: status,
		Body:       ioutil.NopCloser(strings.NewReader(content)),
	}
}

// testStorageContext is a struct implementing the storageContext interface
// used in test.  It will return, via getContainer() and getStorageContext()
// the objects used at creation time.
type testStorageContext struct {
	container      string
	storageContext *gwacl.StorageContext
}

func (context *testStorageContext) getContainer() string {
	return context.container
}

func (context *testStorageContext) getStorageContext() (*gwacl.StorageContext, error) {
	return context.storageContext, nil
}

// makeAzureStorage creates a test azureStorage object that will talk to a
// fake http server set up to always return the given http.Response object.
// makeAzureStorage returns a azureStorage object and a TestTransport object.
// The TestTransport object can be used to check that the expected query has
// been issue to the test server.
func makeAzureStorage(response *http.Response, container string) (azureStorage, *TestTransport) {
	transport := &TestTransport{Response: response}
	client := &http.Client{Transport: transport}
	context := &testStorageContext{container: container, storageContext: gwacl.NewTestStorageContext(client)}
	azureStorage := azureStorage{context}
	return azureStorage, transport
}

var blobListResponse = `
  <?xml version="1.0" encoding="utf-8"?>
  <EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
    <Prefix>prefix</Prefix>
    <Marker>marker</Marker>
    <MaxResults>maxresults</MaxResults>
    <Delimiter>delimiter</Delimiter>
    <Blobs>
      <Blob>
        <Name>prefix-1</Name>
        <Url>blob-url1</Url>
      </Blob>
      <Blob>
        <Name>prefix-2</Name>
        <Url>blob-url2</Url>
      </Blob>
    </Blobs>
    <NextMarker />
  </EnumerationResults>`

func (StorageSuite) TestList(c *C) {
	container := "container"
	response := makeResponse(blobListResponse, http.StatusOK)
	azureStorage, transport := makeAzureStorage(response, container)
	prefix := "prefix"
	names, err := azureStorage.List(prefix)
	c.Assert(err, IsNil)
	// The prefix has been passed down as a query parameter.
	c.Check(transport.Request.URL.Query()["prefix"], DeepEquals, []string{prefix})
	// The container name is used in the requested URL.
	c.Check(transport.Request.URL.String(), Matches, ".*"+container+".*")
	c.Check(names, DeepEquals, []string{"prefix-1", "prefix-2"})
}

func (StorageSuite) TestGet(c *C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	response := makeResponse(blobContent, http.StatusOK)
	azureStorage, transport := makeAzureStorage(response, container)
	reader, err := azureStorage.Get(filename)
	c.Assert(err, IsNil)
	c.Assert(reader, NotNil)
	defer reader.Close()

	context, err := azureStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, blobContent)
}

func (StorageSuite) TestPut(c *C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	response := &http.Response{
		Status:     fmt.Sprintf("%d", http.StatusCreated),
		StatusCode: http.StatusCreated,
	}
	azureStorage, transport := makeAzureStorage(response, container)
	err := azureStorage.Put(filename, strings.NewReader(blobContent), 10)
	c.Assert(err, IsNil)

	context, err := azureStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
}

func (StorageSuite) TestRemove(c *C) {
	container := "container"
	filename := "blobname"
	response := &http.Response{
		Status:     fmt.Sprintf("%d", http.StatusAccepted),
		StatusCode: http.StatusAccepted,
	}
	azureStorage, transport := makeAzureStorage(response, container)
	err := azureStorage.Remove(filename)
	c.Assert(err, IsNil)

	context, err := azureStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
	c.Check(transport.Request.Method, Equals, "DELETE")
}
