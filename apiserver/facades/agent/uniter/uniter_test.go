// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
	internalrelation "github.com/juju/juju/internal/relation"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type uniterSuite struct {
	testing.IsolationSuite

	applicationService *MockApplicationService

	uniter *UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) TestCharmArchiveSha256Local(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.LocalSource,
		Name:     "foo",
		Revision: 1,
	}).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "local:foo-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Charmhub(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 1,
	}).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Errors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 1,
	}).Return("", applicationerrors.CharmNotFound)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 2,
	}).Return("", applicationerrors.CharmNotFound)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 3,
	}).Return("", applicationerrors.CharmNotResolved)

	results, err := s.uniter.CharmArchiveSha256(context.Background(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
			{URL: "ch:foo-2"},
			{URL: "ch:foo-3"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: &params.Error{Message: `charm "foo-1" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-2" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-3" not available`, Code: params.CodeNotYetAvailable}},
		},
	})
}

func (s *uniterSuite) TestLeadershipSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.uniter.Merge(context.Background(), struct{}{}, struct{}{})
	s.uniter.Read(context.Background(), struct{}{}, struct{}{})
	s.uniter.WatchLeadershipSettings(context.Background(), struct{}{}, struct{}{})
}

func (s *uniterSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)

	s.uniter = &UniterAPI{
		applicationService: s.applicationService,
	}

	return ctrl
}

type leadershipSettings interface {
	// Merge merges in the provided leadership settings. Only leaders for
	// the given service may perform this operation.
	Merge(ctx context.Context, bulkArgs params.MergeLeadershipSettingsBulkParams) (params.ErrorResults, error)

	// Read reads leadership settings for the provided service ID. Any
	// unit of the service may perform this operation.
	Read(ctx context.Context, bulkArgs params.Entities) (params.GetLeadershipSettingsBulkResults, error)

	// WatchLeadershipSettings will block the caller until leadership settings
	// for the given service ID change.
	WatchLeadershipSettings(ctx context.Context, bulkArgs params.Entities) (params.NotifyWatchResults, error)
}

type leadershipUniterSuite struct {
	testing.IsolationSuite

	watcherRegistry *MockWatcherRegistry

	uniter leadershipSettings

	setupMocks func(c *gc.C) *gomock.Controller
}

func (s *leadershipUniterSuite) TestLeadershipSettingsMerge(c *gc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.Merge(context.Background(), params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: "app1",
				Settings: params.Settings{
					"key1": "value1",
				},
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *leadershipUniterSuite) TestLeadershipSettingsRead(c *gc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.Read(context.Background(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: "app1",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.GetLeadershipSettingsBulkResults{
		Results: []params.GetLeadershipSettingsResult{{}},
	})
}

func (s *leadershipUniterSuite) TestLeadershipSettingsWatchLeadershipSettings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.WatchLeadershipSettings(context.Background(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: "app1",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "watcher1",
		}},
	})
}

type uniterv19Suite struct {
	leadershipUniterSuite
}

var _ = gc.Suite(&uniterv19Suite{})

func (s *uniterv19Suite) SetUpTest(c *gc.C) {
	s.setupMocks = func(c *gc.C) *gomock.Controller {
		ctrl := gomock.NewController(c)

		s.watcherRegistry = NewMockWatcherRegistry(ctrl)
		s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("watcher1", nil).AnyTimes()

		s.uniter = &UniterAPIv19{
			UniterAPIv20: &UniterAPIv20{
				UniterAPI: &UniterAPI{
					watcherRegistry: s.watcherRegistry,
				},
			},
		}

		return ctrl
	}
}

type uniterv20Suite struct {
	leadershipUniterSuite
}

var _ = gc.Suite(&uniterv20Suite{})

func (s *uniterv20Suite) SetUpTest(c *gc.C) {
	s.setupMocks = func(c *gc.C) *gomock.Controller {
		ctrl := gomock.NewController(c)

		s.watcherRegistry = NewMockWatcherRegistry(ctrl)
		s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("watcher1", nil).AnyTimes()

		s.uniter = &UniterAPIv20{
			UniterAPI: &UniterAPI{
				watcherRegistry: s.watcherRegistry,
			},
		}

		return ctrl
	}
}

