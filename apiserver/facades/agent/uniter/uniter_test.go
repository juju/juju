// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/transform"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type uniterSuite struct {
	testing.IsolationSuite

	badTag names.Tag

	applicationService *MockApplicationService
	resolveService     *MockResolveService

	uniter *UniterAPI
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.badTag = nil
}

func (s *uniterSuite) TestClearResolvedUnauthorised(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/0")
	res, err := s.uniter.ClearResolved(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestClearResolvedNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().ClearResolved(gomock.Any(), unitName).Return(resolveerrors.UnitNotFound)

	res, err := s.uniter.ClearResolved(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestClearResolved(c *gc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().ClearResolved(gomock.Any(), unitName).Return(nil)

	res, err := s.uniter.ClearResolved(context.Background(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

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
	s.resolveService = NewMockResolveService(ctrl)

	s.uniter = &UniterAPI{
		applicationService: s.applicationService,
		resolveService:     s.resolveService,
		accessUnit: func() (common.AuthFunc, error) {
			return func(tag names.Tag) bool {
				return tag != s.badTag
			}, nil
		},
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
				modelUUID:       model.UUID(coretesting.ModelTag.Id()),
				modelType:       model.IAAS,
				watcherRegistry: s.watcherRegistry,
			},
		}

		return ctrl
	}
}

type uniterRelationSuite struct {
	testing.IsolationSuite

	wordpressAppTag  names.ApplicationTag
	authTag          names.Tag
	wordpressUnitTag names.UnitTag

	applicationService *MockApplicationService
	relationService    *MockRelationService
	statusService      *MockStatusService
	watcherRegistry    *MockWatcherRegistry

	uniter *UniterAPI
}

var _ = gc.Suite(&uniterRelationSuite{})

func (s *uniterRelationSuite) SetUpSuite(_ *gc.C) {
	s.wordpressAppTag = names.NewApplicationTag("wordpress")
	s.wordpressUnitTag = names.NewUnitTag("wordpress/0")
	s.authTag = s.wordpressUnitTag
}

func (s *uniterRelationSuite) TestRelation(c *gc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relKey := relationtesting.GenNewKey(c, relTag.Id())

	relUUID := relationtesting.GenRelationUUID(c)
	relID := 42

	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	s.expectGetRelationDetails(c, relUUID, relID, relTag)

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
	defer s.setupMocks(c).Finish()
	// arrange
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relTagFail := names.NewRelationTag("foo:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTagFail.Id()), "", relationerrors.RelationNotFound)

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
	relIDNotFound := -1
	relID := 31
	relIDUnexpectedAppName := 42

	s.expectGetRelationUUIDByID(relIDNotFound, relUUID, nil)
	s.expectGetRelationDetailsNotFound(relUUID)

	s.expectGetRelationUUIDByID(relID, relUUID, nil)
	s.expectGetRelationDetails(c, relUUID, relID, relTag)

	s.expectGetRelationUUIDByID(relIDUnexpectedAppName, relUUID, nil)
	s.expectGetRelationDetailsUnexpectedAppName(c, relUUID)

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
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetLocalRelationApplicationSettings(coreunit.Name(s.wordpressUnitTag.Id()), relUUID, appID, settings)

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

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
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
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
			},
		}, {
			description: "relation tag parsing fail",
			arg:         params.RelationUnit{Relation: "application-wordpress", Unit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "unit arg not unit nor application",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "user-foo"},
			arrange: func() {
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
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

func (s *uniterRelationSuite) TestReadSettingsForLocalApplication(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetLocalRelationApplicationSettings(coreunit.Name(s.wordpressUnitTag.Id()), relUUID, appID, settings)

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
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
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

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
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

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(remoteAppTag.Id(), appID)
	s.expectGetRemoteRelationApplicationSettings(relUUID, appID, settings)

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

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRemoteRelationApplicationSettings(relUUID, appID, settings)

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

func (s *uniterRelationSuite) TestRelationStatus(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.expectGetUnitUUID(s.wordpressUnitTag.Id(), unitUUID, nil)
	relTagOne := names.NewRelationTag("mysql:database wordpress:mysql")
	relTagTwo := names.NewRelationTag("redis:endpoint wordpress:endpoint")
	expectedRelationUnitStatus := []params.RelationUnitStatus{
		{
			RelationTag: relTagOne.String(),
			InScope:     true,
			Suspended:   false,
		}, {
			RelationTag: relTagTwo.String(),
			InScope:     true,
			Suspended:   true,
		},
	}
	s.expectedGetRelationsStatusForUnit(c, unitUUID, expectedRelationUnitStatus)

	// act
	args := params.Entities{Entities: []params.Entity{{Tag: s.wordpressUnitTag.String()}}}
	result, err := s.uniter.RelationsStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationUnitStatusResults{
		Results: []params.RelationUnitStatusResult{
			{RelationResults: expectedRelationUnitStatus},
		},
	})
}

// TestRelationsStatusUnitTagNotUnitNorApplication test that a valid tag not of
// the type application nor unit fails with unauthorized.
func (s *uniterRelationSuite) TestRelationsStatusUnitTagNotUnitNorApplication(c *gc.C) {
	// act
	args := params.Entities{Entities: []params.Entity{{Tag: "machine-0"}}}
	result, err := s.uniter.RelationsStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

// TestRelationsStatusUnitTagCannotAccess tests that a valid unit tag which is not
// the authorized one will fail.
func (s *uniterRelationSuite) TestRelationsStatusUnitTagCannotAccess(c *gc.C) {
	// act
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}}
	result, err := s.uniter.RelationsStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *uniterRelationSuite) TestSetRelationStatus(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relID := 42
	relationUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByID(relID, relationUUID, nil)
	relStatus := status.StatusInfo{
		Status: status.Joined,
		Since:  ptr(s.uniter.clock.Now()),
	}
	s.expectSetRelationStatus(s.wordpressUnitTag.Id(), relationUUID, relStatus)

	// act
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{
			{UnitTag: s.wordpressUnitTag.String(), RelationId: relID, Status: params.Joined},
		},
	}
	result, err := s.uniter.SetRelationStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, gc.DeepEquals, emptyErrorResults)
}

func (s *uniterRelationSuite) TestSetRelationStatusUnitTagNotValid(c *gc.C) {
	// act
	args := params.RelationStatusArgs{Args: []params.RelationStatusArg{{UnitTag: "foo"}}}
	result, err := s.uniter.SetRelationStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.ErrorMatches, "\"foo\" is not a valid tag")
}

func (s *uniterRelationSuite) TestSetRelationStatusRelationNotFound(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relID := 42
	relationUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByID(relID, relationUUID, relationerrors.RelationNotFound)

	// act
	args := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    s.wordpressUnitTag.String(),
		RelationId: relID,
		Status:     params.Joined,
	}}}
	result, err := s.uniter.SetRelationStatus(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Check(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *uniterRelationSuite) TestEnterScopeErrUnauthorized(c *gc.C) {
	c.Skip("Until unit PublicAddress() is implemented in its domain")
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	failRelTag := names.NewRelationTag("postgresql:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, failRelTag.Id()), "", relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// relation tag not parsable
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		// not found relation key
		{Relation: failRelTag.String(), Unit: "unit-wordpress-0"},
		// authorization on unit tag fails
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := s.uniter.EnterScope(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestEnterScope(c *gc.C) {
	c.Skip("Until unit PublicAddress() is implemented in its domain")
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	settings := map[string]string{"ingress-address": "x.x.x.x"}
	s.expectEnterScope(relUUID, coreunit.Name(s.wordpressUnitTag.Id()), settings, nil)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.EnterScope(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, gc.DeepEquals, emptyErrorResults)
}

// TestEnterScopeReturnsPotentialRelationUnitNotValid tests that if EnterScope
// returns PotentialRelationUnitNotValid the facade method still returns no
// error.
func (s *uniterRelationSuite) TestEnterScopeReturnsPotentialRelationUnitNotValid(c *gc.C) {
	c.Skip("Until unit PublicAddress() is implemented in its domain")
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	settings := map[string]string{"ingress-address": "x.x.x.x"}
	s.expectEnterScope(relUUID, coreunit.Name(s.wordpressUnitTag.Id()), settings,
		relationerrors.PotentialRelationUnitNotValid)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.EnterScope(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, gc.DeepEquals, emptyErrorResults)
}

// TestLeaveScopeFails tests for unauthorized errors, unit tag
// validation, and ensures the method works in bulk.
func (s *uniterRelationSuite) TestLeaveScopeFails(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	failRelTag := names.NewRelationTag("postgresql:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, failRelTag.Id()), "",
		relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// Not the authorized unit
		{Relation: "relation-42", Unit: "unit-foo-0"},
		// Invalid relation tag
		{Relation: "relation-42", Unit: s.wordpressUnitTag.String()},
		// Relation key not found
		{Relation: failRelTag.String(), Unit: s.wordpressUnitTag.String()},
		// Invalid unit tag
		{Relation: relTag.String(), Unit: "application-wordpress"},
	}}
	result, err := s.uniter.LeaveScope(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: `"application-wordpress" is not a valid unit tag`}},
		},
	})
}

