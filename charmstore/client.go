// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"
)

var logger = loggo.GetLogger("juju.charmstore")

// TODO(natefinch): Ideally, this whole package would live in the
// charmstore-client repo, so as to keep it near the API it wraps (and make it
// more available to tools outside juju-core).

// MacaroonCache represents a value that can store and retrieve macaroons for
// charms.  It is used when we are requesting data from the charmstore for
// private charms.
type MacaroonCache interface {
	Set(*charm.URL, macaroon.Slice) error
	Get(*charm.URL) (macaroon.Slice, error)
}

// NewCachingClient returns a Juju charm store client that stores and retrieves
// macaroons for calls in the given cache. If not nil, the client will use server
// as the charmstore url, otherwise it will default to the standard juju
// charmstore url.
func NewCachingClient(cache MacaroonCache, server *url.URL) (Client, error) {
	return newCachingClient(cache, server, makeWrapper)
}

func newCachingClient(
	cache MacaroonCache,
	server *url.URL,
	makeWrapper func(*httpbakery.Client, *url.URL) csWrapper,
) (Client, error) {
	bakeryClient := &httpbakery.Client{
		Client: httpbakery.NewHTTPClient(),
	}
	client := makeWrapper(bakeryClient, server)
	server, err := url.Parse(client.ServerURL())
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	jar, err := newMacaroonJar(cache, server)
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	bakeryClient.Jar = jar
	return Client{client, jar}, nil
}

// TODO(natefinch): we really shouldn't let something like a bakeryclient
// leak out of our abstraction like this. Instead, pass more salient details.

// NewCustomClient returns a juju charmstore client that relies on the passed-in
// httpbakery.Client to store and retrieve macaroons.  If not nil, the client
// will use server as the charmstore url, otherwise it will default to the
// standard juju charmstore url.
func NewCustomClient(bakeryClient *httpbakery.Client, server *url.URL) (Client, error) {
	return newCustomClient(bakeryClient, server, makeWrapper)
}

func newCustomClient(
	bakeryClient *httpbakery.Client,
	server *url.URL,
	makeWrapper func(*httpbakery.Client, *url.URL) csWrapper,
) (Client, error) {
	client := makeWrapper(bakeryClient, server)
	return Client{csWrapper: client}, nil
}

func makeWrapper(bakeryClient *httpbakery.Client, server *url.URL) csWrapper {
	p := csclient.Params{
		BakeryClient: bakeryClient,
	}
	if server != nil {
		p.URL = server.String()
	}
	return csclientImpl{csclient.New(p)}
}

// Client wraps charmrepo/csclient (the charm store's API client
// library) in a higher level API.
type Client struct {
	csWrapper
	jar *macaroonJar
}

// CharmRevision holds the data returned from the charmstore about the latest
// revision of a charm.  Notet hat this may be different per channel.
type CharmRevision struct {
	// Revision is newest revision for the charm.
	Revision int

	// Err holds any error that occurred while making the request.
	Err error
}

// LatestRevisions returns the latest revisions of the given charms, using the given metadata.
func (c Client) LatestRevisions(charms []CharmID, metadata map[string][]string) ([]CharmRevision, error) {
	// Due to the fact that we cannot use multiple macaroons per API call,
	// we need to perform one call at a time, rather than making bulk calls.
	// We could bulk the calls that use non-private charms, but we'd still need
	// to do one bulk call per channel, due to how channels are used by the
	// underlying csclient.
	results := make([]CharmRevision, len(charms))
	for i, cid := range charms {
		revisions, err := c.csWrapper.Latest(cid.Channel, []*charm.URL{cid.URL}, metadata)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rev := revisions[0]
		results[i] = CharmRevision{Revision: rev.Revision, Err: rev.Err}
	}
	return results, nil
}

func (c Client) latestRevisions(channel csparams.Channel, cid CharmID, metadata map[string][]string) (CharmRevision, error) {
	if err := c.jar.Activate(cid.URL); err != nil {
		return CharmRevision{}, errors.Trace(err)
	}
	defer c.jar.Deactivate()
	revisions, err := c.csWrapper.Latest(cid.Channel, []*charm.URL{cid.URL}, metadata)
	if err != nil {
		return CharmRevision{}, errors.Trace(err)
	}
	rev := revisions[0]
	return CharmRevision{Revision: rev.Revision, Err: rev.Err}, nil
}

// ResourceRequest is the data needed to request a resource from the charmstore.
type ResourceRequest struct {
	// Charm is the URL of the charm for which we're requesting a resource.
	Charm *charm.URL

	// Channel is the channel from which to request the resource info.
	Channel csparams.Channel

	// Name is the name of the resource we're asking about.
	Name string

	// Revision is the specific revision of the resource we're asking about.
	Revision int
}

