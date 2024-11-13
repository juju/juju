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

	coreassumes "github.com/juju/juju/core/assumes"
	charmtesting "github.com/juju/juju/core/charm/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
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
	s.expectCharmConfig(c)
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
	s.expectCharmConfig(c)

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
	s.expectCharmConfig(c)
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
	s.expectCharmConfig(c)
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

func (s *applicationSuite) expectApplicationNotFound(c *gc.C, name string) {
	s.backend.EXPECT().Application(name).Return(nil, errors.NotFoundf("application %q", name))
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
	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmID(gomock.Any(), applicationcharm.GetCharmArgs{
		Name:     name,
		Revision: ptr(42),
	}).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(s.charm, applicationcharm.CharmOrigin{
		Revision: 42,
	}, nil)
}

func (s *applicationSuite) expectCharmNotFound(c *gc.C, name string) {
	s.applicationService.EXPECT().GetCharmID(gomock.Any(), applicationcharm.GetCharmArgs{
		Name:     name,
		Revision: ptr(42),
	}).Return("", applicationerrors.CharmNotFound)
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
	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmIDByApplicationName(gomock.Any(), name).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(s.charm, applicationcharm.CharmOrigin{
		Revision: 42,
	}, nil)

	s.charm.EXPECT().Manifest().Return(&internalcharm.Manifest{
		Bases: []internalcharm.Base{{
			Name:          "ubuntu",
			Channel:       internalcharm.Channel{Track: "24.04"},
			Architectures: []string{"amd64"},
		}},
	}).Times(2)
	s.charm.EXPECT().Meta().Return(&internalcharm.Meta{}).Times(2)
}

func (s applicationSuite) expectCharmFormatCheckDowngrade(c *gc.C, name string) {
	id := charmtesting.GenCharmID(c)

	s.applicationService.EXPECT().GetCharmIDByApplicationName(gomock.Any(), name).Return(id, nil)

	s.applicationService.EXPECT().GetCharm(gomock.Any(), id).Return(s.charm, applicationcharm.CharmOrigin{
		Revision: 42,
	}, nil)

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
	s.applicationService.EXPECT().UpdateApplicationCharm(gomock.Any(), name, gomock.Any()).DoAndReturn(func(_ context.Context, _ string, params applicationservice.UpdateCharmParams) error {
		c.Assert(params.Charm, gc.DeepEquals, &domainCharm{
			charm: s.charm,
			origin: applicationcharm.CharmOrigin{
				Revision: 42,
			},
		})
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
