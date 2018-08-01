// goose/swift - Go package to interact with OpenStack Object-Storage (Swift) API.
// See http://docs.openstack.org/api/openstack-object-storage/1.0/content/.

package swift

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"gopkg.in/goose.v2/client"
	"gopkg.in/goose.v2/errors"
	goosehttp "gopkg.in/goose.v2/http"
	"gopkg.in/goose.v2/internal/httpfile"
)

// Client provides a means to access the OpenStack Object Storage Service.
type Client struct {
	client client.Client
}

// New creates a new Client.
func New(client client.Client) *Client {
	return &Client{client}
}

type ACL string

const (
	Private    = ACL("")
	PublicRead = ACL(".r:*,.rlistings")
)

// CreateContainer creates a container with the given name.
func (c *Client) CreateContainer(containerName string, acl ACL) error {
	// [sodre]: Due to a possible bug in ceph-radosgw, we need to split the
	// creation of the bucket and the changing its ACL.
	requestData := goosehttp.RequestData{
		ExpectedStatus: []int{http.StatusAccepted, http.StatusCreated},
	}
	err := c.client.SendRequest(client.PUT, "object-store", "", containerName, &requestData)
	if err != nil {
		err = maybeNotFound(err, "failed to create container: %s", containerName)
		return err
	}
	// Normally accessing a container or objects within requires a token
	// for the tenant. Setting an ACL using the X-Container-Read header
	// can be used to allow unauthenticated HTTP access.
	headers := make(http.Header)
	headers.Add("X-Container-Read", string(acl))
	requestData = goosehttp.RequestData{
		ReqHeaders:     headers,
		ExpectedStatus: []int{http.StatusAccepted, http.StatusNoContent},
	}
	err = c.client.SendRequest(client.POST, "object-store", "", containerName, &requestData)
	if err != nil {
		err = maybeNotFound(err, "failed to update container read acl: %s", containerName)
	}
	return err
}

// DeleteContainer deletes the specified container.
func (c *Client) DeleteContainer(containerName string) error {
	requestData := goosehttp.RequestData{
		ExpectedStatus: []int{http.StatusNoContent},
	}
	err := c.client.SendRequest(client.DELETE, "object-store", "", containerName, &requestData)
	if err != nil {
		err = maybeNotFound(err, "failed to delete container: %s", containerName)
	}
	return err
}

// HeadObject retrieves object metadata and other standard HTTP headers.
func (c *Client) HeadObject(containerName, objectName string) (http.Header, error) {
	req := &objectRequest{
		method:    client.HEAD,
		container: containerName,
		object:    objectName,
	}
	err := c.sendRequest(req)
	return req.RespHeaders, err
}

// GetObject retrieves the specified object's data.
func (c *Client) GetObject(containerName, objectName string) (obj []byte, err error) {
	rc, _, err := c.GetReader(containerName, objectName)
	if err != nil {
		return
	}
	defer rc.Close()
	return ioutil.ReadAll(rc)
}

// DeleteObject removes an object from the storage system permanently.
func (c *Client) DeleteObject(containerName, objectName string) error {
	return c.sendRequest(&objectRequest{
		method:    client.DELETE,
		container: containerName,
		object:    objectName,
		RequestData: goosehttp.RequestData{
			ExpectedStatus: []int{http.StatusNoContent},
		},
	})
}

// PutObject writes, or overwrites, an object's content and metadata.
func (c *Client) PutObject(containerName, objectName string, data []byte) error {
	r := bytes.NewReader(data)
	return c.PutReader(containerName, objectName, r, int64(len(data)))
}

// PutReader writes, or overwrites, an object's content and metadata.
// The object's content will be read from r, and should have
// the given length. If r does not implement io.Seeker, the entire
// content will be read into memory before sending the request (so
// that the request can be retried if needed) otherwise
// Seek will be used to rewind the request on retry.
func (c *Client) PutReader(containerName, objectName string, r io.Reader, length int64) error {
	return c.sendRequest(&objectRequest{
		method:    client.PUT,
		container: containerName,
		object:    objectName,
		RequestData: goosehttp.RequestData{
			ReqReader:      r,
			ReqLength:      int(length),
			ExpectedStatus: []int{http.StatusCreated},
		},
	})
}

// ContainerContents describes a single container and its contents.
type ContainerContents struct {
	Name         string `json:"name"`
	Hash         string `json:"hash"`
	LengthBytes  int    `json:"bytes"`
	ContentType  string `json:"content_type"`
	LastModified string `json:"last_modified"`
}

// List lists the objects in a bucket.
// TODO describe prefix, delim, marker, limit parameters.
func (c *Client) List(containerName, prefix, delim, marker string, limit int) (contents []ContainerContents, err error) {
	params := make(url.Values)
	params.Add("prefix", prefix)
	params.Add("delimiter", delim)
	params.Add("marker", marker)
	params.Add("format", "json")
	if limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", limit))
	}
	requestData := goosehttp.RequestData{
		Params:    &params,
		RespValue: &contents,
	}
	err = c.client.SendRequest(client.GET, "object-store", "", containerName, &requestData)
	if err != nil {
		err = errors.Newf(err, "failed to list contents of container: %s", containerName)
	}
	return
}

