// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicrossmodel "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type offeredApplicationsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authoriser testing.FakeAuthorizer
	api        *crossmodel.OfferedApplicationAPI

	calls       []string
	offerLister *mockOfferLister
	offers      map[string]jujucrossmodel.OfferedApplication
}

var _ = gc.Suite(&offeredApplicationsSuite{})

func (s *offeredApplicationsSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.authoriser = testing.FakeAuthorizer{
		EnvironManager: true,
	}
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })

	s.calls = []string{}
	s.offers = make(map[string]jujucrossmodel.OfferedApplication)
	s.offerLister = s.constructOfferLister()

	var err error
	s.api, err = crossmodel.CreateOfferedApplicationAPI(&mockState{}, s.offerLister, s.resources, s.authoriser)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *offeredApplicationsSuite) TestUnauthorised(c *gc.C) {
	s.authoriser = testing.FakeAuthorizer{
		EnvironManager: false,
	}
	_, err := crossmodel.CreateOfferedApplicationAPI(&mockState{}, s.offerLister, s.resources, s.authoriser)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *offeredApplicationsSuite) TestWatchOfferedApplications(c *gc.C) {
	authoriser := testing.FakeAuthorizer{
		EnvironManager: true,
	}
	resources := common.NewResources()
	defer resources.StopAll()

	watcher := &mockStringsWatcher{
		changes: make(chan []string, 1),
	}
	watcher.changes <- []string{"local:/u/me/service", "local:/u/me/service"}
	state := &mockState{
		watchOfferedApplications: func() state.StringsWatcher {
			return watcher
		},
	}

	api, err := crossmodel.CreateOfferedApplicationAPI(state, s.constructOfferLister(), resources, authoriser)
	c.Assert(err, jc.ErrorIsNil)
	watches, err := api.WatchOfferedApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{"local:/u/me/service", "local:/u/me/service"},
	})
	c.Assert(resources.Get("1"), gc.Equals, watcher)
}

func (s *offeredApplicationsSuite) assertCalls(c *gc.C, expectedCalls []string) {
	c.Assert(s.calls, jc.SameContents, expectedCalls)
}

func (s *offeredApplicationsSuite) constructOfferLister() *mockOfferLister {
	return &mockOfferLister{
		listOffers: func(filters ...jujucrossmodel.OfferedApplicationFilter) ([]jujucrossmodel.OfferedApplication, error) {
			s.calls = append(s.calls, "listoffers")
			var result []jujucrossmodel.OfferedApplication
			for _, filter := range filters {
				if offer, ok := s.offers[filter.ApplicationURL]; ok {
					result = append(result, offer)
				}
			}
			return result, nil
		},
	}
}

func (s *offeredApplicationsSuite) TestListOffers(c *gc.C) {
	s.offers["local:/u/user/servicename"] = jujucrossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/user/servicename",
		ApplicationName: "service",
		CharmName:       "charm",
		Description:     "description",
		Registered:      true,
		Endpoints:       map[string]string{"foo": "bar"},
	}
	s.offers["local:/u/user/anothername"] = jujucrossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/user/anothername",
		ApplicationName: "service2",
		CharmName:       "charm2",
		Description:     "description2",
		Registered:      true,
		Endpoints:       map[string]string{"db": "db"},
	}
	results, err := s.api.OfferedApplications(params.ApplicationURLs{
		ApplicationURLs: []string{"local:/u/user/servicename"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	s.assertCalls(c, []string{"listoffers"})
	offer := apicrossmodel.MakeOfferedApplicationFromParams(results.Results[0].Result)
	c.Assert(offer, jc.DeepEquals, s.offers["local:/u/user/servicename"])
}

func (s *offeredApplicationsSuite) TestListOffersNoneFound(c *gc.C) {
	s.offers["local:/u/user/servicename"] = jujucrossmodel.OfferedApplication{
		ApplicationURL:  "local:/u/user/servicename",
		ApplicationName: "service",
		CharmName:       "charm",
		Description:     "description",
		Registered:      true,
		Endpoints:       map[string]string{"foo": "bar"},
	}
	results, err := s.api.OfferedApplications(params.ApplicationURLs{
		ApplicationURLs: []string{"local:/u/user/bogus"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	s.assertCalls(c, []string{"listoffers"})
	c.Assert(results.Results[0].Error.ErrorCode(), gc.Equals, params.CodeNotFound)
}
