// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/juju/charm/v12"
	charmresource "github.com/juju/charm/v12/resource"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/http/mocks"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&charmsMockSuite{})
var one = 1

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockClientFacade := basemocks.NewMockClientFacade(ctrl)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	curl := charm.MustParseURL("ch:a-charm")
	curl2 := charm.MustParseURL("ch:amd64/jammy/dummy-1")
	no := ""
	edge := "edge"
	stable := "stable"

	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub"}
	edgeChannelParamsOrigin := params.CharmOrigin{Revision: &one, Architecture: "amd64", ID: "1", Hash: "#", Source: "charm-hub", Risk: edge}
	stableChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub", Risk: stable}

	facadeArgs := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Origin: noChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
		},
	}
	resolve := new(params.ResolveCharmWithChannelResults)
	results := params.ResolveCharmWithChannelResults{
		Results: []params.ResolveCharmWithChannelResult{
			{
				URL:    curl.String(),
				Origin: stableChannelParamsOrigin,
				SupportedBases: []params.Base{
					{Name: "ubuntu", Channel: "18.04/stable"},
					{Name: "ubuntu", Channel: "20.04/stable"},
					{Name: "ubuntu", Channel: "16.04/stable"},
				},
			}, {
				URL:    curl2.String(),
				Origin: edgeChannelParamsOrigin,
				SupportedBases: []params.Base{
					{Name: "ubuntu", Channel: "18.04/stable"},
					{Name: "ubuntu", Channel: "20.04/stable"},
					{Name: "ubuntu", Channel: "16.04/stable"},
				},
			},
			{
				URL:    curl2.String(),
				Origin: edgeChannelParamsOrigin,
				SupportedBases: []params.Base{
					{Name: "ubuntu", Channel: "20.04/stable"},
				},
			},
		}}

	mockClientFacade.EXPECT().BestAPIVersion().Return(7).AnyTimes()
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResolveCharms", facadeArgs, resolve).SetArg(3, results).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, mockClientFacade)

	noChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmHub, Risk: no}
	edgeChannelOrigin := apicharm.Origin{Revision: &one, Architecture: "amd64", ID: "1", Hash: "#", Source: apicharm.OriginCharmHub, Risk: edge}
	stableChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmHub, Risk: stable}
	args := []charms.CharmToResolve{
		{URL: curl, Origin: noChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
	}
	got, err := client.ResolveCharms(args)
	c.Assert(err, gc.IsNil)

	want := []charms.ResolvedCharm{
		{
			URL:    curl,
			Origin: stableChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@18.04"),
				corebase.MustParseBaseFromString("ubuntu@20.04"),
				corebase.MustParseBaseFromString("ubuntu@16.04"),
			},
		}, {
			URL:    curl2,
			Origin: edgeChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@18.04"),
				corebase.MustParseBaseFromString("ubuntu@20.04"),
				corebase.MustParseBaseFromString("ubuntu@16.04"),
			},
		}, {
			URL:    curl2,
			Origin: edgeChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@20.04"),
			},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestResolveCharmsLegacy(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockClientFacade := basemocks.NewMockClientFacade(ctrl)
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	curl := charm.MustParseURL("ch:a-charm")
	curl2 := charm.MustParseURL("ch:amd64/jammy/dummy-1")
	no := ""
	edge := "edge"
	stable := "stable"

	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub"}
	edgeChannelParamsOrigin := params.CharmOrigin{Revision: &one, Architecture: "amd64", ID: "1", Hash: "#", Source: "charm-hub", Risk: edge}
	stableChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub", Risk: stable}

	facadeArgs := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Origin: noChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
		},
	}
	resolve := new(params.ResolveCharmWithChannelResultsV6)
	results := params.ResolveCharmWithChannelResultsV6{
		Results: []params.ResolveCharmWithChannelResultV6{
			{
				URL:             curl.String(),
				Origin:          stableChannelParamsOrigin,
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			}, {
				URL:             curl2.String(),
				Origin:          edgeChannelParamsOrigin,
				SupportedSeries: []string{"bionic", "focal", "xenial"},
			},
			{
				URL:             curl2.String(),
				Origin:          edgeChannelParamsOrigin,
				SupportedSeries: []string{"focal"},
			},
		}}

	mockClientFacade.EXPECT().BestAPIVersion().Return(6).AnyTimes()
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ResolveCharms", facadeArgs, resolve).SetArg(3, results).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, mockClientFacade)

	noChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmHub, Risk: no}
	edgeChannelOrigin := apicharm.Origin{Revision: &one, Architecture: "amd64", ID: "1", Hash: "#", Source: apicharm.OriginCharmHub, Risk: edge}
	stableChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmHub, Risk: stable}
	args := []charms.CharmToResolve{
		{URL: curl, Origin: noChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
	}
	got, err := client.ResolveCharms(args)
	c.Assert(err, gc.IsNil)

	want := []charms.ResolvedCharm{
		{
			URL:    curl,
			Origin: stableChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@18.04"),
				corebase.MustParseBaseFromString("ubuntu@20.04"),
				corebase.MustParseBaseFromString("ubuntu@16.04"),
			},
		}, {
			URL:    curl2,
			Origin: edgeChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@18.04"),
				corebase.MustParseBaseFromString("ubuntu@20.04"),
				corebase.MustParseBaseFromString("ubuntu@16.04"),
			},
		}, {
			URL:    curl2,
			Origin: edgeChannelOrigin,
			SupportedBases: []corebase.Base{
				corebase.MustParseBaseFromString("ubuntu@20.04"),
			},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestGetDownloadInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("ch:a-charm")
	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	facadeArgs := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{
			{CharmURL: curl.String(), Origin: noChannelParamsOrigin},
		},
	}

	var resolve params.DownloadInfoResults

	results := params.DownloadInfoResults{
		Results: []params.DownloadInfoResult{{
			URL:    "http://someplace.com",
			Origin: noChannelParamsOrigin,
		}},
	}

	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetDownloadInfos", facadeArgs, &resolve).SetArg(3, results).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	origin, err := apicharm.APICharmOrigin(noChannelParamsOrigin)
	c.Assert(err, jc.ErrorIsNil)
	got, err := client.GetDownloadInfo(curl, origin)
	c.Assert(err, gc.IsNil)

	want := charms.DownloadInfo{
		URL:    "http://someplace.com",
		Origin: origin,
	}

	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestAddCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("ch:testme-2")
	origin := apicharm.Origin{
		Source:       "charm-hub",
		ID:           "",
		Hash:         "",
		Risk:         "stable",
		Revision:     &curl.Revision,
		Track:        nil,
		Architecture: arch.DefaultArchitecture,
		Base:         corebase.MakeDefaultBase("ubuntu", "18.04"),
	}
	facadeArgs := params.AddCharmWithOrigin{
		URL:    curl.String(),
		Origin: origin.ParamsCharmOrigin(),
	}
	result := new(params.CharmOriginResult)
	actualResult := params.CharmOriginResult{
		Origin: origin.ParamsCharmOrigin(),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "AddCharm", facadeArgs, result).SetArg(3, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	got, err := client.AddCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, origin)
}

