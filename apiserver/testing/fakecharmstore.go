// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/macaroon.v1"

	// jc "github.com/juju/testing/checkers"
	"github.com/juju/errors"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/testcharms"
	"github.com/juju/testing"
)

// noopProgress implements csclient.Progress
type noopProgress struct{}

func (noopProgress) Start(uploadId string, expires time.Time) {}

func (noopProgress) Transferred(total int64) {}

func (noopProgress) Error(err error) {}

func (noopProgress) Finalizing() {}

var _ csclient.Progress = (*noopProgress)(nil)

// Datastore is a small in-memory store that can be used
// for stubbing out HTTP calls.
type Datastore struct {
	data map[string]interface{}
}

// NewDatastore returns an empty Datastore
func NewDatastore() *Datastore {
	return &Datastore{make(map[string]interface{})}
}

// Get retrieves data at path. If nothing exists at path,
// an error satisfying errors.IsNotFound is returned.
func (d *Datastore) Get(path string, data interface{}) error {
	current := d.data[path]
	if current == nil {
		return errors.NotFoundf(path)
	}
	data = current
	return nil
}

// Put stores data at path. It will be accessible later via Get.
// Data already at path will is overwritten and no
// revision history is saved.
func (d *Datastore) Put(path string, data interface{}) error {
	d.data[path] = data
	return nil
}

// PutReader data from a an io.Reader at path. It will be accessible later via Get.
// Data already at path will is overwritten and no
// revision history is saved.
func (d *Datastore) PutReader(path string, data io.Reader) error {
	buffer := []byte{}
	_, err := data.Read(buffer)
	if err != nil {
		return errors.Trace(err)
	}
	return d.Put(path, buffer)
}

// Client is a stand-in charmstore.Client
type Client struct {
	repo Repository
}

var _ charmstore.CharmstoreWrapper = (*Client)(nil)
var _ testcharms.Charmstore = (*Client)(nil)

func NewClient(repo Repository) *Client {
	return &Client{repo}
}

// Get retrieves data from path
func (c Client) Get(path string, value interface{}) error {
	return c.repo.resourcesData.Get(path, value)
}

// Put stores data at path
func (c Client) Put(path string, value interface{}) error {
	return c.repo.resourcesData.Put(path, value)
}

func (c Client) WithChannel(channel params.Channel) testcharms.ChannelAwareCharmstore {
	newClient := NewClient(c.repo.WithChannel(channel))
	return &ChannelAwareClient{channel, *newClient}
}

func (c Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return nil
}

func (c Client) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

// Latest returns the latest revision
func (c Client) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) (revisions []params.CharmRevision, err error) {
	originalChannel := c.repo.channel
	defer func() { c.repo.channel = originalChannel }()
	return c.repo.WithChannel(channel).Latest(ids, headers)
}

func (c Client) ListResources(channel params.Channel, id *charm.URL) ([]params.Resource, error) {
	originalChannel := c.repo.channel
	defer func() { c.repo.channel = originalChannel }()
	return c.repo.WithChannel(channel).ListResources(id)
}

func (c Client) GetResource(channel params.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	originalChannel := c.repo.channel
	defer func() { c.repo.channel = originalChannel }()
	return c.repo.WithChannel(channel).GetResource(id, name, revision)
}

func (c Client) ResourceMeta(channel params.Channel, id *charm.URL, name string, revision int) (params.Resource, error) {
	originalChannel := c.repo.channel
	defer func() { c.repo.channel = originalChannel }()
	return c.repo.WithChannel(channel).ResourceMeta(id, name, revision)
}

// ServerURL returns the empty string, as it is not accessing an actual charm store.
//
// ServerURL is part of the charmstore.CharmstoreWrapper interface
func (c Client) ServerURL() string {
	return ""
}

func (c Client) UploadCharm(id *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	return c.repo.UploadCharm(id, charmData)
}

func (c Client) UploadCharmWithRevision(id *charm.URL, charmData charm.Charm, promulgatedRevision int) error {
	return c.repo.UploadCharmWithRevision(id, charmData, promulgatedRevision)
}

func (c Client) UploadBundle(id *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	return c.repo.UploadBundle(id, bundleData)
}

func (c Client) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	return c.repo.UploadBundleWithRevision(id, bundleData, promulgatedRevision)
}

func (c Client) UploadResource(id *charm.URL, name, path string, file io.ReadSeeker, size int64, progress csclient.Progress) (revision int, err error) {
	return c.repo.UploadResource(id, name, path, file, size, progress)
}

type ChannelAwareClient struct {
	channel    params.Channel
	charmstore Client
}

var _ testcharms.ChannelAwareCharmstore = (*ChannelAwareClient)(nil)

func (c ChannelAwareClient) Latest(ids []*charm.URL, headers map[string][]string) (revisions []params.CharmRevision, err error) {
	return c.charmstore.Latest(c.channel, ids, headers)
}

func (c ChannelAwareClient) ListResources(id *charm.URL) ([]params.Resource, error) {
	return c.charmstore.ListResources(c.channel, id)
}

