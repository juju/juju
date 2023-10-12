// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"

	"github.com/juju/charm/v11"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	apiapplication "github.com/juju/juju/api/client/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/application"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/caas/kubernetes/provider"
	k8stesting "github.com/juju/juju/caas/kubernetes/provider/testing"
	corearch "github.com/juju/juju/core/arch"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type getSuite struct {
	jujutesting.JujuConnSuite

	applicationAPI *application.APIBase
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&getSuite{})

func (s *getSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	storageAccess, err := application.GetStorageState(s.State)
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(s.State)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	api, err := application.NewAPIBase(
		application.GetState(s.State),
		storageAccess,
		s.authorizer,
		nil,
		blockChecker,
		application.GetModel(model),
		nil, // leadership not used in this suite.
		application.CharmToStateCharm,
		application.DeployApplication,
		&mockStoragePoolManager{},
		&mockStorageRegistry{},
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
	)
	c.Assert(err, jc.ErrorIsNil)
	s.applicationAPI = api
}

func (s *getSuite) TestClientApplicationGetIAASModelSmokeTest(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	results, err := s.applicationAPI.Get(params.ApplicationGet{ApplicationName: "wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationGetResults{
		Application: "wordpress",
		Charm:       "wordpress",
		CharmConfig: map[string]interface{}{
			"blog-title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the blog.",
				"source":      "default",
				"type":        "string",
				"value":       "My Title",
			},
		},
		ApplicationConfig: map[string]interface{}{
			"trust": map[string]interface{}{
				"default":     false,
				"description": "Does this application have access to trusted credentials",
				"source":      "default",
				"type":        environschema.Tbool,
				"value":       false,
			}},
		Constraints: constraints.MustParse("arch=amd64"),
		Base:        params.Base{Name: "ubuntu", Channel: "12.10/stable"},
		EndpointBindings: map[string]string{
			"":                network.AlphaSpaceName,
			"admin-api":       network.AlphaSpaceName,
			"cache":           network.AlphaSpaceName,
			"db":              network.AlphaSpaceName,
			"db-client":       network.AlphaSpaceName,
			"foo-bar":         network.AlphaSpaceName,
			"logging-dir":     network.AlphaSpaceName,
			"monitoring-port": network.AlphaSpaceName,
			"url":             network.AlphaSpaceName,
		},
	})
}