func (s charmsMockSuite) TestCheckCharmPlacement(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facadeArgs := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: "winnie",
			CharmURL:    "ch:poo",
		}},
	}

	var result params.ErrorResults
	actualResult := params.ErrorResults{
		Results: make([]params.ErrorResult, 1),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CheckCharmPlacement", facadeArgs, &result).SetArg(3, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	err := client.CheckCharmPlacement("winnie", charm.MustParseURL("poo"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s charmsMockSuite) TestCheckCharmPlacementError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	facadeArgs := params.ApplicationCharmPlacements{
		Placements: []params.ApplicationCharmPlacement{{
			Application: "winnie",
			CharmURL:    "ch:poo",
		}},
	}

	var result params.ErrorResults
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "CheckCharmPlacement", facadeArgs, &result).Return(errors.Errorf("trap"))

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	err := client.CheckCharmPlacement("winnie", charm.MustParseURL("poo"))
	c.Assert(err, gc.ErrorMatches, "trap")
}

func (s *charmsMockSuite) TestListCharmResources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("a-charm")
	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub"}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	facadeArgs := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{
			{CharmURL: curl.String(), Origin: noChannelParamsOrigin},
		},
	}

	var resolve params.CharmResourcesResults

	results := params.CharmResourcesResults{
		Results: [][]params.CharmResourceResult{{{
			CharmResource: params.CharmResource{
				Type:     "oci-image",
				Origin:   "upload",
				Name:     "a-charm-res-1",
				Path:     "res.txt",
				Revision: 2,
				Size:     1024,
			},
		}}},
	}

	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ListCharmResources", facadeArgs, &resolve).SetArg(3, results).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	origin, err := apicharm.APICharmOrigin(noChannelParamsOrigin)
	c.Assert(err, jc.ErrorIsNil)
	got, err := client.ListCharmResources(curl, origin)
	c.Assert(err, gc.IsNil)

	want := []charmresource.Resource{{
		Meta: charmresource.Meta{
			Type: charmresource.TypeContainerImage,
			Name: "a-charm-res-1",
			Path: "res.txt",
		},
		Origin:   charmresource.OriginUpload,
		Revision: 2,
		Size:     1024,
	}}

	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestZipHasHooksOnly(c *gc.C) {
	ch := testcharms.Repo.CharmDir("storage-filesystem-subordinate") // has hooks only
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	f := *charms.HasHooksOrDispatch
	hasHooks, err := f(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasHooks, jc.IsTrue)
}

