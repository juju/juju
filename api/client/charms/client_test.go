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

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v9"
	charmresource "github.com/juju/charm/v9/resource"
	csparams "github.com/juju/charmrepo/v7/csclient/params"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon.v2"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/api/http/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
	coretesting "github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type charmsMockSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&charmsMockSuite{})

func (s *charmsMockSuite) TestIsMeteredFalse(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	url := "local:quantal/dummy-1"
	args := params.CharmURL{URL: url}
	metered := new(params.IsMeteredResult)
	p := params.IsMeteredResult{Metered: true}

	mockFacadeCaller.EXPECT().FacadeCall("IsMetered", args, metered).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.IsMetered(url)
	c.Assert(err, gc.IsNil)
	c.Assert(got, jc.IsTrue)
}

func (s *charmsMockSuite) TestResolveCharms(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	curl := charm.MustParseURL("cs:a-charm")
	curl2 := charm.MustParseURL("cs:jammy/dummy-1")
	no := string(csparams.NoChannel)
	edge := string(csparams.EdgeChannel)
	stable := string(csparams.StableChannel)

	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-store"}
	edgeChannelParamsOrigin := params.CharmOrigin{Source: "charm-store", Risk: edge}
	stableChannelParamsOrigin := params.CharmOrigin{Source: "charm-store", Risk: stable}

	facadeArgs := params.ResolveCharmsWithChannel{
		Resolve: []params.ResolveCharmWithChannel{
			{Reference: curl.String(), Origin: noChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
			{Reference: curl2.String(), Origin: edgeChannelParamsOrigin},
		},
	}
	resolve := new(params.ResolveCharmWithChannelResults)
	p := params.ResolveCharmWithChannelResults{
		Results: []params.ResolveCharmWithChannelResult{
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

	mockFacadeCaller.EXPECT().FacadeCall("ResolveCharms", facadeArgs, resolve).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)

	noChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: no}
	edgeChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: edge}
	stableChannelOrigin := apicharm.Origin{Source: apicharm.OriginCharmStore, Risk: stable}
	args := []charms.CharmToResolve{
		{URL: curl, Origin: noChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
		{URL: curl2, Origin: edgeChannelOrigin},
	}
	got, err := client.ResolveCharms(args)
	c.Assert(err, gc.IsNil)

	want := []charms.ResolvedCharm{
		{
			URL:             curl,
			Origin:          stableChannelOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Origin:          edgeChannelOrigin,
			SupportedSeries: []string{"bionic", "focal", "xenial"},
		}, {
			URL:             curl2,
			Origin:          edgeChannelOrigin,
			SupportedSeries: []string{"focal"},
		},
	}
	c.Assert(got, gc.DeepEquals, want)
}

func (s *charmsMockSuite) TestGetDownloadInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:a-charm")
	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-store", Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"}}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	facadeArgs := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{
			{CharmURL: curl.String(), Origin: noChannelParamsOrigin},
		},
	}

	var resolve params.DownloadInfoResults

	p := params.DownloadInfoResults{
		Results: []params.DownloadInfoResult{{
			URL:    "http://someplace.com",
			Origin: noChannelParamsOrigin,
		}},
	}

	mockFacadeCaller.EXPECT().FacadeCall("GetDownloadInfos", facadeArgs, &resolve).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	origin, err := apicharm.APICharmOrigin(noChannelParamsOrigin)
	c.Assert(err, jc.ErrorIsNil)
	got, err := client.GetDownloadInfo(curl, origin, nil)
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

	curl := charm.MustParseURL("cs:testme-2")
	origin := apicharm.Origin{
		Source:       "charm-store",
		ID:           "",
		Hash:         "",
		Risk:         "stable",
		Revision:     &curl.Revision,
		Track:        nil,
		Architecture: arch.DefaultArchitecture,
		Base:         series.MakeDefaultBase("ubuntu", "18.04"),
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
	mockFacadeCaller.EXPECT().FacadeCall("AddCharm", facadeArgs, result).SetArg(2, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.AddCharm(curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, origin)
}

func (s *charmsMockSuite) TestAddCharmWithAuthorization(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := charm.MustParseURL("cs:testme-2")
	origin := apicharm.Origin{
		Source:       "charm-store",
		ID:           "",
		Hash:         "",
		Risk:         "stable",
		Revision:     &curl.Revision,
		Track:        nil,
		Architecture: arch.DefaultArchitecture,
		Base:         series.MakeDefaultBase("ubuntu", "18.04"),
	}
	facadeArgs := params.AddCharmWithAuth{
		URL:                curl.String(),
		CharmStoreMacaroon: &macaroon.Macaroon{},
		Origin:             origin.ParamsCharmOrigin(),
	}
	result := new(params.CharmOriginResult)
	actualResult := params.CharmOriginResult{
		Origin: origin.ParamsCharmOrigin(),
	}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("AddCharmWithAuthorization", facadeArgs, result).SetArg(2, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
	got, err := client.AddCharmWithAuthorization(curl, origin, &macaroon.Macaroon{}, false)
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
	mockFacadeCaller.EXPECT().FacadeCall("CheckCharmPlacement", facadeArgs, &result).SetArg(2, actualResult).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
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
	mockFacadeCaller.EXPECT().FacadeCall("CheckCharmPlacement", facadeArgs, &result).Return(errors.Errorf("trap"))

	client := charms.NewClientWithFacade(mockFacadeCaller)
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

	p := params.CharmResourcesResults{
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

	mockFacadeCaller.EXPECT().FacadeCall("ListCharmResources", facadeArgs, &resolve).SetArg(2, p).Return(nil)

	client := charms.NewClientWithFacade(mockFacadeCaller)
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

	client := charms.NewClientWithFacade(mockFacadeCaller)
	vers := version.MustParse("2.6.6")
	// Test the sanity checks first.
	_, err := client.AddLocalCharm(charm.MustParseURL("cs:quantal/wordpress-1"), nil, false, vers)
	c.Assert(err, gc.ErrorMatches, `expected charm URL with local: schema, got "cs:quantal/wordpress-1"`)

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

	client := charms.NewClientWithFacade(mockFacadeCaller)

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
	client := charms.NewClientWithFacade(mockFacadeCaller)

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

	client := charms.NewClientWithFacade(mockFacadeCaller)

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
	client := charms.NewClientWithFacade(mockFacadeCaller)
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

	client := charms.NewClientWithFacade(mockFacadeCaller)

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

	client := charms.NewClientWithFacade(mockFacadeCaller)

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

	client := charms.NewClientWithFacade(mockFacadeCaller)

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
