// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The csclient package provides access to the charm store API.
//
// Errors returned from the remote API server with an associated error
// code will have a cause of type params.ErrorCode holding that code.
//
// If a call to the API returns an error because authorization has been
// denied, an error with a cause satisfying IsAuthorizationError will be
// returned. Note that these errors can also include errors returned by
// httpbakery when it attempts to discharge macaroons.
package csclient

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

const apiVersion = "v5"

// ServerURL holds the default location of the global charm store.
// An alternate location can be configured by changing the URL field in the
// Params struct.
// For live testing or QAing the application, a different charm store
// location should be used, for instance "https://api.staging.jujucharms.com".
var ServerURL = "https://api.jujucharms.com/charmstore"

// Client represents the client side of a charm store.
type Client struct {
	params        Params
	bclient       httpClient
	header        http.Header
	statsDisabled bool
	channel       params.Channel
}

// Params holds parameters for creating a new charm store client.
type Params struct {
	// URL holds the root endpoint URL of the charmstore,
	// with no trailing slash, not including the version.
	// For example https://api.jujucharms.com/charmstore
	// If empty, the default charm store client location is used.
	URL string

	// User holds the name to authenticate as for the client. If User is empty,
	// no credentials will be sent.
	User string

	// Password holds the password for the given user, for authenticating the
	// client.
	Password string

	// BakeryClient holds the bakery client to use when making
	// requests to the store. This is used in preference to
	// HTTPClient.
	BakeryClient *httpbakery.Client

	// HTTPClient holds the HTTP client to use when making
	// requests to the store. If nil, httpbakery.NewHTTPClient will
	// be used.
	HTTPClient *http.Client

	// VisitWebPage is called when authorization requires that
	// the user visits a web page to authenticate themselves.
	// If nil, no interaction will be allowed. This field
	// is ignored if BakeryClient is provided.
	VisitWebPage func(url *url.URL) error

	// Auth holds a list of macaroons that will be added to the cookie jar of
	// the HTTP Client that is used by this client.
	Auth macaroon.Slice
}

type httpClient interface {
	DoWithBody(*http.Request, io.ReadSeeker) (*http.Response, error)
}

// New returns a new charm store client.
func New(p Params) *Client {
	if p.URL == "" {
		p.URL = ServerURL
	}
	bclient := p.BakeryClient
	if bclient == nil {
		if p.HTTPClient == nil {
			p.HTTPClient = httpbakery.NewHTTPClient()
		}
		bclient = &httpbakery.Client{
			Client:       p.HTTPClient,
			VisitWebPage: p.VisitWebPage,
		}
	}
	if len(p.Auth) > 0 {
		url, err := url.Parse(p.URL)
		// A non-nil error here will get caught at request time when we try
		// to parse the URL, and without a valid URL, the macaroons don't matter
		// anyway.
		if err == nil {
			httpbakery.SetCookie(bclient.Jar, url, p.Auth)
		}
	}
	return &Client{
		bclient: bclient,
		params:  p,
	}
}

// ServerURL returns the charm store URL used by the client.
func (c *Client) ServerURL() string {
	return c.params.URL
}

// DisableStats disables incrementing download stats when retrieving archives
// from the charm store.
func (c *Client) DisableStats() {
	c.statsDisabled = true
}

// WithChannel returns a new client whose requests are done using the
// given channel.
func (c *Client) WithChannel(channel params.Channel) *Client {
	client := *c
	client.channel = channel
	return &client
}

// Channel returns the currently set channel.
func (c *Client) Channel() params.Channel {
	return c.channel
}

// SetHTTPHeader sets custom HTTP headers that will be sent to the charm store
// on each request.
func (c *Client) SetHTTPHeader(header http.Header) {
	c.header = header
}

