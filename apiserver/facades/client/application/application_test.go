// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coreassumes "github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/resource/testing"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
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

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) TestSetCharm(c *gc.C) {
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
	s.expectSetCharm(c, "foo", func(c *gc.C, config state.SetCharmConfig) {
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
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.CharmOrigin, gc.DeepEquals, &state.CharmOrigin{
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

func (s *applicationSuite) TestSetCharmInvalidCharmOrigin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmURL:        "local:foo-42",
		CharmOrigin:     &params.CharmOrigin{},
	})
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestSetCharmApplicationNotFound(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmEndpointBindings(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectSpaceName(c, "bar")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *gc.C, config state.SetCharmConfig) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CharmOrigin, gc.DeepEquals, &state.CharmOrigin{
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

func (s *applicationSuite) TestSetCharmEndpointBindingsNotFound(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmGetCharmNotFound(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *applicationSuite) TestSetCharmInvalidConfig(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, `parsing config settings: unknown option "blach!"`)
}

func (s *applicationSuite) TestSetCharmWithoutTrust(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 1)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *gc.C, config state.SetCharmConfig) {
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
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.CharmOrigin, gc.DeepEquals, &state.CharmOrigin{
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

func (s *applicationSuite) TestSetCharmFormatDowngrade(c *gc.C) {
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
	c.Assert(err, gc.ErrorMatches, "cannot downgrade from v2 charm format to v1")
}

func (s *applicationSuite) TestDeploy(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 2)
	s.expectCharmMeta("foo", nil, 8)
	s.expectReadSequence("foo", 1)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errorResults.Results, gc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, gc.IsNil)
}

// TestDeployWithResources test the scenario of deploying
// local charms, or charms via bundles that have resources.
// Deploy rather than DeployFromRepository is called by the
// clients. In this case PendingResources, uuids, must be
// provided for all charm resources.
func (s *applicationSuite) TestDeployWithPendingResources(c *gc.C) {
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
	s.expectReadSequence("foo", 1)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errorResults.Results, gc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, gc.IsNil)
}

func (s *applicationSuite) TestDeployFailureDeletesPendingResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c, 2)
	s.expectCharmMeta("foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
	}, 8)
	s.expectReadSequence("foo", 1)
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errorResults.Results, gc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, gc.NotNil)
}

// TestDeployMismatchedResources validates Deploy fails if the charm resource
// count and pending resource count do not match.
func (s *applicationSuite) TestDeployMismatchedResources(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errorResults.Results, gc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, gc.NotNil)
}

func (s *applicationSuite) TestDeployInvalidSource(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errorResults.Results, gc.HasLen, 1)
	c.Assert(errorResults.Results[0].Error, gc.ErrorMatches, "\"bad\" not a valid charm origin source")
}

func (s *applicationSuite) TestGetApplicationConstraintsAppNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res.Results[0].Error, gc.ErrorMatches, "application foo not found")
}

func (s *applicationSuite) TestGetApplicationConstraintsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), application.ID("app-foo")).Return(constraints.Value{}, errors.New("boom"))

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestGetApplicationConstraints(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().GetApplicationConstraints(gomock.Any(), application.ID("app-foo")).Return(constraints.Value{Mem: ptr(uint64(42))}, nil)

	res, err := s.api.GetConstraints(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(res.Results[0].Constraints, gc.DeepEquals, constraints.Value{Mem: ptr(uint64(42))})
}

func (s *applicationSuite) TestSetApplicationConstraintsAppNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "foo",
		Constraints:     constraints.Value{Mem: ptr(uint64(42))},
	})
	c.Assert(err, gc.ErrorMatches, "application foo not found")
}

func (s *applicationSuite) TestSetApplicationConstraintsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID("app-foo"), nil)
	s.applicationService.EXPECT().SetApplicationConstraints(gomock.Any(), application.ID("app-foo"), constraints.Value{Mem: ptr(uint64(42))}).Return(errors.New("boom"))

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{
		ApplicationName: "foo",
		Constraints:     constraints.Value{Mem: ptr(uint64(42))},
	})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *applicationSuite) TestSetApplicationConstraints(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestAddRelation(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.AddRelationResults{
		Endpoints: map[string]params.CharmRelation{
			appName1: encodeRelation(ep1.Relation),
			appName2: encodeRelation(ep2.Relation),
		},
	})
}

func (s *applicationSuite) TestAddRelationError(c *gc.C) {
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
	c.Assert(err, jc.ErrorIs, boom)
}