func (c ChannelAwareClient) GetResource(id *charm.URL, name string, revision int) (csclient.ResourceData, error) {
	return c.charmstore.GetResource(c.channel, id, name, revision)
}

func (c ChannelAwareClient) ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error) {
	return c.charmstore.ResourceMeta(c.channel, id, name, revision)
}

func (c ChannelAwareClient) ServerURL() string {
	return c.charmstore.ServerURL()
}

func (c ChannelAwareClient) Get(path string, value interface{}) error {
	return c.charmstore.Get(path, value)
}

// Put stores data at path
func (c ChannelAwareClient) Put(path string, value interface{}) error {
	return c.charmstore.Put(path, value)
}

func (c ChannelAwareClient) UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error) {
	return c.charmstore.UploadCharm(id, ch)
}

func (c ChannelAwareClient) UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error {
	return c.charmstore.UploadCharmWithRevision(id, ch, promulgatedRevision)
}

func (c ChannelAwareClient) UploadBundle(id *charm.URL, bundle charm.Bundle) (*charm.URL, error) {
	return c.charmstore.UploadBundle(id, bundle)
}

func (c ChannelAwareClient) UploadBundleWithRevision(id *charm.URL, bundle charm.Bundle, promulgatedRevision int) error {
	return c.charmstore.UploadBundleWithRevision(id, bundle, promulgatedRevision)
}

func (c ChannelAwareClient) UploadResource(id *charm.URL, name, path string, file io.ReadSeeker, size int64, progress csclient.Progress) (revision int, err error) {
	return c.charmstore.UploadResource(id, name, path, file, size, progress)
}

func (c ChannelAwareClient) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return c.charmstore.Publish(id, channels, resources)
}

func (c ChannelAwareClient) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return c.charmstore.AddDockerResource(id, resourceName, imageName, digest)
}

// Repository is a stand-in charmrepo.
type Repository struct {
	channel       params.Channel
	charms        map[params.Channel]map[charm.URL]charm.Charm
	bundles       map[params.Channel]map[charm.URL]charm.Bundle
	resources     map[params.Channel]map[charm.URL][]params.Resource
	revisions     map[params.Channel]map[charm.URL]int
	added         map[string][]charm.URL
	resourcesData Datastore
}

var _ charmrepo.Interface = (*Repository)(nil)

func NewRepository() Repository {
	repo := Repository{
		channel:   params.NoChannel,
		charms:    make(map[params.Channel]map[charm.URL]charm.Charm),
		bundles:   make(map[params.Channel]map[charm.URL]charm.Bundle),
		resources: make(map[params.Channel]map[charm.URL][]params.Resource),
		revisions: make(map[params.Channel]map[charm.URL]int),
		added:     make(map[string][]charm.URL),
		// commonInfo:    make(map[charm.URL]map[string]interface{}),
		// extraInfo:     make(map[charm.URL]map[string]interface{}),
		resourcesData: *NewDatastore(),
	}
	for _, channel := range params.OrderedChannels {
		repo.charms[channel] = make(map[charm.URL]charm.Charm)
		repo.bundles[channel] = make(map[charm.URL]charm.Bundle)
		repo.resources[channel] = make(map[charm.URL][]params.Resource)
		repo.revisions[channel] = make(map[charm.URL]int)
	}
	return repo
}

func (r *Repository) addRevision(ref *charm.URL) *charm.URL {
	revision := r.revisions[r.channel][*ref]
	return ref.WithRevision(revision)
}

// Resolve disambiguates a charm to a specific revision.
//
// Part of the charmrepo.Interface
func (r Repository) Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error) {
	return r.addRevision(ref), []string{"trusty", "wily"}, nil
}

func (r Repository) ResolveWithChannel(ref *charm.URL) (*charm.URL, params.Channel, []string, error) {
	canonRef, supportedSeries, err := r.Resolve(ref)
	return canonRef, r.channel, supportedSeries, err
}

func (r Repository) AddCharm(id *charm.URL, channel params.Channel, force bool) error {
	alreadyAdded := r.added[string(channel)]

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

	r.added[string(channel)] = append(alreadyAdded, *id)
	return nil
}

func (r Repository) AddCharmWithAuthorization(id *charm.URL, channel params.Channel, macaroon *macaroon.Macaroon, force bool) error {
	return r.AddCharm(id, channel, force)
}

// Get retrieves a charm from the repository.
//
// Part of the charmrepo.Interface
func (r Repository) Get(id *charm.URL) (charm.Charm, error) {
	withRevision := r.addRevision(id)
	charmData := r.charms[r.channel][*withRevision]
	if charmData == nil {
		return charmData, NotFoundError(fmt.Sprintf("cannot retrieve \"%v\": charm", id.String()))
	}
	return charmData, nil
}

// GetBundle retrieves a bundle from the repository.
//
// Part of the charmrepo.Interface
func (r Repository) GetBundle(id *charm.URL) (charm.Bundle, error) {
	bundleData := r.bundles[r.channel][*id]
	if bundleData == nil {
		return bundleData, NotFoundError(id.String())
	}
	return bundleData, nil
}

