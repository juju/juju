// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms_test

import (
	"archive/zip"
	context "context"
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/charms"
	apicharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testcharms"
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
	got, err := client.ResolveCharms(context.Background(), args)
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
	got, err := client.GetDownloadInfo(context.Background(), curl, origin)
	c.Assert(err, gc.IsNil)

	want := charms.DownloadInfo{
		URL:    "http://someplace.com",
		Origin: origin,
	}

	c.Assert(got, gc.DeepEquals, want)
}

func (s *addCharmSuite) TestAddCharm(c *gc.C) {
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
	got, err := client.AddCharm(context.Background(), curl, origin, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.DeepEquals, origin)
}

func (s *charmsMockSuite) TestCheckCharmPlacement(c *gc.C) {
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
	err := client.CheckCharmPlacement(context.Background(), "winnie", charm.MustParseURL("poo"))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *charmsMockSuite) TestCheckCharmPlacementError(c *gc.C) {
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
	err := client.CheckCharmPlacement(context.Background(), "winnie", charm.MustParseURL("poo"))
	c.Assert(err, gc.ErrorMatches, "trap")
}

func (s *charmsMockSuite) TestListCharmResources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	curl := "a-charm"
	noChannelParamsOrigin := params.CharmOrigin{Source: "charm-hub"}

	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)

	facadeArgs := params.CharmURLAndOrigins{
		Entities: []params.CharmURLAndOrigin{
			{CharmURL: curl, Origin: noChannelParamsOrigin},
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
	got, err := client.ListCharmResources(context.Background(), curl, origin)
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

func (s *charmsMockSuite) TestZipHasNoHooksNorDispatch(c *gc.C) {
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

// TestZipHasSingleHook tests that an archive containing only a single hook
// file (and no zip entry for the hooks directory) is still validated as a
// charm with hooks.
func (s *charmsMockSuite) TestZipHasSingleHook(c *gc.C) {
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()

	zipWriter := zip.NewWriter(tempFile)
	// add a single install hook
	_, err = zipWriter.Create("hooks/install")
	c.Assert(err, jc.ErrorIsNil)
	err = zipWriter.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Verify created zip is as expected
	zipReader, err := zip.OpenReader(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zipReader.File), gc.Equals, 1)
	c.Assert(zipReader.File[0].Name, gc.Equals, "hooks/install")
	c.Assert(zipReader.File[0].Mode().IsRegular(), jc.IsTrue)

	// Verify this is validated as having a hook
	hasHooks, err := (*charms.HasHooksOrDispatch)(tempFile.Name())
	c.Check(err, jc.ErrorIsNil)
	c.Check(hasHooks, jc.IsTrue)
}

// TestZipEmptyHookDir tests that an archive containing only an empty hooks
// directory is not validated as a charm with hooks.
func (s *charmsMockSuite) TestZipEmptyHookDir(c *gc.C) {
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()

	zipWriter := zip.NewWriter(tempFile)
	// add an empty hooks directory
	_, err = zipWriter.Create("hooks/")
	c.Assert(err, jc.ErrorIsNil)
	err = zipWriter.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Verify created zip is as expected
	zipReader, err := zip.OpenReader(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zipReader.File), gc.Equals, 1)
	c.Assert(zipReader.File[0].Name, gc.Equals, "hooks/")
	c.Assert(zipReader.File[0].Mode().IsDir(), jc.IsTrue)

	// Verify this is validated as having no hooks
	hasHooks, err := (*charms.HasHooksOrDispatch)(tempFile.Name())
	c.Check(err, jc.ErrorIsNil)
	c.Check(hasHooks, jc.IsFalse)
}

// TestZipSubfileHook tests that an archive containing nested subfiles inside
// the hooks directory (i.e. not in the top level) is not validated as a charm
// with hooks.
func (s *charmsMockSuite) TestZipSubfileHook(c *gc.C) {
	tempFile, err := os.CreateTemp(c.MkDir(), "charm")
	c.Assert(err, jc.ErrorIsNil)
	defer tempFile.Close()

	zipWriter := zip.NewWriter(tempFile)
	// add some files inside a subdir of hooks
	_, err = zipWriter.Create("hooks/foo/bar.sh")
	c.Assert(err, jc.ErrorIsNil)
	_, err = zipWriter.Create("hooks/hooks/install")
	c.Assert(err, jc.ErrorIsNil)
	_, err = zipWriter.Create("foo/hooks/install")
	c.Assert(err, jc.ErrorIsNil)
	err = zipWriter.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Verify created zip is as expected
	zipReader, err := zip.OpenReader(tempFile.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(zipReader.File), gc.Equals, 3)
	for _, f := range zipReader.File {
		c.Assert(f.Mode().IsRegular(), jc.IsTrue)
	}

	// Verify this is not validated as having a hook
	hasHooks, err := (*charms.HasHooksOrDispatch)(tempFile.Name())
	c.Check(err, jc.ErrorIsNil)
	c.Check(hasHooks, jc.IsFalse)
}