func (s *getSuite) TestClientApplicationGetCAASModelSmokeTest(c *gc.C) {
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
	st := s.Factory.MakeCAASModel(c, nil)
	defer st.Close()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "dashboard4miner", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "dashboard4miner", Charm: ch,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable"}},
	})

	schemaFields, err := caas.ConfigSchema(provider.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(provider.ConfigDefaults())

	schemaFields, defaults, err = application.AddTrustSchemaAndDefaults(schemaFields, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appConfig, err := coreconfig.NewConfig(map[string]interface{}{"juju-external-hostname": "ext"}, schemaFields, defaults)
	c.Assert(err, jc.ErrorIsNil)
	err = app.UpdateApplicationConfig(appConfig.Attributes(), nil, schemaFields, defaults)
	c.Assert(err, jc.ErrorIsNil)

	expectedAppConfig := make(map[string]interface{})
	for name, field := range schemaFields {
		info := map[string]interface{}{
			"description": field.Description,
			"source":      "unset",
			"type":        field.Type,
		}
		expectedAppConfig[name] = info
	}

	for name, val := range appConfig.Attributes() {
		field := schemaFields[name]
		info := map[string]interface{}{
			"description": field.Description,
			"source":      "unset",
			"type":        field.Type,
		}
		if val != nil {
			info["source"] = "user"
			info["value"] = val
		}
		if defaultVal := defaults[name]; defaultVal != nil {
			info["default"] = defaultVal
			info["source"] = "default"
			if val != defaultVal {
				info["source"] = "user"
			}
		}
		expectedAppConfig[name] = info
	}

	storageAccess, err := application.GetStorageState(st)
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(st)
	mod, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	api, err := application.NewAPIBase(
		application.GetState(st),
		storageAccess,
		s.authorizer,
		nil,
		blockChecker,
		application.GetModel(mod),
		nil, // leadership not used in this suite.
		application.CharmToStateCharm,
		application.DeployApplication,
		&mockStoragePoolManager{},
		&mockStorageRegistry{},
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.Get(params.ApplicationGet{ApplicationName: "dashboard4miner"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationGetResults{
		Application: "dashboard4miner",
		Charm:       "dashboard4miner",
		CharmConfig: map[string]interface{}{
			"port": map[string]interface{}{
				"default":     int64(443),
				"description": "https port",
				"source":      "default",
				"type":        "int",
				"value":       int64(443),
			},
		},
		ApplicationConfig: expectedAppConfig,
		Constraints:       constraints.MustParse("arch=amd64"),
		Base:              params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		EndpointBindings: map[string]string{
			"":      network.AlphaSpaceName,
			"miner": network.AlphaSpaceName,
		},
	})
}

func (s *getSuite) TestApplicationGetUnknownApplication(c *gc.C) {
	_, err := s.applicationAPI.Get(params.ApplicationGet{ApplicationName: "unknown"})
	c.Assert(err, gc.ErrorMatches, `application "unknown" not found`)
}

var getTests = []struct {
	about       string
	charm       string
	constraints string
	origin      *state.CharmOrigin
	config      charm.Settings
	expect      params.ApplicationGetResults
}{{
	about:       "deployed application",
	charm:       "dummy",
	constraints: "arch=amd64 mem=2G cpu-power=400",
	config: charm.Settings{
		// Different from default.
		"title": "Look To Windward",
		// Same as default.
		"username": "admin001",
		// Use default (but there's no charm default)
		"skill-level": nil,
		// Outlook is left unset.
	},
	origin: &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: intPtr(7),
		Channel: &state.Channel{
			Risk: "stable",
		},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: corearch.DefaultArchitecture},
	},
	expect: params.ApplicationGetResults{
		CharmConfig: map[string]interface{}{
			"title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the application.",
				"source":      "user",
				"type":        "string",
				"value":       "Look To Windward",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"source":      "unset",
				"type":        "string",
			},
			"username": map[string]interface{}{
				"default":     "admin001",
				"description": "The name of the initial account (given admin permissions).",
				"source":      "default",
				"type":        "string",
				"value":       "admin001",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"source":      "unset",
				"type":        "int",
			},
		},
		ApplicationConfig: map[string]interface{}{
			"trust": map[string]interface{}{
				"value":       false,
				"default":     false,
				"description": "Does this application have access to trusted credentials",
				"source":      "default",
				"type":        "bool",
			},
		},
		Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		EndpointBindings: map[string]string{
			"": network.AlphaSpaceName,
		},
	},
}, {
	about:       "deployed application  #2",
	charm:       "dummy",
	constraints: "arch=amd64",
	config: charm.Settings{
		// Set title to default.
		"title": nil,
		// Value when there's a default.
		"username": "foobie",
		// Numeric value.
		"skill-level": 0,
		// String value.
		"outlook": "phlegmatic",
	},
	origin: &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: intPtr(7),
		Channel: &state.Channel{
			Risk: "stable",
		},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: corearch.DefaultArchitecture},
	},
	expect: params.ApplicationGetResults{
		CharmConfig: map[string]interface{}{
			"title": map[string]interface{}{
				"default":     "My Title",
				"description": "A descriptive title used for the application.",
				"source":      "default",
				"type":        "string",
				"value":       "My Title",
			},
			"outlook": map[string]interface{}{
				"description": "No default outlook.",
				"type":        "string",
				"source":      "user",
				"value":       "phlegmatic",
			},
			"username": map[string]interface{}{
				"default":     "admin001",
				"description": "The name of the initial account (given admin permissions).",
				"source":      "user",
				"type":        "string",
				"value":       "foobie",
			},
			"skill-level": map[string]interface{}{
				"description": "A number indicating skill.",
				"source":      "user",
				"type":        "int",
				// TODO(jam): 2013-08-28 bug #1217742
				// we have to use float64() here, because the
				// API does not preserve int types. This used
				// to be int64() but we end up with a type
				// mismatch when comparing the content
				"value": float64(0),
			},
		},
		ApplicationConfig: map[string]interface{}{
			"trust": map[string]interface{}{
				"value":       false,
				"default":     false,
				"description": "Does this application have access to trusted credentials",
				"source":      "default",
				"type":        "bool",
			},
		},
		Base: params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		EndpointBindings: map[string]string{
			"": network.AlphaSpaceName,
		},
	},
}, {
	about: "subordinate application",
	charm: "logging",
	origin: &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: intPtr(7),
		Channel: &state.Channel{
			Risk: "stable",
		},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: corearch.DefaultArchitecture},
	},
	expect: params.ApplicationGetResults{
		CharmConfig: map[string]interface{}{},
		Base:        params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		ApplicationConfig: map[string]interface{}{
			"trust": map[string]interface{}{
				"value":       false,
				"default":     false,
				"description": "Does this application have access to trusted credentials",
				"source":      "default",
				"type":        "bool",
			},
		},
		EndpointBindings: map[string]string{
			"":                  network.AlphaSpaceName,
			"info":              network.AlphaSpaceName,
			"logging-client":    network.AlphaSpaceName,
			"logging-directory": network.AlphaSpaceName,
		},
	},
}, {
	about: "charmhub subordinate application",
	charm: "logging",
	origin: &state.CharmOrigin{
		Source:   "charm-hub",
		Revision: intPtr(7),
		Channel: &state.Channel{
			Risk:   "stable",
			Branch: "foo",
		},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: corearch.DefaultArchitecture},
	},
	expect: params.ApplicationGetResults{
		CharmConfig: map[string]interface{}{},
		Base:        params.Base{Name: "ubuntu", Channel: "22.04/stable"},
		ApplicationConfig: map[string]interface{}{
			"trust": map[string]interface{}{
				"value":       false,
				"default":     false,
				"description": "Does this application have access to trusted credentials",
				"source":      "default",
				"type":        "bool",
			},
		},
		EndpointBindings: map[string]string{
			"":                  network.AlphaSpaceName,
			"info":              network.AlphaSpaceName,
			"logging-client":    network.AlphaSpaceName,
			"logging-directory": network.AlphaSpaceName,
		},
		Channel: "stable/foo",
	},
}}

