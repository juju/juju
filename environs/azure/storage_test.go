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
	// TODO: uncomment this.
	// context := storage.storageContext.getStorageContext()
	// c.Check(context.Key, Equals, accountKey)
	// c.Check(context.Account, Equals, accountName)
}

type TestTransport struct {
	Request  *http.Request
	Response *http.Response
	Error    error
}

func (t *TestTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	t.Request = req
	return t.Response, t.Error
}

func makeOKResponse(responseContent string) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d", http.StatusOK),
		StatusCode: http.StatusOK,
		Body:       ioutil.NopCloser(strings.NewReader(responseContent)),
	}
}

// testStorageContext is a test storageContext.
type testStorageContext struct {
	container      string
	storageContext *gwacl.StorageContext
}

func (context *testStorageContext) getContainer() string {
	return context.container
}

func (context *testStorageContext) getStorageContext() *gwacl.StorageContext {
	return context.storageContext
}

func makeAzureStorage(response *http.Response, containerName string) (azureStorage, *TestTransport) {
	transport := &TestTransport{Response: response}
	client := &http.Client{Transport: transport}
	context := &testStorageContext{container: containerName, storageContext: gwacl.NewTestStorageContext(client)}
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
	containerName := "container"
	response := makeOKResponse(blobListResponse)
	azureStorage, transport := makeAzureStorage(response, containerName)
	prefix := "prefix"
	names, err := azureStorage.List(prefix)
	c.Assert(err, IsNil)
	// The prefix has been passed down as a query parameter.
	c.Check(transport.Request.URL.Query()["prefix"], DeepEquals, []string{prefix})
	// The container name is used in the requested URL.
	c.Check(transport.Request.URL.String(), Matches, ".*"+containerName+".*")
	c.Check(names, DeepEquals, []string{"prefix-1", "prefix-2"})
}

func (StorageSuite) TestGet(c *C) {
	blobContent := "test blob"
	containerName := "container"
	filename := "blobname"
	response := makeOKResponse(blobContent)
	azureStorage, transport := makeAzureStorage(response, containerName)
	reader, err := azureStorage.Get(filename)
	c.Assert(err, IsNil)
	c.Assert(reader, NotNil)
	defer reader.Close()

	context := azureStorage.getStorageContext()
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(containerName, filename)+"?.*")
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, blobContent)
}

func (StorageSuite) TestPut(c *C) {
	blobContent := "test blob"
	containerName := "container"
	filename := "blobname"
	response := &http.Response{
		Status:     fmt.Sprintf("%d", http.StatusCreated),
		StatusCode: http.StatusCreated,
	}
	azureStorage, transport := makeAzureStorage(response, containerName)
	err := azureStorage.Put(filename, strings.NewReader(blobContent), 10)
	c.Assert(err, IsNil)

	context := azureStorage.getStorageContext()
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(containerName, filename)+"?.*")
}

func (StorageSuite) TestRemove(c *C) {
	containerName := "container"
	filename := "blobname"
	response := &http.Response{
		Status:     fmt.Sprintf("%d", http.StatusAccepted),
		StatusCode: http.StatusAccepted,
	}
	azureStorage, transport := makeAzureStorage(response, containerName)
	err := azureStorage.Remove(filename)
	c.Assert(err, IsNil)

	context := azureStorage.getStorageContext()
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(containerName, filename)+"?.*")
	c.Check(transport.Request.Method, Equals, "DELETE")
}
