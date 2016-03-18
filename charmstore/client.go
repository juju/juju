// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"io"
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

var logger = loggo.GetLogger("juju.charmstore")

// TODO(ericsnow) Build around charmrepo.CharmStore instead of csclient.Client.

// BaseClient exposes the functionality of the charm store, as provided
// by github.com/juju/charmrepo/csclient.Client.
//
// Note that the following csclient.Client methods are used as well,
// but only in tests:
//  - Put(path string, val interface{}) error
//  - UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error)
//  - UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error
//  - UploadBundleWithRevision()
type BaseClient interface {
	ResourcesClient

	// TODO(ericsnow) This should really be returning a type from
	// charmrepo/csclient/params, but we don't have one yet.

	// LatestRevisions returns the latest revision for each of the
	// identified charms in the given channel. The revisions in the provided
	// URLs are ignored.
	LatestRevisions(urls []*charm.URL, channel string) ([]charmrepo.CharmRevision, error)

	// TODO(ericsnow) Replace use of Get with use of more specific API
	// methods? We only use Get() for authorization on the Juju client
	// side.

	// Get makes a GET request to the given path in the charm store. The
	// path must have a leading slash, but must not include the host
	// name or version prefix. The result is parsed as JSON into the
	// given result value, which should be a pointer to the expected
	// data, but may be nil if no result is desired.
	Get(path string, result interface{}) error
}

// ResourcesClient exposes the charm store client functionality for
// dealing with resources.
type ResourcesClient interface {
	// TODO(ericsnow) Just embed resource/charmstore.BaseClient (or vice-versa)?

	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources(charmURLs []*charm.URL, channel string) ([][]charmresource.Resource, error)

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	GetResource(cURL *charm.URL, resourceName string, revision int) (charmresource.Resource, io.ReadCloser, error)
}

type baseClient struct {
	*csclient.Client
}

// TODO(ericsnow) We must make the Juju metadata available here since
// we must use charmrepo.NewCharmStore(), which doesn't give us an
// alternative.

func newBaseClient(raw *csclient.Client, config ClientConfig, meta JujuMetadata) *baseClient {
	base := &baseClient{
		Client: raw,
	}
	return base
}

// LatestRevisions implements BaseClient.
func (base baseClient) LatestRevisions(cURLs []*charm.URL, channel string) ([]charmrepo.CharmRevision, error) {
	// TODO(ericsnow) Fix this:
	// We must use charmrepo.CharmStore since csclient.Client does not
	// have the "Latest" method.

	// unfortunately, the way the csclient.Client uses channels is pretty
	// awkward for our uses.  It's an unexported field that only gets set by
	// WithChannel, so we have to do this:
	repo := charmrepo.NewCharmStoreFromClient(base.Client.WithChannel(params.Channel(channel)))
	return repo.Latest(cURLs...)
}

// GetResource implements BaseClient by calling csclient.Client's GetResource function.
func (base baseClient) GetResource(cURL *charm.URL, resourceName string, revision int) (charmresource.Resource, io.ReadCloser, error) {
	data, err := base.Client.GetResource(cURL, revision, resourceName)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}

	fp, err := charmresource.ParseFingerprint(data.Hash)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}
	// TODO(natefinch): We don't actually have the full metadata here... how do
	// we fix that?
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name: resourceName,
		},
		Revision:    data.Revision,
		Fingerprint: fp,
		Size:        data.Size,
	}, data.ReadCloser, nil
}

