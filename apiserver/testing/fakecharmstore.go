// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/sha512"
	"fmt"
	"io"

	// jc "github.com/juju/testing/checkers"
	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/testcharms"
)

type Client struct {
	charms    map[charm.URL]charm.Charm
	bundles   map[charm.URL]charm.Bundle
	resources map[charm.URL][]params.Resource
	revisions map[charm.URL]int
	added     map[string][]charm.URL
}

var _ charmrepo.Interface = (*Client)(nil)
var _ testcharms.MinimalCharmstoreClient = (*Client)(nil)
var _ testcharms.StatefulCharmstoreClient = (*Client)(nil)
var _ testcharms.CharmAdder = (*Client)(nil)
var _ testcharms.CharmUploader = (*Client)(nil)
var _ testcharms.Repository = (*Client)(nil)

func NewCharmstoreClient() Client {
	return Client{
		charms:    make(map[charm.URL]charm.Charm),
		bundles:   make(map[charm.URL]charm.Bundle),
		resources: make(map[charm.URL][]params.Resource),
		revisions: make(map[charm.URL]int),
		added:     make(map[string][]charm.URL),
	}
}

func (c Client) Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error) {
	return ref.WithRevision(c.revisions[*ref]), []string{"trusty", "wily"}, nil
}

func (c Client) AddCharm(id *charm.URL, channel params.Channel, force bool) error {
	alreadyAdded := c.added[string(channel)]

	for _, charm := range alreadyAdded {
		if *id == charm {
			return nil
			// TODO(tsm) check expected behaviour
			//
			// if force {
			// 	return nil
			// } else {
			// 	return errors.NewAlreadyExists(errors.NewErr("%v already added in channel %v", id, channel))
			// }
		}
	}

	c.added[string(channel)] = append(alreadyAdded, *id)
	return nil
}

func (c Client) AddCharmWithAuthorization(id *charm.URL, channel params.Channel, macaroon *macaroon.Macaroon, force bool) error {
	return c.AddCharm(id, channel, force)
}

func (c Client) Get(id *charm.URL) (charm.Charm, error) {
	withRevision := id.WithRevision(c.revisions[*id])
	charmData := c.charms[*withRevision]
	if charmData == nil {
		return charmData, NotFoundError(fmt.Sprintf("cannot retrieve \"%v\": charm", id.String()))
	}
	return charmData, nil
}

func (c Client) GetBundle(id *charm.URL) (charm.Bundle, error) {
	bundleData := c.bundles[*id]
	if bundleData == nil {
		return bundleData, NotFoundError(id.String())
	}
	return bundleData, nil
}

func (c Client) UploadCharm(id *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	withRevision := id.WithRevision(c.revisions[*id])
	c.charms[*withRevision] = charmData
	return withRevision, nil
}

func (c Client) UploadCharmWithRevision(id *charm.URL, charmData charm.Charm, promulgatedRevision int) error {
	c.revisions[*id] = promulgatedRevision
	_, err := c.UploadCharm(id, charmData)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c Client) UploadBundle(id *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	c.bundles[*id] = bundleData
	return id, nil
}

func (c Client) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	_, err := c.UploadBundle(id, bundleData)
	if err != nil {
		return errors.Trace(err)
	}
	c.revisions[*id] = promulgatedRevision
	return nil
}

func (c Client) GetResource(id *charm.URL, name string, revision int) (result csclient.ResourceData, err error) {
	return csclient.ResourceData{}, nil
}

func (c Client) ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error) {
	resources := c.resources[*id]
	if len(resources) == 0 {
		return params.Resource{}, NotFoundError("unable to find any resources for " + name)
	}
	return resources[len(resources)-1], nil
}

// ListResources returns Resourc metadata that have been generated
// by UploadResource
func (c Client) ListResources(id *charm.URL) ([]params.Resource, error) {
	return c.resources[*id], nil
}

