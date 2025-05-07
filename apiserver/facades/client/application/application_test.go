// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreassumes "github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/testing"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type applicationSuite struct {
	baseSuite

	application *MockApplication
	charm       *MockCharm
}

var _ = tc.Suite(&applicationSuite{})

func (s *applicationSuite) TestSetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// The amount of requests to set a charm config is ridiculous.
	// We're requesting the new charm and the old charm, more than we require.
	// We should fix this when we refactor the application service.

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *tc.C, config state.SetCharmConfig) {
		result = config
	})
	s.expectSetCharmWithTrust(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
			"trust":        "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
		ForceUnits:         true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.CharmOrigin, tc.DeepEquals, &state.CharmOrigin{
		Type:     "charm",
		Source:   "local",
		Revision: ptr(42),
		Channel: &state.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Platform: &state.Platform{
			OS:           "ubuntu",
			Channel:      "24.04",
			Architecture: "amd64",
		},
	})
}

func (s *applicationSuite) TestSetCharmInvalidCharmOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin:     &params.CharmOrigin{},
	})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestSetCharmApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplicationNotFound(c, "foo")

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
	})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmEndpointBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectSpaceName(c, "bar")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *tc.C, config state.SetCharmConfig) {
		result = config
	})
	s.expectSetCharmWithTrust(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
			"trust":        "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
		EndpointBindings: map[string]string{
			"baz": "bar",
		},
		ForceUnits: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.CharmOrigin, tc.DeepEquals, &state.CharmOrigin{
		Type:     "charm",
		Source:   "local",
		Revision: ptr(42),
		Channel: &state.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Platform: &state.Platform{
			OS:           "ubuntu",
			Channel:      "24.04",
			Architecture: "amd64",
		},
	})
}

func (s *applicationSuite) TestSetCharmEndpointBindingsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectSpaceNameNotFound(c, "bar")

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
			"trust":        "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
		EndpointBindings: map[string]string{
			"baz": "bar",
		},
	})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmGetCharmNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharmNotFound(c, "foo")

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
			"trust":        "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
	})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmInvalidConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"blach!": "foo",
			"trust":  "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
	})
	c.Assert(err, tc.ErrorMatches, `parsing config settings: unknown option "blach!"`)
}

func (s *applicationSuite) TestSetCharmWithoutTrust(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *tc.C, config state.SetCharmConfig) {
		result = config
	})

	// There is no expectation that the trust is set on the application.

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
		ForceUnits:         true,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.CharmOrigin, tc.DeepEquals, &state.CharmOrigin{
		Type:     "charm",
		Source:   "local",
		Revision: ptr(42),
		Channel: &state.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Platform: &state.Platform{
			OS:           "ubuntu",
			Channel:      "24.04",
			Architecture: "amd64",
		},
	})
}

func (s *applicationSuite) TestSetCharmFormatDowngrade(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheckDowngrade(c, "foo")

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin: &params.CharmOrigin{
			Type:   "charm",
			Source: "local",
			Base: params.Base{
				Name:    "ubuntu",
				Channel: "24.04",
			},
			Architecture: "amd64",
			Revision:     ptr(42),
			Track:        ptr("1.0"),
			Risk:         "stable",
		},
		ConfigSettings: map[string]string{
			"stringOption": "foo",
			"trust":        "true",
		},
		ConfigSettingsYAML: `foo: {"stringOption": "bar"}`,
	})
	c.Assert(err, tc.ErrorMatches, "cannot downgrade from v2 charm format to v1")
}

func (s *applicationSuite) TestDeploy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 2)
	s.expectCharmMeta("foo", nil, 8)
	s.expectAddApplication()
	s.expectCreateApplicationForDeploy("foo", nil)

	errorResults, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				ApplicationName: "foo",
				CharmURL:        "local:foo-42",
				CharmOrigin: &params.CharmOrigin{
					Type:   "charm",
					Source: "local",
					Base: params.Base{
						Name:    "ubuntu",
						Channel: "24.04",
					},
					Architecture: "amd64",
					Revision:     ptr(42),
					Track:        ptr("1.0"),
					Risk:         "stable",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errorResults.Results, tc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, tc.IsNil)
}