// URL returns a non-signed URL that allows retrieving the object at path.
// It only works if the object is publicly readable (see SignedURL).
func (c *Client) URL(containerName, file string) (string, error) {
	return c.client.MakeServiceURL("object-store", "", []string{containerName, file})
}

// SignedURL returns a signed URL that allows anyone holding the URL
// to retrieve the object at path. The signature is valid until expires.
func (c *Client) SignedURL(containerName, file string, expires time.Time) (string, error) {
	// expiresUnix := expires.Unix()
	// TODO(wallyworld) - 2013-02-11 bug=1121677
	// retrieve the signed URL, for now just return the public URL
	rawURL, err := c.URL(containerName, file)
	if err != nil {
		return "", err
	}
	return rawURL, nil
}

func (c *Client) sendRequest(req *objectRequest) error {
	err := c.client.SendRequest(req.method, "object-store", "", req.container+"/"+req.object, &req.RequestData)
	if err != nil {
		return maybeNotFound(err, "failed to %s object %s from container %s", req.method, req.object, req.container)
	}
	return nil
}

// OpenObject opens an object for reading. The readAhead parameter governs
// how much data is scheduled to be read speculatively from the object before
// actual Read requests are issued. If readAhead is -1, an indefinite amount
// of data will be requested; if readAhead is 0, no data will be requested.
func (c *Client) OpenObject(containerName, objectName string, readAhead int64) (Object, http.Header, error) {
	client := &objectClient{
		client:    c,
		container: containerName,
		object:    objectName,
	}
	f, h, err := httpfile.Open(client, readAhead)
	if err != nil {
		if err == httpfile.ErrNotFound {
			err = errors.NewNotFoundf(nil, "", "object %q in container %q not found", objectName, containerName)
		}
		// TODO it seems weird to return the headers when we're
		// returning an error. The original use case (explained in
		// commit 6a06aeb5776abe26d86c38210732009de9f74335)
		// doesn't seem like it would require the headers in this case.
		// If we didn't need to return the headers on error, we could
		// just make them available on the Object interface, thus
		// simplifying this API for the common case.
		return nil, h, err
	}
	return f, h, nil
}

// GetReader returns a reader from which the object's data can be read,
// and the HTTP header of the initial response.
func (c *Client) GetReader(containerName, objectName string) (_ io.ReadCloser, _ http.Header, err error) {
	// We only want a reader, so we don't need all the httpfile logic.
	client := &objectClient{
		client:    c,
		container: containerName,
		object:    objectName,
	}
	resp, err := client.Do(&httpfile.Request{
		Method: "GET",
	})
	defer func() {
		if err != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if err != nil {
		return nil, resp.Header, err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, resp.Header, errors.NewNotFoundf(nil, "", "object %q in container %q not found", objectName, containerName)
		}
		return nil, resp.Header, fmt.Errorf("unexpected response status %v", resp.StatusCode)
	}
	return resp.Body, resp.Header, nil
}

// The following defines a ReadCloser implementation which reads no data.
// It is used instead of returning a nil pointer, which is the same as http.Request.Body.
var emptyReadCloser noData

type noData struct {
	io.ReadCloser
}

// Object is the interface provided by an Swift object.
type Object interface {
	// Size returns the size of the object.
	Size() int64

	io.ReadSeeker
	io.Closer
}

func maybeNotFound(err error, format string, arg ...interface{}) error {
	if !errors.IsNotFound(err) {
		if error, ok := err.(*goosehttp.HttpError); ok {
			// The OpenStack API says that attempts to operate on non existent containers or objects return a status code
			// of 412 (StatusPreconditionFailed).
			if error.StatusCode == http.StatusPreconditionFailed {
				err = errors.NewNotFoundf(err, "", format, arg...)
			}
		}
	}
	return errors.Newf(err, format, arg...)
}

type objectClient struct {
	client    *Client
	container string
	object    string
}

type objectRequest struct {
	container string
	object    string
	method    string
	goosehttp.RequestData
}

func (c *objectClient) Do(req *httpfile.Request) (*httpfile.Response, error) {
	gooseReq := &objectRequest{
		object:    c.object,
		container: c.container,
		method:    req.Method,
		RequestData: goosehttp.RequestData{
			// Signal to the client that we want the reader back
			// by placing a dummy reader there.
			// (RespReader gets replaced).
			RespReader:     &emptyReadCloser,
			ReqHeaders:     req.Header,
			ExpectedStatus: httpfile.SupportedResponseStatuses,
		},
	}
	err := c.client.sendRequest(gooseReq)
	if gooseReq.RespReader == &emptyReadCloser {
		// sendRequest hasn't actually replaced the body.
		gooseReq.RespReader = nil
	}
	return &httpfile.Response{
		StatusCode:    gooseReq.RespStatusCode,
		Header:        gooseReq.RespHeaders,
		ContentLength: gooseReq.RespLength,
		Body:          gooseReq.RespReader,
	}, err
}
