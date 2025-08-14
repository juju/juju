// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"fmt"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/core/application"
	corearch "github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/resource"
	coreunit "github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/environs/bootstrap"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/rpc/params"
)

type applicationSuite struct {
	baseSuite
}

func TestApplicationSuite(t *stdtesting.T) {
	tc.Run(t, &applicationSuite{})
}

func (s *applicationSuite) TestDeploy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo", nil)
	s.expectCreateApplicationForDeploy("foo", nil)

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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
	resourceUUID := resource.GenUUID(c)
	s.expectCharm(c, "foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
	})
	s.expectCreateApplicationForDeploy("foo", nil)

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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

func (s *applicationSuite) TestDeployWithApplicationConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	s.expectCharm(c, "foo", nil)
	config := map[string]interface{}{"stringOption": "hey"}
	s.expectCreateApplicationForDeployWithConfig(c, "foo", config, nil)

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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
				Config: map[string]string{"stringOption": "hey"},
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
	s.expectCharm(c, "foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
	})
	resourceUUID := resource.GenUUID(c)
	s.expectDeletePendingResources([]resource.UUID{resourceUUID})
	s.expectCreateApplicationForDeploy("foo", errors.Errorf("fail test"))

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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
	s.expectCharm(c, "foo", map[string]charmresource.Meta{
		"bar": {
			Name: "bar",
		},
		"foo": {
			Name: "foo",
		},
	})
	resourceUUID := resource.GenUUID(c)
	s.expectDeletePendingResources([]resource.UUID{resourceUUID})

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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

	errorResults, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{
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

func (s *applicationSuite) TestGetCharmURLOriginAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "foo").Return(applicationcharm.CharmLocator{}, applicationerrors.ApplicationNotFound)

	res, err := s.api.GetCharmURLOrigin(c.Context(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestGetCharmURLOrigin(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "foo").Return(applicationcharm.CharmLocator{
		Name:         "foo",
		Revision:     42,
		Source:       applicationcharm.CharmHubSource,
		Architecture: architecture.ARM64,
	}, nil)
	s.applicationService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "foo").Return(domainapplication.CharmOrigin{
		Source:   "charmhub",
		Revision: 42,
		Channel: &deployment.Channel{
			Track: "1.0",
			Risk:  "stable",
		},
		Platform: deployment.Platform{
			OSType:       deployment.Ubuntu,
			Channel:      "24.04",
			Architecture: architecture.ARM64,
		},
	}, nil)

	res, err := s.api.GetCharmURLOrigin(c.Context(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.URL, tc.Equals, "ch:arm64/foo-42")
	c.Check(res.Origin, tc.DeepEquals, params.CharmOrigin{
		Source:       "charm-hub",
		Revision:     ptr(42),
		Risk:         "stable",
		Track:        ptr("1.0"),
		Architecture: "arm64",
		Base: params.Base{
			Name:    "ubuntu",
			Channel: "24.04",
		},
		InstanceKey: res.Origin.InstanceKey,
	})
}

func (s *applicationSuite) TestGetCharmURLOriginNoOptionals(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	arch := corearch.DefaultArchitecture

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "foo").Return(applicationcharm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}, nil)
	s.applicationService.EXPECT().GetApplicationCharmOrigin(gomock.Any(), "foo").Return(domainapplication.CharmOrigin{
		Source:   "local",
		Revision: 42,
		Platform: deployment.Platform{
			OSType:  deployment.Ubuntu,
			Channel: "24.04",
		},
	}, nil)

	res, err := s.api.GetCharmURLOrigin(c.Context(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.URL, tc.Equals, fmt.Sprintf("local:%s/foo-42", arch))
	c.Check(res.Origin, tc.DeepEquals, params.CharmOrigin{
		Source:       "local",
		Revision:     ptr(42),
		Architecture: arch,
		Base: params.Base{
			Name:    "ubuntu",
			Channel: "24.04",
		},
		InstanceKey: res.Origin.InstanceKey,
	})
}

func (s *applicationSuite) TestCharmRelations(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	appID := application.GenID(c)
	rels := []string{"foo", "bar"}

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "doink").Return(appID, nil)
	s.applicationService.EXPECT().GetApplicationEndpointNames(gomock.Any(), appID).Return(rels, nil)

	res, err := s.api.CharmRelations(c.Context(), params.ApplicationCharmRelations{
		ApplicationName: "doink",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.CharmRelations, tc.SameContents, rels)
}

func (s *applicationSuite) TestCharmRelationsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "doink").Return("", applicationerrors.ApplicationNotFound)

	_, err := s.api.CharmRelations(c.Context(), params.ApplicationCharmRelations{
		ApplicationName: "doink",
	})
	c.Assert(err, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestDestroyUnitIsSubordinate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "foo").Return(true, nil)

	// Act:
	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("foo/0").String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.ErrorMatches, `.*unit "foo/0" is a subordinate.*`)
}