func intPtr(val int) *int {
	return &val
}

func (s *getSuite) TestApplicationGet(c *gc.C) {
	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		ch := s.AddTestingCharm(c, t.charm)
		app := s.AddTestingApplicationWithOrigin(c, fmt.Sprintf("test%d", i), ch, t.origin)

		var constraintsv constraints.Value
		if t.constraints != "" {
			constraintsv = constraints.MustParse(t.constraints)
			err := app.SetConstraints(constraintsv)
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.config != nil {
			err := app.UpdateCharmConfig(model.GenerationMaster, t.config)
			c.Assert(err, jc.ErrorIsNil)
		}
		expect := t.expect
		expect.Constraints = constraintsv
		expect.Application = app.Name()
		expect.Charm = ch.Meta().Name
		client := apiapplication.NewClient(s.APIState)
		got, err := client.Get(model.GenerationMaster, app.Name())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(*got, jc.DeepEquals, expect)
	}
}

func (s *getSuite) TestGetMaxResolutionInt(c *gc.C) {
	// See the bug http://pad.lv/1217742
	// Get ends up pushing a map[string]interface{} which contains
	// an int64 through a JSON Marshal & Unmarshal which ends up changing
	// the int64 into a float64. We will fix it if we find it is actually a
	// problem.
	const nonFloatInt = (int64(1) << 54) + 1
	const asFloat = float64(nonFloatInt)
	c.Assert(int64(asFloat), gc.Not(gc.Equals), nonFloatInt)
	c.Assert(int64(asFloat)+1, gc.Equals, nonFloatInt)

	ch := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "test-application", ch)

	err := app.UpdateCharmConfig(model.GenerationMaster, map[string]interface{}{"skill-level": nonFloatInt})
	c.Assert(err, jc.ErrorIsNil)
	client := apiapplication.NewClient(s.APIState)
	got, err := client.Get(model.GenerationMaster, app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.CharmConfig["skill-level"], jc.DeepEquals, map[string]interface{}{
		"description": "A number indicating skill.",
		"source":      "user",
		"type":        "int",
		"value":       asFloat,
	})
}