func (s *uniterRelationSuite) TestWatchRelationUnits(c *gc.C) {
	// arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := relationtesting.GenRelationUUID(c)
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relKey, err := corerelation.ParseKeyFromTagString(relTag.String())
	c.Assert(err, jc.ErrorIsNil)
	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	watcherID := "watch1"
	unitUUIDs := []coreunit.UUID{
		unittesting.GenUnitUUID(c),
		unittesting.GenUnitUUID(c),
	}
	appUUIDs := []coreapplication.ID{
		applicationtesting.GenApplicationUUID(c),
	}
	unitName := coreunit.Name(s.wordpressUnitTag.Id())

	// Changes and expected results should matches.
	changes := watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{
			unitUUIDs[0].String(): {Version: 42},
		},
		AppChanged: map[string]int64{
			appUUIDs[0].String(): 47,
		},
		Departed: []string{unitUUIDs[1].String()},
	}
	expectedResult := params.RelationUnitsWatchResults{Results: []params.RelationUnitsWatchResult{
		{
			RelationUnitsWatcherId: watcherID,
			Changes: params.RelationUnitsChange{
				Changed: map[string]params.UnitSettings{
					unitUUIDs[0].String(): {Version: 42},
				},
				AppChanged: map[string]int64{
					appUUIDs[0].String(): 47,
				},
				Departed: []string{unitUUIDs[1].String()},
			},
		},
	}}
	s.expectWatchRelatedUnitsChange(unitName, relUUID, unitUUIDs, appUUIDs, watcherID, changes)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{relTag.String(), s.wordpressUnitTag.String()}},
	}
	result, err := s.uniter.WatchRelationUnits(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, expectedResult)
}

