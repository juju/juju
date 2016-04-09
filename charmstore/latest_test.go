// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
)

type LatestCharmInfoSuite struct {
	testing.IsolationSuite

	lowLevel *fakeWrapper
}

var _ = gc.Suite(&LatestCharmInfoSuite{})

func (s *LatestCharmInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.lowLevel = &fakeWrapper{
		stableStub: &testing.Stub{},
		devStub:    &testing.Stub{},
	}
}

func (s *LatestCharmInfoSuite) TestSuccess(c *gc.C) {
	spam := charm.MustParseURL("cs:quantal/spam-17")
	eggs := charm.MustParseURL("cs:quantal/eggs-2")
	ham := charm.MustParseURL("cs:quantal/ham-1")
	charms := []CharmID{
		{spam, "stable"},
		{eggs, "stable"},
		{ham, "stable"},
	}
	notFound := errors.New("not found")
	s.lowLevel.ReturnLatestStable = []charmrepo.CharmRevision{{
		Revision: 17,
	}, {
		Revision: 3,
	}, {
		Err: notFound,
	}}

	fakeRes := fakeParamsResource("foo", nil)

	s.lowLevel.ReturnListResourcesStable = map[string][]params.Resource{
		"cs:quantal/spam-17": []params.Resource{fakeRes},
	}

	uuid := "foobar"
	client := Client{lowLevel: s.lowLevel}
	results, err := LatestCharmInfo(client, charms, uuid)
	c.Assert(err, jc.ErrorIsNil)

	expectedIds := []*charm.URL{spam, eggs, ham}

	s.lowLevel.stableStub.CheckCallNames(c, "Latest", "ListResources")
	s.lowLevel.stableStub.CheckCall(c, 0, "Latest", charm.Channel("stable"), expectedIds, map[string]string{"environment_uuid": uuid})
	s.lowLevel.stableStub.CheckCall(c, 1, "ListResources", charm.Channel("stable"), expectedIds)

	expectedRes, err := params.API2Resource(fakeRes)
	c.Assert(err, jc.ErrorIsNil)

	timestamp := results[0].Timestamp
	results[2].Error = errors.Cause(results[2].Error)
	c.Check(results, jc.DeepEquals, []CharmInfoResult{{
		CharmInfo: CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/spam-17"),
			Timestamp:      timestamp,
			LatestRevision: 17,
			LatestResources: []charmresource.Resource{
				expectedRes,
			},
		},
	}, {
		CharmInfo: CharmInfo{
			OriginalURL:    charm.MustParseURL("cs:quantal/eggs-2"),
			Timestamp:      timestamp,
			LatestRevision: 3,
		},
	}, {
		CharmInfo: CharmInfo{
			OriginalURL: charm.MustParseURL("cs:quantal/ham-1"),
			Timestamp:   timestamp,
		},
		Error: notFound,
	}})
}