// GetArchive retrieves the archive for the given charm or bundle, returning a
// reader its data can be read from, the fully qualified id of the
// corresponding entity, the hex-encoded SHA384 hash of the data and its size.
func (c *Client) GetArchive(id *charm.URL) (r io.ReadCloser, eid *charm.URL, hash string, size int64, err error) {
	// Create the request.
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		return nil, nil, "", 0, errgo.Notef(err, "cannot make new request")
	}

	// Send the request.
	v := url.Values{}
	if c.statsDisabled {
		v.Set("stats", "0")
	}
	u := url.URL{
		Path:     "/" + id.Path() + "/archive",
		RawQuery: v.Encode(),
	}
	resp, err := c.Do(req, u.String())
	if err != nil {
		return nil, nil, "", 0, errgo.NoteMask(err, "cannot get archive", isAPIError)
	}

	// Validate the response headers.
	entityId := resp.Header.Get(params.EntityIdHeader)
	if entityId == "" {
		resp.Body.Close()
		return nil, nil, "", 0, errgo.Newf("no %s header found in response", params.EntityIdHeader)
	}
	eid, err = charm.ParseURL(entityId)
	if err != nil {
		// The server did not return a valid id.
		resp.Body.Close()
		return nil, nil, "", 0, errgo.Notef(err, "invalid entity id found in response")
	}
	if eid.Revision == -1 {
		// The server did not return a fully qualified entity id.
		resp.Body.Close()
		return nil, nil, "", 0, errgo.Newf("archive get returned not fully qualified entity id %q", eid)
	}
	hash = resp.Header.Get(params.ContentHashHeader)
	if hash == "" {
		resp.Body.Close()
		return nil, nil, "", 0, errgo.Newf("no %s header found in response", params.ContentHashHeader)
	}

	// Validate the response contents.
	if resp.ContentLength < 0 {
		// TODO frankban: handle the case the contents are chunked.
		resp.Body.Close()
		return nil, nil, "", 0, errgo.Newf("no content length found in response")
	}
	return resp.Body, eid, hash, resp.ContentLength, nil
}

// ListResources retrieves the metadata about resources for the given charms.
// It returns a slice with an element for each of the given ids, holding the
// resources for the respective id.
func (c *Client) ListResources(id *charm.URL) ([]params.Resource, error) {
	var result []params.Resource
	if err := c.Get("/"+id.Path()+"/meta/resources", &result); err != nil {
		return nil, errgo.NoteMask(err, "cannot get resource metadata from the charm store", isAPIError)
	}
	return result, nil
}

// UploadResource uploads the bytes for a resource.
func (c *Client) UploadResource(id *charm.URL, name, path string, file io.ReadSeeker) (revision int, err error) {
	hash, size, err := readerHashAndSize(file)
	if err != nil {
		return -1, errgo.Mask(err)
	}

	// Prepare the request.
	req, err := http.NewRequest("POST", "", nil)
	if err != nil {
		return -1, errgo.Notef(err, "cannot make new request")
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = size

	hash = url.QueryEscape(hash)
	path = url.QueryEscape(path)

	url := fmt.Sprintf("/%s/resource/%s?hash=%s&filename=%s", id.Path(), name, hash, path)

	// Send the request.
	resp, err := c.DoWithBody(req, url, file)
	if err != nil {
		return -1, errgo.NoteMask(err, "cannot post resource", isAPIError)
	}
	defer resp.Body.Close()

	// Parse the response.
	var result params.ResourceUploadResponse
	if err := parseResponseBody(resp.Body, &result); err != nil {
		return -1, errgo.Mask(err)
	}
	return result.Revision, nil
}

// Publish tells the charmstore to mark the given charm as published with the
// given resource revisions to the given channels.
func (s *Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	if len(channels) == 0 {
		return nil
	}
	val := &params.PublishRequest{
		Resources: resources,
		Channels:  channels,
	}
	if err := s.Put("/"+id.Path()+"/publish", val); err != nil {
		return errgo.Mask(err, isAPIError)
	}
	return nil
}

// ResourceData holds information about a resource.
// It must be closed after use.
type ResourceData struct {
	io.ReadCloser
	Hash string
	Size int64
}

// GetResource retrieves byes of the resource with the given name and revision
// for the given charm, returning a reader its data can be read from,  the
// SHA384 hash of the data and its size.
//
// Note that the result must be closed after use.
func (c *Client) GetResource(id *charm.URL, name string, revision int) (result ResourceData, err error) {
	if revision < 0 {
		return result, errgo.New("revision must be a non-negative integer")
	}
	// Create the request.
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		return result, errgo.Notef(err, "cannot make new request")
	}

	url := "/" + id.Path() + "/resource/" + name
	if revision >= 0 {
		url += "/" + strconv.Itoa(revision)
	}
	resp, err := c.Do(req, url)
	if err != nil {
		return result, errgo.NoteMask(err, "cannot get resource", isAPIError)
	}
	defer func() {
		if err != nil {
			resp.Body.Close()
		}
	}()

	// Validate the response headers.
	hash := resp.Header.Get(params.ContentHashHeader)
	if hash == "" {
		return result, errgo.Newf("no %s header found in response", params.ContentHashHeader)
	}

	// Validate the response contents.
	if resp.ContentLength < 0 {
		return result, errgo.Newf("no content length found in response")
	}
	return ResourceData{
		ReadCloser: resp.Body,
		Hash:       hash,
		Size:       resp.ContentLength,
	}, nil
}

