// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource/resourcetesting"
)

type LatestCharmInfoSuite struct {
	testing.IsolationSuite

	stub   *testing.Stub
	client *stubClient
}

var _ = gc.Suite(&LatestCharmInfoSuite{})

func (s *LatestCharmInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.client = &stubClient{Stub: s.stub}
}

func (s *LatestCharmInfoSuite) TestSuccess(c *gc.C) {
	cURLs := []*charm.URL{
		charm.MustParseURL("cs:quantal/spam-17"),
		charm.MustParseURL("cs:quantal/eggs-2"),
		charm.MustParseURL("cs:quantal/ham-1"),
	}
	notFound := errors.New("not found")
	s.client.ReturnLatestRevisions = []charmrepo.CharmRevision{{
		Revision: 17,
	}, {
		Revision: 3,
	}, {
		Err: notFound,
	}}
	s.client.ReturnListResources = [][]charmresource.Resource{
		{
			resourcetesting.NewCharmResource(c, "spam", "<some data>"),
		},
		nil,
		nil,
	}

	results, err := charmstore.LatestCharmInfo(s.client, cURLs, "stable")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "LatestRevisions", "ListResources")
	s.stub.CheckCall(c, 0, "LatestRevisions", cURLs, "stable")
	s.stub.CheckCall(c, 1, "ListResources", cURLs, "stable")
	timestamp := results[0].Timestamp
	results[2].Error = errors.Cause(results[2].Error)
	c.Check(results, jc.DeepEquals, []charmstore.CharmInfoResult{{
		CharmInfo: charmstore.CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/spam-17"),
			Timestamp:      timestamp,
			LatestRevision: 17,
			LatestResources: []charmresource.Resource{
				resourcetesting.NewCharmResource(c, "spam", "<some data>"),
			},
		},
	}, {
		CharmInfo: charmstore.CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/eggs-2"),
			Timestamp:      timestamp,
			LatestRevision: 3,
		},
	}, {
		CharmInfo: charmstore.CharmInfo{
			OriginalURL: charm.MustParseURL("cs:quantal/ham-1"),
			Timestamp:   timestamp,
		},
		Error: notFound,
	}})
}