func (s *applicationSuite) TestDestroyUnitControllerUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	charmLocator := applicationcharm.CharmLocator{
		Name:     "ctrl",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	s.applicationService.EXPECT().IsSubordinateApplicationByName(gomock.Any(), "ctrl").Return(false, nil)
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "ctrl").Return(charmLocator, nil)
	s.applicationService.EXPECT().GetCharmMetadataName(gomock.Any(), charmLocator).Return(bootstrap.ControllerCharmName, nil)

	// Act:
	res, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{{
			UnitTag: names.NewUnitTag("ctrl/0").String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotSupported)
}

func (s *applicationSuite) TestDestroyApplicationController(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.setupAPI(c)

	charmLocator := applicationcharm.CharmLocator{
		Name:     "ctrl",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "ctrl").Return(charmLocator, nil)
	s.applicationService.EXPECT().GetCharmMetadataName(gomock.Any(), charmLocator).Return(bootstrap.ControllerCharmName, nil)

	// Act:
	res, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: names.NewApplicationTag("ctrl").String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotSupported)
}

func (s *applicationSuite) TestGetApplicationConstraintsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	res, err := s.api.GetConstraints(c.Context(), params.Entities{
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

	res, err := s.api.GetConstraints(c.Context(), params.Entities{
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

	res, err := s.api.GetConstraints(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: "application-foo"}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results[0].Constraints, tc.DeepEquals, constraints.Value{Mem: ptr(uint64(42))})
}

func (s *applicationSuite) TestSetApplicationConstraintsAppNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(application.ID(""), applicationerrors.ApplicationNotFound)

	err := s.api.SetConstraints(c.Context(), params.SetConstraints{
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

	err := s.api.SetConstraints(c.Context(), params.SetConstraints{
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

	err := s.api.SetConstraints(c.Context(), params.SetConstraints{
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
	results, err := s.api.AddRelation(c.Context(), params.AddRelation{
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
	_, err := s.api.AddRelation(c.Context(), params.AddRelation{
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
	_, err := s.api.AddRelation(c.Context(), params.AddRelation{
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
	_, err := s.api.AddRelation(c.Context(), params.AddRelation{
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
	_, err := s.api.AddRelation(c.Context(), params.AddRelation{
		Endpoints: []string{"1", "2", "3"},
	})

	// Assert:
	c.Assert(err, tc.ErrorIs, errors.BadRequest)
}

func (s *applicationSuite) TestCharmConfigApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return("", applicationerrors.ApplicationNotFound)

	res, err := s.api.CharmConfig(c.Context(), params.ApplicationGetArgs{
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
	appID := application.GenID(c)

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

	res, err := s.api.CharmConfig(c.Context(), params.ApplicationGetArgs{
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

	res, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{
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

	res, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{
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

	res, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{
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
	appID := application.GenID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().UpdateApplicationConfig(gomock.Any(), appID, gomock.Any()).Return(applicationerrors.InvalidApplicationConfig)

	res, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{
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
	appID := application.GenID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil)
	s.applicationService.EXPECT().UpdateApplicationConfig(gomock.Any(), appID, map[string]string{"foo": "bar"}).Return(nil)

	res, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{
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

	_, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
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

	res, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
		All: true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 0)
}

func (s *applicationSuite) TestResolveUnitErrorsAllRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	s.resolveService.EXPECT().ResolveAllUnits(gomock.Any(), resolve.ResolveModeRetryHooks).Return(nil)

	res, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
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

	res, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
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

	res, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
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

	res, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{
		Tags: params.Entities{
			Entities: []params.Entity{{Tag: names.NewUnitTag(unitName.String()).String()}},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestMergeBindings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	appName := "doink"
	appID := application.GenID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.applicationService.EXPECT().MergeApplicationEndpointBindings(gomock.Any(), appID, map[string]network.SpaceName{
		"foo": "alpha",
		"bar": "beta",
	}, false).Return(nil)

	ret, err := s.api.MergeBindings(c.Context(), params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{{
			ApplicationTag: names.NewApplicationTag(appName).String(),
			Bindings: map[string]string{
				"foo": "alpha",
				"bar": "beta",
			},
			Force: false,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ret.Results, tc.HasLen, 1)
	c.Assert(ret.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestMergeBindingsForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	appName := "doink"
	appID := application.GenID(c)

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(appID, nil)
	s.applicationService.EXPECT().MergeApplicationEndpointBindings(gomock.Any(), appID, map[string]network.SpaceName{
		"foo": "alpha",
		"bar": "beta",
	}, true).Return(nil)

	ret, err := s.api.MergeBindings(c.Context(), params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{{
			ApplicationTag: names.NewApplicationTag(appName).String(),
			Bindings: map[string]string{
				"foo": "alpha",
				"bar": "beta",
			},
			Force: true,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ret.Results, tc.HasLen, 1)
	c.Assert(ret.Results[0].Error, tc.IsNil)
}

func (s *applicationSuite) TestMergeBindingsNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)

	appName := "doink"

	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return("", applicationerrors.ApplicationNotFound)

	ret, err := s.api.MergeBindings(c.Context(), params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{{
			ApplicationTag: names.NewApplicationTag(appName).String(),
			Bindings: map[string]string{
				"foo": "alpha",
				"bar": "beta",
			},
			Force: false,
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ret.Results, tc.HasLen, 1)
	c.Assert(ret.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestDestroyRelationByEndpoints(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.setupAPI(c)
	getUUIDArgs := relation.GetRelationUUIDForRemovalArgs{
		Endpoints: []string{"foo:require", "bar:provide"},
	}
	relUUID := s.expectGetRelationUUIDForRemoval(c, getUUIDArgs, nil)
	s.expectRemoveRelation(c, relUUID, false, 0, nil)

	arg := params.DestroyRelation{
		Endpoints: []string{"foo:require", "bar:provide"},
	}

	// Act
	err := s.api.DestroyRelation(c.Context(), arg)

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
	err := s.api.DestroyRelation(c.Context(), arg)

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

	s.expectRemoveRelation(c, relUUID, false, 0, nil)

	arg := params.DestroyRelation{
		RelationId: getUUIDArgs.RelationID,
	}

	// Act
	err := s.api.DestroyRelation(c.Context(), arg)

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
	s.expectRemoveRelation(c, relUUID, true, maxWait, nil)

	arg := params.DestroyRelation{
		RelationId: getUUIDArgs.RelationID,
		Force:      ptr(true),
		MaxWait:    &maxWait,
	}

	// Act
	err := s.api.DestroyRelation(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

func (s *applicationSuite) TestUnitsInfoCAASUnitTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.testUnitsInfoCAAS(c, names.NewUnitTag("foo/666"), coreunit.Name("foo/666"))
}

func (s *applicationSuite) TestUnitsInfoCAASApplicationTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return([]coreunit.Name{"foo/666"}, nil)

	s.testUnitsInfoCAAS(c, names.NewApplicationTag("foo"), coreunit.Name("foo/666"))
}

func (s *applicationSuite) testUnitsInfoCAAS(c *tc.C, inputTag names.Tag, resultingUnitName coreunit.Name) {
	// Arrange
	s.setupAPI(c)

	s.leadershipReader.EXPECT().Leaders().Return(map[string]string{
		resultingUnitName.Application(): resultingUnitName.String(),
	}, nil)

	appID := application.GenID(c)
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), "foo").Return(appID, nil).AnyTimes()

	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), resultingUnitName).Return(life.Alive, nil)
	s.applicationService.EXPECT().GetUnitWorkloadVersion(gomock.Any(), resultingUnitName).Return("1.0.0", nil)
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), "foo").Return(applicationcharm.CharmLocator{
		Name:     "foo",
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}, nil)

	s.relationService.EXPECT().ApplicationRelationsInfo(gomock.Any(), appID).Return([]relation.EndpointRelationData{{
		RelationID:      3,
		Endpoint:        "relation",
		RelatedEndpoint: "fake-provides",
		ApplicationData: map[string]interface{}{},
		UnitRelationData: map[string]relation.RelationData{
			"foo/0": {
				InScope:  true,
				UnitData: map[string]interface{}{"foo": "bar"},
			},
			"foo/1": {
				InScope:  true,
				UnitData: map[string]interface{}{"foo": "baz"},
			},
		},
	}}, nil)

	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), resultingUnitName).Return("", applicationerrors.UnitMachineNotAssigned)

	s.applicationService.EXPECT().GetUnitK8sPodInfo(gomock.Any(), resultingUnitName).Return(domainapplication.K8sPodInfo{
		ProviderID: "provider-id",
		Address:    "10.0.0.0",
		Ports:      []string{"666", "667"},
	}, nil)

	// Act
	result, err := s.api.UnitsInfo(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: inputTag.String()}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.UnitInfoResults{
		Results: []params.UnitInfoResult{{
			Result: &params.UnitResult{
				Tag:             names.NewUnitTag(resultingUnitName.String()).String(),
				Charm:           "local:amd64/foo-42",
				Leader:          true,
				WorkloadVersion: "1.0.0",
				OpenedPorts:     []string{"666", "667"},
				Address:         "10.0.0.0",
				ProviderId:      "provider-id",
				Life:            "alive",
				RelationData: []params.EndpointRelationData{{
					RelationId:      3,
					Endpoint:        "relation",
					RelatedEndpoint: "fake-provides",
					ApplicationData: map[string]interface{}{},
					UnitRelationData: map[string]params.RelationData{
						"foo/0": {
							InScope:  true,
							UnitData: map[string]interface{}{"foo": "bar"},
						},
						"foo/1": {
							InScope:  true,
							UnitData: map[string]interface{}{"foo": "baz"},
						},
					},
				}},
			},
		}},
	})
}

func (s *applicationSuite) TestUnitsInfoUnitNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.leadershipReader.EXPECT().Leaders().Return(map[string]string{}, nil)
	s.applicationService.EXPECT().GetUnitLife(gomock.Any(), coreunit.Name("foo/666")).Return("", applicationerrors.UnitNotFound)

	s.setupAPI(c)

	// Act
	res, err := s.api.UnitsInfo(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag("foo/666").String()}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *applicationSuite) TestUnitsInfoApplicationNotFound(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	s.leadershipReader.EXPECT().Leaders().Return(map[string]string{}, nil)
	s.applicationService.EXPECT().GetUnitNamesForApplication(gomock.Any(), "foo").Return(nil, applicationerrors.ApplicationNotFound)

	s.setupAPI(c)

	// Act
	res, err := s.api.UnitsInfo(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag("foo").String()}},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
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
	return ctrl
}

func (s *applicationSuite) setupAPI(c *tc.C) {
	s.expectAuthClient()
	s.expectAnyPermissions()
	s.expectAnyChangeOrRemoval()

	s.newIAASAPI(c)
}

// expectCreateApplicationForDeploy should only be used when calling
// api.Deploy(). DO NOT use for DeployFromRepository(), the expectations
// are different.
func (s *applicationSuite) expectCreateApplicationForDeploy(name string, retErr error) {
	s.applicationService.EXPECT().CreateIAASApplication(gomock.Any(),
		name,
		gomock.Any(),
		gomock.Any(),
		gomock.AssignableToTypeOf(applicationservice.AddApplicationArgs{}),
	).Return(application.ID("app-"+name), retErr)
}

// expectCreateApplicationForDeploy should only be used when calling
// api.Deploy(). DO NOT use for DeployFromRepository(), the expectations
// are different.
func (s *applicationSuite) expectCreateApplicationForDeployWithConfig(c *tc.C, name string, config config.ConfigAttributes, retErr error) {
	s.applicationService.EXPECT().CreateIAASApplication(gomock.Any(),
		name,
		gomock.Any(),
		gomock.Any(),
		gomock.AssignableToTypeOf(applicationservice.AddApplicationArgs{}),
	).DoAndReturn(func(ctx context.Context, s string, charm internalcharm.Charm, origin corecharm.Origin, args applicationservice.AddApplicationArgs, arg ...applicationservice.AddIAASUnitArg) (application.ID, error) {
		c.Check(args.ApplicationConfig, tc.DeepEquals, config)
		return application.ID("app-" + name), retErr
	})
}

func (s *applicationSuite) expectDeletePendingResources(resSlice []resource.UUID) {
	s.resourceService.EXPECT().DeleteResourcesAddedBeforeApplication(gomock.Any(), resSlice).Return(nil)
}

func (s *applicationSuite) expectCharm(c *tc.C, name string, resources map[string]charmresource.Meta) {
	locator := applicationcharm.CharmLocator{
		Name:     name,
		Revision: 42,
		Source:   applicationcharm.LocalSource,
	}

	cfg, err := internalcharm.ReadConfig(strings.NewReader(`
options:
    stringOption:
        default: bar
        description: string option
        type: string
`))
	c.Assert(err, tc.ErrorIsNil)
	metadata := &internalcharm.Meta{
		Name:      "foo",
		Resources: resources,
	}
	charm := internalcharm.NewCharmBase(metadata, nil, cfg, nil, nil)
	s.applicationService.EXPECT().GetCharm(gomock.Any(), locator).Return(charm, locator, true, nil)

	s.applicationService.EXPECT().IsCharmAvailable(gomock.Any(), locator).Return(true, nil)
}

func (s *applicationSuite) expectGetRelationUUIDForRemoval(c *tc.C, args relation.GetRelationUUIDForRemovalArgs, err error) corerelation.UUID {
	relUUID := corerelation.GenRelationUUID(c)
	s.relationService.EXPECT().GetRelationUUIDForRemoval(gomock.Any(), args).Return(relUUID, err)
	return relUUID
}

func (s *applicationSuite) expectRemoveRelation(c *tc.C, uuid corerelation.UUID, force bool, maxWait time.Duration, err error) {
	rUUID, _ := removal.NewUUID()
	s.removalService.EXPECT().RemoveRelation(gomock.Any(), uuid, force, maxWait).Return(rUUID, err)
}