func (s *charmsMockSuite) TestZipHasDispatchFileOnly(c *gc.C) {
	ch := testcharms.Repo.CharmDir("category-dispatch") // has dispatch file only
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	f := *charms.HasHooksOrDispatch
	hasDispatch, err := f(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasDispatch, jc.IsTrue)
}

func (s *charmsMockSuite) TestZipHasNoHooksNorDispath(c *gc.C) {
	ch := testcharms.Repo.CharmDir("category") // has no hooks nor dispatch file
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())
	err = ch.ArchiveTo(tempFile)
	c.Assert(err, jc.ErrorIsNil)
	f := *charms.HasHooksOrDispatch
	hasHooks, err := f(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(hasHooks, jc.IsFalse)
}

type charmUploadMatcher struct {
	expectedURL string
}

func (m charmUploadMatcher) Matches(x interface{}) bool {
	req, ok := x.(*http.Request)
	if !ok {
		return false
	}
	if req.URL.String() != m.expectedURL {
		return false
	}
	return true
}

func (charmUploadMatcher) String() string {
	return "matches charm upload requests"
}

func (s *charmsMockSuite) TestAddLocalCharm(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).MinTimes(1)

	curl, charmArchive := s.testCharm(c)
	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	vers := version.MustParse("2.6.6")
	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("ch:wordpress-1"), nil, false, vers)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "ch:wordpress-1"`)

	// Upload an archive with its original revision.
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.String())

	// Upload a charm directory with changed revision.
	resp.Body = io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-42"}`))
	charmDir := testcharms.Repo.ClonedDir(c.MkDir(), "dummy")
	err = charmDir.SetDiskRevision(42)
	c.Assert(err, jc.ErrorIsNil)
	savedURL, err = client.AddLocalCharm(curl, charmDir, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.Revision, gc.Equals, 42)

	// Upload a charm directory again, revision should be bumped.
	resp.Body = io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-43"}`))
	savedURL, err = client.AddLocalCharm(curl, charmDir, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, curl.WithRevision(43).String())
}

func (s *charmsMockSuite) TestAddLocalCharmFindingHooksError(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return true, fmt.Errorf("bad zip")
		},
		`bad zip`)
}

func (s *charmsMockSuite) TestAddLocalCharmNoHooks(c *gc.C) {
	s.assertAddLocalCharmFailed(c,
		func(string) (bool, error) {
			return false, nil
		},
		`invalid charm \"dummy\": has no hooks nor dispatch file`)
}

func (s *charmsMockSuite) TestAddLocalCharmWithLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).MinTimes(1)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/lxd-profile-0"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=0&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, "local:quantal/lxd-profile-0")
}

func (s *charmsMockSuite) TestAddLocalCharmWithInvalidLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile-fail")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.ErrorMatches, "invalid lxd-profile.yaml: contains device type \"unix-disk\"")
}

func (s *charmsMockSuite) TestAddLocalCharmWithValidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile", c)
}

func (s *charmsMockSuite) TestAddLocalCharmWithInvalidLXDProfileWithForceSucceeds(c *gc.C) {
	s.testAddLocalCharmWithForceSucceeds("lxd-profile-fail", c)
}

func (s *charmsMockSuite) testAddLocalCharmWithForceSucceeds(name string, c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).MinTimes(1)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/lxd-profile-0"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=0&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "lxd-profile")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)

	vers := version.MustParse("2.6.6")
	savedURL, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedURL.String(), gc.Equals, "local:quantal/lxd-profile-0")
}

