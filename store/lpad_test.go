// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store_test

import (
	"fmt"

	gitjujutesting "github.com/juju/testing"
	gc "launchpad.net/gocheck"
	"launchpad.net/lpad"

	"github.com/juju/juju/charm"
	"github.com/juju/juju/store"
)

var jsonType = map[string]string{
	"Content-Type": "application/json",
}

func (s *StoreSuite) TestPublishCharmDistro(c *gc.C) {
	branch := s.dummyBranch(c, "~joe/charms/oneiric/dummy/trunk")

	// The Distro call will look for bare /charms, first.
	gitjujutesting.Server.Response(200, jsonType, []byte("{}"))

	// And then it picks up the tips.
	data := fmt.Sprintf(`[`+
		`["file://%s", "rev1", ["oneiric", "precise"]],`+
		`["file://%s", "%s", []],`+
		`["file:///non-existent/~jeff/charms/precise/bad/trunk", "rev2", []],`+
		`["file:///non-existent/~jeff/charms/precise/bad/skip-me", "rev3", []]`+
		`]`,
		branch.path(), branch.path(), branch.digest())
	gitjujutesting.Server.Response(200, jsonType, []byte(data))

	apiBase := lpad.APIBase(gitjujutesting.Server.URL)
	err := store.PublishCharmsDistro(s.store, apiBase)

	// Should have a single failure from the trunk branch that doesn't
	// exist. The redundant update with the known digest should be
	// ignored, and skip-me isn't a supported branch name so it's
	// ignored as well.
	c.Assert(err, gc.ErrorMatches, `1 branch\(es\) failed to be published`)
	berr := err.(store.PublishBranchErrors)[0]
	c.Assert(berr.URL, gc.Equals, "file:///non-existent/~jeff/charms/precise/bad/trunk")
	c.Assert(berr.Err, gc.ErrorMatches, "(?s).*bzr: ERROR: Not a branch.*")

	for _, url := range []string{"cs:oneiric/dummy", "cs:precise/dummy-0", "cs:~joe/oneiric/dummy-0"} {
		dummy, err := s.store.CharmInfo(charm.MustParseURL(url))
		c.Assert(err, gc.IsNil)
		c.Assert(dummy.Meta().Name, gc.Equals, "dummy")
	}

	// The known digest should have been ignored, so revision is still at 0.
	_, err = s.store.CharmInfo(charm.MustParseURL("cs:~joe/oneiric/dummy-1"))
	c.Assert(err, gc.Equals, store.ErrNotFound)

	// bare /charms lookup
	req := gitjujutesting.Server.WaitRequest()
	c.Assert(req.Method, gc.Equals, "GET")
	c.Assert(req.URL.Path, gc.Equals, "/charms")

	// tips request
	req = gitjujutesting.Server.WaitRequest()
	c.Assert(req.Method, gc.Equals, "GET")
	c.Assert(req.URL.Path, gc.Equals, "/charms")
	c.Assert(req.Form["ws.op"], gc.DeepEquals, []string{"getBranchTips"})
	c.Assert(req.Form["since"], gc.IsNil)

	// Request must be signed by juju.
	c.Assert(req.Header.Get("Authorization"), gc.Matches, `.*oauth_consumer_key="juju".*`)
}
