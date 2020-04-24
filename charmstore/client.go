// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/resource"
	"github.com/juju/charmrepo/v5/csclient"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"
)

var logger = loggo.GetLogger("juju.charmstore")

// TODO(natefinch): Ideally, this whole package would live in the
// charmstore-client repo, so as to keep it near the API it wraps (and make it
// more available to tools outside juju-core).

// MacaroonCache represents a value that can store and retrieve macaroons for
// charms. It is used when we are requesting data from the charmstore for
// private charms.
type MacaroonCache interface {
	Set(*charm.URL, macaroon.Slice) error
	Get(*charm.URL) (macaroon.Slice, error)
}

// NewCachingClient returns a Juju charm store client that stores and retrieves
// macaroons for calls in the given cache. The client will use server as the
// charmstore url.
func NewCachingClient(cache MacaroonCache, server string) (Client, error) {
	return newCachingClient(cache, server, makeWrapper)
}

func newCachingClient(
	cache MacaroonCache,
	server string,
	makeWrapper func(*httpbakery.Client, string) (csWrapper, error),
) (Client, error) {
	bakeryClient := &httpbakery.Client{
		Client: httpbakery.NewHTTPClient(),
	}
	client, err := makeWrapper(bakeryClient, server)
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	serverURL, err := url.Parse(client.ServerURL())
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	jar, err := newMacaroonJar(cache, serverURL)
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	bakeryClient.Jar = jar
	return Client{client, jar}, nil
}

func NewCustomClient(base csWrapper) Client {
	return Client{
		csWrapper: base,
	}
}

// TODO(natefinch): we really shouldn't let something like a bakeryclient
// leak out of our abstraction like this. Instead, pass more salient details.

// NewCustomClientAtURL returns a juju charmstore client that relies on the passed-in
// httpbakery.Client to store and retrieve macaroons.  If not nil, the client
// will use server as the charmstore url, otherwise it will default to the
// standard juju charmstore url.
func NewCustomClientAtURL(bakeryClient *httpbakery.Client, server string) (Client, error) {
	return newCustomClient(bakeryClient, server, makeWrapper)
}

func newCustomClient(
	bakeryClient *httpbakery.Client,
	server string,
	makeWrapper func(*httpbakery.Client, string) (csWrapper, error),
) (Client, error) {
	client, err := makeWrapper(bakeryClient, server)
	if err != nil {
		return Client{}, errors.Trace(err)
	}
	return Client{csWrapper: client}, nil
}

func makeWrapper(bakeryClient *httpbakery.Client, server string) (csWrapper, error) {
	if server == "" {
		return csclientImpl{}, errors.NotValidf("empty charmstore URL")
	}
	p := csclient.Params{
		BakeryClient: bakeryClient,
		URL:          server,
	}
	return csclientImpl{csclient.New(p)}, nil
}

// Client wraps charmrepo/csclient (the charm store's API client
// library) in a higher level API.
type Client struct {
	csWrapper
	jar *macaroonJar
}

// CharmRevision holds the data returned from the charmstore about the latest
// revision of a charm. Note that this may be different per channel.
type CharmRevision struct {
	// Revision is newest revision for the charm.
	Revision int

	// Err holds any error that occurred while making the request.
	Err error
}

// LatestRevisions returns the latest revisions of the given charms, using the given metadata.
func (c Client) LatestRevisions(charms []CharmID, modelMetadata map[string]string) ([]CharmRevision, error) {
	// Due to the fact that we cannot use multiple macaroons per API call,
	// we need to perform one call at a time, rather than making bulk calls.
	// We could bulk the calls that use non-private charms, but we'd still need
	// to do one bulk call per channel, due to how channels are used by the
	// underlying csclient.
	results := make([]CharmRevision, len(charms))
	for i, cid := range charms {
		revisions, err := c.csWrapper.Latest(
			cid.Channel, []*charm.URL{cid.URL}, makeMetadataHeader(modelMetadata, cid.Metadata))
		if err != nil {
			return nil, errors.Trace(err)
		}
		rev := revisions[0]
		results[i] = CharmRevision{Revision: rev.Revision, Err: rev.Err}
	}
	return results, nil
}