type uniterRelationSuite struct {
	testing.IsolationSuite

	wordpressAppTag   names.ApplicationTag
	authTag           names.Tag
	leadershipChecker *fakeLeadershipChecker
	wordpressUnitTag  names.UnitTag

	applicationService *MockApplicationService
	modelInfoService   *MockModelInfoService
	relationService    *MockRelationService

	uniter *UniterAPI
}

var _ = gc.Suite(&uniterRelationSuite{})

func (s *uniterRelationSuite) SetUpSuite(_ *gc.C) {
	s.wordpressAppTag = names.NewApplicationTag("wordpress")
	s.wordpressUnitTag = names.NewUnitTag("wordpress/0")
	s.authTag = s.wordpressUnitTag
}

func (s *uniterRelationSuite) SetUpTest(_ *gc.C) {
	// Ensure each test starts without the auth unit being the leader.
	s.leadershipChecker = &fakeLeadershipChecker{}
}

func (s *uniterRelationSuite) TestRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")

	relUUID := relationtesting.GenRelationUUID(c)
	relID := 42
	details := relation.RelationDetails{
		Life: life.Alive,
		UUID: relUUID,
		ID:   relID,
		Key:  relTag.Id(),
		Endpoint: []relation.Endpoint{
			{
				ApplicationName: "wordpress",
				Relation: charm.Relation{
					Name:      "database",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
			{
				ApplicationName: "mysql",
				Relation: charm.Relation{
					Name:      "mysql",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}

	s.expectModelUUID(c)
	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectGetRelationDetailsForUnit(relUUID, details)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.Relation(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{
				Id:   relID,
				Key:  relTag.Id(),
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation: params.CharmRelation{
						Name:      "database",
						Role:      string(charm.RoleRequirer),
						Interface: "mysql",
						Optional:  false,
						Limit:     0,
						Scope:     string(charm.ScopeGlobal),
					},
				},
				OtherApplication: params.RelatedApplicationDetails{
					ApplicationName: "mysql",
					ModelUUID:       coretesting.ModelTag.Id(),
				},
			},
		},
	})
}

// TestRelationUnauthorized tests the different scenarios where
// ErrUnauthorized will be returned. It also tests the bulk
// functionality of the Relation facade method.
func (s *uniterRelationSuite) TestRelationUnauthorized(c *gc.C) {
	// arrange
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relTagFail := names.NewRelationTag("foo:database wordpress:mysql")
	s.expectGetRelationUUIDFromKey(corerelation.Key(relTagFail.Id()), "", errors.NotFound)

	// act
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			// "relation-42" is not a valid relation key.
			{Relation: "relation-42", Unit: "unit-wordpress-0"},
			// "user-foo" is not a parsable unit tag.
			{Unit: "user-foo"},
			// "unit-mysql-0" is not the authorizing tag, though
			// is part of the relation.
			{Relation: relTag.String(), Unit: "unit-mysql-0"},
			// Not found relation with correct unit.
			{Relation: relTagFail.String(), Unit: "unit-wordpress-0"},
		},
	}
	result, err := s.uniter.Relation(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestRelationById(c *gc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relID := 37
	relIDNotFound := -1
	relIDUnexpectedAppName := 42

	s.expectModelUUID(c)
	s.expectGetRelationDetails(relUUID, relID, relTag)
	s.expectGetRelationDetailsNotFound(relIDNotFound)
	s.expectGetRelationDetailsUnexpectedAppName(c, relIDUnexpectedAppName)
	args := params.RelationIds{
		RelationIds: []int{
			// The relation ID does not exist: ErrUnauthorized.
			relIDNotFound,
			// Successful result.
			relID,
			// The auth application is not part of the relation: ErrUnauthorized.
			relIDUnexpectedAppName,
		},
	}
	result, err := s.uniter.RelationById(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:   relID,
				Key:  relTag.Id(),
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation: params.CharmRelation{
						Name:      "database",
						Role:      string(charm.RoleRequirer),
						Interface: "mysql",
						Optional:  false,
						Limit:     0,
						Scope:     string(charm.ScopeGlobal),
					},
				},
				OtherApplication: params.RelatedApplicationDetails{
					ApplicationName: "mysql",
					ModelUUID:       coretesting.ModelTag.Id(),
				},
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsApplication(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	s.leadershipChecker.isLeader = true
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectRelationEndpoints(relUUID, []internalrelation.Endpoint{})
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsUnit(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectGetRelationUnit(relUUID, relUnitUUID, s.wordpressUnitTag.Id())
	s.expectGetRelationUnitSettings(relUnitUUID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.ReadSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsErrUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)

	errAuthTests := []struct {
		description string
		arg         params.RelationUnit
		arrange     func()
	}{
		{
			description: "unauthorized unit",
			arg:         params.RelationUnit{Relation: "relation-42", Unit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "remote unit, valid in relation, not this call",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "unit-mysql-0"},
			arrange: func() {
				s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
			},
		}, {
			description: "relation tag parsing fail",
			arg:         params.RelationUnit{Relation: "application-wordpress", Unit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "unit arg not unit nor application",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "user-foo"},
			arrange: func() {
				s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
			},
		}, {
			description: "read application settings when not the leader",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "application-wordpress"},
			arrange: func() {
				s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
				s.expectRelationEndpoints(relUUID, []internalrelation.Endpoint{})
			},
		},
	}

	for i, tc := range errAuthTests {
		c.Logf("test %d: %s", i, tc.description)
		tc.arrange()
		args := params.RelationUnits{RelationUnits: []params.RelationUnit{tc.arg}}
		result, err := s.uniter.ReadSettings(context.Background(), args)
		if c.Check(err, jc.ErrorIsNil) {
			if !c.Check(result.Results, gc.HasLen, 1) {
				continue
			}
			c.Check(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
		}
	}
}

func (s *uniterRelationSuite) TestReadSettingsForApplicationInPeerRelation(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	s.leadershipChecker.isLeader = true
	relTag := names.NewRelationTag("wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectRelationEndpoints(relUUID, []internalrelation.Endpoint{
		{ApplicationName: "wordpress", Relation: charm.Relation{Role: charm.RolePeer}},
	})
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadLocalApplicationSettingsWhenNotLeader(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectRelationEndpoints(relUUID, []internalrelation.Endpoint{})

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestReadRemoteSettingsErrUnauthorized(c *gc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)

	errAuthTests := []struct {
		description string
		arg         params.RelationUnitPair
		arrange     func()
	}{
		{
			description: "local unit fails parsing",
			arg:         params.RelationUnitPair{LocalUnit: "foo-0"},
			arrange:     func() {},
		}, {
			description: "remote unit fails parsing",
			arg:         params.RelationUnitPair{LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: ""},
			arrange:     func() {},
		}, {
			description: "local unit cannot access",
			arg:         params.RelationUnitPair{LocalUnit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "bad relation tag",
			arg:         params.RelationUnitPair{Relation: "failme-76", LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: "unit-one-2"},
			arrange:     func() {},
		}, {
			description: "remote unit tag not unit nor application kinds",
			arg:         params.RelationUnitPair{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: "machine-2"},
			arrange: func() {
				s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
			},
		},
	}

	for i, tc := range errAuthTests {
		c.Logf("test %d: %s", i, tc.description)
		tc.arrange()
		args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{tc.arg}}
		result, err := s.uniter.ReadRemoteSettings(context.Background(), args)
		if c.Check(err, jc.ErrorIsNil) {
			if !c.Check(result.Results, gc.HasLen, 1) {
				continue
			}
			c.Check(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
		}
	}
}

// TestReadRemoteSettingsForUnit tests a local unit's ability to read the
// unit settings from the unit at the other end of the relation.
// local = wordpress
// remote = mysql
func (s *uniterRelationSuite) TestReadRemoteSettingsForUnit(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	remoteUnitTag := names.NewUnitTag("mysql/2")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectGetRelationUnit(relUUID, relUnitUUID, remoteUnitTag.Id())
	s.expectGetRelationUnitSettings(relUnitUUID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: remoteUnitTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

// TestReadRemoteSettingsForApplication tests a local unit's ability to read the
// application settings from the application at the other end of the relation.
// local = wordpress
// remote = mysql
func (s *uniterRelationSuite) TestReadRemoteSettingsForApplication(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	remoteAppTag := names.NewApplicationTag("mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(remoteAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: remoteAppTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

// TestReadRemoteApplicationSettingsWithLocalApplication tests a local unit's
// ability to read the application settings of its own application via the
// ReadRemoteSettings method .
// local = wordpress
func (s *uniterRelationSuite) TestReadRemoteApplicationSettingsWithLocalApplication(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDFromKey(corerelation.Key(relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.relationService = NewMockRelationService(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.wordpressUnitTag.Id()
		}, nil
	}

	appAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.wordpressAppTag.Id()
		}, nil
	}

	authorizer := &apiservertesting.FakeAuthorizer{
		Tag:        s.authTag,
		Controller: true,
	}

	s.uniter = &UniterAPI{
		accessApplication: appAuthFunc,
		accessUnit:        unitAuthFunc,
		auth:              authorizer,
		leadershipChecker: s.leadershipChecker,

		applicationService: s.applicationService,
		modelInfoService:   s.modelInfoService,
		relationService:    s.relationService,
	}

	return ctrl
}

func (s *uniterRelationSuite) expectModelUUID(c *gc.C) {
	modelInfo := model.ModelInfo{
		UUID: model.UUID(coretesting.ModelTag.Id()),
	}
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(modelInfo, nil)
}

func (s *uniterRelationSuite) expectGetRelationUUIDFromKey(key corerelation.Key, relUUID corerelation.UUID, err error) {
	s.relationService.EXPECT().GetRelationUUIDFromKey(gomock.Any(), key).Return(relUUID, err)
}

func (s *uniterRelationSuite) expectGetRelationDetails(relUUID corerelation.UUID, relID int, relTag names.RelationTag) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relUUID,
		ID:   relID,
		Key:  relTag.Id(),
		Endpoint: []relation.Endpoint{
			{
				ApplicationName: "wordpress",
				Relation: charm.Relation{
					Name:      "database",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
			{
				ApplicationName: "mysql",
				Relation: charm.Relation{
					Name:      "mysql",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}, nil)
}

func (s *uniterRelationSuite) expectGetRelationDetailsForUnit(
	relUUID corerelation.UUID,
	details relation.RelationDetails,
) {
	s.relationService.EXPECT().GetRelationDetailsForUnit(gomock.Any(), relUUID, s.wordpressUnitTag.Id()).Return(details, nil)
}

func (s *uniterRelationSuite) expectGetRelationDetailsNotFound(relID int) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relID).Return(relation.RelationDetails{}, errors.NotFound)
}

func (s *uniterRelationSuite) expectGetRelationDetailsUnexpectedAppName(c *gc.C, relID int) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relationtesting.GenRelationUUID(c),
		ID:   relID,
		Endpoint: []relation.Endpoint{
			{
				ApplicationName: "failure-application",
				Relation: charm.Relation{
					Name:      "database",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
			{
				ApplicationName: "mysql",
				Relation: charm.Relation{
					Name:      "mysql",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}, nil)
}

func (s *uniterRelationSuite) expectRelationEndpoints(uuid corerelation.UUID, eps []internalrelation.Endpoint) {
	s.relationService.EXPECT().GetRelationEndpoints(gomock.Any(), uuid).Return(eps, nil)
}

func (s *uniterRelationSuite) expectGetApplicationIDByName(appName string, id coreapplication.ID) {
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(id, nil)
}

func (s *uniterRelationSuite) expectGetRelationApplicationSettings(uuid corerelation.UUID, id coreapplication.ID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationApplicationSettings(gomock.Any(), uuid, id).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnit(relUUID corerelation.UUID, uuid corerelation.UnitUUID, unitTagID string) {
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, unitTagID).Return(uuid, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnitSettings(uuid corerelation.UnitUUID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationUnitSettings(gomock.Any(), uuid).Return(settings, nil)
}

type fakeLeadershipChecker struct {
	isLeader bool
}

// Temporarily make the static analysis happy until the tests are finished
// and using leadership.
type token struct {
	isLeader          bool
	unit, application string
}

func (t *token) Check() error {
	if !t.isLeader {
		return leadership.NewNotLeaderError(t.unit, t.application)
	}
	return nil
}

func (f *fakeLeadershipChecker) LeadershipCheck(applicationName, unitName string) leadership.Token {
	return &token{isLeader: f.isLeader, unit: unitName, application: applicationName}
}