// TestDeployWithResources test the scenario of deploying
// local charms, or charms via bundles that have resources.
// Deploy rather than DeployFromRepository is called by the
// clients. In this case PendingResources, uuids, must be
// provided for all charm resources.
func (s *applicationSuite) TestDeployWithPendingResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	resourceUUID := testing.GenResourceUUID(c)
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 2)
	s.expectCharmMeta("foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
	}, 8)
	s.expectAddApplication()
	s.expectCreateApplicationForDeploy("foo", nil)

	errorResults, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				ApplicationName: "foo",
				CharmURL:        "local:foo-42",
				CharmOrigin: &params.CharmOrigin{
					Type:   "charm",
					Source: "local",
					Base: params.Base{
						Name:    "ubuntu",
						Channel: "24.04",
					},
					Architecture: "amd64",
					Revision:     ptr(42),
					Track:        ptr("1.0"),
					Risk:         "stable",
				},
				Resources: map[string]string{"foo": resourceUUID.String()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errorResults.Results, tc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestDeployFailureDeletesPendingResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 2)
	s.expectCharmMeta("foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
	}, 8)
	resourceUUID := testing.GenResourceUUID(c)
	s.expectDeletePendingResources([]resource.UUID{resourceUUID})
	s.expectAddApplication()
	s.expectCreateApplicationForDeploy("foo", errors.Errorf("fail test"))

	errorResults, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				ApplicationName: "foo",
				CharmURL:        "local:foo-42",
				CharmOrigin: &params.CharmOrigin{
					Type:   "charm",
					Source: "local",
					Base: params.Base{
						Name:    "ubuntu",
						Channel: "24.04",
					},
					Architecture: "amd64",
					Revision:     ptr(42),
					Track:        ptr("1.0"),
					Risk:         "stable",
				},
				Resources: map[string]string{"bar": resourceUUID.String()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errorResults.Results, tc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, tc.NotNil)
}

// TestDeployMismatchedResources validates Deploy fails if the charm resource
// count and pending resource count do not match.
func (s *applicationSuite) TestDeployMismatchedResources(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo")
	s.expectCharmMeta("foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
		"foo": {
			Name: "foo",
		},
	}, 2)
	resourceUUID := testing.GenResourceUUID(c)
	s.expectDeletePendingResources([]resource.UUID{resourceUUID})

	errorResults, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				ApplicationName: "foo",
				CharmURL:        "local:foo-42",
				CharmOrigin: &params.CharmOrigin{
					Type:   "charm",
					Source: "local",
					Base: params.Base{
						Name:    "ubuntu",
						Channel: "24.04",
					},
					Architecture: "amd64",
					Revision:     ptr(42),
					Track:        ptr("1.0"),
					Risk:         "stable",
				},
				Resources: map[string]string{"bar": resourceUUID.String()},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errorResults.Results, tc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, tc.NotNil)
}

func (s *applicationSuite) TestDeployInvalidSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	errorResults, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{
			{
				ApplicationName: "foo",
				CharmURL:        "bad:foo-42",
				CharmOrigin: &params.CharmOrigin{
					Type:   "charm",
					Source: "bad",
					Base: params.Base{
						Name:    "ubuntu",
						Channel: "24.04",
					},
					Architecture: "amd64",
					Revision:     ptr(42),
					Track:        ptr("1.0"),
					Risk:         "stable",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(errorResults.Results, tc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, tc.ErrorMatches, "\"bad\" not a valid charm origin source")
}

func (s *applicationSuite) TestGetApplicationConstraintsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].Error, tc.ErrorMatches, "application foo not found")
}

func (s *applicationSuite) TestGetApplicationConstraintsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), application.ID("app-foo")).Return(constraints.Value{}, errors.New("boom"))

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].Error, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestGetApplicationConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), application.ID("app-foo")).Return(constraints.Value{Mem: ptr(uint64(42))}, nil)

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].Constraints, tc.DeepEquals, constraints.Value{Mem: ptr(uint64(42))})
}

func (s *applicationSuite) TestSetApplicationConstraintsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "foo",
		Constraints:     constraints.Value{Mem: ptr(uint64(42))},
	})
	c.Assert(err, tc.ErrorMatches, "application foo not found")
}