// ListResources implements BaseClient by calling csclient.Client's ListResources function.
func (base baseClient) ListResources(charmURLs []*charm.URL, channel string) ([][]charmresource.Resource, error) {
	client := base.Client.WithChannel(params.Channel(channel))

	resmap, err := client.ListResources(charmURLs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make([][]charmresource.Resource, len(charmURLs))
	for i, id := range charmURLs {
		resources, ok := resmap[id.String()]
		if !ok {
			continue
		}
		list := make([]charmresource.Resource, len(resources))
		for j, res := range resources {
			resource, err := apiResource2Resource(res)
			if err != nil {
				return nil, errors.Annotatef(err, "got bad data from server for resource %q", res.Name)
			}
			list[j] = resource
		}
		result[i] = list
	}
	return result, nil

}

// ClientConfig holds the configuration of a charm store client.
type ClientConfig struct {
	charmrepo.NewCharmStoreParams
}

func (config ClientConfig) newCSClient() *csclient.Client {
	return csclient.New(csclient.Params{
		URL:          config.URL,
		HTTPClient:   config.HTTPClient,
		VisitWebPage: config.VisitWebPage,
	})
}

func (config ClientConfig) newCSRepo() *charmrepo.CharmStore {
	return charmrepo.NewCharmStore(config.NewCharmStoreParams)
}

// TODO(ericsnow) Factor out a metadataClient type that embeds "client",
// and move the "meta" field there?

// Client adapts csclient.Client to the needs of Juju.
type Client struct {
	BaseClient
	io.Closer

	newCopy func() *Client
	meta    JujuMetadata
}

// NewClient returns a Juju charm store client for the given client
// config.
func NewClient(config ClientConfig) *Client {
	base := config.newCSClient()
	closer := ioutil.NopCloser(nil)
	var meta JujuMetadata
	return newClient(base, config, meta, closer)
}

func newClient(base *csclient.Client, config ClientConfig, meta JujuMetadata, closer io.Closer) *Client {
	c := &Client{
		BaseClient: newBaseClient(base, config, meta),
		Closer:     closer,
		meta:       meta,
	}
	c.newCopy = func() *Client {
		newBase := *base // a copy
		copied := newClient(&newBase, config, c.meta, closer)
		return copied
	}
	return c
}

// NewDefaultClient returns a Juju charm store client using a default
// client config.
func NewDefaultClient() *Client {
	return NewClient(ClientConfig{})
}

// WithMetadata returns a copy of the the client that will use the
// provided metadata during client requests.
func (c Client) WithMetadata(meta JujuMetadata) (*Client, error) {
	newClient := c.newCopy()
	newClient.meta = meta
	// Note that we don't call meta.setOnClient() at this point.
	// That is because not all charm store requests should include
	// the metadata. The following do so:
	//  - LatestRevisions()
	//
	// If that changed then we would call meta.setOnClient() here.
	// TODO(ericsnow) Use the metadata for *all* requests?
	return newClient, nil
}

// Metadata returns a copy of the Juju metadata set on the client.
func (c Client) Metadata() JujuMetadata {
	// Note the value receiver, meaning that the returned metadata
	// is a copy.
	return c.meta
}

// LatestRevisions returns the latest revision for each of the
// identified charms. The revisions in the provided URLs are ignored.
// Note that this differs from BaseClient.LatestRevisions() exclusively
// due to taking into account Juju metadata (if any).
func (c *Client) LatestRevisions(cURLs []*charm.URL, channel string) ([]charmrepo.CharmRevision, error) {
	if !c.meta.IsZero() {
		c = c.newCopy()
		if err := c.meta.setOnClient(c.BaseClient); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return c.BaseClient.LatestRevisions(cURLs, channel)
}

func apiResource2Resource(res params.Resource) (charmresource.Resource, error) {
	var result charmresource.Resource
	resType, err := apiResourceType2ResourceType(res.Type)
	if err != nil {
		return result, errors.Trace(err)
	}
	origin, err := apiOrigin2Origin(res.Origin)
	if err != nil {
		return result, errors.Trace(err)
	}
	fp, err := charmresource.NewFingerprint(res.Fingerprint)
	if err != nil {
		return result, errors.Trace(err)
	}
	return charmresource.Resource{
		Meta: charmresource.Meta{
			Name:        res.Name,
			Type:        resType,
			Path:        res.Path,
			Description: res.Description,
		},
		Origin:      origin,
		Revision:    res.Revision,
		Fingerprint: fp,
		Size:        res.Size,
	}, nil
}

func apiResourceType2ResourceType(t string) (charmresource.Type, error) {
	switch t {
	case "file":
		return charmresource.TypeFile, nil
	default:
		return 0, errors.Errorf("unknown resource type: %v", t)
	}
}

func apiOrigin2Origin(origin string) (charmresource.Origin, error) {
	switch origin {
	case "store":
		return charmresource.OriginStore, nil
	case "ulpoad":
		return charmresource.OriginUpload, nil
	default:
		return 0, errors.Errorf("unknown origin: %v", origin)
	}
}