// ResourceMeta returns the metadata for the resource on charm id with the
// given name and revision. If the revision is negative, the latest version
// of the resource will be returned.
func (c *Client) ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error) {
	path := fmt.Sprintf("/%s/meta/resources/%s", id.Path(), name)
	if revision >= 0 {
		path += fmt.Sprintf("/%d", revision)
	}
	var result params.Resource
	if err := c.Get(path, &result); err != nil {
		return result, errgo.NoteMask(err, fmt.Sprintf("cannot get %q", path), isAPIError)
	}
	return result, nil
}

// StatsUpdate updates the download stats for the given id and specific time.
func (c *Client) StatsUpdate(req params.StatsUpdateRequest) error {
	return c.Put("/stats/update", req)
}

// UploadCharm uploads the given charm to the charm store with the given id,
// which must not specify a revision.
// The accepted charm implementations are charm.CharmDir and
// charm.CharmArchive.
//
// UploadCharm returns the id that the charm has been given in the
// store - this will be the same as id except the revision.
func (c *Client) UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error) {
	if id.Revision != -1 {
		return nil, errgo.Newf("revision specified in %q, but should not be specified", id)
	}
	r, hash, size, err := openArchive(ch)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open charm archive")
	}
	defer r.Close()
	return c.uploadArchive(id, r, hash, size, -1)
}

// UploadCharmWithRevision uploads the given charm to the
// given id in the charm store, which must contain a revision.
// If promulgatedRevision is not -1, it specifies that the charm
// should be marked as promulgated with that revision.
//
// This method is provided only for testing and should not
// generally be used otherwise.
func (c *Client) UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error {
	if id.Revision == -1 {
		return errgo.Newf("revision not specified in %q", id)
	}
	r, hash, size, err := openArchive(ch)
	if err != nil {
		return errgo.Notef(err, "cannot open charm archive")
	}
	defer r.Close()
	_, err = c.uploadArchive(id, r, hash, size, promulgatedRevision)
	return errgo.Mask(err, isAPIError)
}

// UploadBundle uploads the given charm to the charm store with the given id,
// which must not specify a revision.
// The accepted bundle implementations are charm.BundleDir and
// charm.BundleArchive.
//
// UploadBundle returns the id that the bundle has been given in the
// store - this will be the same as id except the revision.
func (c *Client) UploadBundle(id *charm.URL, b charm.Bundle) (*charm.URL, error) {
	if id.Revision != -1 {
		return nil, errgo.Newf("revision specified in %q, but should not be specified", id)
	}
	r, hash, size, err := openArchive(b)
	if err != nil {
		return nil, errgo.Notef(err, "cannot open bundle archive")
	}
	defer r.Close()
	return c.uploadArchive(id, r, hash, size, -1)
}

