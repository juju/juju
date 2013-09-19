// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/errors"
	jc "launchpad.net/juju-core/testing/checkers"
)

type storageSuite struct {
	providerSuite
}

var _ = gc.Suite(&storageSuite{})

func makeResponse(content string, status int) *http.Response {
	return &http.Response{
		Status:     fmt.Sprintf("%d", status),
		StatusCode: status,
		Body:       ioutil.NopCloser(strings.NewReader(content)),
	}
}

// MockingTransportExchange is a recording of a request and a response over
// HTTP.
type MockingTransportExchange struct {
	Request  *http.Request
	Response *http.Response
	Error    error
}

// MockingTransport is used as an http.Client.Transport for testing.  It
// records the sequence of requests, and returns a predetermined sequence of
// Responses and errors.
type MockingTransport struct {
	Exchanges     []*MockingTransportExchange
	ExchangeCount int
}

// MockingTransport implements the http.RoundTripper interface.
var _ http.RoundTripper = &MockingTransport{}

func (t *MockingTransport) AddExchange(response *http.Response, err error) {
	exchange := MockingTransportExchange{Response: response, Error: err}
	t.Exchanges = append(t.Exchanges, &exchange)
}

func (t *MockingTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	exchange := t.Exchanges[t.ExchangeCount]
	t.ExchangeCount++
	exchange.Request = req
	return exchange.Response, exchange.Error
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

// makeFakeStorage creates a test azureStorage object that will talk to a
// fake HTTP server set up to always return preconfigured http.Response objects.
// The MockingTransport object can be used to check that the expected query has
// been issued to the test server.
func makeFakeStorage(container, account, key string) (*azureStorage, *MockingTransport) {
	transport := &MockingTransport{}
	client := &http.Client{Transport: transport}
	storageContext := gwacl.NewTestStorageContext(client)
	storageContext.Account = account
	storageContext.Key = key
	context := &testStorageContext{container: container, storageContext: storageContext}
	azStorage := &azureStorage{storageContext: context}
	return azStorage, transport
}

// setStorageEndpoint sets a given Azure API endpoint on a given azureStorage.
func setStorageEndpoint(azStorage *azureStorage, endpoint gwacl.APIEndpoint) {
	// Ugly, because of the confusingly similar layers of nesting.
	testContext := azStorage.storageContext.(*testStorageContext)
	var gwaclContext *gwacl.StorageContext = testContext.storageContext
	gwaclContext.AzureEndpoint = endpoint
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

func (*storageSuite) TestList(c *gc.C) {
	container := "container"
	response := makeResponse(blobListResponse, http.StatusOK)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)

	prefix := "prefix"
	names, err := storage.List(azStorage, prefix)
	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	// The prefix has been passed down as a query parameter.
	c.Check(transport.Exchanges[0].Request.URL.Query()["prefix"], gc.DeepEquals, []string{prefix})
	// The container name is used in the requested URL.
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, ".*"+container+".*")
	c.Check(names, gc.DeepEquals, []string{"prefix-1", "prefix-2"})
}

func (*storageSuite) TestListWithNonexistentContainerReturnsNoFiles(c *gc.C) {
	// If Azure returns a 404 it means the container doesn't exist. In this
	// case the provider should interpret this as "no files" and return nil.
	container := "container"
	response := makeResponse("", http.StatusNotFound)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)

	names, err := storage.List(azStorage, "prefix")
	c.Assert(err, gc.IsNil)
	c.Assert(names, gc.IsNil)
}

func (*storageSuite) TestGet(c *gc.C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	response := makeResponse(blobContent, http.StatusOK)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)

	reader, err := storage.Get(azStorage, filename)
	c.Assert(err, gc.IsNil)
	c.Assert(reader, gc.NotNil)
	defer reader.Close()

	context, err := azStorage.getStorageContext()
	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, context.GetFileURL(container, filename)+"?.*")
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, blobContent)
}

func (*storageSuite) TestGetReturnsNotFoundIf404(c *gc.C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("not found", http.StatusNotFound)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)
	_, err := storage.Get(azStorage, filename)
	c.Assert(err, gc.NotNil)
	c.Check(err, jc.Satisfies, errors.IsNotFoundError)
}

func (*storageSuite) TestPut(c *gc.C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	azStorage, transport := makeFakeStorage(container, "account", "")
	// The create container call makes two exchanges.
	transport.AddExchange(makeResponse("", http.StatusNotFound), nil)
	transport.AddExchange(makeResponse("", http.StatusCreated), nil)
	putResponse := makeResponse("", http.StatusCreated)
	transport.AddExchange(putResponse, nil)
	transport.AddExchange(putResponse, nil)
	err := azStorage.Put(filename, strings.NewReader(blobContent), int64(len(blobContent)))
	c.Assert(err, gc.IsNil)

	context, err := azStorage.getStorageContext()
	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 4)
	c.Check(transport.Exchanges[2].Request.URL.String(), gc.Matches, context.GetFileURL(container, filename)+"?.*")
}

func (*storageSuite) TestRemove(c *gc.C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusAccepted)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)
	err := azStorage.Remove(filename)
	c.Assert(err, gc.IsNil)

	context, err := azStorage.getStorageContext()
	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, context.GetFileURL(container, filename)+"?.*")
	c.Check(transport.Exchanges[0].Request.Method, gc.Equals, "DELETE")
}

func (*storageSuite) TestRemoveErrors(c *gc.C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusForbidden)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)
	err := azStorage.Remove(filename)
	c.Assert(err, gc.NotNil)
}

