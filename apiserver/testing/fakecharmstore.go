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

const charmstoreVersion = "v5"
const charmstoreURL = "https://api.staging.jujucharms.com/charmstore"

type Client struct {
	//params csclient.Params // might be useful?
	charms    map[charm.URL]charm.Charm
	bundles   map[charm.URL]charm.Bundle
	resources map[charm.URL][]params.Resource
	revisions map[charm.URL]int
	rawstore  map[string][]string
	version   string
	url       string

	//resources map[params.Channel]map[string][]string
}

var _ testcharms.MinimalCharmstoreClient = (*Client)(nil)

func New() *Client {
	return &Client{version: charmstoreVersion, url: charmstoreURL}
}

func (c *Client) Get(charmId *charm.URL) (charm.Charm, error) {
	charmData := c.charms[*charmId]
	if charmData == nil {
		return charmData, NotFoundError(charmId.String())
	}
	return charmData, nil
}

func (c *Client) GetBundle(bundleId *charm.URL) (charm.Bundle, error) {
	bundleData := c.bundles[*bundleId]
	if bundleData == nil {
		return bundleData, NotFoundError(bundleId.String())
	}
	return bundleData, nil
}

func (c *Client) UploadCharm(charmId *charm.URL, charmData charm.Charm) (*charm.URL, error) {
	c.charms[*charmId] = charmData
	return charmId, nil
}

func (c *Client) UploadCharmWithRevision(id *charm.URL, ch charm.Charm, promulgatedRevision int) error {
	_, err := c.UploadCharm(id, ch)
	if err != nil {
		return errors.Trace(err)
	}
	c.revisions[*id] = promulgatedRevision
}

func (c *Client) UploadBundle(bundleLocation *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	c.bundles[*bundleLocation] = bundleData
	return bundleLocation, nil
}

func (c *Client) UploadBundleWithRevision(id *charm.URL, bundleData charm.Bundle, promulgatedRevision int) error {
	_, err := c.UploadBundle(id, bundleData)
	if err != nil {
		return errors.Trace(err)
	}
	c.revisions[*id] = promulgatedRevision
	return nil
}

// ListResources returns Resourc metadata that have been generated
// by UploadResource
func (c *Client) ListResources(id *charm.URL) ([]params.Resource, error) {
	return c.resources[id], nil
}

func signature(r io.ReadSeeker) (hash []byte, err error) {
	h := sha512.New384()
	_, err = io.Copy(h, r)
	if err != nil {
		return "", 0, errors.Trace(err, "cannot calculate hash")
	}
	_, err = r.Seek(0, 0)
	if err != nil {
		return "", 0, errors.Trace(err, "cannot seek")
	}
	hash = []byte(fmt.Sprintf("%x", h.Sum(nil)))
	return hash, nil
}

// UploadResource "uploads" data from file and stores a
func (c *Client) UploadResource(id *charm.URL, name, path string, file io.ReaderAt, size int64, progress csclient.Progress) (revision int, err error) {
	resources := c.resources[id]
	revision = len(resources)
	result := params.ResourceUploadResponse{Revision: revision}
	//progress.Start() // ignoring progress for now, hoping that it's not material to the tests
	hash, err := signature(file)
	if err != nil {
		//progress.Error(err)
		result.Revision = -1
		return result, errors.Trace(err)
	}
	resource := params.Resource{
		Name:        name,
		Path:        path,
		Revision:    revision,
		Size:        size,
		Fingerprint: hash,
	}
	resources = append(resources, resource)
	c.resources[id] = resources
	//progress.Transferred() // it looks like this method is never used by csclient anyway?
	//progress.Finalizing()
	return result, nil
}

func (c *Client) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

func (c *Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return nil
}

func (c *Client) WithChannel(channel params.Channel) Client {
	return c
}

func (c *Client) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	result := make([]params.CharmRevision, len(ids))

	for i, id := range ids {
		revision := c.revisions[*id]
		result[i] = params.CharmRevision{Revision: revision}
	}

	return result, nil
}

// Put puts data into a location for later retrieval.
func (c *Client) Put(path string, data []string) error {
	c.rawstore[path] = data // clobber pre-existing data
	return nil
}

// Put puts data into a location for later retrieval.
func (c *Client) Get(path string, result interface{}) error {
	data := c.rawstore[path]
	if len(data) == 0 {
		return nil
	}
	result = *data[len(data)]
	return nil
}

// currentCharmURL := charm.MustParseURL("cs:quantal/foo-1")

// {
// 	"mysql":     "quantal/mysql-23",
// 	"dummy":     "quantal/dummy-24",
// 	"riak":      "quantal/riak-25",
// 	"wordpress": "quantal/wordpress-26",
// 	"logging":   "quantal/logging-27",
// }

type CharmStoreSuite struct {
	testing.CleanupSuite
	Client *testcharms.MinimalCharmstoreClient
}

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)

	s.Client = client{}
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	// s.discharger.Close()
	// //s.handler.Close()
	// s.Srv.Close()
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