// UploadBundleWithRevision uploads the given bundle to the
// given id in the charm store, which must contain a revision.
// If promulgatedRevision is not -1, it specifies that the charm
// should be marked as promulgated with that revision.
//
// This method is provided only for testing and should not
// generally be used otherwise.
func (c *Client) UploadBundleWithRevision(id *charm.URL, b charm.Bundle, promulgatedRevision int) error {
	if id.Revision == -1 {
		return errgo.Newf("revision not specified in %q", id)
	}
	r, hash, size, err := openArchive(b)
	if err != nil {
		return errgo.Notef(err, "cannot open charm archive")
	}
	defer r.Close()
	_, err = c.uploadArchive(id, r, hash, size, promulgatedRevision)
	return errgo.Mask(err, isAPIError)
}

// uploadArchive pushes the archive for the charm or bundle represented by
// the given body, its hex-encoded SHA384 hash and its size. It returns
// the resulting entity reference. The given id should include the series
// and should not include the revision.
func (c *Client) uploadArchive(id *charm.URL, body io.ReadSeeker, hash string, size int64, promulgatedRevision int) (*charm.URL, error) {
	// When uploading archives, it can be a problem that the
	// an error response is returned while we are still writing
	// the body data.
	// To avoid this, we log in first so that we don't need to
	// do the macaroon exchange after POST.
	// Unfortunately this won't help matters if the user is logged in but
	// doesn't have privileges to write to the stated charm.
	// A better solution would be to fix https://github.com/golang/go/issues/3665
	// and use the 100-Continue client functionality.
	//
	// We only need to do this when basic auth credentials are not provided.
	if c.params.User == "" {
		if err := c.Login(); err != nil {
			return nil, errgo.NoteMask(err, "cannot log in", isAPIError)
		}
	}
	method := "POST"
	promulgatedArg := ""
	if id.Revision != -1 {
		method = "PUT"
		if promulgatedRevision != -1 {
			pr := *id
			pr.User = ""
			pr.Revision = promulgatedRevision
			promulgatedArg = "&promulgated=" + pr.Path()
		}
	}

	// Prepare the request.
	req, err := http.NewRequest(method, "", nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot make new request")
	}
	req.Header.Set("Content-Type", "application/zip")
	req.ContentLength = size

	// Send the request.
	resp, err := c.DoWithBody(
		req,
		"/"+id.Path()+"/archive?hash="+hash+promulgatedArg,
		body,
	)
	if err != nil {
		return nil, errgo.NoteMask(err, "cannot post archive", isAPIError)
	}
	defer resp.Body.Close()

	// Parse the response.
	var result params.ArchiveUploadResponse
	if err := parseResponseBody(resp.Body, &result); err != nil {
		return nil, errgo.Mask(err)
	}
	return result.Id, nil
}

// PutExtraInfo puts extra-info data for the given id.
// Each entry in the info map causes a value in extra-info with
// that key to be set to the associated value.
// Entries not set in the map will be unchanged.
func (c *Client) PutExtraInfo(id *charm.URL, info map[string]interface{}) error {
	return c.Put("/"+id.Path()+"/meta/extra-info", info)
}

// PutCommonInfo puts common-info data for the given id.
// Each entry in the info map causes a value in common-info with
// that key to be set to the associated value.
// Entries not set in the map will be unchanged.
func (c *Client) PutCommonInfo(id *charm.URL, info map[string]interface{}) error {
	return c.Put("/"+id.Path()+"/meta/common-info", info)
}