func (*storageSuite) TestRemoveAll(c *gc.C) {
	// When we ask gwacl to remove all blobs, it calls DeleteContainer.
	response := makeResponse("", http.StatusAccepted)
	storage, transport := makeFakeStorage("cntnr", "account", "")
	transport.AddExchange(response, nil)

	err := storage.RemoveAll()
	c.Assert(err, gc.IsNil)

	_, err = storage.getStorageContext()
	c.Assert(err, gc.IsNil)
	// Without going too far into gwacl's innards, this is roughly what
	// it needs to do in order to delete a container.
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Exchanges[0].Request.Method, gc.Equals, "DELETE")
}

func (*storageSuite) TestRemoveNonExistentBlobSucceeds(c *gc.C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusNotFound)
	azStorage, transport := makeFakeStorage(container, "account", "")
	transport.AddExchange(response, nil)
	err := azStorage.Remove(filename)
	c.Assert(err, gc.IsNil)
}

func (*storageSuite) TestURL(c *gc.C) {
	container := "container"
	filename := "blobname"
	account := "account"
	key := "bWFkZXlvdWxvb2sK"
	azStorage, _ := makeFakeStorage(container, account, key)
	// Use a realistic service endpoint for this test, so that we can see
	// that we're really getting the expected kind of URL.
	setStorageEndpoint(azStorage, gwacl.GetEndpoint("West US"))
	URL, err := azStorage.URL(filename)
	c.Assert(err, gc.IsNil)
	parsedURL, err := url.Parse(URL)
	c.Assert(err, gc.IsNil)
	c.Check(parsedURL.Host, gc.Matches, fmt.Sprintf("%s.blob.core.windows.net", account))
	c.Check(parsedURL.Path, gc.Matches, fmt.Sprintf("/%s/%s", container, filename))
	values, err := url.ParseQuery(parsedURL.RawQuery)
	c.Assert(err, gc.IsNil)
	signature := values.Get("sig")
	// The query string contains a non-empty signature.
	c.Check(signature, gc.Not(gc.HasLen), 0)
	// The signature is base64-encoded.
	_, err = base64.StdEncoding.DecodeString(signature)
	c.Assert(err, gc.IsNil)
	// If Key is empty, query string does not contain a signature.
	key = ""
	azStorage, _ = makeFakeStorage(container, account, key)
	URL, err = azStorage.URL(filename)
	c.Assert(err, gc.IsNil)
	parsedURL, err = url.Parse(URL)
	c.Assert(err, gc.IsNil)
	values, err = url.ParseQuery(parsedURL.RawQuery)
	c.Assert(err, gc.IsNil)
	c.Check(values.Get("sig"), gc.HasLen, 0)
}

func (*storageSuite) TestCreateContainerCreatesContainerIfDoesNotExist(c *gc.C) {
	azStorage, transport := makeFakeStorage("", "account", "")
	transport.AddExchange(makeResponse("", http.StatusNotFound), nil)
	transport.AddExchange(makeResponse("", http.StatusCreated), nil)

	err := azStorage.createContainer("cntnr")

	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 2)
	// Without going too far into gwacl's innards, this is roughly what
	// it needs to do in order to call GetContainerProperties.
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Exchanges[0].Request.Method, gc.Equals, "GET")

	// ... and for CreateContainer.
	c.Check(transport.Exchanges[1].Request.URL.String(), gc.Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Exchanges[1].Request.Method, gc.Equals, "PUT")
}

func (*storageSuite) TestCreateContainerIsDoneIfContainerAlreadyExists(c *gc.C) {
	container := ""
	azStorage, transport := makeFakeStorage(container, "account", "")
	header := make(http.Header)
	header.Add("Last-Modified", "last-modified")
	header.Add("ETag", "etag")
	header.Add("X-Ms-Lease-Status", "status")
	header.Add("X-Ms-Lease-State", "state")
	header.Add("X-Ms-Lease-Duration", "duration")
	response := makeResponse("", http.StatusOK)
	response.Header = header
	transport.AddExchange(response, nil)

	err := azStorage.createContainer("cntnr")

	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	// Without going too far into gwacl's innards, this is roughly what
	// it needs to do in order to call GetContainerProperties.
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Exchanges[0].Request.Method, gc.Equals, "GET")
}

func (*storageSuite) TestCreateContainerFailsIfContainerInaccessible(c *gc.C) {
	azStorage, transport := makeFakeStorage("", "account", "")
	transport.AddExchange(makeResponse("", http.StatusInternalServerError), nil)

	err := azStorage.createContainer("cntnr")
	c.Assert(err, gc.NotNil)

	// createContainer got an error when trying to query for an existing
	// container of the right name.  But it does not mistake that error for
	// "this container does not exist yet so go ahead and create it."
	// The proper response to the situation is to report the failure.
	c.Assert(err, gc.ErrorMatches, ".*Internal Server Error.*")
}

func (*storageSuite) TestDeleteContainer(c *gc.C) {
	azStorage, transport := makeFakeStorage("", "account", "")
	transport.AddExchange(makeResponse("", http.StatusAccepted), nil)

	err := azStorage.deleteContainer("cntnr")

	c.Assert(err, gc.IsNil)
	c.Assert(transport.ExchangeCount, gc.Equals, 1)
	// Without going too far into gwacl's innards, this is roughly what
	// it needs to do in order to call GetContainerProperties.
	c.Check(transport.Exchanges[0].Request.URL.String(), gc.Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Exchanges[0].Request.Method, gc.Equals, "DELETE")
}
