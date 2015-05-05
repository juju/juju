// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"net/http"

	gitjujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5/charmrepo"
	"gopkg.in/juju/charmstore.v4"
	"gopkg.in/juju/charmstore.v4/charmstoretesting"
	"gopkg.in/macaroon-bakery.v0/bakery/checkers"
	"gopkg.in/macaroon-bakery.v0/bakerytest"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/client"
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
	s.PatchValue(&client.NewCharmStore, func(p charmrepo.NewCharmStoreParams) charmrepo.Interface {
		p.URL = s.Srv.URL()
		return charmrepo.NewCharmStore(p)
	})
}

func (s *CharmStoreSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.Srv.Close()
	s.CleanupSuite.TearDownTest(c)
}