// Meta fetches metadata on the charm or bundle with the
// given id. The result value provides a value
// to be filled in with the result, which must be
// a pointer to a struct containing members corresponding
// to possible metadata include parameters
// (see https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmeta).
//
// It returns the fully qualified id of the entity.
//
// The name of the struct member is translated to
// a lower case hyphen-separated form; for example,
// ArchiveSize becomes "archive-size", and BundleMachineCount
// becomes "bundle-machine-count", but may also
// be specified in the field's tag
//
// This example will fill in the result structure with information
// about the given id, including information on its archive
// size (include archive-size), upload time (include archive-upload-time)
// and digest (include extra-info/digest).
//
//	var result struct {
//		ArchiveSize params.ArchiveSizeResponse
//		ArchiveUploadTime params.ArchiveUploadTimeResponse
//		Digest string `csclient:"extra-info/digest"`
//	}
//	id, err := client.Meta(id, &result)
func (c *Client) Meta(id *charm.URL, result interface{}) (*charm.URL, error) {
	if result == nil {
		return nil, fmt.Errorf("expected valid result pointer, not nil")
	}
	resultv := reflect.ValueOf(result)
	resultt := resultv.Type()
	if resultt.Kind() != reflect.Ptr {
		return nil, fmt.Errorf("expected pointer, not %T", result)
	}
	resultt = resultt.Elem()
	if resultt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected pointer to struct, not %T", result)
	}
	resultv = resultv.Elem()

	// At this point, resultv refers to the struct value pointed
	// to by result, and resultt is its type.

	numField := resultt.NumField()
	includes := make([]string, 0, numField)

	// results holds an entry for each field in the result value,
	// pointing to the value for that field.
	results := make(map[string]reflect.Value)
	for i := 0; i < numField; i++ {
		field := resultt.Field(i)
		if field.PkgPath != "" {
			// Field is private; ignore it.
			continue
		}
		if field.Anonymous {
			// At some point in the future, it might be nice to
			// support anonymous fields, but for now the
			// additional complexity doesn't seem worth it.
			return nil, fmt.Errorf("anonymous fields not supported")
		}
		apiName := field.Tag.Get("csclient")
		if apiName == "" {
			apiName = hyphenate(field.Name)
		}
		includes = append(includes, "include="+apiName)
		results[apiName] = resultv.FieldByName(field.Name).Addr()
	}
	// We unmarshal into rawResult, then unmarshal each field
	// separately into its place in the final result value.
	// Note that we can't use params.MetaAnyResponse because
	// that will unpack all the values inside the Meta field,
	// but we want to keep them raw so that we can unmarshal
	// them ourselves.
	var rawResult struct {
		Id   *charm.URL
		Meta map[string]json.RawMessage
	}
	path := "/" + id.Path() + "/meta/any"
	if len(includes) > 0 {
		path += "?" + strings.Join(includes, "&")
	}
	if err := c.Get(path, &rawResult); err != nil {
		return nil, errgo.NoteMask(err, fmt.Sprintf("cannot get %q", path), isAPIError)
	}
	// Note that the server is not required to send back values
	// for all fields. "If there is no metadata for the given meta path, the
	// element will be omitted"
	// See https://github.com/juju/charmstore/blob/v4/docs/API.md#get-idmetaany
	for name, r := range rawResult.Meta {
		v, ok := results[name]
		if !ok {
			// The server has produced a result that we
			// don't know about. Ignore it.
			continue
		}
		// Unmarshal the raw JSON into the final struct field.
		err := json.Unmarshal(r, v.Interface())
		if err != nil {
			return nil, errgo.Notef(err, "cannot unmarshal %s", name)
		}
	}
	return rawResult.Id, nil
}

// hyphenate returns the hyphenated version of the given
// field name, as specified in the Client.Meta method.
func hyphenate(s string) string {
	// TODO hyphenate FooHTTPBar as foo-http-bar?
	var buf bytes.Buffer
	var prevLower bool
	for _, r := range s {
		if !unicode.IsUpper(r) {
			prevLower = true
			buf.WriteRune(r)
			continue
		}
		if prevLower {
			buf.WriteRune('-')
		}
		buf.WriteRune(unicode.ToLower(r))
		prevLower = false
	}
	return buf.String()
}

// Get makes a GET request to the given path in the charm store (not
// including the host name or version prefix but including a leading /),
// parsing the result as JSON into the given result value, which should
// be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *Client) Get(path string, result interface{}) error {
	req, err := http.NewRequest("GET", "", nil)
	if err != nil {
		return errgo.Notef(err, "cannot make new request")
	}
	resp, err := c.Do(req, path)
	if err != nil {
		return errgo.Mask(err, isAPIError)
	}
	defer resp.Body.Close()
	// Parse the response.
	if err := parseResponseBody(resp.Body, result); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Put makes a PUT request to the given path in the charm store
// (not including the host name or version prefix, but including a leading /),
// marshaling the given value as JSON to use as the request body.
func (c *Client) Put(path string, val interface{}) error {
	return c.PutWithResponse(path, val, nil)
}

