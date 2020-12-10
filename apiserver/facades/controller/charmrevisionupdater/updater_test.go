// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/charm/v8/resource"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/controller/charmrevisionupdater"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/charmhub/transport"
	statemocks "github.com/juju/juju/state/mocks"
)

type updaterSuite struct{}

var _ = gc.Suite(&updaterSuite{})

func (s *updaterSuite) TestNewAuthSuccess(c *gc.C) {
	authoriser := apiservertesting.FakeAuthorizer{Controller: true}
	facadeCtx := facadeContextShim{state: nil, authorizer: authoriser}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(facadeCtx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updater, gc.NotNil)
}

func (s *updaterSuite) TestNewAuthFailure(c *gc.C) {
	authoriser := apiservertesting.FakeAuthorizer{Controller: false}
	facadeCtx := facadeContextShim{state: nil, authorizer: authoriser}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPI(facadeCtx)
	c.Assert(updater, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *updaterSuite) TestCharmhubUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubRefreshClient(ctrl)
	matcher := charmhubConfigMatcher{expected: []charmhubConfigExpected{
		{"charm-1", 22},
		{"charm-2", 41},
	}}
	client.EXPECT().Refresh(gomock.Any(), matcher).Return([]transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{Revision: 23},
			ID:     "charm-1",
			Name:   "mysql",
		},
		{
			Entity: transport.RefreshEntity{Revision: 42},
			ID:     "charm-2",
			Name:   "postgresql",
		},
	}, nil)

	state := makeState(c, ctrl, nil)
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "mysql", "charm-1", "app-1", 22),
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 41),
	}, nil).AnyTimes()
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:mysql-23")).Return(nil)
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	newClient := func(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return client, nil
	}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmhubUpdateWithResources(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	expectedResources := []resource.Resource{
		makeResource(c, "reza", 7, 5, "59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f"),
		makeResource(c, "rezb", 1, 6, "03130092073c5ac523ecb21f548b9ad6e1387d1cb05f3cb892fcc26029d01428afbe74025b6c567b6564a3168a47179a"),
	}
	resources := statemocks.NewMockResources(ctrl)
	resources.EXPECT().SetCharmStoreResources("app-1", expectedResources, gomock.Any()).Return(nil).AnyTimes()

	client := NewMockCharmhubRefreshClient(ctrl)
	matcher := charmhubConfigMatcher{expected: []charmhubConfigExpected{
		{"charm-3", 1},
	}}
	client.EXPECT().Refresh(gomock.Any(), matcher).Return([]transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{
				Resources: []transport.ResourceRevision{
					{
						Download: transport.ResourceDownload{
							HashSHA384: "59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f",
							Size:       5,
						},
						Name:     "reza",
						Revision: 7,
						Type:     "file",
					},
					{
						Download: transport.ResourceDownload{
							HashSHA384: "03130092073c5ac523ecb21f548b9ad6e1387d1cb05f3cb892fcc26029d01428afbe74025b6c567b6564a3168a47179a",
							Size:       6,
						},
						Name:     "rezb",
						Revision: 1,
						Type:     "file",
					},
				},
				Revision: 1,
			},
			ID:   "charm-3",
			Name: "resourcey",
		},
	}, nil)

	state := makeState(c, ctrl, resources)
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "resourcey", "charm-3", "app-1", 1),
	}, nil).AnyTimes()

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:resourcey-1")).Return(nil)

	newClient := func(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return client, nil
	}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmhubNoUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubRefreshClient(ctrl)
	matcher := charmhubConfigMatcher{expected: []charmhubConfigExpected{
		{"charm-2", 42},
	}}
	client.EXPECT().Refresh(gomock.Any(), matcher).Return([]transport.RefreshResponse{
		{
			Entity: transport.RefreshEntity{Revision: 42},
			ID:     "charm-2",
			Name:   "postgresql",
		},
	}, nil)

	state := makeState(c, ctrl, nil)
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "postgresql", "charm-2", "app-2", 42),
	}, nil).AnyTimes()
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("ch:postgresql-42")).Return(nil)

	newClient := func(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return client, nil
	}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, nil, newClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmNotInStore(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	charmhubClient := NewMockCharmhubRefreshClient(ctrl)
	charmhubClient.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{}, nil)

	state := makeState(c, ctrl, nil)
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "ch", "varnish", "charm-5", "app-1", 1),
		makeApplication(ctrl, "cs", "varnish", "charm-6", "app-2", 2),
	}, nil).AnyTimes()

	newCharmhubClient := func(st charmrevisionupdater.State, metadata map[string]string) (charmrevisionupdater.CharmhubRefreshClient, error) {
		return charmhubClient, nil
	}
	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, newFakeCharmstoreClient, newCharmhubClient)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}

func (s *updaterSuite) TestCharmstoreUpdate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	state := makeState(c, ctrl, nil)

	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "cs", "mysql", "charm-1", "app-1", 22),
		makeApplication(ctrl, "cs", "wordpress", "charm-2", "app-2", 26),
		makeApplication(ctrl, "cs", "varnish", "charm-3", "app-3", 5), // doesn't exist in store
	}, nil)

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:mysql-23")).Return(nil)
	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:wordpress-26")).Return(nil)

	updater, err := charmrevisionupdater.NewCharmRevisionUpdaterAPIState(state, newFakeCharmstoreClient, nil)
	c.Assert(err, jc.ErrorIsNil)

	result, err := updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)

	// Update mysql version and run update again.
	state.EXPECT().AllApplications().Return([]charmrevisionupdater.Application{
		makeApplication(ctrl, "cs", "mysql", "charm-1", "app-1", 23),
	}, nil)

	state.EXPECT().AddCharmPlaceholder(charm.MustParseURL("cs:mysql-23")).Return(nil)

	result, err = updater.UpdateLatestRevisions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
}