func signature(r io.ReadSeeker) (hash []byte, err error) {
	h := sha512.New384()
	_, err = io.Copy(h, r)
	if err != nil {
		return []byte(""), errors.Trace(err)
	}
	_, err = r.Seek(0, 0)
	if err != nil {
		return []byte(""), errors.Trace(err)
	}
	hash = []byte(fmt.Sprintf("%x", h.Sum(nil)))
	return hash, nil
}

// UploadResource "uploads" data from file and stores a
func (c Client) UploadResource(id *charm.URL, name, path string, file io.ReadSeeker, size int64, progress csclient.Progress) (revision int, err error) {
	resources := c.resources[*id]
	revision = len(resources)
	//progress.Start() // ignoring progress for now, hoping that it's not material to the tests
	hash, err := signature(file)
	if err != nil {
		// progress.Error(err)
		return -1, errors.Trace(err)
	}
	resource := params.Resource{
		Name:        name,
		Path:        path,
		Revision:    revision,
		Size:        size,
		Fingerprint: hash,
	}
	resources = append(resources, resource)
	c.resources[*id] = resources
	// progress.Transferred() // it looks like this method is never used by csclient anyway?
	// progress.Finalizing()
	return revision, nil
}

func (c Client) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

func (c Client) DockerResourceUploadInfo(id *charm.URL, resourceName string) (*params.DockerInfoResponse, error) {
	return &params.DockerInfoResponse{}, nil
}

func (c Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return nil
}

func (c Client) WithChannel(channel params.Channel) testcharms.MinimalCharmstoreClient {
	return &c
}

func (c Client) Latest(ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	result := make([]params.CharmRevision, len(ids))
	for i, id := range ids {
		revision := c.revisions[*id]
		result[i] = params.CharmRevision{Revision: revision}
	}
	return result, nil
	//	return result, c.NextErr()
}

// func (c Client) LatestRevisions(charms []charmstore.CharmID, metadata map[string][]string) ([]charmstore.CharmRevision, error) {
// 	result := make([]charmstore.CharmRevision, len(charms))
// 	for i, cid := range charms {
// 		revisions, err := c.Latest(cid.Channel, []*charm.URL{cid.URL}, make(map[string][]string))
// 		if err != nil {
// 			return nil, errors.Trace(err)
// 		}
// 		rev := revisions[0]
// 		result[i] = charmstore.CharmRevision{Revision: rev.Revision, Err: rev.Err}
// 	}
// 	return result, nil
// }

// Put puts data into a location for later retrieval.
func (c Client) Put(path string, data interface{}) error {
	return nil
}

// func (c *Client) Get(path string, result interface{}) error {
// 	data := c.rawstore[path]
// 	if len(data) == 0 {
// 		return nil
// 	}
// 	result = *data[len(data)]
// 	return nil
// }

// InternalClient implements the github.com/juju/juju/charmstore.CharmstoreWrapper,
// which deals with channels differently
type InternalClient struct {
	base Client
}

var _ charmstore.CharmstoreWrapper = (*InternalClient)(nil)

func (c InternalClient) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	return c.base.Latest(ids, headers)
}

func (c InternalClient) ListResources(channel params.Channel, id *charm.URL) ([]params.Resource, error) {
	return c.base.ListResources(id)
}

func (c InternalClient) GetResource(channel params.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	// resources := c.base.GetResource(id, name, -1)
	return csclient.ResourceData{}, nil
}

func (c InternalClient) ResourceMeta(channel params.Channel, id *charm.URL, name string, revision int) (params.Resource, error) {
	return c.base.ResourceMeta(id, name, -1)
}

func (c InternalClient) ServerURL() string {
	return "#"
}

func InternalClientFromClient(base Client) InternalClient {
	return InternalClient{base}
}

type CharmStoreSuite struct {
	testing.CleanupSuite
	Client testcharms.StatefulCharmstoreClient
}

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.Client = NewCharmstoreClient()
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
}

func (s *CharmStoreSuite) UploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharm(c, s.Client, url, name)
}

func (s *CharmStoreSuite) UploadCharmMultiSeries(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharmMultiSeries(c, s.Client, url, name)
}