// PutWithResponse makes a PUT request to the given path in the charm store
// (not including the host name or version prefix, but including a leading /),
// marshaling the given value as JSON to use as the request body. Additionally,
// this method parses the result as JSON into the given result value, which
// should be a pointer to the expected data, but may be nil if no result is
// desired.
func (c *Client) PutWithResponse(path string, val, result interface{}) error {
	req, _ := http.NewRequest("PUT", "", nil)
	req.Header.Set("Content-Type", "application/json")
	data, err := json.Marshal(val)
	if err != nil {
		return errgo.Notef(err, "cannot marshal PUT body")
	}
	body := bytes.NewReader(data)
	resp, err := c.DoWithBody(req, path, body)
	if err != nil {
		return errgo.Mask(err, isAPIError)
	}
	defer resp.Body.Close()
	// Parse the response.
	if err := parseResponseBody(resp.Body, result); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func parseResponseBody(body io.Reader, result interface{}) error {
	data, err := ioutil.ReadAll(body)
	if err != nil {
		return errgo.Notef(err, "cannot read response body")
	}
	if result == nil {
		// The caller doesn't care about the response body.
		return nil
	}
	if err := json.Unmarshal(data, result); err != nil {
		return errgo.Notef(err, "cannot unmarshal response %q", sizeLimit(data))
	}
	return nil
}

// DoWithBody is like Do except that the given body is used
// as the body of the HTTP request.
//
// Any error returned from the underlying httpbakery.DoWithBody
// request will have an unchanged error cause.
func (c *Client) DoWithBody(req *http.Request, path string, body io.ReadSeeker) (*http.Response, error) {
	if c.params.User != "" {
		userPass := c.params.User + ":" + c.params.Password
		authBasic := base64.StdEncoding.EncodeToString([]byte(userPass))
		req.Header.Set("Authorization", "Basic "+authBasic)
	}

	// Prepare the request.
	if !strings.HasPrefix(path, "/") {
		return nil, errgo.Newf("path %q is not absolute", path)
	}
	for k, vv := range c.header {
		req.Header[k] = append(req.Header[k], vv...)
	}
	u, err := url.Parse(c.params.URL + "/" + apiVersion + path)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	if c.channel != params.NoChannel {
		values := u.Query()
		values.Set("channel", string(c.channel))
		u.RawQuery = values.Encode()
	}
	req.URL = u

	// Send the request.
	resp, err := c.bclient.DoWithBody(req, body)
	if err != nil {
		return nil, errgo.Mask(err, isAPIError)
	}

	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}
	defer resp.Body.Close()

	// Parse the response error.
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errgo.Notef(err, "cannot read response body")
	}
	var perr params.Error
	if err := json.Unmarshal(data, &perr); err != nil {
		return nil, errgo.Notef(err, "cannot unmarshal error response %q", sizeLimit(data))
	}
	if perr.Message == "" {
		return nil, errgo.Newf("error response with empty message %s", sizeLimit(data))
	}
	return nil, &perr
}

// Do makes an arbitrary request to the charm store.
// It adds appropriate headers to the given HTTP request,
// sends it to the charm store, and returns the resulting
// response. Do never returns a response with a status
// that is not http.StatusOK.
//
// The URL field in the request is ignored and overwritten.
//
// This is a low level method - more specific Client methods
// should be used when possible.
//
// For requests with a body (for example PUT or POST) use DoWithBody
// instead.
func (c *Client) Do(req *http.Request, path string) (*http.Response, error) {
	if req.Body != nil {
		return nil, errgo.New("body unexpectedly provided in http request - use DoWithBody")
	}
	return c.DoWithBody(req, path, nil)
}

func sizeLimit(data []byte) []byte {
	const max = 1024
	if len(data) < max {
		return data
	}
	return append(data[0:max], fmt.Sprintf(" ... [%d bytes omitted]", len(data)-max)...)
}