// TestWatchRelationUnitsFails tests for unauthorized errors, unit tag
// validation, and ensures the method works in bulk.
func (s *uniterRelationSuite) TestWatchRelationUnitsFails(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	failRelTag := names.NewRelationTag("postgresql:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, failRelTag.Id()), "",
		relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// Not the authorized unit
		{Relation: "relation-42", Unit: "unit-foo-0"},
		// Invalid relation tag
		{Relation: "relation-42", Unit: s.wordpressUnitTag.String()},
		// Relation key not found
		{Relation: failRelTag.String(), Unit: s.wordpressUnitTag.String()},
		// Invalid unit tag
		{Relation: relTag.String(), Unit: "application-wordpress"},
	}}
	result, err := s.uniter.WatchRelationUnits(context.Background(), args)

	// assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResults{
		Results: []params.RelationUnitsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestWatchUnitRelations(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	unitUUID := unittesting.GenUnitUUID(c)
	watcherID := "watcher-id"
	relationKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relationChanges := make(chan []string, 1)
	change := []string{relationKey.String()}
	relationChanges <- change
	watch := watchertest.NewMockStringsWatcher(relationChanges)
	s.expectGetUnitUUID(s.wordpressUnitTag.Id(), unitUUID, nil)
	s.expectWatchLifeSuspendedStatus(unitUUID, watch, nil)
	s.expectWatcherRegistry(watcherID, watch, nil)

	// Act
	results, err := s.uniter.WatchUnitRelations(context.Background(),
		params.Entities{
			Entities: []params.Entity{
				{Tag: s.wordpressUnitTag.String()},
			}})

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: watcherID,
				Changes:          change,
				Error:            nil,
			},
		},
	})
}

