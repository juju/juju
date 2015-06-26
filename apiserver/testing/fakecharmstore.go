// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net/http"

	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/juju/charmstore.v4"
	"gopkg.in/juju/charmstore.v4/charmstoretesting"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon-bakery.v0/bakerytest"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/service"
	"github.com/juju/juju/testcharms"
)

type CharmStoreSuite struct {
	gitjujutesting.CleanupSuite

	Session *mgo.Session
	// DischargeUser holds the identity of the user
	// that the 3rd party caveat discharger will issue
	// macaroons for. If it is empty, no caveats will be discharged.
	DischargeUser string

	discharger *bakerytest.Discharger
	Srv        *charmstoretesting.Server
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
	s.Srv = charmstoretesting.OpenServer(c, s.Session, charmstore.ServerParams{
		IdentityLocation: s.discharger.Location(),
		PublicKeyLocator: s.discharger,
	})
	s.PatchValue(&charmrepo.CacheDir, c.MkDir())
	s.PatchValue(&service.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL()
		return charmrepo.NewCharmStore(p)
	})
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.Srv.Close()
	s.CleanupSuite.TearDownTest(c)
}

func (s *CharmStoreSuite) UploadCharm(c *gc.C, url, name string) (*charm.URL, charm.Charm) {
	id := charm.MustParseReference(url)
	promulgated := false
	if id.User == "" {
		id.User = "who"
		promulgated = true
	}
	ch := testcharms.Repo.CharmArchive(c.MkDir(), name)
	id = s.Srv.UploadCharm(c, ch, id, promulgated)
	curl := (*charm.URL)(id)
	return curl, ch
}