func (s *applicationSuite) TestSetApplicationConstraintsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().SetApplicationConstraints(gomock.Any(), application.ID("app-foo"), constraints.Value{Mem: ptr(uint64(42))}).Return(errors.New("boom"))

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "foo",
		Constraints:     constraints.Value{Mem: ptr(uint64(42))},
	})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestSetApplicationConstraints(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().SetApplicationConstraints(gomock.Any(), application.ID("app-foo"), constraints.Value{Mem: ptr(uint64(42))}).Return(nil)
	// TODO(nvinuesa): Remove the double-write to mongodb once machines
	// are fully migrated to dqlite domain.
	s.expectApplication(c, "foo")
	s.application.EXPECT().SetConstraints(constraints.Value{Mem: ptr(uint64(42))}).Return(nil)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "foo",
		Constraints:     constraints.Value{Mem: ptr(uint64(42))},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestAddRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)
	epStr1 := "mattermost"
	epStr2 := "postgresql:db"
	appName1 := "mattermost"
	appName2 := "postgresql"
	ep1 := relation.Endpoint{
		ApplicationName: appName1,
		Relation: internalcharm.Relation{
			Name:      "relation-1",
			Role:      internalcharm.RoleProvider,
			Interface: "db",
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	ep2 := relation.Endpoint{
		ApplicationName: appName2,
		Relation: internalcharm.Relation{
			Name:      "relation-1",
			Role:      internalcharm.RoleRequirer,
			Interface: "db",
			Scope:     internalcharm.ScopeGlobal,
		},
	}
	s.relationService.EXPECT().AddRelation(gomock.Any(), epStr1, epStr2).Return(
		ep1, ep2, nil,
	)

	// Act:
	results, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"mattermost", "postgresql:db"},
		ViaCIDRs:  nil,
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.AddRelationResults{
		Endpoints: map[string]params.CharmRelation{
			appName1: encodeRelation(ep1.Relation),
			appName2: encodeRelation(ep2.Relation),
		},
	})
}

func (s *applicationSuite) TestAddRelationError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)
	epStr1 := "mattermost"
	epStr2 := "postgresql:db"
	boom := errors.Errorf("boom")
	s.relationService.EXPECT().AddRelation(gomock.Any(), epStr1, epStr2).Return(
		relation.Endpoint{}, relation.Endpoint{}, boom,
	)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"mattermost", "postgresql:db"},
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, boom)
}

func (s *applicationSuite) TestAddRelationNoEndpointsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{},
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestAddRelationOneEndpoint(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"1"},
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestAddRelationTooManyEndpointsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"1", "2", "3"},
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestCharmConfigApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	res, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{
		Args: []params.ApplicationGet{{
			ApplicationName: "foo",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestCharmConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().GetApplicationAndCharmConfig(gomock.Any(), appID).Return(applicationservice.ApplicationConfig{
		CharmName: "ch",
		ApplicationConfig: config.ConfigAttributes{
			"foo": "doink",
			"bar": 18,
		},
		CharmConfig: internalcharm.Config{
			Options: map[string]internalcharm.Option{
				"foo": {
					Type:        "string",
					Description: "a foo",
				},
				"bar": {
					Type:        "int",
					Description: "a bar",
					Default:     17,
				},
			},
		},
		Trust: true,
	}, nil)

	res, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{
		Args: []params.ApplicationGet{{
			ApplicationName: "foo",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
	c.Assert(res.Results[0].Config, tc.DeepEquals, map[string]interface{}{
		"foo": map[string]interface{}{
			"description": "a foo",
			"type":        "string",
			"value":       "doink",
			"source":      "user",
		},
		"bar": map[string]interface{}{
			"description": "a bar",
			"type":        "int",
			"value":       18,
			"source":      "user",
			"default":     17,
		},
	})
}

func (s *applicationSuite) TestSetConfigsYAMLNotImplemented(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			ConfigYAML:      "foo: bar",
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotImplemented)
}

func (s *applicationSuite) TestSetConfigsApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestSetConfigsNotValidApplicationName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNameNotValid)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotValid)
}

func (s *applicationSuite) TestSetConfigsInvalidConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().UpdateApplicationConfig(gomock.Any(), appID, gomock.Any()).Return(applicationerrors.InvalidApplicationConfig)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotValid)
}

func (s *applicationSuite) TestSetConfigs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	appID := applicationtesting.GenApplicationUUID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().UpdateApplicationConfig(gomock.Any(), appID, map[string]string{"foo": "bar"}).Return(nil)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestResolveUnitErrorsAllAndEntitesMutuallyExclusive(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		Tags: params.Entities{
			Entities: []params.Entity{{Tag: "unit-1"}},
		},
		All: true,
	})
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestResolveUnitErrorsAllNoRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.resolveService.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeNoHooks).Return(nil)

	res, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		All: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 0)
}

func (s *applicationSuite) TestResolveUnitErrorsAllRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.resolveService.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(nil)

	res, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		All:   true,
		Retry: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 0)
}