func (s *uniterRelationSuite) TestWatchUnitRelationsErrUnauthorized(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	args := params.Entities{Entities: []params.Entity{
		// Bad unit tag.
		{Tag: "application"},
		// Not the authorized unit
		{Tag: "unit-mysql-4"},
	}}

	// Act
	results, err := s.uniter.WatchUnitRelations(context.Background(), args)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

}

func (s *uniterRelationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

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
		modelUUID:         model.UUID(coretesting.ModelTag.Id()),
		modelType:         model.IAAS,
		accessApplication: appAuthFunc,
		accessUnit:        unitAuthFunc,
		auth:              authorizer,
		clock:             testclock.NewClock(time.Now()),
		logger:            loggertesting.WrapCheckLog(c),

		applicationService: s.applicationService,
		relationService:    s.relationService,
		statusService:      s.statusService,
		watcherRegistry:    s.watcherRegistry,
	}

	return ctrl
}

func (s *uniterRelationSuite) expectGetRelationUUIDByKey(key corerelation.Key, relUUID corerelation.UUID, err error) {
	s.relationService.EXPECT().GetRelationUUIDByKey(gomock.Any(), key).Return(relUUID, err)
}

func (s *uniterRelationSuite) expectGetRelationDetails(c *gc.C, relUUID corerelation.UUID, relID int, relTag names.RelationTag) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relUUID,
		ID:   relID,
		Key:  relationtesting.GenNewKey(c, relTag.Id()),
		Endpoints: []relation.Endpoint{
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

func (s *uniterRelationSuite) expectGetRelationDetailsNotFound(relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{}, relationerrors.RelationNotFound)
}

