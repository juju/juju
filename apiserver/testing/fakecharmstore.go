// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/charmstore.v5"
	"gopkg.in/macaroon-bakery.v2-unstable/bakery/checkers"
	"gopkg.in/macaroon-bakery.v2-unstable/bakerytest"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/testcharms"
)

type charmstoreClientShim struct {
	*csclient.Client
}

func (c charmstoreClientShim) WithChannel(channel params.Channel) testcharms.CharmstoreClient {
	client := c.Client.WithChannel(channel)
	return charmstoreClientShim{client}
}

type CharmStoreSuite struct {
	testing.CleanupSuite

	Session *mgo.Session
	// DischargeUser holds the identity of the user
	// that the 3rd party caveat discharger will issue
	// macaroons for. If it is empty, no caveats will be discharged.
	DischargeUser string

	discharger *bakerytest.Discharger
	handler    charmstore.HTTPCloseHandler
	Srv        *httptest.Server
	Client     *csclient.Client
}

func (s *CharmStoreSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)

	s.discharger = bakerytest.NewDischarger(nil, func(_ *http.Request, cond string, arg string) ([]checkers.Caveat, error) {
		if s.DischargeUser == "" {
			return nil, fmt.Errorf("discharge denied")
		}
		return []checkers.Caveat{
			checkers.DeclaredCaveat("username", s.DischargeUser),
		}, nil
	})
	db := s.Session.DB("juju-testing")
	params := charmstore.ServerParams{
		AuthUsername:     "test-user",
		AuthPassword:     "test-password",
		IdentityLocation: s.discharger.Location(),
		PublicKeyLocator: s.discharger,
	}
	handler, err := charmstore.NewServer(db, nil, "", params, charmstore.V5)
	c.Assert(err, jc.ErrorIsNil)
	s.handler = handler
	s.Srv = httptest.NewServer(handler)
	s.Client = csclient.New(csclient.Params{
		URL:      s.Srv.URL,
		User:     params.AuthUsername,
		Password: params.AuthPassword,
	})

	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
	s.PatchValue(&csclient.ServerURL, s.Srv.URL)
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.handler.Close()
	s.Srv.Close()
	s.CleanupSuite.TearDownTest(c)
}

func (s *CharmStoreSuite) UploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	client := &charmstoreClientShim{s.Client}
	return testcharms.UploadCharm(c, client, url, name)
}

func (s *CharmStoreSuite) UploadCharmMultiSeries(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	client := &charmstoreClientShim{s.Client}
	return testcharms.UploadCharmMultiSeries(c, client, url, name)
}

func (s *CharmStoreSuite) UploadCharmWithSeries(c *gc.C, url, name, series string) (*charm.URL, charm.Charm) {
	client := &charmstoreClientShim{s.Client}
	return testcharms.UploadCharmWithSeries(c, client, url, name, series)
}