func (s *applicationSuite) TestResolveUnitErrorsSpecificNoRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	unitName := coreunit.Name("foo/1")
	s.resolveService.EXPECT().ResolveUnit(gomock.Any(), unitName, resolve.ResolveModeNoHooks).Return(nil)

	res, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		Tags: params.Entities{
			Entities: []params.Entity{{Tag: names.NewUnitTag(unitName.String()).String()}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestResolveUnitErrorsSpecificRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	unitName := coreunit.Name("foo/1")
	s.resolveService.EXPECT().ResolveUnit(gomock.Any(), unitName, resolve.ResolveModeRetryHooks).Return(nil)

	res, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		Tags: params.Entities{
			Entities: []params.Entity{{Tag: names.NewUnitTag(unitName.String()).String()}},
		},
		Retry: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestResolveUnitErrorsUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	unitName := coreunit.Name("foo/1")
	s.resolveService.EXPECT().ResolveUnit(gomock.Any(), unitName, resolve.ResolveModeNoHooks).Return(resolveerrors.UnitNotFound)

	res, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{
		Tags: params.Entities{
			Entities: []params.Entity{{Tag: names.NewUnitTag(unitName.String()).String()}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestDestroyRelationByEndpoints(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}
	relUUID := s.expectGetRelationUUIDForRemoval(c, getUUIDArgs, nil)
	s.expectRemoveRelation(relUUID, false, 0, nil)

	arg := params.DestroyRelation{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := s.api.DestroyRelation(context.Background(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestDestroyRelationRelationNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}
	_ = s.expectGetRelationUUIDForRemoval(c, getUUIDArgs, relationerrors.RelationNotFound)
	arg := params.DestroyRelation{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := s.api.DestroyRelation(context.Background(), arg)

	// Assert
	c.Assert(err, tc.ErrorIs, relationerrors.RelationNotFound)
}

func (s *applicationSuite) TestDestroyRelationByID(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 7,
	}
	relUUID := s.expectGetRelationUUIDForRemoval(c, getUUIDArgs, nil)

	s.expectRemoveRelation(relUUID, false, 0, nil)

	arg := params.DestroyRelation{
		RelationId: getUUIDArgs.RelationID,
	}

	// Act
	err := s.api.DestroyRelation(context.Background(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestDestroyRelationWithForceMaxWait(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		RelationID: 7,
	}
	relUUID := s.expectGetRelationUUIDForRemoval(c, getUUIDArgs, nil)
	maxWait := time.Second
	s.expectRemoveRelation(relUUID, true, maxWait, nil)

	arg := params.DestroyRelation{
		RelationId: getUUIDArgs.RelationID,
		Force:      ptr(true),
		MaxWait:    &maxWait,
	}

	// Act
	err := s.api.DestroyRelation(context.Background(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestSetRelationsSuspendedStub(c *tc.C) {
	c.Skip("Suspending relation requires CMR support, which is not yet implemented.\n" +
		"Once it will be implemented, at minimum, the following tests should be added:\n" +
		"- TestSetRelationsSuspended\n" +
		"- TestSetRelationsReestablished\n" +
		"- TestSetRelationsSuspendedPermissionError\n" +
		"- TestSetRelationsSuspendedNoOffer")
}

func (s *applicationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.application = NewMockApplication(ctrl)
	s.charm = NewMockCharm(ctrl)

	return ctrl
}

func (s *applicationSuite) setupAPI(c *tc.C) {
	s.expectAuthClient()
	s.expectAnyPermissions()
	s.expectAnyChangeOrRemoval()

	s.newIAASAPI(c)
}

func (s *applicationSuite) expectApplication(c *tc.C, name string) {
	s.backend.EXPECT().Application(name).Return(s.application, nil)
}

func (s *applicationSuite) expectApplicationNotFound(c *tc.C, name string) {
	s.backend.EXPECT().Application(name).Return(nil, errors.NotFoundf("application %q", name))
}

func (s *applicationSuite) expectAddApplication() {
	s.backend.EXPECT().AddApplication(gomock.Any(), s.objectStore).Return(s.application, nil)
}

// expectCreateApplicationForDeploy should only be used when calling
// api.Deploy(). DO NOT use for DeployFromRepository(), the expectations
// are different.
func (s *applicationSuite) expectCreateApplicationForDeploy(name string, retErr error) {
	s.applicationService.EXPECT().CreateApplication(gomock.Any(),
		name,
		gomock.Any(),
		gomock.Any(),
		gomock.AssignableToTypeOf(applicationservice.AddApplicationArgs{}),
	).Return(application.ID("app-"+name), retErr)
}

func (s *applicationSuite) expectDeletePendingResources(resSlice []resource.UUID) {
	s.resourceService.EXPECT().DeleteResourcesAddedBeforeApplication(gomock.Any(), resSlice).Return(nil)
}

func (s *applicationSuite) expectSpaceName(c *tc.C, name string) {
	s.networkService.EXPECT().SpaceByName(gomock.Any(), name).Return(&network.SpaceInfo{
		ID: "space-1",
	}, nil)
}

func (s *applicationSuite) expectSpaceNameNotFound(c *tc.C, name string) {
	s.networkService.EXPECT().SpaceByName(gomock.Any(), name).Return(nil, errors.NotFoundf("space %q", name))
}

func (s *applicationSuite) expectCharm(c *tc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     name,
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(s.charm, locator, true, nil)

	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)
}

func (s *applicationSuite) expectCharmNotFound(c *tc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     name,
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(nil, applicationcharm.CharmLocator{}, false, applicationerrors.CharmNotFound)
}

func (s *applicationSuite) expectCharmConfig(c *tc.C, times int) {
	cfg, err := internalcharm.ReadConfig(strings.NewReader(`
options:
    stringOption:
        default: bar
        description: string option
        type: string
    `))
	c.Assert(err, tc.ErrorIsNil)

	s.charm.EXPECT().Config().Return(cfg).Times(times)
}

func (s *applicationSuite) expectCharmMeta(name string, resources map[string]charmresource.Meta, times int) {
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name:      name,
		Resources: resources,
	}).Times(times)
}

func (s *applicationSuite) expectCharmAssumes(c *tc.C) {
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Assumes: &assumes.ExpressionTree{
			Expression: assumes.CompositeExpression{
				ExprType:       assumes.AllOfExpression,
				SubExpressions: []assumes.Expression{},
			},
		},
	})

	var fs coreassumes.FeatureSet
	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil)
}

func (s *applicationSuite) expectCharmFormatCheck(c *tc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     "ubuntu",
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), name).Return(locator, nil)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(s.charm, locator, true, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)

	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Track: "24.04"},
			Architectures: []string{"amd64"},
		}},
	}).Times(2)
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{}).Times(2)
}

