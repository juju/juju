// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/apiserver/charmrevisionupdater/testing"
	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/charmstore"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/version"
)

type charmVersionSuite struct {
	testing.CharmSuite
	jujutesting.JujuConnSuite

	charmrevisionupdater *charmrevisionupdater.CharmRevisionUpdaterAPI
	resources            *common.Resources
	authoriser           apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&charmVersionSuite{})

func (s *charmVersionSuite) SetUpSuite(c *gc.C) {
	s.JujuConnSuite.SetUpSuite(c)
	s.CharmSuite.SetUpSuite(c, &s.JujuConnSuite)
}

func (s *charmVersionSuite) TearDownSuite(c *gc.C) {
	s.CharmSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *charmVersionSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })
	s.authoriser = apiservertesting.FakeAuthorizer{
		EnvironManager: true,
	}
	var err error
	s.charmrevisionupdater, err = charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmVersionSuite) TearDownTest(c *gc.C) {
	s.CharmSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIAcceptsStateManager(c *gc.C) {
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *charmVersionSuite) TestNewCharmRevisionUpdaterAPIRefusesNonStateManager(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.EnvironManager = false
	endPoint, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *charmVersionSuite) TestUpdateRevisions(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.SetupScenario(c)

	curl := charm.MustParseURL("cs:quantal/mysql")
	_, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	curl = charm.MustParseURL("cs:quantal/mysql")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")

	// Latest wordpress is already deployed, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Varnish has an error when updating, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/varnish")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Update mysql version and run update again.
	svc, err := s.State.Application("mysql")
	c.Assert(err, jc.ErrorIsNil)
	ch := s.AddCharmWithRevision(c, "mysql", 23)
	cfg := state.SetCharmConfig{
		Charm:      ch,
		ForceUnits: true,
	}
	err = svc.SetCharm(cfg)
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Latest mysql is now deployed, so no pending charm.
	curl = charm.MustParseURL("cs:quantal/mysql")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *charmVersionSuite) TestWordpressCharmNoReadAccessIsntVisible(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.SetupScenario(c)

	// Disallow read access to the wordpress charm in the charm store.
	err := s.Client.Put("/quantal/wordpress/meta/perm/read", nil)
	c.Assert(err, jc.ErrorIsNil)

	// Run the revision updater and check that the public charm updates are
	// still properly notified.
	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	curl := charm.MustParseURL("cs:quantal/mysql")
	pending, err := s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(pending.String(), gc.Equals, "cs:quantal/mysql-23")

	// No pending charm for wordpress.
	curl = charm.MustParseURL("cs:quantal/wordpress")
	_, err = s.State.LatestPlaceholderCharm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *charmVersionSuite) TestJujuMetadataHeaderIsSent(c *gc.C) {
	s.AddMachine(c, "0", state.JobManageModel)
	s.SetupScenario(c)

	// Set up a charm store server that stores the request header.
	var header http.Header
	received := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// the first request is the one with the UUID.
		if !received {
			header = r.Header
			received = true
		}
		s.Handler.ServeHTTP(w, r)
	}))
	defer srv.Close()

	// Point the charm repo initializer to the testing server.
	s.PatchValue(&charmrevisionupdater.NewCharmStoreClient, func(st *state.State) (charmstore.Client, error) {
		csURL, err := url.Parse(srv.URL)
		c.Assert(err, jc.ErrorIsNil)
		return charmstore.NewCachingClient(state.MacaroonCache{st}, csURL)
	})

	result, err := s.charmrevisionupdater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	env, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.State.Cloud(env.Cloud())
	c.Assert(err, jc.ErrorIsNil)
	expected_header := []string{
		"environment_uuid=" + env.UUID(),
		"cloud=" + env.Cloud(),
		"cloud_region=" + env.CloudRegion(),
		"provider=" + cloud.Type,
		"controller_version=" + version.Current.String(),
	}
	for i, expected := range expected_header {
		c.Assert(header[charmrepo.JujuMetadataHTTPHeader][i], gc.Equals, expected)
	}
}
