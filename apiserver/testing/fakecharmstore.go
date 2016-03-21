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
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient"
	"gopkg.in/juju/charmstore.v5-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/service"
	"github.com/juju/juju/testcharms"
)

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
	s.PatchValue(&service.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL
		return charmrepo.NewCharmStore(p)
	})
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.handler.Close()
	s.Srv.Close()
	s.CleanupSuite.TearDownTest(c)
}

func (s *CharmStoreSuite) UploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharm(c, s.Client, url, name)
}

func (s *CharmStoreSuite) UploadCharmMultiSeries(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	return testcharms.UploadCharmMultiSeries(c, s.Client, url, name)
}