func (r Repository) UploadCharm(id *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	if len(r.charms[r.channel]) == 0 {
		r.charms[r.channel] = make(map[charm.URL]charm.Charm)
	}
	withRevision := r.addRevision(id)
	r.charms[r.channel][*withRevision] = charmData
	return withRevision, nil
}

func (r Repository) UploadCharmWithRevision(id *charm.URL, charmData charm.Charm, promulgatedRevision int) error {
	if len(r.revisions[r.channel]) == 0 {
		r.revisions[r.channel] = make(map[charm.URL]int)
	}
	r.revisions[r.channel][*id] = promulgatedRevision
	_, err := r.UploadCharm(id, charmData)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (r Repository) UploadBundle(id *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	r.bundles[r.channel][*id] = bundleData
	return id, nil
}

func (r Repository) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	_, err := r.UploadBundle(id, bundleData)
	if err != nil {
		return errors.Trace(err)
	}
	r.revisions[r.channel][*id] = promulgatedRevision
	return nil
}

func (r Repository) GetResource(id *charm.URL, name string, revision int) (result csclient.ResourceData, err error) {
	_, err = r.ResourceMeta(id, name, revision)
	if err != nil {
		return csclient.ResourceData{}, errors.Trace(err)
	}

	buffer := bytes.NewBuffer([]byte{})
	err = r.resourcesData.Get(id.String(), buffer)
	if err != nil {
		return csclient.ResourceData{}, errors.Trace(err)
	}
	data := ioutil.NopCloser(buffer)

	fingerprint, err := charmresource.GenerateFingerprint(data)
	if err != nil {
		return csclient.ResourceData{}, errors.Trace(err)
	}

	return csclient.ResourceData{data, fingerprint.String()}, nil
}

func (r Repository) ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error) {
	resources, err := r.ListResources(id)
	if err != nil {
		return params.Resource{}, errors.Trace(err)
	}
	for _, resource := range resources {
		if resource.Name == name && resource.Revision == revision {
			return resource, nil
		}
	}
	return params.Resource{}, NotFoundError(fmt.Sprintf("no resources for %v with name=%v and revision=%v", id.String(), name, revision))
}

// ListResources returns Resource metadata that have been generated
// by UploadResource
func (r Repository) ListResources(id *charm.URL) ([]params.Resource, error) {
	return r.resources[r.channel][*id], nil
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

// UploadResource "uploads" data from file and stores it at path
func (r Repository) UploadResource(id *charm.URL, name, path string, file io.ReadSeeker, size int64, progress csclient.Progress) (revision int, err error) {
	if len(r.resources[r.channel]) == 0 {
		r.resources[r.channel] = make(map[charm.URL][]params.Resource)
	}
	resources, err := r.ListResources(id)
	if err != nil {
		return -1, errors.Trace(err)
	}
	revision = len(resources)
	if len(resources) == 0 {
		r.resources[r.channel][*id] = make([]params.Resource, 1)
	}
	//progress.Start() // ignoring progress for now, hoping that it's not material to the tests
	hash, err := signature(file)
	if err != nil {
		// progress.Error(err)
		return -1, errors.Trace(err)
	}
	r.resources[r.channel][*id] = append(resources, params.Resource{
		Name:        name,
		Path:        path,
		Revision:    revision,
		Size:        size,
		Fingerprint: hash,
	})
	// progress.Transferred() // it looks like this method is never used by charmrepo anyway?
	err = r.resourcesData.PutReader(path, file)
	if err != nil {
		return -1, errors.Trace(err)
	}
	// progress.Finalizing()
	return revision, nil
}

// func (r Repository) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
// 	return -1, nil
// }

// func (r Repository) DockerResourceUploadInfo(id *charm.URL, resourceName string) (*params.DockerInfoResponse, error) {
// 	return &params.DockerInfoResponse{}, nil
// }

// func (r Repository) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
// 	return nil
// }

func (r Repository) WithChannel(channel params.Channel) Repository {
	r.channel = channel
	return r
}

func (r Repository) Latest(ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	result := make([]params.CharmRevision, len(ids))
	haveRevisions := len(r.revisions[r.channel]) > 0
	for i, id := range ids {
		if haveRevisions {
			revision := r.revisions[r.channel][*id]
			result[i] = params.CharmRevision{Revision: revision}
		} else {
			result[i] = params.CharmRevision{Err: NotFoundError(id.String())}
		}
	}
	return result, nil
}

// // Put puts data into a location for later retrieval.
// func (r Repository) Put(path string, data interface{}) error {
// 	return nil
// }

type CharmStoreSuite struct {
	testing.CleanupSuite
	charmstore testcharms.Charmstore
	charmrepo  charmrepo.Interface
}

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	repo := NewRepository()
	s.CleanupSuite.SetUpTest(c)
	s.charmrepo = repo
	s.charmstore = NewClient(repo)
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
}

func (s *CharmStoreSuite) UploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharm(c, s.charmstore, url, name)
}

func (s *CharmStoreSuite) UploadCharmMultiSeries(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharmMultiSeries(c, s.charmstore, url, name)
}