// makeMetadataHeader takes the input model and charm metadata and transforms
// it into a header suitable for supply with a "Latest" request via the client.
func makeMetadataHeader(modelMetadata, charmMetadata map[string]string) map[string][]string {
	if len(modelMetadata) == 0 && len(charmMetadata) == 0 {
		return nil
	}

	headers := make([]string, 0, len(modelMetadata)+len(charmMetadata))

	// We expect the deployed architecture for a charm to be singular,
	// but it is possible for deployment across multiple architectures.
	// We need to handle this, which violates the general case following.
	if arch, ok := charmMetadata["arch"]; ok {
		for _, a := range strings.Split(arch, ",") {
			headers = append(headers, fmt.Sprintf("arch=%s", a))
		}
		delete(charmMetadata, "arch")
	}

	addHeaders := func(metadata map[string]string) {
		for k, v := range metadata {
			headers = append(headers, fmt.Sprintf("%s=%s", k, v))
		}
	}
	addHeaders(modelMetadata)
	addHeaders(charmMetadata)
	sort.Strings(headers)
	return map[string][]string{jujuMetadataHTTPHeader: headers}
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
	Resource resource.Resource
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
	if data.Resource.Type == resource.TypeFile {
		fpHash := data.Resource.Fingerprint.String()
		if resData.Hash != fpHash {
			return ResourceData{},
				errors.Errorf("fingerprint for data (%s) does not match fingerprint in metadata (%s)", resData.Hash, fpHash)
		}
	}
	return data, nil
}

// ResourceInfo returns the metadata for the given resource from the charmstore.
func (c Client) ResourceInfo(req ResourceRequest) (resource.Resource, error) {
	if err := c.jar.Activate(req.Charm); err != nil {
		return resource.Resource{}, errors.Trace(err)
	}
	defer c.jar.Deactivate()
	meta, err := c.csWrapper.ResourceMeta(req.Channel, req.Charm, req.Name, req.Revision)
	if err != nil {
		return resource.Resource{}, errors.Trace(err)
	}
	res, err := csparams.API2Resource(meta)
	if err != nil {
		return resource.Resource{}, errors.Trace(err)
	}
	return res, nil
}

// ListResources returns a list of resources for each of the given charms.
func (c Client) ListResources(charms []CharmID) ([][]resource.Resource, error) {
	results := make([][]resource.Resource, len(charms))
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

func (c Client) listResources(ch CharmID) ([]resource.Resource, error) {
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
	revs, err := client.Latest(ids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]csparams.CharmRevision, len(revs))
	for i, r := range revs {
		result[i] = csparams.CharmRevision{
			Revision: r.Revision,
			Err:      r.Err,
		}
	}
	return result, nil
}

// ListResources gets the latest resources for the charm URL on the channel.
func (c csclientImpl) ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error) {
	client := c.WithChannel(channel)
	return client.ListResources(id)
}

// GetResource downloads the bytes and some metadata about the bytes for the revisioned resource.
func (c csclientImpl) GetResource(channel csparams.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	client := c.WithChannel(channel)
	return client.GetResource(id, name, revision)
}

// ResourceInfo gets the full metadata for the revisioned resource.
func (c csclientImpl) ResourceMeta(channel csparams.Channel, id *charm.URL, name string, revision int) (csparams.Resource, error) {
	client := c.WithChannel(channel)
	return client.ResourceMeta(id, name, revision)
}

func api2resources(res []csparams.Resource) ([]resource.Resource, error) {
	result := make([]resource.Resource, len(res))
	for i, r := range res {
		var err error
		result[i], err = csparams.API2Resource(r)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return result, nil
}
