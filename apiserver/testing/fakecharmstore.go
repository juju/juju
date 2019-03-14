// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	// "fmt"

	// "net/http"
	// "net/http/httptest"
	"crypto/sha512"
	"fmt"
	"io"
	"time"

	"github.com/juju/testing"
	// jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	"github.com/juju/errors"

	// "gopkg.in/juju/charmrepo.v3"

	// "gopkg.in/juju/charmstore.v5"
	// "gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	// "gopkg.in/macaroon-bakery.v2-unstable/bakerytest"
	// "gopkg.in/mgo.v2"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/testcharms"
)

// type fakeCharmstoreServer struct {
// 	//pool        *Pool
// 	mux      *router.ServeMux
// 	handlers []fakeCharmstoreHandler
// 	//blobstoreGC *blobstoreGC
// }

// // ServeHTTP implements http.Handler.ServeHTTP.
// func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
// 	s.mux.ServeHTTP(w, req)
// }

// // Close cleans up the server. It is part of the charmstore's server interface.
// func (s *fakeCharmstoreServer) Close() {
// 	for _, h := range s.handlers {
// 		h.Close()
// 	}
// }

// func NewServer() *fakeCharmstoreServer {
// 	server := &fakeCharmstoreServer{mux: router.NewServeMux()}
// 	server.mux.Handle("/debug")
// }

// type fakeCharmstoreHandler interface {
// 	http.Handler
// 	Close()
// }

// type fakeCharmstore struct {
// 	api map[string]fakeCharmstoreHandler
// }

// needed

// UploadCharm - returns a URL
// curl, ch := s.UploadCharm(c, "utopic/storage-block-10", "storage-block")

// s.assertCharmsUploaded
// s.assertApplicationsDeployed
// s.assertCharmsUploaded
// s.assertUnitsCreated

// also mocked here
// /home/tsm/Work/src/github.com/juju/juju/apiserver/facades/client/application/charmstore_test.go

// and here
// /home/tsm/Work/src/github.com/juju/juju/cmd/juju/application/upgradecharm_resources_test.go

// and also defined here
// /home/tsm/Work/src/github.com/juju/juju/resource/resourceadapters/charmstore_test.go

// doesn't appear to do anything
// /home/tsm/Work/src/github.com/juju/juju/cmd/juju/application/upgradecharm_test.go

// try to replace charmstore.UploadCharm with "github.com/juju/juju/testcharms".UploadCharm

// mock charmrepo.v3/csclient.Client
// - 11 methods that we would implement
// - PUT, uploadcharm, uploadbundle, ...

//
// interface
// -- declare the methods that are actually required
//
// modify the applicationsuite
//

// Client
// UploadCharm()
// UploadCharmWithRevision()

// s.resolveCharm = func(
// 	resolveWithChannel func(*charm.URL) (*charm.URL, csclientparams.Channel, []string, error),
// 	url *charm.URL,
// ) (*charm.URL, csclientparams.Channel, []string, error) {
// 	s.AddCall("ResolveCharm", resolveWithChannel, url)
// 	if err := s.NextErr(); err != nil {
// 		return nil, csclientparams.NoChannel, nil, err
// 	}
// 	return s.resolvedCharmURL, csclientparams.StableChannel, []string{"quantal"}, nil
// }

type fakeProgress struct{}

// Start is part of the csclient.Progress interface.
func (p fakeProgress) Start(uploadId string, expires time.Time) {}

// Transferred is part of the csclient.Progress interface.
func (p fakeProgress) Transferred(total int64) {}

// Error is part of the csclient.Progress interface.
func (p fakeProgress) Error(err error) {}

// Finalizing is part of the csclient.Progress interface.
func (p fakeProgress) Finalizing() {}

// type csWrapper interface {
// 	Latest(channel csparams.Channel, ids []*charm.URL, headers map[string][]string) ([]csparams.CharmRevision, error)
// 	ListResources(channel csparams.Channel, id *charm.URL) ([]csparams.Resource, error)
// 	GetResource(channel csparams.Channel, id *charm.URL, name string, revision int) (csclient.ResourceData, error)
// 	ResourceMeta(channel csparams.Channel, id *charm.URL, name string, revision int) (csparams.Resource, error)
// 	ServerURL() string
// }

// func UploadCharmWithMeta(c *gc.C, client *csclient.Client, charmURL, meta, metrics string, revision int) (*charm.URL, charm.Charm) {
// 	ch := testing.NewCharm(c, testing.CharmSpec{
// 		Meta:     meta,
// 		Metrics:  metrics,
// 		Revision: revision,
// 	})
// 	chURL, err := client.UploadCharm(charm.MustParseURL(charmURL), ch)
// 	c.Assert(err, jc.ErrorIsNil)
// 	SetPublic(c, client, chURL)
// 	return chURL, ch
// }

type Client struct {
	//params csclient.Params // might be useful?
	charms    map[charm.URL]charm.Charm
	bundles   map[charm.URL]charm.Bundle
	resources map[charm.URL][]params.Resource
	revisions map[charm.URL]int
	version   string
	url       string

	//	testing.Stub

	//resources map[params.Channel]map[string][]string
}

var _ testcharms.MinimalCharmstoreClient = (*Client)(nil)

func NewCharmstoreClient() Client {
	return Client{
		charms:    make(map[charm.URL]charm.Charm),
		bundles:   make(map[charm.URL]charm.Bundle),
		resources: make(map[charm.URL][]params.Resource),
		revisions: make(map[charm.URL]int),
	}
}