func (s *uniterRelationSuite) expectGetRelationDetailsUnexpectedAppName(c *gc.C, relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relationtesting.GenRelationUUID(c),
		ID:   101,
		Endpoints: []relation.Endpoint{
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

func (s *uniterRelationSuite) expectGetApplicationIDByName(appName string, id coreapplication.ID) {
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(id, nil)
}

func (s *uniterRelationSuite) expectGetLocalRelationApplicationSettings(unitName coreunit.Name, uuid corerelation.UUID, id coreapplication.ID, settings map[string]string) {
	s.relationService.EXPECT().GetLocalRelationApplicationSettings(gomock.Any(), unitName, uuid, id).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetRemoteRelationApplicationSettings(uuid corerelation.UUID, id coreapplication.ID, settings map[string]string) {
	s.relationService.EXPECT().GetRemoteRelationApplicationSettings(gomock.Any(), uuid, id).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnit(relUUID corerelation.UUID, uuid corerelation.UnitUUID, unitTagID string) {
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, coreunit.Name(unitTagID)).Return(uuid, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnitSettings(uuid corerelation.UnitUUID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationUnitSettings(gomock.Any(), uuid).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetUnitUUID(name string, unitUUID coreunit.UUID, err error) {
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name(name)).Return(unitUUID, err)
}

func (s *uniterRelationSuite) expectedGetRelationsStatusForUnit(c *gc.C, uuid coreunit.UUID, input []params.RelationUnitStatus) {
	expectedStatuses := make([]relation.RelationUnitStatus, len(input))
	for i, in := range input {
		// The caller created the tag, programing error if this fails.
		tag, _ := names.ParseRelationTag(in.RelationTag)
		expectedStatuses[i] = relation.RelationUnitStatus{
			Key:       relationtesting.GenNewKey(c, tag.Id()),
			InScope:   in.InScope,
			Suspended: in.Suspended,
		}
	}
	s.relationService.EXPECT().GetRelationsStatusForUnit(gomock.Any(), uuid).Return(expectedStatuses, nil)
}

func (s *uniterRelationSuite) expectGetRelationUUIDByID(relID int, relUUID corerelation.UUID, err error) {
	s.relationService.EXPECT().GetRelationUUIDByID(gomock.Any(), relID).Return(relUUID, err)
}

func (s *uniterRelationSuite) expectSetRelationStatus(unitName string, relUUID corerelation.UUID, relStatus status.StatusInfo) {
	name, _ := coreunit.NewName(unitName)
	s.statusService.EXPECT().SetRelationStatus(gomock.Any(), name, relUUID, relStatus).Return(nil)
}

func (s *uniterRelationSuite) expectEnterScope(uuid corerelation.UUID, name coreunit.Name, settings map[string]string, err error) {
	s.relationService.EXPECT().EnterScope(gomock.Any(), uuid, name, settings, gomock.Any()).Return(err)
}

func (s *uniterRelationSuite) expectWatchLifeSuspendedStatus(unitUUID coreunit.UUID, watch watcher.StringsWatcher, err error) {
	s.relationService.EXPECT().WatchLifeSuspendedStatus(gomock.Any(), unitUUID).Return(watch, err)
}

func (s *uniterRelationSuite) expectWatcherRegistry(watchID string, watch *watchertest.MockStringsWatcher, err error) {
	s.watcherRegistry.EXPECT().Register(watch).Return(watchID, err).AnyTimes()
}

func (s *uniterRelationSuite) expectWatchRelatedUnitsChange(
	unitName coreunit.Name,
	relUUID corerelation.UUID,
	unitUUIDs []coreunit.UUID,
	appUUIDS []coreapplication.ID,
	watcherID string,
	changes watcher.RelationUnitsChange,
) {
	channel := make(chan []string, 1)
	mockWatcher := watchertest.NewMockStringsWatcher(channel)
	channel <- append(transform.Slice(unitUUIDs, relation.EncodeUnitUUID), transform.Slice(appUUIDS,
		relation.EncodeApplicationUUID)...)
	close(channel)
	s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), unitName, relUUID).Return(mockWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return(watcherID, nil)
	s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs, appUUIDS).Return(changes, nil)
}

type commitHookChangesSuite struct {
	testing.IsolationSuite

	applicationService *MockApplicationService
	relationService    *MockRelationService

	uniter *UniterAPI
}

var _ = gc.Suite(&commitHookChangesSuite{})

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettings(c *gc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitTag := names.NewUnitTag("wordpress/0")
	relTag := names.NewRelationTag("wordpress:db mysql:db")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	appSettings := map[string]string{"wanda": "firebaugh", "deleteme": ""}
	unitSettings := map[string]string{"wanda": "firebaugh", "deleteme": ""}
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID)
	s.expectGetRelationUnit(relUUID, relUnitUUID, unitTag.Id())
	s.expectedSetRelationApplicationAndUnitSettings(coreunit.Name(unitTag.Id()), relUnitUUID, appSettings, unitSettings)
	canAccess := func(tag names.Tag) bool {
		return true
	}
	arg := params.RelationUnitSettings{
		Relation:            relTag.String(),
		Unit:                unitTag.String(),
		Settings:            unitSettings,
		ApplicationSettings: appSettings,
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(context.Background(), arg, canAccess)

	// assert
	c.Assert(err, gc.IsNil)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsBadUnitTag(c *gc.C) {
	// arrange
	arg := params.RelationUnitSettings{
		Unit: "machine-9",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(context.Background(), arg, nil)

	// assert
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsFailCanAccess(c *gc.C) {
	// arrange
	canAccess := func(tag names.Tag) bool {
		return false
	}
	arg := params.RelationUnitSettings{
		Unit: "unit-failauth-2",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(context.Background(), arg, canAccess)

	// assert
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsBadRelationTag(c *gc.C) {
	// arrange
	canAccess := func(tag names.Tag) bool {
		return true
	}
	arg := params.RelationUnitSettings{
		Unit:     "unit-wordpress-2",
		Relation: "failme",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(context.Background(), arg, canAccess)

	// assert
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)

	s.uniter = &UniterAPI{
		logger: loggertesting.WrapCheckLog(c),

		applicationService: s.applicationService,
		relationService:    s.relationService,
	}

	return ctrl
}

func (s *commitHookChangesSuite) expectGetRelationUUIDByKey(key corerelation.Key, relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationUUIDByKey(gomock.Any(), key).Return(relUUID, nil)
}

func (s *commitHookChangesSuite) expectGetRelationUnit(relUUID corerelation.UUID, uuid corerelation.UnitUUID, unitTagID string) {
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, coreunit.Name(unitTagID)).Return(uuid, nil)
}

func (s *commitHookChangesSuite) expectedSetRelationApplicationAndUnitSettings(unitName coreunit.Name, uuid corerelation.UnitUUID, appSettings, unitSettings map[string]string) {
	s.relationService.EXPECT().SetRelationApplicationAndUnitSettings(gomock.Any(), unitName, uuid, appSettings, unitSettings).Return(nil)
}
