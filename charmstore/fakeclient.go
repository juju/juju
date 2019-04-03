// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// fakeclient provides two charmstore client sthat do not use any HTTP connections
// or authentication mechanisms. Why two? Each reflects one of two informally-defined
// styles for interacting with the charmstore.
//
//  - FakeClient has methods that include a channel parameter
//  - ChannelAwareFakeClient maintains a record of the channel that it is currently talking about
//
// fakeclient also includes a Repository type. The Repository preforms the role of a substitute charmrepo.
// More technically, it implements the gopkg.in/juju/charmrepo Interface.
//
// Each of these three types are interrelated:
//
//  Repository     -->       FakeClient          -->       ChannelAwareFakeClient
//   |         [extended by]     |           [extended by]    |
//   \                           \                            \
//    provides storage        provides front-end        provides alternative
//    for charms,                                       front end
//    bundles,
//    resources

package charmstore

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
)

// datastore is a small, in-memory key/value store. Its primary use case is to
// fake HTTP calls.
//
// Note
//
// datastore's methods support a path argument that represents a URL path. However,
// no normalisation is performed on this parameter. Therefore,
// two strings that refer to the same canonical URL will not match within datastore.
type datastore map[string]interface{}

// Get retrieves contents stored at path and saves it to data. If nothing exists at path,
// an error satisfying errors.IsNotFound is returned.
func (d datastore) Get(path string, data interface{}) error {
	current := d[path]
	if current == nil {
		return errors.NotFoundf(path)
	}
	data = current
	return nil
}

// Put stores data at path. It will be accessible later via Get.
// Data already at path will is overwritten and no
// revision history is saved.
func (d datastore) Put(path string, data interface{}) error {
	d[path] = data
	return nil
}

// PutReader data from a an io.Reader at path. It will be accessible later via Get.
// Data already at path will is overwritten and no
// revision history is saved.
func (d *datastore) PutReader(path string, data io.Reader) error {
	buffer := []byte{}
	_, err := data.Read(buffer)
	if err != nil {
		return errors.Trace(err)
	}
	return d.Put(path, buffer)
}

// FakeClient is a stand-in for the gopkg.in/juju/charmrepo.v3/csclient Client type.
// Its "stores" data within several an in-memory map for each object that charmstores know about, primarily charms, bundles and resources.
//
// An abridged session would look something like this, where charmId is a *charm.URL:
//  // initialise a new charmstore with an empty repository
//  repo := NewRepository()
//  client := NewFakeClient(repo)
//  client.UploadCharm(charmId)
//  // later on
//  charm := client.Get(charmId)
type FakeClient struct {
	repo *Repository
}

//var _ csWrapper = (*FakeClient)(nil)

// NewFakeClient returns a FakeClient that is initialised
// with repo.
func NewFakeClient(repo *Repository) *FakeClient {
	if repo == nil {
		repo = NewRepository()
	}
	return &FakeClient{repo}
}

// Get retrieves data from path
func (c FakeClient) Get(path string, value interface{}) error {
	return c.repo.resourcesData.Get(path, value)
}

// Put uploads data to path, overwriting any data that is already present
func (c FakeClient) Put(path string, value interface{}) error {
	return c.repo.resourcesData.Put(path, value)
}

// AddDockerResource adds a docker resource to the repository
func (c FakeClient) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

func (c FakeClient) UploadCharm(id *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	return c.repo.UploadCharm(id, charmData)
}

func (c FakeClient) UploadCharmWithRevision(id *charm.URL, charmData charm.Charm, promulgatedRevision int) error {
	return c.repo.UploadCharmWithRevision(id, charmData, promulgatedRevision)
}

func (c FakeClient) UploadBundle(id *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	return c.repo.UploadBundle(id, bundleData)
}

func (c FakeClient) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	return c.repo.UploadBundleWithRevision(id, bundleData, promulgatedRevision)
}

func (c FakeClient) UploadResource(id *charm.URL, name, path string, file io.ReaderAt, size int64, progress csclient.Progress) (revision int, err error) {
	return c.repo.UploadResource(id, name, path, file, size, progress)
}

// ListResources returns Resource metadata for resources that have been
// uploaded to the repository for id. To upload a resource, use UploadResource.
// Although id is type *charm.URL, resources are not restricted to charms. That
// type is also used for other entities in the charmstore, such as bundles.
//
// Returns an error that satisfies errors.IsNotFound when no resources
// are present for id.
func (c FakeClient) ListResources(channel params.Channel, id *charm.URL) ([]params.Resource, error) {
	originalChannel := c.repo.channel
	defer func() { c.repo.channel = originalChannel }()
	c.repo.channel = channel
	return c.repo.ListResources(id)
}