func (s *charmsMockSuite) assertAddLocalCharmFailed(c *gc.C, f func(string) (bool, error), msg string) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, ch := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, f)

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	client := charms.NewClientWithFacade(mockFacadeCaller, nil)
	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *charmsMockSuite) TestAddLocalCharmDefinitelyWithHooks(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, ch := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).MinTimes(1)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).MinTimes(1)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	vers := version.MustParse("2.6.6")
	savedCURL, err := client.AddLocalCharm(curl, ch, false, vers)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedCURL.String(), gc.Equals, curl.String())
}

func (s *charmsMockSuite) testCharm(c *gc.C) (*charm.URL, charm.Charm) {
	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	return curl, charmArchive
}

func (s *charmsMockSuite) TestAddLocalCharmError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl, charmArchive := s.testCharm(c)
	s.PatchValue(charms.HasHooksOrDispatch, func(string) (bool, error) {
		return true, nil
	})
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).MinTimes(1)
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).MinTimes(1)
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).MinTimes(1)

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(nil, errors.New("boom")).MinTimes(1)

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	vers := version.MustParse("2.6.6")
	_, err := client.AddLocalCharm(curl, charmArchive, false, vers)
	c.Assert(err, gc.ErrorMatches, `.*boom$`)
}

func (s *charmsMockSuite) TestMinVersionLocalCharm(c *gc.C) {
	tests := []minverTest{
		{"2.0.0", "1.0.0", false, true},
		{"1.0.0", "2.0.0", false, false},
		{"1.25.0", "1.24.0", false, true},
		{"1.24.0", "1.25.0", false, false},
		{"1.25.1", "1.25.0", false, true},
		{"1.25.0", "1.25.1", false, false},
		{"1.25.0", "1.25.0", false, true},
		{"1.25.0", "1.25-alpha1", false, true},
		{"1.25-alpha1", "1.25.0", false, true},
		{"2.0.0", "1.0.0", true, true},
		{"1.0.0", "2.0.0", true, false},
		{"1.25.0", "1.24.0", true, true},
		{"1.24.0", "1.25.0", true, false},
		{"1.25.1", "1.25.0", true, true},
		{"1.25.0", "1.25.1", true, false},
		{"1.25.0", "1.25.0", true, true},
		{"1.25.0", "1.25-alpha1", true, true},
		{"1.25-alpha1", "1.25.0", true, true},
	}
	for _, t := range tests {
		testMinVer(t, c)
	}
}

type minverTest struct {
	juju  string
	charm string
	force bool
	ok    bool
}

func testMinVer(t minverTest, c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockCaller := basemocks.NewMockAPICaller(ctrl)
	mockHttpDoer := mocks.NewMockHTTPClient(ctrl)
	reqClient := &httprequest.Client{
		BaseURL: "http://somewhere.invalid",
		Doer:    mockHttpDoer,
	}

	mockCaller.EXPECT().Context().Return(context.TODO()).AnyTimes()
	mockCaller.EXPECT().HTTPClient().Return(reqClient, nil).AnyTimes()
	mockFacadeCaller.EXPECT().RawAPICaller().Return(mockCaller).AnyTimes()

	resp := &http.Response{
		StatusCode: 200,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"charm-url": "local:quantal/dummy-1"}`)),
	}
	resp.Header.Add("Content-Type", "application/json")
	mockHttpDoer.EXPECT().Do(
		&charmUploadMatcher{"http://somewhere.invalid/charms?revision=1&schema=local&series=quantal"},
	).Return(resp, nil).AnyTimes()

	client := charms.NewClientWithFacade(mockFacadeCaller, nil)

	charmMinVer := version.MustParse(t.charm)
	jujuVer := version.MustParse(t.juju)

	charmArchive := testcharms.Repo.CharmArchive(c.MkDir(), "dummy")
	curl := charm.MustParseURL(
		fmt.Sprintf("local:quantal/%s-%d", charmArchive.Meta().Name, charmArchive.Revision()),
	)
	charmArchive.Meta().MinJujuVersion = charmMinVer

	_, err := client.AddLocalCharm(curl, charmArchive, t.force, jujuVer)

	if t.ok {
		if err != nil {
			c.Errorf("Unexpected non-nil error for jujuver %v, minver %v: %#v", t.juju, t.charm, err)
		}
	} else {
		if err == nil {
			c.Errorf("Unexpected nil error for jujuver %v, minver %v", t.juju, t.charm)
		} else if !jujuversion.IsMinVersionError(err) {
			c.Errorf("Wrong error for jujuver %v, minver %v: expected minVersionError, got: %#v", t.juju, t.charm, err)
		}
	}
}
