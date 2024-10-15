// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"context"
	"fmt"

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
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

type getSuite struct {
	jujutesting.ApiServerSuite

	applicationAPI *application.APIBase
	authorizer     apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&getSuite{})

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *getSuite) modelConfigService(c *gc.C) application.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *getSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)
	s.ApiServerSuite.SeedCAASCloud(c)

	domainServices := s.DefaultModelDomainServices(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	st := s.ControllerModel(c).State()
	storageAccess, err := application.GetStorageState(st)
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(domainServices.BlockCommand())
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	envFunc := stateenvirons.GetNewEnvironFunc(environs.New)
	env, err := envFunc(s.ControllerModel(c), domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	c.Assert(err, jc.ErrorIsNil)
	registry := stateenvirons.NewStorageProviderRegistry(env)

	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: registry,
		Secrets:         service.NotImplementedSecretService{},
	})

	api, err := application.NewAPIBase(
		application.GetState(st, s.modelConfigService(c)),
		nil,
		domainServices.Network(),
		storageAccess,
		s.authorizer,
		nil,
		blockChecker,
		application.GetModel(m),
		model.ReadOnlyModel{},
		domainServices.Config(),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Machine(),
		applicationService,
		domainServices.Port(),
		domainServices.Stub(),
		nil, // leadership not used in this suite.
		application.CharmToStateCharm,
		application.DeployApplication,
		domainServices.Storage(registry),
		registry,
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
		jujutesting.NewObjectStore(c, st.ControllerModelUUID()),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.applicationAPI = api
}

func (s *getSuite) TestClientApplicationGetIAASModelSmokeTest(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))
	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})

	results, err := s.applicationAPI.Get(context.Background(), params.ApplicationGet{ApplicationName: "wordpress"})
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
		Channel:     "stable",
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
	c.Skip("TODO(units): fails because test models not dual written to mongo and dqlite")
	s.PatchValue(&provider.NewK8sClients, k8stesting.NoopFakeK8sClients)
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	st := f.MakeCAASModel(c, nil)
	defer st.Close()
	f2, release := s.NewFactory(c, st.ModelUUID())
	defer release()
	f2 = f2.WithModelConfigService(s.modelConfigService(c))
	ch := f2.MakeCharm(c, &factory.CharmParams{Name: "dashboard4miner", Series: "focal"})
	app := f2.MakeApplication(c, &factory.ApplicationParams{
		Name: "dashboard4miner", Charm: ch,
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-hub",
			Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: "amd64"}},
	})

	schemaFields, defaults, err := application.ConfigSchema()
	c.Assert(err, jc.ErrorIsNil)

	appConfig, err := coreconfig.NewConfig(map[string]interface{}{"trust": true}, schemaFields, defaults)
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

	domainServices := s.DefaultModelDomainServices(c)

	storageAccess, err := application.GetStorageState(st)
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(domainServices.BlockCommand())
	mod, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	registry, err := stateenvirons.NewStorageProviderRegistryForModel(
		mod,
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Config(),
		stateenvirons.GetNewEnvironFunc(environs.New),
		stateenvirons.GetNewCAASBrokerFunc(caas.New),
	)
	c.Assert(err, jc.ErrorIsNil)

	applicationService := domainServices.Application(service.ApplicationServiceParams{
		StorageRegistry: registry,
		Secrets:         service.NotImplementedSecretService{},
	})

	api, err := application.NewAPIBase(
		application.GetState(st, s.modelConfigService(c)),
		nil,
		domainServices.Network(),
		storageAccess,
		s.authorizer,
		nil,
		blockChecker,
		application.GetModel(mod),
		model.ReadOnlyModel{},
		domainServices.Config(),
		domainServices.Cloud(),
		domainServices.Credential(),
		domainServices.Machine(),
		applicationService,
		domainServices.Port(),
		domainServices.Stub(),
		nil, // leadership not used in this suite.
		application.CharmToStateCharm,
		application.DeployApplication,
		domainServices.Storage(registry),
		registry,
		common.NewResources(),
		nil, // CAAS Broker not used in this suite.
		jujutesting.NewObjectStore(c, st.ControllerModelUUID()),
		loggertesting.WrapCheckLog(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := api.Get(context.Background(), params.ApplicationGet{ApplicationName: "dashboard4miner"})
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
	_, err := s.applicationAPI.Get(context.Background(), params.ApplicationGet{ApplicationName: "unknown"})
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
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: "amd64"},
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
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: "amd64"},
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
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: "amd64"},
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
		Source: "charm-hub",
		Channel: &state.Channel{
			Risk:   "stable",
			Branch: "foo",
		},
		Platform: &state.Platform{OS: "ubuntu", Channel: "22.04/stable", Architecture: "amd64"},
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

func (s *getSuite) TestApplicationGet(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))

	for i, t := range getTests {
		c.Logf("test %d. %s", i, t.about)
		ch := f.MakeCharm(c, &factory.CharmParams{Name: t.charm})
		app := f.MakeApplication(c, &factory.ApplicationParams{
			Name:        fmt.Sprintf("test%d", i),
			Charm:       ch,
			CharmOrigin: t.origin,
		})
		var constraintsv constraints.Value
		if t.constraints != "" {
			constraintsv = constraints.MustParse(t.constraints)
			err := app.SetConstraints(constraintsv)
			c.Assert(err, jc.ErrorIsNil)
		}
		if t.config != nil {
			err := app.UpdateCharmConfig(t.config)
			c.Assert(err, jc.ErrorIsNil)
		}
		expect := t.expect
		expect.Constraints = constraintsv
		expect.Application = app.Name()
		expect.Charm = ch.Meta().Name
		client := apiapplication.NewClient(s.OpenControllerModelAPI(c))
		got, err := client.Get(context.Background(), app.Name())
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(s.modelConfigService(c))
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "test-application",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "dummy"}),
	})

	err := app.UpdateCharmConfig(map[string]interface{}{"skill-level": nonFloatInt})
	c.Assert(err, jc.ErrorIsNil)
	client := apiapplication.NewClient(s.OpenControllerModelAPI(c))
	got, err := client.Get(context.Background(), app.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got.CharmConfig["skill-level"], jc.DeepEquals, map[string]interface{}{
		"description": "A number indicating skill.",
		"source":      "user",
		"type":        "int",
		"value":       asFloat,
	})
}