func (c FakeClient) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return c.repo.Publish(id, channels, resources)
}

func (c FakeClient) WithChannel(channel params.Channel) *ChannelAwareFakeClient {
	return &ChannelAwareFakeClient{channel, c}
}

// ChannelAwareFakeClient is a charmstore client that stores the channel that its methods
// refer to across calls. That is, it is stateful. It is modelled on the Client type defined in
// gopkg.in/juju/charmrepo.v3/csclient.
//
// Constructing ChannelAwareFakeClient
//
// ChannelAwareFakeClient does not have a NewChannelAwareFakeClient method. To construct an
// instance, use the following pattern:
//  NewFakeClient(nil).WithChannel(channel)
//
// Setting the channel
//
// To set ChannelAwareFakeClient's channel, its the WithChannel method.
type ChannelAwareFakeClient struct {
	channel    params.Channel
	charmstore FakeClient
}

func (c ChannelAwareFakeClient) Get(path string, value interface{}) error {
	return c.charmstore.Get(path, value)
}

// Put uploads data to path, overwriting any data that is already present
func (c ChannelAwareFakeClient) Put(path string, value interface{}) error {
	return c.charmstore.Put(path, value)
}

func (c ChannelAwareFakeClient) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return c.charmstore.AddDockerResource(id, resourceName, imageName, digest)
}

func (c ChannelAwareFakeClient) UploadCharm(id *charm.URL, ch charm.Charm) (*charm.URL, error) {
	return c.charmstore.UploadCharm(id, ch)
}

func (c ChannelAwareFakeClient) UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error {
	return c.charmstore.UploadCharmWithRevision(id, ch, promulgatedRevision)
}

func (c ChannelAwareFakeClient) UploadBundle(id *charm.URL, bundle charm.Bundle) (*charm.URL, error) {
	return c.charmstore.UploadBundle(id, bundle)
}

func (c ChannelAwareFakeClient) UploadBundleWithRevision(id *charm.URL, bundle charm.Bundle, promulgatedRevision int) error {
	return c.charmstore.UploadBundleWithRevision(id, bundle, promulgatedRevision)
}

func (c ChannelAwareFakeClient) UploadResource(id *charm.URL, name, path string, file io.ReaderAt, size int64, progress csclient.Progress) (revision int, err error) {
	return c.charmstore.UploadResource(id, name, path, file, size, progress)
}

// ListResources returns Resource metadata for resources that have been
// uploaded to the repository for id. To upload a resource, use UploadResource.
// Although id is type *charm.URL, resources are not restricted to charms. That
// type is also used for other entities in the charmstore, such as bundles.
//
// Returns an error that satisfies errors.IsNotFound when no resources
// are present for id.
func (c ChannelAwareFakeClient) ListResources(id *charm.URL) ([]params.Resource, error) {
	return c.charmstore.ListResources(c.channel, id)
}

func (c ChannelAwareFakeClient) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return c.charmstore.Publish(id, channels, resources)
}

func (c ChannelAwareFakeClient) WithChannel(channel params.Channel) *ChannelAwareFakeClient {
	c.channel = channel
	return &c
}

// Repository provides in-memory access to charms and other objects
// held in a charmstore (or locally), such as bundles and resources.
// Its intended use case is to act as a fake charmrepo for testing purposes.
//
// Warnings
//
// No guarantees are made that Repository maintains its invariants or that the behaviour
// matches the behaviour of the actual charm store. For example, Repository's information
// about which charm revisions it knows about is decoupled from the charm data that it currently
// stores.
//
// Related Interfaces
//
// Repository implements gopkg.in/juju/charmrepo Interface and derivative
// interfaces, such as github.com/juju/juju/cmd/juju/application DeployAPI.
type Repository struct {
	channel       params.Channel
	charms        map[params.Channel]map[charm.URL]charm.Charm
	bundles       map[params.Channel]map[charm.URL]charm.Bundle
	resources     map[params.Channel]map[charm.URL][]params.Resource
	revisions     map[params.Channel]map[charm.URL]int
	added         map[string][]charm.URL
	resourcesData datastore
	published     map[params.Channel]set.Strings
	generations   map[model.GenerationVersion]string
}