func (s *applicationSuite) TestAddRelationNoEndpointsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{},
	})

	// Assert:
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestAddRelationOneEndpoint(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"1"},
	})

	// Assert:
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestAddRelationTooManyEndpointsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	// Act:
	_, err := s.api.AddRelation(context.Background(), params.AddRelation{
		Endpoints: []string{"1", "2", "3"},
	})

	// Assert:
	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestCharmConfigApplicationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	res, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{
		Args: []params.ApplicationGet{{
			ApplicationName: "foo",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestCharmConfig(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
	c.Assert(res.Results[0].Config, gc.DeepEquals, map[string]interface{}{
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

func (s *applicationSuite) TestSetConfigsYAMLNotImplemented(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			ConfigYAML:      "foo: bar",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotImplemented)
}

func (s *applicationSuite) TestSetConfigsApplicationNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestSetConfigsNotValidApplicationName(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNameNotValid)

	res, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "foo",
			Config:          map[string]string{"foo": "bar"},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotValid)
}

func (s *applicationSuite) TestSetConfigsInvalidConfig(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, jc.Satisfies, params.IsCodeNotValid)
}

func (s *applicationSuite) TestSetConfigs(c *gc.C) {
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *applicationSuite) TestDestroyRelationStub(c *gc.C) {
	c.Skip("Destroy relation isn't implemented yet.\n" +
		"Once it will be implemented, the following tests should be added:\n" +
		"- TestDestroyRelation\n" +
		"- TestDestroyPeerRelation\n" +
		"- TestDestroyRelationUnknown\n" +
		"- TestDestroyPeerRelationUnknown\n" +
		"- TestDestroyRelationWithForce")
}

func (s *applicationSuite) TestSetRelationsSuspendedStub(c *gc.C) {
	c.Skip("Suspending relation requires CMR support, which is not yet implemented.\n" +
		"Once it will be implemented, at minimum, the following tests should be added:\n" +
		"- TestSetRelationsSuspended\n" +
		"- TestSetRelationsReestablished\n" +
		"- TestSetRelationsSuspendedPermissionError\n" +
		"- TestSetRelationsSuspendedNoOffer")
}

func (s *applicationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.application = NewMockApplication(ctrl)
	s.charm = NewMockCharm(ctrl)

	return ctrl
}

func (s *applicationSuite) setupAPI(c *gc.C) {
	s.expectAuthClient()
	s.expectAnyPermissions()
	s.expectAnyChangeOrRemoval()

	s.newIAASAPI(c)
}

func (s *applicationSuite) expectApplication(c *gc.C, name string) {
	s.backend.EXPECT().Application(name).Return(s.application, nil)
}

func (s *applicationSuite) expectApplicationNotFound(c *gc.C, name string) {
	s.backend.EXPECT().Application(name).Return(nil, errors.NotFoundf("application %q", name))
}

func (s *applicationSuite) expectReadSequence(name string, seqResult int) {
	s.backend.EXPECT().ReadSequence(name).Return(seqResult, nil)
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

func (s *applicationSuite) expectSpaceName(c *gc.C, name string) {
	s.networkService.EXPECT().SpaceByName(gomock.Any(), name).Return(&network.SpaceInfo{
		ID: "space-1",
	}, nil)
}

func (s *applicationSuite) expectSpaceNameNotFound(c *gc.C, name string) {
	s.networkService.EXPECT().SpaceByName(gomock.Any(), name).Return(nil, errors.NotFoundf("space %q", name))
}

func (s *applicationSuite) expectCharm(c *gc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     name,
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(s.charm, locator, true, nil)

	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)
}

func (s *applicationSuite) expectCharmNotFound(c *gc.C, name string) {
	locator := applicationcharm.CharmLocator{
		Name:     name,
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(nil, applicationcharm.CharmLocator{}, false, applicationerrors.CharmNotFound)
}

func (s *applicationSuite) expectCharmConfig(c *gc.C, times int) {
	cfg, err := internalcharm.ReadConfig(strings.NewReader(`
options:
    stringOption:
        default: bar
        description: string option
        type: string
    `))
	c.Assert(err, jc.ErrorIsNil)

	s.charm.EXPECT().Config().Return(cfg).Times(times)
}

func (s *applicationSuite) expectCharmMeta(name string, resources map[string]charmresource.Meta, times int) {
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{
		Name:      name,
		Resources: resources,
	}).Times(times)
}

func (s *applicationSuite) expectCharmAssumes(c *gc.C) {
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

func (s *applicationSuite) expectCharmFormatCheck(c *gc.C, name string) {
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

func (s *applicationSuite) expectCharmFormatCheckDowngrade(c *gc.C, name string) {
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

func (s *applicationSuite) expectSetCharm(c *gc.C, name string, fn func(*gc.C, state.SetCharmConfig)) {
	s.application.EXPECT().SetCharm(gomock.Any(), gomock.Any()).DoAndReturn(func(config state.SetCharmConfig, _ objectstore.ObjectStore) error {
		fn(c, config)
		return nil
	})

	// TODO (stickupkid): This isn't actually checking much here...
	s.applicationService.EXPECT().SetApplicationCharm(gomock.Any(), name, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, params applicationservice.UpdateCharmParams) error {
		c.Assert(params.Charm, gc.DeepEquals, &domainCharm{
			charm: s.charm,
			locator: applicationcharm.CharmLocator{
				Name:     "foo",
				Revision: 42,
				Source:   applicationcharm.LocalSource,
			},
			available: true,
		})
		c.Assert(params.CharmUpgradeOnError, gc.Equals, true)
		return nil
	})
}

func (s *applicationSuite) expectSetCharmWithTrust(c *gc.C) {
	appSchema, appDefaults, err := ConfigSchema()
	c.Assert(err, jc.ErrorIsNil)

	s.application.EXPECT().UpdateApplicationConfig(map[string]any{
		"trust": true,
	}, nil, appSchema, appDefaults).Return(nil)
}
