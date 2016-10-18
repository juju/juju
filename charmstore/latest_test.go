// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"

	"github.com/juju/juju/version"
)

type LatestCharmInfoSuite struct {
	testing.IsolationSuite

	lowLevel *fakeWrapper
	cache    *fakeMacCache
}

var _ = gc.Suite(&LatestCharmInfoSuite{})

func (s *LatestCharmInfoSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.lowLevel = &fakeWrapper{
		stub:       &testing.Stub{},
		stableStub: &testing.Stub{},
		devStub:    &testing.Stub{},
	}

	s.cache = &fakeMacCache{
		stub: &testing.Stub{},
	}
}

func (s *LatestCharmInfoSuite) TestSuccess(c *gc.C) {
	spam := charm.MustParseURL("cs:quantal/spam-17")
	eggs := charm.MustParseURL("cs:quantal/eggs-2")
	ham := charm.MustParseURL("cs:quantal/ham-1")
	charms := []CharmID{
		{URL: spam, Channel: "stable"},
		{URL: eggs, Channel: "stable"},
		{URL: ham, Channel: "stable"},
	}
	notFound := errors.New("not found")
	s.lowLevel.ReturnLatestStable = [][]params.CharmRevision{{{
		Revision: 17,
	}}, {{
		Revision: 3,
	}}, {{
		Err: notFound,
	}}}

	fakeRes := fakeParamsResource("foo", nil)

	s.lowLevel.ReturnListResourcesStable = []resourceResult{
		oneResourceResult(fakeRes),
		resourceResult{err: params.ErrNotFound},
		resourceResult{err: params.ErrUnauthorized},
	}

	client, err := newCachingClient(s.cache, nil, s.lowLevel.makeWrapper)
	c.Assert(err, jc.ErrorIsNil)

	metadata := map[string]string{
		"environment_uuid": "foouuid",
		"cloud":            "foocloud",
		"cloud_region":     "fooregion",
		"provider":         "fooprovider",
	}
	results, err := LatestCharmInfo(client, charms, metadata)
	c.Assert(err, jc.ErrorIsNil)

	header := []string{"environment_uuid=foouuid", "cloud=foocloud", "cloud_region=fooregion", "provider=fooprovider", "controller_version=" + version.Current.String()}
	s.lowLevel.stableStub.CheckCall(c, 0, "Latest", params.StableChannel, []*charm.URL{spam}, map[string][]string{"Juju-Metadata": header})
	s.lowLevel.stableStub.CheckCall(c, 1, "Latest", params.StableChannel, []*charm.URL{eggs}, map[string][]string{"Juju-Metadata": header})
	s.lowLevel.stableStub.CheckCall(c, 2, "Latest", params.StableChannel, []*charm.URL{ham}, map[string][]string{"Juju-Metadata": header})
	s.lowLevel.stableStub.CheckCall(c, 3, "ListResources", params.StableChannel, spam)
	s.lowLevel.stableStub.CheckCall(c, 4, "ListResources", params.StableChannel, eggs)
	s.lowLevel.stableStub.CheckCall(c, 5, "ListResources", params.StableChannel, ham)

	expectedRes, err := params.API2Resource(fakeRes)
	c.Assert(err, jc.ErrorIsNil)

	timestamp := results[0].Timestamp
	results[2].Error = errors.Cause(results[2].Error)
	expected := []CharmInfoResult{{
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
	}}
	sort.Sort(byURL(results))
	sort.Sort(byURL(expected))
	c.Check(results, jc.DeepEquals, expected)
}

type byURL []CharmInfoResult

func (b byURL) Len() int           { return len(b) }
func (b byURL) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byURL) Less(i, j int) bool { return b[i].OriginalURL.String() < b[j].OriginalURL.String() }
