// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"strings"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreassumes "github.com/juju/juju/core/assumes"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/objectstore"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
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

	// This is ridiculous, the amount of requests to set a charm config.
	// We're requesting the new charm and the old charm, more than we require.
	// We should fix this when we refactor the application service.

	s.setupAPI(c)
	s.expectApplication(c, "foo")
	s.expectCharm(c, "foo")
	s.expectCharmConfig(c)
	s.expectCharmAssumes(c)
	s.expectCharmFormatCheck(c, "foo")

	var result state.SetCharmConfig
	s.expectSetCharm(c, "foo", func(c *gc.C, config state.SetCharmConfig) {
		result = config
	})

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

func (s *applicationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.application = NewMockApplication(ctrl)
	s.charm = NewMockCharm(ctrl)

	return ctrl
}

func (s *applicationSuite) setupAPI(c *gc.C) {
	s.expectAuthClient(c)
	s.expectAnyPermissions(c)
	s.expectAnyChangeOrRemoval(c)

	s.transformCharm = func(ch Charm) *state.Charm {
		return nil
	}

	s.newIAASAPI(c)
}

func (s *applicationSuite) expectApplication(c *gc.C, name string) {
	s.backend.EXPECT().Application(name).Return(s.application, nil)
}

func (s *applicationSuite) expectCharm(c *gc.C, name string) {
	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), applicationcharm.GetCharmArgs{
		Name:     name,
		Revision: ptr(42),
	}).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(s.charm, applicationcharm.CharmOrigin{
		Revision: 42,
	}, nil)
}

func (s *applicationSuite) expectCharmConfig(c *gc.C) {
	cfg, err := internalcharm.ReadConfig(strings.NewReader(`
options:
    stringOption:
        default: bar
        description: string option
        type: string
    `))
	c.Assert(err, jc.ErrorIsNil)

	s.charm.EXPECT().Config().Return(cfg)
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
	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Track: "24.04"},
			Architectures: []string{"amd64"},
		}},
	}).Times(2)
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{}).Times(2)

	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmIDByApplicationName(gomock.Any(), name).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(s.charm, applicationcharm.CharmOrigin{
		Revision: 42,
	}, nil)
}

func (s *applicationSuite) expectSetCharm(c *gc.C, name string, fn func(*gc.C, state.SetCharmConfig)) {
	s.application.EXPECT().SetCharm(gomock.Any(), gomock.Any()).DoAndReturn(func(config state.SetCharmConfig, _ objectstore.ObjectStore) error {
		fn(c, config)
		return nil
	})

	// TODO (stickupkid): This isn't actually checking much here...
	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), name, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, params applicationservice.UpdateCharmParams) error {
		c.Assert(params.Charm, gc.DeepEquals, &domainCharm{
			charm: s.charm,
			origin: applicationcharm.CharmOrigin{
				Revision: 42,
			},
		})
		return nil
	})

	appSchema, appDefaults, err := ConfigSchema()
	c.Assert(err, jc.ErrorIsNil)

	s.application.EXPECT().UpdateApplicationConfig(map[string]any{
		"trust": true,
	}, nil, appSchema, appDefaults)
}