func (c Client) Get(id *charm.URL) (charm.Charm, error) {
	//	c.MethodCall(c, "Get", id)
	charmData := c.charms[*id]
	if charmData == nil {
		return charmData, NotFoundError(id.String()) //? m.NextErr()
	}
	//	return charmData, c.NextErr()
	return charmData, nil
}

func (c Client) GetBundle(id *charm.URL) (charm.Bundle, error) {
	//c.MethodCall(c, "GetBundle", id)
	bundleData := c.bundles[*id]
	if bundleData == nil {
		return bundleData, NotFoundError(id.String()) //? m.NextErr()
	}
	//return bundleData, c.NextErr()
	return bundleData, nil
}

func (c Client) UploadCharm(id *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	//c.MethodCall(c, "UploadCharm", charmId, charmData)
	c.charms[*id] = charmData
	// return id, c.NextErr()
	return id, nil
}

func (c Client) UploadCharmWithRevision(id *charm.URL, charmData charm.Charm, promulgatedRevision int) error {
	// c.MethodCall(c, "UploadCharmWithRevision", id, charmData, promulgatedRevision)
	_, err := c.UploadCharm(id, charmData)
	if err != nil {
		return errors.Trace(err)
	}
	c.revisions[*id] = promulgatedRevision
	//return c.NextErr()
	return nil
}

func (c Client) UploadBundle(id *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	// c.MethodCall(c, "UploadBundle", id, bundleData)
	c.bundles[*id] = bundleData
	//return id, c.NextErr()
	return id, nil
}

func (c Client) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	//c.MethodCall(c, "UploadBundleWithRevision", id, bundleData, promulgatedRevision)
	_, err := c.UploadBundle(id, bundleData)
	if err != nil {
		return errors.Trace(err)
	}
	c.revisions[*id] = promulgatedRevision
	//return c.NextErr()
	return nil
}

func (c Client) GetResource(id *charm.URL, name string, revision int) (result csclient.ResourceData, err error) {
	//c.MethodCall(c, "GetResource", name, revision)
	//return csclient.ResourceData{}, c.NextErr()
	return csclient.ResourceData{}, nil
}

func (c Client) ResourceMeta(id *charm.URL, name string, revision int) (params.Resource, error) {
	//c.MethodCall(c, "ResourceMeta", id, name)
	resources := c.resources[*id]
	if len(resources) == 0 {
		return params.Resource{}, NotFoundError("unable to find any resources for " + name)
	}
	// return resources[len(resources)-1], c.NextErr()
	return resources[len(resources)-1], nil
}

// ListResources returns Resourc metadata that have been generated
// by UploadResource
func (c Client) ListResources(id *charm.URL) ([]params.Resource, error) {
	//	c.MethodCall(c, "ListResources", id)
	//	return c.resources[*id], c.NextErr()
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
	// c.MethodCall(c, "UploadResource", id, name, path, file, size, progress)
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
	// return revision, c.NextErr()
	return revision, nil
}

func (c Client) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	// c.MethodCall(c, "AddDockerResource", id, resourceName, imageName, digest)
	// return -1, c.NextErr()
	return -1, nil
}

func (c Client) DockerResourceUploadInfo(id *charm.URL, resourceName string) (*params.DockerInfoResponse, error) {
	// c.MethodCall(c, "DockerResourceUploadInfo", id, resourceName)
	//return &params.DockerInfoResponse{}, c.NextErr()
	return &params.DockerInfoResponse{}, nil
}

func (c Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	//c.MethodCall(c, "Publish", id, channels, resources)
	//return c.NextErr()
	return nil
}

func (c Client) WithChannel(channel params.Channel) testcharms.MinimalCharmstoreClient {
	//	c.MethodCall(c, "WithChannel", channel)
	//	c.PopNoErr()
	return &c
}

func (c Client) Latest(ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	//	c.MethodCall(c, "Latest", channel, ids, headers)
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
	// c.MethodCall(c, "Put", path, data)
	//return c.NextErr()
	return nil
}

// // Put puts data into a location for later retrieval.
// func (c *Client) Get(path string, result interface{}) error {
// 	data := c.rawstore[path]
// 	if len(data) == 0 {
// 		return nil
// 	}
// 	result = *data[len(data)]
// 	return nil
// }

// currentCharmURL := charm.MustParseURL("cs:quantal/foo-1")

// {
// 	"mysql":     "quantal/mysql-23",
// 	"dummy":     "quantal/dummy-24",
// 	"riak":      "quantal/riak-25",
// 	"wordpress": "quantal/wordpress-26",
// 	"logging":   "quantal/logging-27",
// }

// InternalClient implements the github.com/juju/juju/charmstore.CharmstoreWrapper,
// which deals with channels differently
type InternalClient struct {
	base Client
}

var _ charmstore.CharmstoreWrapper = (*Client)(nil)

func (c InternalClient) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	return c.base.Latest(ids, headers)
}

func (c InternalClient) ListResources(channel params.Channel, id *charm.URL) ([]params.Resource, error) {
	return c.base.ListResources(id)
}

func (c InternalClient) GetResource(channel params.Channel, id *charm.URL, name string) ([]params.Resource, error) {
	return c.base.GetResource(id, name, -1)
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

// UploadBundle uploads a bundle using the given charm store client, and
// returns the resulting bundle URL and bundle.
//func UploadBundle(c *gc.C, client *csclient.Client, url, name string) (*charm.URL, charm.Bundle) {