func (s *applicationSuite) expectCharmFormatCheckDowngrade(c *tc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     "ubuntu",
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), name).Return(locator, nil)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(s.charm, locator, true, nil)
	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)

	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Track: "24.04"},
			Architectures: []string{"amd64"},
		}},
	})
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{})
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{}).Times(2)
}

func (s *applicationSuite) expectSetCharm(c *tc.C, name string, fn func(*tc.C, state.SetCharmConfig)) {
	s.application.EXPECT().SetCharm(gomock.Any(), gomock.Any()).DoAndReturn(func(config state.SetCharmConfig, _ objectstore.ObjectStore) error {
		fn(c, config)
		return nil
	})

	// TODO (stickupkid): This isn't actually checking much here...
	s.applicationService.EXPECT().SetApplicationCharm(gomock.Any(), name, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, params domainapplication.UpdateCharmParams) error {
		c.Assert(params.Charm, tc.DeepEquals, &domainCharm{
			charm: s.charm,
			locator: applicationcharm.CharmLocator{
				Name:     "foo",
				Revision: 42,
				Source:   applicationcharm.LocalSource,
			},
			available: true,
		})
		c.Assert(params.CharmUpgradeOnError, tc.Equals, true)
		return nil
	})
}

func (s *applicationSuite) expectSetCharmWithTrust(c *tc.C) {
	appSchema, appDefaults, err := ConfigSchema()
	c.Assert(err, tc.ErrorIsNil)

	s.application.EXPECT().UpdateApplicationConfig(map[string]any{
		"trust": true,
	}, nil, appSchema, appDefaults).Return(nil)
}

func (s *applicationSuite) expectGetRelationUUIDForRemoval(c *tc.C, args relation.GetRelationUUIDForRemovalArgs, err error) corerelation.UUID {
	relUUID := relationtesting.GenRelationUUID(c)
	s.relationService.EXPECT().GetRelationUUIDForRemoval(context.Background(), args).Return(relUUID, err)
	return relUUID
}

func (s *applicationSuite) expectRemoveRelation(uuid corerelation.UUID, force bool, maxWait time.Duration, err error) {
	rUUID, _ := removal.NewUUID()
	s.removalService.EXPECT().RemoveRelation(context.Background(), uuid, force, maxWait).Return(rUUID, err)
}
