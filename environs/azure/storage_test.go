// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/errors"
)

type StorageSuite struct {
	ProviderSuite
}

var _ = Suite(new(StorageSuite))

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
// makeAzureStorage returns an azureStorage object and a TestTransport object.
// The TestTransport object can be used to check that the expected query has
// been issued to the test server.
func makeAzureStorage(response *http.Response, container string, account string) (azureStorage, *TestTransport) {
	transport := &TestTransport{Response: response}
	client := &http.Client{Transport: transport}
	storageContext := gwacl.NewTestStorageContext(client)
	storageContext.Account = account
	context := &testStorageContext{container: container, storageContext: storageContext}
	azStorage := azureStorage{context}
	return azStorage, transport
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

func (*StorageSuite) TestList(c *C) {
	container := "container"
	response := makeResponse(blobListResponse, http.StatusOK)
	azStorage, transport := makeAzureStorage(response, container, "account")
	prefix := "prefix"
	names, err := azStorage.List(prefix)
	c.Assert(err, IsNil)
	// The prefix has been passed down as a query parameter.
	c.Check(transport.Request.URL.Query()["prefix"], DeepEquals, []string{prefix})
	// The container name is used in the requested URL.
	c.Check(transport.Request.URL.String(), Matches, ".*"+container+".*")
	c.Check(names, DeepEquals, []string{"prefix-1", "prefix-2"})
}

func (*StorageSuite) TestGet(c *C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	response := makeResponse(blobContent, http.StatusOK)
	azStorage, transport := makeAzureStorage(response, container, "account")
	reader, err := azStorage.Get(filename)
	c.Assert(err, IsNil)
	c.Assert(reader, NotNil)
	defer reader.Close()

	context, err := azStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
	data, err := ioutil.ReadAll(reader)
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, blobContent)
}

func (*StorageSuite) TestGetReturnsNotFoundIf404(c *C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("not found", http.StatusNotFound)
	azStorage, _ := makeAzureStorage(response, container, "account")
	_, err := azStorage.Get(filename)
	c.Assert(err, NotNil)
	c.Check(errors.IsNotFoundError(err), Equals, true)
}

func (*StorageSuite) TestPut(c *C) {
	blobContent := "test blob"
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusCreated)
	azStorage, transport := makeAzureStorage(response, container, "account")
	err := azStorage.Put(filename, strings.NewReader(blobContent), int64(len(blobContent)))
	c.Assert(err, IsNil)

	context, err := azStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
}

func (*StorageSuite) TestRemove(c *C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusAccepted)
	azStorage, transport := makeAzureStorage(response, container, "account")
	err := azStorage.Remove(filename)
	c.Assert(err, IsNil)

	context, err := azStorage.getStorageContext()
	c.Assert(err, IsNil)
	c.Check(transport.Request.URL.String(), Matches, context.GetFileURL(container, filename)+"?.*")
	c.Check(transport.Request.Method, Equals, "DELETE")
}

func (*StorageSuite) TestRemoveErrors(c *C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusForbidden)
	azStorage, _ := makeAzureStorage(response, container, "account")
	err := azStorage.Remove(filename)
	c.Assert(err, NotNil)
}

var emptyBlobList = `
	<?xml version="1.0" encoding="utf-8"?>
	<EnumerationResults ContainerName="http://myaccount.blob.core.windows.net/mycontainer">
	</EnumerationResults>
	`

func (*StorageSuite) TestRemoveAll(c *C) {
	// When we ask gwacl to remove all blobs, first thing it does is
	// list them.  If the list is empty, we're done.
	// Testing for the case where there are files is harder, but not
	// needed: the difference is internal to gwacl, and tested there.
	response := makeResponse(emptyBlobList, http.StatusOK)
	storage, transport := makeAzureStorage(response, "cntnr", "account")

	err := storage.RemoveAll()
	c.Assert(err, IsNil)

	_, err = storage.getStorageContext()
	c.Assert(err, IsNil)
	// Without going too far into gwacl's innards, this is roughly what
	// it needs to do in order to list the files.
	c.Check(transport.Request.URL.String(), Matches, "http.*/cntnr?.*restype=container.*")
	c.Check(transport.Request.Method, Equals, "GET")
}

func (*StorageSuite) TestRemoveNonExistantBlobSucceeds(c *C) {
	container := "container"
	filename := "blobname"
	response := makeResponse("", http.StatusNotFound)
	azStorage, _ := makeAzureStorage(response, container, "account")
	err := azStorage.Remove(filename)
	c.Assert(err, IsNil)
}

func (*StorageSuite) TestURL(c *C) {
	container := "container"
	filename := "blobname"
	account := "account"
	azStorage, _ := makeAzureStorage(nil, container, account)
	URL, err := azStorage.URL(filename)
	c.Assert(err, IsNil)
	parsedURL, err := url.Parse(URL)
	c.Assert(err, IsNil)
	c.Check(parsedURL.Host, Matches, fmt.Sprintf("%s.blob.core.windows.net", account))
	c.Check(parsedURL.Path, Matches, fmt.Sprintf("/%s/%s", container, filename))
	values, err := url.ParseQuery(parsedURL.RawQuery)
	c.Assert(err, IsNil)
	// The query string contains a non-empty signature.
	c.Check(values.Get("sig"), Not(HasLen), 0)
}