// ResourceData represents the response from the charmstore about a request for
// resource bytes.
type ResourceData struct {
	// ReadCloser holds the bytes for the resource.
	io.ReadCloser

	// Resource holds the metadata for the resource.
	Resource charmresource.Resource
}

// GetResource returns the data (bytes) and metadata for a resource from the charmstore.
func (c Client) GetResource(req ResourceRequest) (data ResourceData, err error) {
	if err := c.jar.Activate(req.Charm); err != nil {
		return ResourceData{}, errors.Trace(err)
	}
	defer c.jar.Deactivate()
	meta, err := c.csWrapper.ResourceMeta(req.Channel, req.Charm, req.Name, req.Revision)

	if err != nil {
		return ResourceData{}, errors.Trace(err)
	}
	data.Resource, err = csparams.API2Resource(meta)
	if err != nil {
		return ResourceData{}, errors.Trace(err)
	}
	resData, err := c.csWrapper.GetResource(req.Channel, req.Charm, req.Name, req.Revision)
	if err != nil {
		return ResourceData{}, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			resData.Close()
		}
	}()
	data.ReadCloser = resData.ReadCloser
	fpHash := data.Resource.Fingerprint.String()
	if resData.Hash != fpHash {
		return ResourceData{},
			errors.Errorf("fingerprint for data (%s) does not match fingerprint in metadata (%s)", resData.Hash, fpHash)
	}
	if resData.Size != data.Resource.Size {
		return ResourceData{},
			errors.Errorf("size for data (%d) does not match size in metadata (%d)", resData.Size, data.Resource.Size)
	}
	return data, nil
}

// ResourceInfo returns the metadata for the given resource from the charmstore.
func (c Client) ResourceInfo(req ResourceRequest) (charmresource.Resource, error) {
	if err := c.jar.Activate(req.Charm); err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	defer c.jar.Deactivate()
	meta, err := c.csWrapper.ResourceMeta(req.Channel, req.Charm, req.Name, req.Revision)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	res, err := csparams.API2Resource(meta)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	return res, nil
}

// ListResources returns a list of resources for each of the given charms.
func (c Client) ListResources(charms []CharmID) ([][]charmresource.Resource, error) {
	results := make([][]charmresource.Resource, len(charms))
	for i, ch := range charms {
		res, err := c.listResources(ch)
		if err != nil {
			if csclient.IsAuthorizationError(err) || errors.Cause(err) == csparams.ErrNotFound {
				// Ignore authorization errors and not-found errors so we get some results
				// even if others fail.
				continue
			}
			return nil, errors.Trace(err)
		}
		results[i] = res
	}
	return results, nil
}

func (c Client) listResources(ch CharmID) ([]charmresource.Resource, error) {
	if err := c.jar.Activate(ch.URL); err != nil {
		return nil, errors.Trace(err)
	}
	defer c.jar.Deactivate()
	resources, err := c.csWrapper.ListResources(ch.Channel, ch.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api2resources(resources)
}

// csWrapper is a type that abstracts away the low-level implementation details
// of the charmstore client.
type csWrapper interface {
	Latest(channel csparams.Channel, ids []*charm.URL, headers map[string][]string) ([]csparams.CharmRevision, error)
	ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error)
	GetResource(channel csparams.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error)
	ResourceMeta(channel csparams.Channel, id *charm.URL, name string, revision int) (csparams.Resource, error)
	ServerURL() string
}

// csclientImpl is an implementation of csWrapper that uses csclient.Client.
// It exists for testing purposes to hide away the hard-to-mock parts of
// csclient.Client.
type csclientImpl struct {
	*csclient.Client
}

// Latest gets the latest CharmRevisions for the charm URLs on the channel.
func (c csclientImpl) Latest(channel csparams.Channel, ids []*charm.URL, metadata map[string][]string) ([]csparams.CharmRevision, error) {
	client := c.WithChannel(channel)
	client.SetHTTPHeader(http.Header(metadata))
	return client.Latest(ids)
}

// ListResources gets the latest resources for the charm URL on the channel.
func (c csclientImpl) ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error) {
	client := c.WithChannel(channel)
	return client.ListResources(id)
}

// Getresource downloads the bytes and some metadata about the bytes for the revisioned resource.
func (c csclientImpl) GetResource(channel csparams.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	client := c.WithChannel(channel)
	return client.GetResource(id, name, revision)
}

// ResourceInfo gets the full metadata for the revisioned resource.
func (c csclientImpl) ResourceMeta(channel csparams.Channel, id *charm.URL, name string, revision int) (csparams.Resource, error) {
	client := c.WithChannel(channel)
	return client.ResourceMeta(id, name, revision)
}

func api2resources(res []csparams.Resource) ([]charmresource.Resource, error) {
	result := make([]charmresource.Resource, len(res))
	for i, r := range res {
		var err error
		result[i], err = csparams.API2Resource(r)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}