// NewRepository returns an empty Repository. To populate it with charms, bundles and resources
// use UploadCharm, UploadBundle and/or UploadResource.
func NewRepository() *Repository {
	repo := Repository{
		channel:       params.StableChannel,
		charms:        make(map[params.Channel]map[charm.URL]charm.Charm),
		bundles:       make(map[params.Channel]map[charm.URL]charm.Bundle),
		resources:     make(map[params.Channel]map[charm.URL][]params.Resource),
		revisions:     make(map[params.Channel]map[charm.URL]int),
		added:         make(map[string][]charm.URL),
		published:     make(map[params.Channel]set.Strings),
		resourcesData: make(datastore),
	}
	for _, channel := range params.OrderedChannels {
		repo.charms[channel] = make(map[charm.URL]charm.Charm)
		repo.bundles[channel] = make(map[charm.URL]charm.Bundle)
		repo.resources[channel] = make(map[charm.URL][]params.Resource)
		repo.revisions[channel] = make(map[charm.URL]int)
		repo.published[channel] = set.NewStrings()
	}
	return &repo
}

func (r *Repository) addRevision(ref *charm.URL) *charm.URL {
	revision := r.revisions[r.channel][*ref]
	return ref.WithRevision(revision)
}

// Resolve disambiguates a charm to a specific revision.
//
// Part of the charmrepo.Interface
func (r Repository) Resolve(ref *charm.URL) (canonRef *charm.URL, supportedSeries []string, err error) {
	return r.addRevision(ref), []string{"trusty", "wily", "quantal"}, nil
}

// ResolveWithChannel disambiguates a charm to a specific revision.
//
// Part of the cmd/juju/application.DeployAPI interface
func (r Repository) ResolveWithChannel(ref *charm.URL) (*charm.URL, params.Channel, []string, error) {
	canonRef, supportedSeries, err := r.Resolve(ref)
	return canonRef, r.channel, supportedSeries, err
}

// Get retrieves a charm from the repository.
//
// Part of the charmrepo.Interface
func (r Repository) Get(id *charm.URL) (charm.Charm, error) {
	withRevision := r.addRevision(id)
	charmData := r.charms[r.channel][*withRevision]
	if charmData == nil {
		return charmData, errors.NotFoundf("cannot retrieve \"%v\": charm", id.String())
	}
	return charmData, nil
}

// GetBundle retrieves a bundle from the repository.
//
// Part of the charmrepo.Interface
func (r Repository) GetBundle(id *charm.URL) (charm.Bundle, error) {
	bundleData := r.bundles[r.channel][*id]
	if bundleData == nil {
		return bundleData, errors.NotFoundf(id.String())
	}
	return bundleData, nil
}

// ListResources returns Resource metadata for resources that have been
// uploaded to the repository for id. To upload a resource, use UploadResource.
// Although id is type *charm.URL, resources are not restricted to charms. That
// type is also used for other entities in the charmstore, such as bundles.
//
// Returns an error that satisfies errors.IsNotFound when no resources
// are present for id.
func (r Repository) ListResources(id *charm.URL) ([]params.Resource, error) {
	resources := r.resources[r.channel][*id]
	if len(resources) == 0 {
		return resources, errors.NotFoundf("no resources for %v", id)
	}
	return resources, nil
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

// UploadResource "uploads" data from file and stores it at path
func (r Repository) UploadResource(id *charm.URL, name, path string, file io.ReaderAt, size int64, progress csclient.Progress) (revision int, err error) {
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
	data := []byte{}
	_, err = file.ReadAt(data, 0)
	if err != nil {
		return -1, errors.Trace(err)
	}

	hash, err := signature(bytes.NewBuffer(data))
	if err != nil {
		return -1, errors.Trace(err)
	}
	r.resources[r.channel][*id] = append(resources, params.Resource{
		Name:        name,
		Path:        path,
		Revision:    revision,
		Size:        size,
		Fingerprint: hash,
	})

	err = r.resourcesData.Put(path, data)
	if err != nil {
		return -1, errors.Trace(err)
	}
	return revision, nil
}

// Publish marks a charm or bundle as published within channels.
// In this implementation, the resources parameter is ignored.
func (r Repository) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	for _, channel := range channels {
		published := r.published[channel]
		published.Add(id.String())
		r.published[channel] = published
	}
	return nil
}

// signature creates a SHA384 digest from r
func signature(r io.Reader) (hash []byte, err error) {
	h := sha512.New384()
	_, err = io.Copy(h, r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	hash = []byte(fmt.Sprintf("%x", h.Sum(nil)))
	return hash, nil
}
