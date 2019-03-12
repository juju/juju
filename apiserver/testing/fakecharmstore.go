// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	// "fmt"

	// "net/http"
	// "net/http/httptest"
	"time"

	"github.com/juju/testing"
	// jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"

	// "gopkg.in/juju/charmrepo.v3"
	// "gopkg.in/juju/charmrepo.v3/csclient"

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
	charms  map[charm.URL]charm.Charm
	bundles map[charm.URL]charm.Bundle
	store   map[string][]string
	version string
	url     string

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

func (c *Client) UploadBundle(bundleLocation *charm.URL, bundleData charm.Bundle) (*charm.URL, error) {
	c.bundles[*bundleLocation] = bundleData
	return bundleLocation, nil
}

func (c *Client) UploadResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

func (c *Client) AddDockerResource(id *charm.URL, resourceName string, imageName, digest string) (revision int, err error) {
	return -1, nil
}

func (c *Client) Publish(id *charm.URL, channels []params.Channel, resources map[string]int) error {
	return nil
}

func (c *Client) WithChannel(channel params.Channel) *Client {
	return c
}

func (c *Client) Latest(channel params.Channel, ids []*charm.URL, headers map[string][]string) ([]params.CharmRevision, error) {
	return []params.CharmRevision{}, nil
}

func (c *Client) Put(path string, data []string) error {
	c.store[path] = data // clobber pre-existing data for now
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
	Client *MinimalCharmstoreClient

	// Session *mgo.Session
	// // DischargeUser holds the identity of the user
	// // that the 3rd party caveat discharger will issue
	// // macaroons for. If it is empty, no caveats will be discharged.
	// DischargeUser string

	// discharger *bakerytest.Discharger
	// //handler    charmstore.HTTPCloseHandler
	// Srv    *httptest.Server
	// Client *csclient.Client
}

// func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
// 	s.CleanupSuite.SetUpTest(c)

// 	s.discharger = bakerytest.NewDischarger(nil, func(_ *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
// 		if s.DischargeUser == "" {
// 			return nil, fmt.Errorf("discharge denied")
// 		}
// 		return []checkers.Caveat{
// 			checkers.DeclaredCaveat("username", s.DischargeUser),
// 		}, nil
// 	})
// 	db := s.Session.DB("juju-testing")
// 	params := charmstore.ServerParams{
// 		AuthUsername:     "test-user",
// 		AuthPassword:     "test-password",
// 		IdentityLocation: s.discharger.Location(),
// 		PublicKeyLocator: s.discharger,
// 	}
// 	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V5)
// 	//handler, err := NewServer()
// 	c.Assert(err, jc.ErrorIsNil)
// 	//s.handler = handler
// 	s.Srv = httptest.NewServer(handler)
// 	s.Client = csclient.New(csclient.Params{
// 		URL:      s.Srv.URL,
// 		User:     params.AuthUsername,
// 		Password: params.AuthPassword,
// 	})

// 	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
// 	s.PatchValue(&csclient.ServerURL, s.Srv.URL)
// }

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
