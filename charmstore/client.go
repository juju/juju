// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

// TODO(natefinch): Ideally, this whole package would live in the
// charmstore-client repo, so as to keep it near the API it wraps (and make it
// more available to tools outside juju-core).

import (
	"io"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var logger = loggo.GetLogger("juju.charmstore")

// ClientConfig holds the configuration of a charm store client.
type ClientConfig struct {
	URL          *url.URL
	BakeryClient *httpbakery.Client
}

// NewClient returns a Juju charm store client for the given client
// config.
func NewClient(config ClientConfig) Client {
	var url string
	if config.URL != nil {
		url = config.URL.String()
	}
	cs := csclient.New(csclient.Params{
		URL:          url,
		BakeryClient: config.BakeryClient,
	})
	return Client{lowLevel: csclientImpl{cs}}
}

// Client wraps charmrepo/csclient (the charm store's API client
// library) in a higher level API.
type Client struct {
	lowLevel csWrapper
}

// LatestRevisions returns the latest revisions of the given charms, using the given metadata.
func (c Client) LatestRevisions(charms []CharmID, metadata map[string]string) ([]charmrepo.CharmRevision, error) {
	// The csclient.Client has channel as an unexported field that only gets set
	// by WithChannel on the client (returning a new client), so we have to
	// make one bulk request per channel.

	// collate charms into map[channel]charmRequest
	requests := collate(charms)

	results := make([]charmrepo.CharmRevision, len(charms))

	for channel, request := range requests {
		revisions, err := c.lowLevel.Latest(channel, request.ids, metadata)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for i, rev := range revisions {
			idx := request.indices[i]
			results[idx] = rev
		}
	}
	return results, nil
}

// GetResource returns the data (bytes) and metadata for a resource from the charmstore.
func (c Client) GetResource(cURL *charm.URL, resourceName string, revision int) (res charmresource.Resource, rc io.ReadCloser, err error) {
	defer func() {
		if err != nil && rc != nil {
			rc.Close()
		}
	}()
	meta, err := c.lowLevel.ResourceInfo(cURL, resourceName, revision)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}
	res, err = params.API2Resource(meta)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}

	data, err := c.lowLevel.GetResource(cURL, resourceName, revision)
	if err != nil {
		return charmresource.Resource{}, nil, errors.Trace(err)
	}
	fpHash := res.Fingerprint.String()
	if data.Hash != fpHash {
		return charmresource.Resource{}, nil,
			errors.Errorf("fingerprint for data (%s) does not match fingerprint in metadata (%s)", data.Hash, fpHash)
	}
	if data.Size != res.Size {
		return charmresource.Resource{}, nil,
			errors.Errorf("size for data (%d) does not match size in metadata (%d)", data.Size, res.Size)
	}

	return res, data.ReadCloser, nil
}

// ResourceInfo returns the metadata info for the given resource from the charmstore.
func (c Client) ResourceInfo(cURL *charm.URL, resourceName string, revision int) (charmresource.Resource, error) {
	meta, err := c.lowLevel.ResourceInfo(cURL, resourceName, revision)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	res, err := params.API2Resource(meta)
	if err != nil {
		return charmresource.Resource{}, errors.Trace(err)
	}
	return res, nil
}

// ListResources implements BaseClient by calling csclient.Client's ListResources function.
func (c Client) ListResources(charms []CharmID) ([][]charmresource.Resource, error) {
	requests := collate(charms)
	result := make([][]charmresource.Resource, len(charms))
	for channel, request := range requests {
		// we have to make one bulk call per channel, unfortunately
		resmap, err := c.lowLevel.ListResources(channel, request.ids)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for i, id := range request.ids {
			resources, ok := resmap[id.String()]
			if !ok {
				continue
			}
			list := make([]charmresource.Resource, len(resources))
			for j, res := range resources {
				resource, err := params.API2Resource(res)
				if err != nil {
					return nil, errors.Annotatef(err, "got bad data from server for resource %q", res.Name)
				}
				list[j] = resource
			}
			idx := request.indices[i]
			result[idx] = list
		}
	}
	return result, nil
}

// csWrapper is a type that abstracts away the low-level implementation details
// of the charmstore client.
type csWrapper interface {
	Latest(channel charm.Channel, ids []*charm.URL, headers map[string]string) ([]charmrepo.CharmRevision, error)
	ListResources(channel charm.Channel, ids []*charm.URL) (map[string][]params.Resource, error)
	GetResource(id *charm.URL, name string, revision int) (csclient.ResourceData, error)
	ResourceInfo(id *charm.URL, name string, revision int) (params.Resource, error)
}

// csclientImpl is an implementation of csWrapper that uses the charmstore client.
// It exists for testing purposes to hide away the hard-to-mock parts of
// csclient.Client.
type csclientImpl struct {
	client *csclient.Client
}

// Latest gets the latest CharmRevisions for the charm URLs on the channel.
func (c csclientImpl) Latest(channel charm.Channel, ids []*charm.URL, metadata map[string]string) ([]charmrepo.CharmRevision, error) {
	repo := charmrepo.NewCharmStoreFromClient(c.client.WithChannel(params.Channel(channel)))
	repo = repo.WithJujuAttrs(metadata)
	return repo.Latest(ids...)
}

// Latest gets the latest resources for the charm URLs on the channel.
func (c csclientImpl) ListResources(channel charm.Channel, ids []*charm.URL) (map[string][]params.Resource, error) {
	client := c.client.WithChannel(params.Channel(channel))
	return client.ListResources(ids)
}

// Getresource downloads the bytes and some metadata about the bytes for the revisioned resource.
func (c csclientImpl) GetResource(id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	return c.client.GetResource(id, name, revision)
}

// ResourceInfo gets the full metadata for the revisioned resource.
func (c csclientImpl) ResourceInfo(id *charm.URL, name string, revision int) (params.Resource, error) {
	return c.client.ResourceMeta(id, name, revision)
}

// charmRequest correlates charm URLs with an index in a separate slice.  The two
// slices have a 1:1 correlation via their index.
type charmRequest struct {
	ids     []*charm.URL
	indices []int
}

// collate returns a collection of requests grouped by channel.  The request holds a
// slice of charms for that channel, and a corresponding slice of the index of
// each charm in the orignal slice from which they were collated, so the results
// can be sorted into the original order.
func collate(charms []CharmID) map[charm.Channel]charmRequest {
	results := map[charm.Channel]charmRequest{}
	for i, c := range charms {
		request := results[c.Channel]
		request.ids = append(request.ids, c.URL)
		request.indices = append(request.indices, i)
		results[c.Channel] = request
	}
	return results
}