// Log sends a log message to the charmstore's log database.
func (cs *Client) Log(typ params.LogType, level params.LogLevel, message string, urls ...*charm.URL) error {
	b, err := json.Marshal(message)
	if err != nil {
		return errgo.Notef(err, "cannot marshal log message")
	}

	// Prepare and send the log.
	// TODO (frankban): we might want to buffer logs in order to reduce
	// requests.
	logs := []params.Log{{
		Data:  (*json.RawMessage)(&b),
		Level: level,
		Type:  typ,
		URLs:  urls,
	}}
	b, err = json.Marshal(logs)
	if err != nil {
		return errgo.Notef(err, "cannot marshal log message")
	}

	req, err := http.NewRequest("POST", "", nil)
	if err != nil {
		return errgo.Notef(err, "cannot create log request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := cs.DoWithBody(req, "/log", bytes.NewReader(b))
	if err != nil {
		return errgo.NoteMask(err, "cannot send log message", isAPIError)
	}
	resp.Body.Close()
	return nil
}

// Login explicitly obtains authorization credentials for the charm store
// and stores them in the client's cookie jar. If there was an error
// perfoming a login interaction then the error will have a cause of type
// *httpbakery.InteractionError.
func (cs *Client) Login() error {
	if err := cs.Get("/delegatable-macaroon", &struct{}{}); err != nil {
		return errgo.NoteMask(err, "cannot retrieve the authentication macaroon", isAPIError)
	}
	return nil
}

// WhoAmI returns the user and list of groups associated with the macaroon
// used to authenticate.
func (cs *Client) WhoAmI() (*params.WhoAmIResponse, error) {
	var response params.WhoAmIResponse
	if err := cs.Get("/whoami", &response); err != nil {
		return nil, errgo.Mask(err, isAPIError)
	}
	return &response, nil
}

// Latest returns the most current revision for each of the identified
// charms. The revision in the provided charm URLs is ignored.
func (cs *Client) Latest(curls []*charm.URL) ([]params.CharmRevision, error) {
	if len(curls) == 0 {
		return nil, nil
	}

	// Prepare the request to the charm store.
	urls := make([]string, len(curls))
	values := url.Values{}
	// Include the ignore-auth flag so that non-public results do not generate
	// an error for the whole request.
	values.Add("ignore-auth", "1")
	values.Add("include", "id-revision")
	values.Add("include", "hash256")
	for i, curl := range curls {
		url := curl.WithRevision(-1).String()
		urls[i] = url
		values.Add("id", url)
	}
	u := url.URL{
		Path:     "/meta/any",
		RawQuery: values.Encode(),
	}

	// Execute the request and retrieve results.
	var results map[string]struct {
		Meta struct {
			IdRevision params.IdRevisionResponse `json:"id-revision"`
			Hash256    params.HashResponse       `json:"hash256"`
		}
	}
	if err := cs.Get(u.String(), &results); err != nil {
		return nil, errgo.NoteMask(err, "cannot get metadata from the charm store", isAPIError)
	}

	// Build the response.
	responses := make([]params.CharmRevision, len(curls))
	for i, url := range urls {
		result, found := results[url]
		if !found {
			responses[i] = params.CharmRevision{
				Err: params.ErrNotFound,
			}
			continue
		}
		responses[i] = params.CharmRevision{
			Revision: result.Meta.IdRevision.Revision,
			Sha256:   result.Meta.Hash256.Sum,
		}
	}
	return responses, nil
}

// JujuMetadataHTTPHeader is the HTTP header name used to send Juju metadata
// attributes to the charm store.
const JujuMetadataHTTPHeader = "Juju-Metadata"

// IsAuthorizationError reports whether the given error
// was returned because authorization was denied for a
// charmstore request.
func IsAuthorizationError(err error) bool {
	err = errgo.Cause(err)
	switch {
	case httpbakery.IsDischargeError(err):
		return true
	case httpbakery.IsInteractionError(err):
		return true
	case err == params.ErrUnauthorized:
		return true
	}
	return false
}

func isAPIError(err error) bool {
	if err == nil {
		return false
	}
	err = errgo.Cause(err)
	if _, ok := err.(params.ErrorCode); ok {
		return true
	}
	return IsAuthorizationError(err)
}
