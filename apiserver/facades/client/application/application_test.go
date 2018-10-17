// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	charmresource "gopkg.in/juju/charm.v6/resource"
	"gopkg.in/juju/charmrepo.v3"
	"gopkg.in/juju/charmrepo.v3/csclient"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v2-unstable"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/osenv"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
	statestorage "github.com/juju/juju/state/storage"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/poolmanager"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	jujuversion "github.com/juju/juju/version"
)

type applicationSuite struct {
	jujutesting.JujuConnSuite
	apiservertesting.CharmStoreSuite
	commontesting.BlockHelper

	applicationAPI *application.APIv8
	application    *state.Application
	authorizer     *apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&applicationSuite{})

func (s *applicationSuite) SetUpSuite(c *gc.C) {
	s.CharmStoreSuite.SetUpSuite(c)
	s.JujuConnSuite.SetUpSuite(c)
}

func (s *applicationSuite) TearDownSuite(c *gc.C) {
	s.CharmStoreSuite.TearDownSuite(c)
	s.JujuConnSuite.TearDownSuite(c)
}

func (s *applicationSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.CharmStoreSuite.Session = s.JujuConnSuite.Session
	s.CharmStoreSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	s.application = s.Factory.MakeApplication(c, nil)

	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.applicationAPI = s.makeAPI(c)

	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, feature.LXDProfile)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *applicationSuite) TearDownTest(c *gc.C) {
	s.CharmStoreSuite.TearDownTest(c)
	s.JujuConnSuite.TearDownTest(c)
}

func (s *applicationSuite) makeAPI(c *gc.C) *application.APIv8 {
	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))
	storageAccess, err := application.GetStorageState(s.State)
	c.Assert(err, jc.ErrorIsNil)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	blockChecker := common.NewBlockChecker(s.State)
	registry := stateenvirons.NewStorageProviderRegistry(s.Environ)
	pm := poolmanager.New(state.NewStateSettings(s.State), registry)
	api, err := application.NewAPIBase(
		application.GetState(s.State),
		storageAccess,
		s.authorizer,
		blockChecker,
		model.ModelTag(),
		model.Type(),
		application.CharmToStateCharm,
		application.DeployApplication,
		pm,
	)
	c.Assert(err, jc.ErrorIsNil)
	return &application.APIv8{api}
}

func (s *applicationSuite) TestGetConfig(c *gc.C) {
	fooConfig := map[string]interface{}{
		"title":       "foo",
		"skill-level": 42,
	}
	dummy := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "dummy",
	})
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "foo",
		Charm:       dummy,
		CharmConfig: fooConfig,
	})
	barConfig := map[string]interface{}{
		"title":   "bar",
		"outlook": "fantastic",
	}
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "bar",
		Charm:       dummy,
		CharmConfig: barConfig,
	})
	results, err := s.applicationAPI.GetConfig(params.Entities{
		Entities: []params.Entity{
			{"wat"}, {"machine-0"}, {"user-foo"},
			{"application-foo"}, {"application-bar"}, {"application-wat"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationGetConfigResults{
		Results: []params.ConfigResult{
			{
				Error: &params.Error{Message: `"wat" is not a valid tag`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got machine`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got user`},
			}, {
				Config: map[string]interface{}{
					"outlook": map[string]interface{}{
						"description": "No default outlook.",
						"source":      "unset",
						"type":        "string",
					},
					"skill-level": map[string]interface{}{
						"description": "A number indicating skill.",
						"source":      "user",
						"type":        "int",
						"value":       42,
					},
					"title": map[string]interface{}{
						"default":     "My Title",
						"description": "A descriptive title used for the application.",
						"source":      "user",
						"type":        "string",
						"value":       "foo",
					},
					"username": map[string]interface{}{
						"default":     "admin001",
						"description": "The name of the initial account (given admin permissions).",
						"source":      "default",
						"type":        "string",
						"value":       "admin001",
					},
				},
			}, {
				Config: map[string]interface{}{
					"outlook": map[string]interface{}{
						"description": "No default outlook.",
						"source":      "user",
						"type":        "string",
						"value":       "fantastic",
					},
					"skill-level": map[string]interface{}{
						"description": "A number indicating skill.",
						"source":      "unset",
						"type":        "int",
					},
					"title": map[string]interface{}{
						"default":     "My Title",
						"description": "A descriptive title used for the application.",
						"source":      "user",
						"type":        "string",
						"value":       "bar",
					},
					"username": map[string]interface{}{
						"default":     "admin001",
						"description": "The name of the initial account (given admin permissions).",
						"source":      "default",
						"type":        "string",
						"value":       "admin001",
					},
				},
			}, {
				Error: &params.Error{Message: `application "wat" not found`, Code: "not found"},
			},
		}})

}

func (s *applicationSuite) TestSetMetricCredentials(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: charm,
	})
	tests := []struct {
		about   string
		args    params.ApplicationMetricCredentials
		results params.ErrorResults
	}{
		{
			"test one argument and it passes",
			params.ApplicationMetricCredentials{[]params.ApplicationMetricCredential{{
				s.application.Name(),
				[]byte("creds 1234"),
			}}},
			params.ErrorResults{[]params.ErrorResult{{Error: nil}}},
		},
		{
			"test two arguments and both pass",
			params.ApplicationMetricCredentials{[]params.ApplicationMetricCredential{
				{
					s.application.Name(),
					[]byte("creds 1234"),
				},
				{
					wordpress.Name(),
					[]byte("creds 4567"),
				},
			}},
			params.ErrorResults{[]params.ErrorResult{
				{Error: nil},
				{Error: nil},
			}},
		},
		{
			"test two arguments and second one fails",
			params.ApplicationMetricCredentials{[]params.ApplicationMetricCredential{
				{
					s.application.Name(),
					[]byte("creds 1234"),
				},
				{
					"not-a-application",
					[]byte("creds 4567"),
				},
			}},
			params.ErrorResults{[]params.ErrorResult{
				{Error: nil},
				{Error: &params.Error{Message: `application "not-a-application" not found`, Code: "not found"}},
			}},
		},
	}
	for i, t := range tests {
		c.Logf("Running test %d %v", i, t.about)
		results, err := s.applicationAPI.SetMetricCredentials(t.args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, len(t.results.Results))
		c.Assert(results, gc.DeepEquals, t.results)

		for i, a := range t.args.Creds {
			if t.results.Results[i].Error == nil {
				app, err := s.State.Application(a.ApplicationName)
				c.Assert(err, jc.ErrorIsNil)
				creds := app.MetricCredentials()
				c.Assert(creds, gc.DeepEquals, a.MetricCredentials)
			}
		}
	}
}

func (s *applicationSuite) TestCompatibleSettingsParsing(c *gc.C) {
	// Test the exported settings parsing in a compatible way.
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	app, err := s.State.Application("dummy")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "local:quantal/dummy-1")

	// Empty string will be returned as nil.
	options := map[string]string{
		"title":    "foobar",
		"username": "",
	}
	settings, err := application.ParseSettingsCompatible(ch.Config(), options)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{
		"title":    "foobar",
		"username": nil,
	})

	// Illegal settings lead to an error.
	options = map[string]string{
		"yummy": "didgeridoo",
	}
	_, err = application.ParseSettingsCompatible(ch.Config(), options)
	c.Assert(err, gc.ErrorMatches, `unknown option "yummy"`)
}

func (s *applicationSuite) TestApplicationDeployWithStorage(c *gc.C) {
	curl, ch := s.UploadCharm(c, "utopic/storage-block-10", "storage-block")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	storageConstraints := map[string]storage.Constraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "modelscoped-block",
		},
	}

	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Constraints:     cons,
		Storage:         storageConstraints,
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	app := apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, cons)
	storageConstraintsOut, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageConstraintsOut, gc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "modelscoped-block",
		},
		"allecto": {
			Count: 0,
			Size:  1024,
			Pool:  "loop",
		},
	})
}

func (s *applicationSuite) TestMinJujuVersionTooHigh(c *gc.C) {
	curl, _ := s.UploadCharm(c, "quantal/minjujuversion-0", "minjujuversion")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	match := fmt.Sprintf(`charm's min version (999.999.999) is higher than this juju model's version (%s)`, jujuversion.Current)
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(match))
}

func (s *applicationSuite) TestApplicationDeployWithInvalidStoragePool(c *gc.C) {
	curl, _ := s.UploadCharm(c, "utopic/storage-block-0", "storage-block")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	storageConstraints := map[string]storage.Constraints{
		"data": {
			Pool:  "foo",
			Count: 1,
			Size:  1024,
		},
	}

	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Constraints:     cons,
		Storage:         storageConstraints,
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `.* pool "foo" not found`)
}

func (s *applicationSuite) TestApplicationDeployDefaultFilesystemStorage(c *gc.C) {
	curl, ch := s.UploadCharm(c, "trusty/storage-filesystem-1", "storage-filesystem")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Constraints:     cons,
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	app := apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, cons)
	storageConstraintsOut, err := app.StorageConstraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(storageConstraintsOut, gc.DeepEquals, map[string]state.StorageConstraints{
		"data": {
			Count: 1,
			Size:  1024,
			Pool:  "rootfs",
		},
	})
}

func (s *applicationSuite) TestApplicationDeploy(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Constraints:     cons,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"},
		},
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	app := apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, cons)
	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	// Check that the charm cache dir is cleared out.
	files, err := ioutil.ReadDir(charmrepo.CacheDir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(files, gc.HasLen, 0)
}

func (s *applicationSuite) TestApplicationDeployWithInvalidPlacement(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Constraints:     cons,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "invalid"},
		},
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.NotNil)
	c.Assert(results.Results[0].Error.Error(), gc.Matches, ".* invalid placement is invalid")
}

func (s *applicationSuite) TestApplicationDeploymentRemovesPendingResourcesOnFailure(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy-resource")
	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	pendingID, err := resources.AddPendingResource("haha/borken", "user", charmresource.Resource{
		Meta:   charm.Meta().Resources["dummy"],
		Origin: charmresource.OriginUpload,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "haha/borken",
			NumUnits:        1,
			CharmURL:        charm.URL().String(),
			Resources:       map[string]string{"dummy": pendingID},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `cannot add application "haha/borken": invalid name`)

	res, err := resources.ListPendingResources("haha/borken")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.HasLen, 0)
}

func (s *applicationSuite) TestApplicationDeploymentLeavesResourcesOnSuccess(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy-resource")
	resources, err := s.State.Resources()
	c.Assert(err, jc.ErrorIsNil)
	pendingID, err := resources.AddPendingResource("unborken", "user", charmresource.Resource{
		Meta:   charm.Meta().Resources["dummy"],
		Origin: charmresource.OriginUpload,
	})
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "unborken",
			NumUnits:        1,
			CharmURL:        charm.URL().String(),
			Resources:       map[string]string{"dummy": pendingID},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	res, err := resources.ListResources("unborken")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Resources, gc.HasLen, 1)
}

func (s *applicationSuite) TestApplicationDeploymentWithTrust(c *gc.C) {
	// This test should fail if the configuration parsing does not
	// understand the "trust" configuration parameter
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	config := map[string]string{"trust": "true"}
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Config:          config,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"},
		},
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	app := apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, cons)

	appConfig, err := app.ApplicationConfig()
	c.Assert(err, jc.ErrorIsNil)

	trust := appConfig.GetBool("trust", false)
	c.Assert(trust, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationDeploymentNoTrust(c *gc.C) {
	// This test should fail if the trust configuration setting defaults to
	// anything other than "false" when no configuration parameter for trust
	// is set at deployment.
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		NumUnits:        1,
		Placement: []*instance.Placement{
			{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"},
		},
	}
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	app := apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, cons)
	appConfig, err := app.ApplicationConfig()
	trust := appConfig.GetBool(application.TrustConfigOptionName, true)
	c.Assert(trust, jc.IsFalse)
}

func (s *applicationSuite) testClientApplicationsDeployWithBindings(c *gc.C, endpointBindings, expected map[string]string) {
	curl, _ := s.UploadCharm(c, "utopic/riak-42", "riak")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	var cons constraints.Value
	args := params.ApplicationDeploy{
		ApplicationName:  "application",
		CharmURL:         curl.String(),
		NumUnits:         1,
		Constraints:      cons,
		EndpointBindings: endpointBindings,
	}

	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{args}},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application(args.ApplicationName)
	c.Assert(err, jc.ErrorIsNil)

	retrievedBindings, err := application.EndpointBindings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(retrievedBindings, jc.DeepEquals, expected)
}

func (s *applicationSuite) TestClientApplicationsDeployWithBindings(c *gc.C) {
	s.State.AddSpace("a-space", "", nil, true)
	expected := map[string]string{
		"endpoint": "a-space",
		"ring":     "",
		"admin":    "",
	}
	endpointBindings := map[string]string{"endpoint": "a-space"}
	s.testClientApplicationsDeployWithBindings(c, endpointBindings, expected)
}

func (s *applicationSuite) TestClientApplicationsDeployWithDefaultBindings(c *gc.C) {
	expected := map[string]string{
		"endpoint": "",
		"ring":     "",
		"admin":    "",
	}
	s.testClientApplicationsDeployWithBindings(c, nil, expected)
}

// TODO(wallyworld) - the following charm tests have been moved from the apiserver/client
// package in order to use the fake charm store testing infrastructure. They are legacy tests
// written to use the api client instead of the apiserver logic. They need to be rewritten and
// feature tests added.

func (s *applicationSuite) TestAddCharm(c *gc.C) {
	var blobs blobs
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) statestorage.Storage {
		storage := statestorage.NewStorage(uuid, session)
		return &recordingStorage{Storage: storage, blobs: &blobs}
	})

	client := s.APIState.Client()
	// First test the sanity checks.
	err := client.AddCharm(&charm.URL{Name: "nonsense"}, csparams.StableChannel, false)
	c.Assert(err, gc.ErrorMatches, `cannot parse charm or bundle URL: ":nonsense-0"`)
	err = client.AddCharm(charm.MustParseURL("local:precise/dummy"), csparams.StableChannel, false)
	c.Assert(err, gc.ErrorMatches, "only charm store charm URLs are supported, with cs: schema")
	err = client.AddCharm(charm.MustParseURL("cs:precise/wordpress"), csparams.StableChannel, false)
	c.Assert(err, gc.ErrorMatches, "charm URL must include revision")

	// Add a charm, without uploading it to storage, to
	// check that AddCharm does not try to do it.
	charmDir := testcharms.Repo.CharmDir("dummy")
	ident := fmt.Sprintf("%s-%d", charmDir.Meta().Name, charmDir.Revision())
	curl := charm.MustParseURL("cs:quantal/" + ident)
	info := state.CharmInfo{
		Charm:       charmDir,
		ID:          curl,
		StoragePath: "",
		SHA256:      ident + "-sha256",
	}
	sch, err := s.State.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)

	// AddCharm should see the charm in state and not upload it.
	err = client.AddCharm(sch.URL(), csparams.StableChannel, false)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(blobs.m, gc.HasLen, 0)

	// Now try adding another charm completely.
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = client.AddCharm(curl, csparams.StableChannel, false)
	c.Assert(err, jc.ErrorIsNil)

	// Verify it's in state and it got uploaded.
	storage := statestorage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	sch, err = s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

func (s *applicationSuite) TestAddCharmWithAuthorization(c *gc.C) {
	// Upload a new charm to the charm store.
	curl, _ := s.UploadCharm(c, "cs:~restricted/precise/wordpress-3", "wordpress")

	// Change permissions on the new charm such that only bob
	// can read from it.
	s.DischargeUser = "restricted"
	err := s.Client.Put("/"+curl.Path()+"/meta/perm/read", []string{"bob"})
	c.Assert(err, jc.ErrorIsNil)

	// Try to add a charm to the model without authorization.
	s.DischargeUser = ""
	err = s.APIState.Client().AddCharm(curl, csparams.StableChannel, false)
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: cannot get discharge from "https://.*": third party refused discharge: cannot discharge: discharge denied \(unauthorized access\)`)

	tryAs := func(user string) error {
		client := csclient.New(csclient.Params{
			URL: s.Srv.URL,
		})
		s.DischargeUser = user
		var m *macaroon.Macaroon
		err = client.Get("/delegatable-macaroon", &m)
		c.Assert(err, gc.IsNil)

		return application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
			URL:     curl.String(),
			Channel: string(csparams.StableChannel),
		})
	}
	// Try again with authorization for the wrong user.
	err = tryAs("joe")
	c.Assert(err, gc.ErrorMatches, `cannot retrieve charm "cs:~restricted/precise/wordpress-3": cannot get archive: access denied for user "joe"`)

	// Try again with the correct authorization this time.
	err = tryAs("bob")
	c.Assert(err, gc.IsNil)

	// Verify that it has actually been uploaded.
	_, err = s.State.Charm(curl)
	c.Assert(err, gc.IsNil)
}

func (s *applicationSuite) TestAddCharmConcurrently(c *gc.C) {
	c.Skip("see lp:1596960 -- bad test for bad code")

	var putBarrier sync.WaitGroup
	var blobs blobs
	s.PatchValue(application.NewStateStorage, func(uuid string, session *mgo.Session) statestorage.Storage {
		storage := statestorage.NewStorage(uuid, session)
		return &recordingStorage{Storage: storage, blobs: &blobs, putBarrier: &putBarrier}
	})

	client := s.APIState.Client()
	curl, _ := s.UploadCharm(c, "trusty/wordpress-3", "wordpress")

	// Try adding the same charm concurrently from multiple goroutines
	// to test no "duplicate key errors" are reported (see lp bug
	// #1067979) and also at the end only one charm document is
	// created.

	var wg sync.WaitGroup
	// We don't add them 1-by-1 because that would allow each goroutine to
	// finish separately without actually synchronizing between them
	putBarrier.Add(10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			c.Assert(client.AddCharm(curl, csparams.StableChannel, false), gc.IsNil, gc.Commentf("goroutine %d", index))
			sch, err := s.State.Charm(curl)
			c.Assert(err, gc.IsNil, gc.Commentf("goroutine %d", index))
			c.Assert(sch.URL(), jc.DeepEquals, curl, gc.Commentf("goroutine %d", index))
		}(i)
	}
	wg.Wait()

	blobs.Lock()

	c.Assert(blobs.m, gc.HasLen, 10)

	// Verify there is only a single uploaded charm remains and it
	// contains the correct data.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	storagePath := sch.StoragePath()
	c.Assert(blobs.m[storagePath], jc.IsTrue)
	for path, exists := range blobs.m {
		if path != storagePath {
			c.Assert(exists, jc.IsFalse)
		}
	}

	storage := statestorage.NewStorage(s.State.ModelUUID(), s.State.MongoSession())
	s.assertUploaded(c, storage, sch.StoragePath(), sch.BundleSha256())
}

func (s *applicationSuite) assertUploaded(c *gc.C, storage statestorage.Storage, storagePath, expectedSHA256 string) {
	reader, _, err := storage.Get(storagePath)
	c.Assert(err, jc.ErrorIsNil)
	defer reader.Close()
	downloadedSHA256, _, err := utils.ReadSHA256(reader)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(downloadedSHA256, gc.Equals, expectedSHA256)
}

func (s *applicationSuite) TestAddCharmOverwritesPlaceholders(c *gc.C) {
	client := s.APIState.Client()
	curl, _ := s.UploadCharm(c, "trusty/wordpress-42", "wordpress")

	// Add a placeholder with the same charm URL.
	err := s.State.AddStoreCharmPlaceholder(curl)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.Charm(curl)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Now try to add the charm, which will convert the placeholder to
	// a pending charm.
	err = client.AddCharm(curl, csparams.StableChannel, false)
	c.Assert(err, jc.ErrorIsNil)

	// Make sure the document's flags were reset as expected.
	sch, err := s.State.Charm(curl)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(sch.URL(), jc.DeepEquals, curl)
	c.Assert(sch.IsPlaceholder(), jc.IsFalse)
	c.Assert(sch.IsUploaded(), jc.IsTrue)
}

func (s *applicationSuite) TestApplicationGetCharmURL(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	result, err := s.applicationAPI.GetCharmURL(params.ApplicationGet{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result, gc.Equals, "local:quantal/wordpress-3")
}

func (s *applicationSuite) TestApplicationSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is not marked as forced.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsFalse)
}

func (s *applicationSuite) setupApplicationSetCharm(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) assertApplicationSetCharm(c *gc.C, forceUnits bool) {
	err := s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        "cs:~who/precise/wordpress-3",
		ForceUnits:      forceUnits,
	})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure that the charm is not marked as forced.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	charm, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, "cs:~who/precise/wordpress-3")
}

func (s *applicationSuite) assertApplicationSetCharmBlocked(c *gc.C, msg string) {
	err := s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        "cs:~who/precise/wordpress-3",
	})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyApplicationSetCharm(c *gc.C) {
	s.setupApplicationSetCharm(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationSetCharm")
	s.assertApplicationSetCharm(c, false)
}

func (s *applicationSuite) TestBlockRemoveApplicationSetCharm(c *gc.C) {
	s.setupApplicationSetCharm(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationSetCharm")
	s.assertApplicationSetCharm(c, false)
}

func (s *applicationSuite) TestBlockChangesApplicationSetCharm(c *gc.C) {
	s.setupApplicationSetCharm(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationSetCharm")
	s.assertApplicationSetCharmBlocked(c, "TestBlockChangesApplicationSetCharm")
}

func (s *applicationSuite) TestApplicationSetCharmForceUnits(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceUnits:      true,
	})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that the charm is marked as forced.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(charm.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsTrue)
}

func (s *applicationSuite) TestBlockApplicationSetCharmForce(c *gc.C) {
	s.setupApplicationSetCharm(c)

	// block all changes
	s.BlockAllChanges(c, "TestBlockApplicationSetCharmForce")
	s.BlockRemoveObject(c, "TestBlockApplicationSetCharmForce")
	s.BlockDestroyModel(c, "TestBlockApplicationSetCharmForce")

	s.assertApplicationSetCharm(c, true)
}

func (s *applicationSuite) TestApplicationSetCharmInvalidApplication(c *gc.C) {
	err := s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "badapplication",
		CharmURL:        "cs:precise/wordpress-3",
		ForceSeries:     true,
		ForceUnits:      true,
	})
	c.Assert(err, gc.ErrorMatches, `application "badapplication" not found`)
}

func (s *applicationSuite) TestApplicationAddCharmErrors(c *gc.C) {
	for url, expect := range map[string]string{
		"wordpress":                   "charm URL must include revision",
		"cs:wordpress":                "charm URL must include revision",
		"cs:precise/wordpress":        "charm URL must include revision",
		"cs:precise/wordpress-999999": `cannot retrieve "cs:precise/wordpress-999999": charm not found`,
	} {
		c.Logf("test %s", url)
		err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
			URL: url,
		})
		c.Check(err, gc.ErrorMatches, expect)
	}
}

func (s *applicationSuite) TestApplicationSetCharmLegacy(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Even with forceSeries = true, we can't change a charm where
	// the series is sepcified in the URL.
	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceSeries:     true,
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "cs:~who/trusty/dummy-1": cannot change an application's series`)
}

func (s *applicationSuite) TestApplicationSetCharmUnsupportedSeries(c *gc.C) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			Series:          "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series2")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "cs:~who/multi-series-1": only these series are supported: trusty, wily`)
}

func (s *applicationSuite) assertApplicationSetCharmSeries(c *gc.C, upgradeCharm, series string) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			Series:          "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	url := upgradeCharm
	if series != "" {
		url = series + "/" + upgradeCharm
	}
	curl, _ = s.UploadCharmMultiSeries(c, "~who/"+url, upgradeCharm)
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceSeries:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
	app, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := app.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, "cs:~who/"+url+"-0")
}

func (s *applicationSuite) TestApplicationSetCharmUnsupportedSeriesForce(c *gc.C) {
	s.assertApplicationSetCharmSeries(c, "multi-series2", "")
}

func (s *applicationSuite) TestApplicationSetCharmNoExplicitSupportedSeries(c *gc.C) {
	s.assertApplicationSetCharmSeries(c, "dummy", "precise")
}

func (s *applicationSuite) TestApplicationSetCharmWrongOS(c *gc.C) {
	curl, _ := s.UploadCharmMultiSeries(c, "~who/multi-series", "multi-series")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			Series:          "precise",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	curl, _ = s.UploadCharmMultiSeries(c, "~who/multi-series-windows", "multi-series-windows")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceSeries:     true,
	})
	c.Assert(err, gc.ErrorMatches, `cannot upgrade application "application" to charm "cs:~who/multi-series-windows-0": OS "Ubuntu" not supported by charm`)
}

type testModeCharmRepo struct {
	*charmrepo.CharmStore
	testMode bool
}

// WithTestMode returns a repository Interface where test mode is enabled.
func (s *testModeCharmRepo) WithTestMode() charmrepo.Interface {
	s.testMode = true
	return s.CharmStore.WithTestMode()
}

func (s *applicationSuite) TestSpecializeStoreOnDeployApplicationSetCharmAndAddCharm(c *gc.C) {
	repo := &testModeCharmRepo{}
	s.PatchValue(&csclient.ServerURL, s.Srv.URL)
	newCharmStoreRepo := application.NewCharmStoreRepo
	s.PatchValue(&application.NewCharmStoreRepo, func(c *csclient.Client) charmrepo.Interface {
		repo.CharmStore = newCharmStoreRepo(c).(*charmrepo.CharmStore)
		return repo
	})
	attrs := map[string]interface{}{"test-mode": true}
	err := s.Model.UpdateModelConfig(attrs, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Check that the store's test mode is enabled when calling application Deploy.
	curl, _ := s.UploadCharm(c, "trusty/dummy-1", "dummy")
	err = application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling SetCharm.
	curl, _ = s.UploadCharm(c, "trusty/wordpress-2", "wordpress")
	err = s.applicationAPI.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "application",
		CharmURL:        curl.String(),
	})
	c.Assert(repo.testMode, jc.IsTrue)

	// Check that the store's test mode is enabled when calling AddCharm.
	curl, _ = s.UploadCharm(c, "utopic/riak-42", "riak")
	err = s.APIState.Client().AddCharm(curl, csparams.StableChannel, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(repo.testMode, jc.IsTrue)
}

func (s *applicationSuite) setupApplicationDeploy(c *gc.C, args string) (*charm.URL, charm.Charm, constraints.Value) {
	curl, ch := s.UploadCharm(c, "precise/dummy-42", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	cons := constraints.MustParse(args)
	return curl, ch, cons
}

func (s *applicationSuite) assertApplicationDeployPrincipal(c *gc.C, curl *charm.URL, ch charm.Charm, mem4g constraints.Value) {
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	apiservertesting.AssertPrincipalApplicationDeployed(c, s.State, "application", curl, false, ch, mem4g)
}

func (s *applicationSuite) assertApplicationDeployPrincipalBlocked(c *gc.C, msg string, curl *charm.URL, mem4g constraints.Value) {
	_, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        3,
			Constraints:     mem4g,
		}}})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyApplicationDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupApplicationDeploy(c, "mem=4G")
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockRemoveApplicationDeployPrincipal(c *gc.C) {
	curl, bundle, cons := s.setupApplicationDeploy(c, "mem=4G")
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipal(c, curl, bundle, cons)
}

func (s *applicationSuite) TestBlockChangesApplicationDeployPrincipal(c *gc.C) {
	curl, _, cons := s.setupApplicationDeploy(c, "mem=4G")
	s.BlockAllChanges(c, "TestBlockChangesApplicationDeployPrincipal")
	s.assertApplicationDeployPrincipalBlocked(c, "TestBlockChangesApplicationDeployPrincipal", curl, cons)
}

func (s *applicationSuite) TestApplicationDeploySubordinate(c *gc.C) {
	curl, ch := s.UploadCharm(c, "utopic/logging-47", "logging")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 0)
}

func (s *applicationSuite) combinedSettings(ch *state.Charm, inSettings charm.Settings) charm.Settings {
	result := ch.Config().DefaultSettings()
	for name, value := range inSettings {
		result[name] = value
	}
	return result
}

func (s *applicationSuite) TestApplicationDeployConfig(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	settings, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{"username": "fred"}))
}

func (s *applicationSuite) TestApplicationDeployConfigError(c *gc.C) {
	// TODO(fwereade): test Config/ConfigYAML handling directly on srvClient.
	// Can't be done cleanly until it's extracted similarly to Machiner.
	curl, _ := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  skill-level: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `option "skill-level" expected int, got "fred"`)
	_, err = s.State.Application("application-name")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *applicationSuite) TestApplicationDeployToMachine(c *gc.C) {
	curl, ch := s.UploadCharm(c, "precise/dummy-0", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("precise", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
			NumUnits:        1,
			ConfigYAML:      "application-name:\n  username: fred",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	charm, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(charm.URL(), gc.DeepEquals, curl)
	c.Assert(charm.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(charm.Config(), gc.DeepEquals, ch.Config())

	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithLXDProfile(c *gc.C) {
	curl, ch := s.UploadCharm(c, "quantal/lxd-profile-0", "lxd-profile")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
			NumUnits:        1,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	expected, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(expected.URL(), gc.DeepEquals, curl)
	c.Assert(expected.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), gc.DeepEquals, ch.Config())
	c.Assert(expected.LXDProfile(), gc.DeepEquals, ch.(charm.LXDProfiler).LXDProfile())

	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineWithInvalidLXDProfile(c *gc.C) {
	curl, _ := s.UploadCharm(c, "quantal/lxd-profile-fail-0", "lxd-profile-fail")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, gc.ErrorMatches, `.*invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

func (s *applicationSuite) TestApplicationDeployToMachineWithInvalidLXDProfileAndForceStillSucceeds(c *gc.C) {
	curl, ch := s.UploadCharm(c, "quantal/lxd-profile-fail-0", "lxd-profile-fail")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL:   curl.String(),
		Force: true,
	})
	c.Assert(err, jc.ErrorIsNil)

	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application-name",
			NumUnits:        1,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	application, err := s.State.Application("application-name")
	c.Assert(err, jc.ErrorIsNil)
	expected, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(force, jc.IsFalse)
	c.Assert(expected.URL(), gc.DeepEquals, curl)
	c.Assert(expected.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(expected.Config(), gc.DeepEquals, ch.Config())
	c.Assert(expected.LXDProfile(), gc.DeepEquals, ch.(charm.LXDProfiler).LXDProfile())

	errs, err := s.APIState.UnitAssigner().AssignUnits([]names.UnitTag{names.NewUnitTag("application-name/0")})
	c.Assert(errs, gc.DeepEquals, []error{nil})
	c.Assert(err, jc.ErrorIsNil)

	units, err := application.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(units, gc.HasLen, 1)

	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id())
}

func (s *applicationSuite) TestApplicationDeployToMachineNotFound(c *gc.C) {
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        "cs:precise/application-name-1",
			ApplicationName: "application-name",
			NumUnits:        1,
			Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `cannot deploy "application-name" to machine 42: machine 42 not found`)

	_, err = s.State.Application("application-name")
	c.Assert(err, gc.ErrorMatches, `application "application-name" not found`)
}

func (s *applicationSuite) deployApplicationForUpdateTests(c *gc.C) {
	curl, _ := s.UploadCharm(c, "precise/dummy-1", "dummy")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.applicationAPI.Deploy(params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			CharmURL:        curl.String(),
			ApplicationName: "application",
			NumUnits:        1,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *applicationSuite) checkClientApplicationUpdateSetCharm(c *gc.C, forceCharmURL bool) {
	s.deployApplicationForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Update the charm for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceCharmURL:   forceCharmURL,
	}
	err = s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl.String())
	c.Assert(force, gc.Equals, forceCharmURL)
}

func (s *applicationSuite) TestApplicationUpdateSetCharm(c *gc.C) {
	s.checkClientApplicationUpdateSetCharm(c, false)
}

func (s *applicationSuite) TestBlockDestroyApplicationUpdate(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationUpdate")
	s.checkClientApplicationUpdateSetCharm(c, false)
}

func (s *applicationSuite) TestBlockRemoveApplicationUpdate(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationUpdate")
	s.checkClientApplicationUpdateSetCharm(c, false)
}

func (s *applicationSuite) setupApplicationUpdate(c *gc.C) string {
	s.deployApplicationForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return curl.String()
}

func (s *applicationSuite) TestBlockChangeApplicationUpdate(c *gc.C) {
	curl := s.setupApplicationUpdate(c)
	s.BlockAllChanges(c, "TestBlockChangeApplicationUpdate")
	// Update the charm for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "application",
		CharmURL:        curl,
		ForceCharmURL:   false,
	}
	err := s.applicationAPI.Update(args)
	s.AssertBlocked(c, err, "TestBlockChangeApplicationUpdate")
}

func (s *applicationSuite) TestApplicationUpdateForceSetCharm(c *gc.C) {
	s.checkClientApplicationUpdateSetCharm(c, true)
}

func (s *applicationSuite) TestBlockApplicationUpdateForced(c *gc.C) {
	curl := s.setupApplicationUpdate(c)

	// block all changes. Force should ignore block :)
	s.BlockAllChanges(c, "TestBlockApplicationUpdateForced")
	s.BlockDestroyModel(c, "TestBlockApplicationUpdateForced")
	s.BlockRemoveObject(c, "TestBlockApplicationUpdateForced")

	// Update the charm for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "application",
		CharmURL:        curl,
		ForceCharmURL:   true,
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the charm has been updated and and the force flag correctly set.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)
	ch, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl)
	c.Assert(force, jc.IsTrue)
}

func (s *applicationSuite) TestApplicationUpdateSetCharmNotFound(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	args := params.ApplicationUpdate{
		ApplicationName: "wordpress",
		CharmURL:        "cs:precise/wordpress-999999",
	}
	err := s.applicationAPI.Update(args)
	c.Check(err, gc.ErrorMatches, `charm "cs:precise/wordpress-999999" not found`)
}

func (s *applicationSuite) TestApplicationUpdateSetMinUnits(c *gc.C) {
	application := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set minimum units for the application.
	minUnits := 2
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		MinUnits:        &minUnits,
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(application.Refresh(), gc.IsNil)
	c.Assert(application.MinUnits(), gc.Equals, minUnits)
}

func (s *applicationSuite) TestApplicationUpdateSetMinUnitsWithLXDProfile(c *gc.C) {
	application := s.AddTestingApplication(c, "lxd-profile", s.AddTestingCharm(c, "lxd-profile"))

	// Set minimum units for the application.
	minUnits := 2
	args := params.ApplicationUpdate{
		ApplicationName: "lxd-profile",
		MinUnits:        &minUnits,
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the minimum number of units has been set.
	c.Assert(application.Refresh(), gc.IsNil)
	c.Assert(application.MinUnits(), gc.Equals, minUnits)
}

func (s *applicationSuite) TestApplicationUpdateDoesNotSetMinUnitsWithLXDProfile(c *gc.C) {
	series := "quantal"
	repo := testcharms.RepoForSeries(series)
	ch := repo.CharmDir("lxd-profile-fail")
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL(fmt.Sprintf("local:%s/%s", series, ident))
	storerepo, err := charmrepo.InferRepository(
		curl,
		charmrepo.NewCharmStoreParams{},
		repo.Path())
	c.Assert(err, jc.ErrorIsNil)
	_, err = jujutesting.PutCharm(s.State, curl, storerepo, false, false)
	c.Assert(err, gc.ErrorMatches, `invalid lxd-profile.yaml: contains device type "unix-disk"`)
}

func (s *applicationSuite) TestApplicationUpdateSetMinUnitsError(c *gc.C) {
	application := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Set a negative minimum number of units for the application.
	minUnits := -1
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		MinUnits:        &minUnits,
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, gc.ErrorMatches,
		`cannot set minimum units for application "dummy": cannot set a negative minimum number of units`)

	// Ensure the minimum number of units has not been set.
	c.Assert(application.Refresh(), gc.IsNil)
	c.Assert(application.MinUnits(), gc.Equals, 0)
}

func (s *applicationSuite) TestApplicationUpdateSetSettingsStrings(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", ch)

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		SettingsStrings: map[string]string{"title": "s-title", "username": "s-user"},
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "s-title", "username": "s-user"}
	obtained, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, s.combinedSettings(ch, expected))
}

func (s *applicationSuite) TestApplicationUpdateSetSettingsYAML(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", ch)

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		SettingsYAML:    "dummy:\n  title: y-title\n  username: y-user",
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, s.combinedSettings(ch, expected))
}

func (s *applicationSuite) TestClientApplicationUpdateSetSettingsGetYAML(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	application := s.AddTestingApplication(c, "dummy", ch)

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		SettingsYAML:    "charm: dummy\napplication: dummy\nsettings:\n  title:\n    value: y-title\n    type: string\n  username:\n    value: y-user\n  ignore:\n    blah: true",
	}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the settings have been correctly updated.
	expected := charm.Settings{"title": "y-title", "username": "y-user"}
	obtained, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, s.combinedSettings(ch, expected))
}

func (s *applicationSuite) TestApplicationUpdateSetConstraints(c *gc.C) {
	application := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ApplicationUpdate{
		ApplicationName: "dummy",
		Constraints:     &cons,
	}
	err = s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *applicationSuite) TestApplicationUpdateAllParams(c *gc.C) {
	s.deployApplicationForUpdateTests(c)
	curl, _ := s.UploadCharm(c, "precise/wordpress-3", "wordpress")
	err := application.AddCharmWithAuthorization(application.NewStateShim(s.State), params.AddCharmWithAuthorization{
		URL: curl.String(),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Update all the application attributes.
	minUnits := 3
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	args := params.ApplicationUpdate{
		ApplicationName: "application",
		CharmURL:        curl.String(),
		ForceCharmURL:   true,
		MinUnits:        &minUnits,
		SettingsStrings: map[string]string{"blog-title": "string-title"},
		SettingsYAML:    "application:\n  blog-title: yaml-title\n",
		Constraints:     &cons,
	}
	err = s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the application has been correctly updated.
	application, err := s.State.Application("application")
	c.Assert(err, jc.ErrorIsNil)

	// Check the charm.
	ch, force, err := application.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.URL().String(), gc.Equals, curl.String())
	c.Assert(force, jc.IsTrue)

	// Check the minimum number of units.
	c.Assert(application.MinUnits(), gc.Equals, minUnits)

	// Check the settings: also ensure the YAML settings take precedence
	// over strings ones.
	expectedSettings := charm.Settings{"blog-title": "yaml-title"}
	obtainedSettings, err := application.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedSettings, gc.DeepEquals, expectedSettings)

	// Check the constraints.
	obtainedConstraints, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedConstraints, gc.DeepEquals, cons)
}

func (s *applicationSuite) TestApplicationUpdateNoParams(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	// Calling Update with no parameters set is a no-op.
	args := params.ApplicationUpdate{ApplicationName: "wordpress"}
	err := s.applicationAPI.Update(args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestApplicationUpdateNoApplication(c *gc.C) {
	err := s.applicationAPI.Update(params.ApplicationUpdate{})
	c.Assert(err, gc.ErrorMatches, `"" is not a valid application name`)
}

func (s *applicationSuite) TestApplicationUpdateInvalidApplication(c *gc.C) {
	args := params.ApplicationUpdate{ApplicationName: "no-such-application"}
	err := s.applicationAPI.Update(args)
	c.Assert(err, gc.ErrorMatches, `application "no-such-application" not found`)
}

var (
	validSetTestValue = "a value with spaces\nand newline\nand UTF-8 characters: \U0001F604 / \U0001F44D"
)

func (s *applicationSuite) TestApplicationSet(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	dummy := s.AddTestingApplication(c, "dummy", ch)

	err := s.applicationAPI.Set(params.ApplicationSet{ApplicationName: "dummy", Options: map[string]string{
		"title":    "foobar",
		"username": validSetTestValue,
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	}))

	err = s.applicationAPI.Set(params.ApplicationSet{ApplicationName: "dummy", Options: map[string]string{
		"title":    "barfoo",
		"username": "",
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title":    "barfoo",
		"username": "",
	}))
}

func (s *applicationSuite) assertApplicationSetBlocked(c *gc.C, dummy *state.Application, msg string) {
	err := s.applicationAPI.Set(params.ApplicationSet{
		ApplicationName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) assertApplicationSet(c *gc.C, dummy *state.Application) {
	err := s.applicationAPI.Set(params.ApplicationSet{
		ApplicationName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": validSetTestValue}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := dummy.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title":    "foobar",
		"username": validSetTestValue,
	}))
}

func (s *applicationSuite) TestBlockDestroyApplicationSet(c *gc.C) {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationSet")
	s.assertApplicationSet(c, dummy)
}

func (s *applicationSuite) TestBlockRemoveApplicationSet(c *gc.C) {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationSet")
	s.assertApplicationSet(c, dummy)
}

func (s *applicationSuite) TestBlockChangesApplicationSet(c *gc.C) {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangesApplicationSet")
	s.assertApplicationSetBlocked(c, dummy, "TestBlockChangesApplicationSet")
}

func (s *applicationSuite) TestServerUnset(c *gc.C) {
	ch := s.AddTestingCharm(c, "dummy")
	dummy := s.AddTestingApplication(c, "dummy", ch)

	err := s.applicationAPI.Set(params.ApplicationSet{ApplicationName: "dummy", Options: map[string]string{
		"title":    "foobar",
		"username": "user name",
	}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	}))

	err = s.applicationAPI.Unset(params.ApplicationUnset{ApplicationName: "dummy", Options: []string{"username"}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err = dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title": "foobar",
	}))
}

func (s *applicationSuite) setupServerUnsetBlocked(c *gc.C) *state.Application {
	dummy := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	err := s.applicationAPI.Set(params.ApplicationSet{
		ApplicationName: "dummy",
		Options: map[string]string{
			"title":    "foobar",
			"username": "user name",
		}})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := dummy.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title":    "foobar",
		"username": "user name",
	}))
	return dummy
}

func (s *applicationSuite) assertServerUnset(c *gc.C, dummy *state.Application) {
	err := s.applicationAPI.Unset(params.ApplicationUnset{
		ApplicationName: "dummy",
		Options:         []string{"username"},
	})
	c.Assert(err, jc.ErrorIsNil)
	settings, err := dummy.CharmConfig()
	c.Assert(err, jc.ErrorIsNil)
	ch, _, err := dummy.Charm()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, s.combinedSettings(ch, charm.Settings{
		"title": "foobar",
	}))
}

func (s *applicationSuite) assertServerUnsetBlocked(c *gc.C, dummy *state.Application, msg string) {
	err := s.applicationAPI.Unset(params.ApplicationUnset{
		ApplicationName: "dummy",
		Options:         []string{"username"},
	})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockDestroyModel(c, "TestBlockDestroyServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *applicationSuite) TestBlockRemoveServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockRemoveObject(c, "TestBlockRemoveServerUnset")
	s.assertServerUnset(c, dummy)
}

func (s *applicationSuite) TestBlockChangesServerUnset(c *gc.C) {
	dummy := s.setupServerUnsetBlocked(c)
	s.BlockAllChanges(c, "TestBlockChangesServerUnset")
	s.assertServerUnsetBlocked(c, dummy, "TestBlockChangesServerUnset")
}

var clientAddApplicationUnitsTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	numUnits    int
	expected    []string
	to          string
	err         string
}{
	{
		about:    "returns unit names",
		numUnits: 3,
		expected: []string{"dummy/0", "dummy/1", "dummy/2"},
	},
	{
		about: "fails trying to add zero units",
		err:   "must add at least one unit",
	},
	{
		// Note: chained-state, we add 1 unit here, but the 3 units
		// from the first condition still exist
		about:    "force the unit onto bootstrap machine",
		numUnits: 1,
		expected: []string{"dummy/3"},
		to:       "0",
	},
	{
		about:       "unknown application name",
		application: "unknown-application",
		numUnits:    1,
		err:         `application "unknown-application" not found`,
	},
}

func (s *applicationSuite) TestClientAddApplicationUnits(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	for i, t := range clientAddApplicationUnitsTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		args := params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        t.numUnits,
		}
		if t.to != "" {
			args.Placement = []*instance.Placement{instance.MustParsePlacement(t.to)}
		}
		result, err := s.applicationAPI.AddUnits(args)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
	}
	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/3")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *applicationSuite) TestAddApplicationUnitsToNewContainer(c *gc.C) {
	app := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        1,
		Placement:       []*instance.Placement{instance.MustParsePlacement("lxd:" + machine.Id())},
	})
	c.Assert(err, jc.ErrorIsNil)

	units, err := app.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	mid, err := units[0].AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(mid, gc.Equals, machine.Id()+"/lxd/0")
}

var addApplicationUnitTests = []struct {
	about       string
	application string // if not set, defaults to 'dummy'
	expected    []string
	machineIds  []string
	placement   []*instance.Placement
	err         string
}{
	{
		about:      "valid placement directives",
		expected:   []string{"dummy/0"},
		placement:  []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "valid"}},
		machineIds: []string{"1"},
	}, {
		about:      "direct machine assignment placement directive",
		expected:   []string{"dummy/1", "dummy/2"},
		placement:  []*instance.Placement{{"#", "1"}, {"lxd", "1"}},
		machineIds: []string{"1", "1/lxd/0"},
	}, {
		about:     "invalid placement directive",
		err:       ".* invalid placement is invalid",
		expected:  []string{"dummy/3"},
		placement: []*instance.Placement{{"deadbeef-0bad-400d-8000-4b1d0d06f00d", "invalid"}},
	},
}

func (s *applicationSuite) TestAddApplicationUnits(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Add a machine for the units to be placed on.
	_, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	for i, t := range addApplicationUnitTests {
		c.Logf("test %d. %s", i, t.about)
		applicationName := t.application
		if applicationName == "" {
			applicationName = "dummy"
		}
		result, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
			ApplicationName: applicationName,
			NumUnits:        len(t.expected),
			Placement:       t.placement,
		})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result.Units, gc.DeepEquals, t.expected)
		for i, unitName := range result.Units {
			u, err := s.BackingState.Unit(unitName)
			c.Assert(err, jc.ErrorIsNil)
			assignedMachine, err := u.AssignedMachineId()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(assignedMachine, gc.Equals, t.machineIds[i])
		}
	}
}

func (s *applicationSuite) assertAddApplicationUnits(c *gc.C) {
	result, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Units, gc.DeepEquals, []string{"dummy/0", "dummy/1", "dummy/2"})

	// Test that we actually assigned the unit to machine 0
	forcedUnit, err := s.BackingState.Unit("dummy/0")
	c.Assert(err, jc.ErrorIsNil)
	assignedMachine, err := forcedUnit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(assignedMachine, gc.Equals, "0")
}

func (s *applicationSuite) TestApplicationCharmRelations(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.applicationAPI.CharmRelations(params.ApplicationCharmRelations{"blah"})
	c.Assert(err, gc.ErrorMatches, `application "blah" not found`)

	result, err := s.applicationAPI.CharmRelations(params.ApplicationCharmRelations{"wordpress"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.CharmRelations, gc.DeepEquals, []string{
		"cache", "db", "juju-info", "logging-dir", "monitoring-port", "url",
	})
}

func (s *applicationSuite) assertAddApplicationUnitsBlocked(c *gc.C, msg string) {
	_, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
	})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroyAddApplicationUnits(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockDestroyModel(c, "TestBlockDestroyAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockRemoveAddApplicationUnits(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockRemoveObject(c, "TestBlockRemoveAddApplicationUnits")
	s.assertAddApplicationUnits(c)
}

func (s *applicationSuite) TestBlockChangeAddApplicationUnits(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	s.BlockAllChanges(c, "TestBlockChangeAddApplicationUnits")
	s.assertAddApplicationUnitsBlocked(c, "TestBlockChangeAddApplicationUnits")
}

func (s *applicationSuite) TestAddUnitToMachineNotFound(c *gc.C) {
	s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	_, err := s.applicationAPI.AddUnits(params.AddApplicationUnits{
		ApplicationName: "dummy",
		NumUnits:        3,
		Placement:       []*instance.Placement{instance.MustParsePlacement("42")},
	})
	c.Assert(err, gc.ErrorMatches, `adding new machine to host unit "dummy/0": machine 42 not found`)
}

func (s *applicationSuite) TestApplicationExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = s.AddTestingApplication(c, name, charm)
		c.Assert(apps[i].IsExposed(), jc.IsFalse)
	}
	err = apps[1].SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), jc.IsTrue)
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err = s.applicationAPI.Expose(params.ApplicationExpose{t.application})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			application, err := s.State.Application(t.application)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(application.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

func (s *applicationSuite) setupApplicationExpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	applicationNames := []string{"dummy-application", "exposed-application"}
	apps := make([]*state.Application, len(applicationNames))
	var err error
	for i, name := range applicationNames {
		apps[i] = s.AddTestingApplication(c, name, charm)
		c.Assert(apps[i].IsExposed(), jc.IsFalse)
	}
	err = apps[1].SetExposed()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(apps[1].IsExposed(), jc.IsTrue)
}

var applicationExposeTests = []struct {
	about       string
	application string
	err         string
	exposed     bool
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "expose a application",
		application: "dummy-application",
		exposed:     true,
	},
	{
		about:       "expose an already exposed application",
		application: "exposed-application",
		exposed:     true,
	},
}

func (s *applicationSuite) assertApplicationExpose(c *gc.C) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(params.ApplicationExpose{t.application})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			application, err := s.State.Application(t.application)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(application.IsExposed(), gc.Equals, t.exposed)
		}
	}
}

func (s *applicationSuite) assertApplicationExposeBlocked(c *gc.C, msg string) {
	for i, t := range applicationExposeTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Expose(params.ApplicationExpose{t.application})
		s.AssertBlocked(c, err, msg)
	}
}

func (s *applicationSuite) TestBlockDestroyApplicationExpose(c *gc.C) {
	s.setupApplicationExpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockRemoveApplicationExpose(c *gc.C) {
	s.setupApplicationExpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationExpose")
	s.assertApplicationExpose(c)
}

func (s *applicationSuite) TestBlockChangesApplicationExpose(c *gc.C) {
	s.setupApplicationExpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationExpose")
	s.assertApplicationExposeBlocked(c, "TestBlockChangesApplicationExpose")
}

var applicationUnexposeTests = []struct {
	about       string
	application string
	err         string
	initial     bool
	expected    bool
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "unexpose a application",
		application: "dummy-application",
		initial:     true,
		expected:    false,
	},
	{
		about:       "unexpose an already unexposed application",
		application: "dummy-application",
		initial:     false,
		expected:    false,
	},
}

func (s *applicationSuite) TestApplicationUnexpose(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	for i, t := range applicationUnexposeTests {
		c.Logf("test %d. %s", i, t.about)
		app := s.AddTestingApplication(c, "dummy-application", charm)
		if t.initial {
			app.SetExposed()
		}
		c.Assert(app.IsExposed(), gc.Equals, t.initial)
		err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{t.application})
		if t.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			app.Refresh()
			c.Assert(app.IsExposed(), gc.Equals, t.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
		err = app.Destroy()
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *applicationSuite) setupApplicationUnexpose(c *gc.C) *state.Application {
	charm := s.AddTestingCharm(c, "dummy")
	app := s.AddTestingApplication(c, "dummy-application", charm)
	app.SetExposed()
	c.Assert(app.IsExposed(), gc.Equals, true)
	return app
}

func (s *applicationSuite) assertApplicationUnexpose(c *gc.C, app *state.Application) {
	err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{"dummy-application"})
	c.Assert(err, jc.ErrorIsNil)
	app.Refresh()
	c.Assert(app.IsExposed(), gc.Equals, false)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) assertApplicationUnexposeBlocked(c *gc.C, app *state.Application, msg string) {
	err := s.applicationAPI.Unexpose(params.ApplicationUnexpose{"dummy-application"})
	s.AssertBlocked(c, err, msg)
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) TestBlockDestroyApplicationUnexpose(c *gc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockDestroyModel(c, "TestBlockDestroyApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockRemoveApplicationUnexpose(c *gc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockRemoveObject(c, "TestBlockRemoveApplicationUnexpose")
	s.assertApplicationUnexpose(c, app)
}

func (s *applicationSuite) TestBlockChangesApplicationUnexpose(c *gc.C) {
	app := s.setupApplicationUnexpose(c)
	s.BlockAllChanges(c, "TestBlockChangesApplicationUnexpose")
	s.assertApplicationUnexposeBlocked(c, app, "TestBlockChangesApplicationUnexpose")
}

var applicationDestroyTests = []struct {
	about       string
	application string
	err         string
}{
	{
		about:       "unknown application name",
		application: "unknown-application",
		err:         `application "unknown-application" not found`,
	},
	{
		about:       "destroy an application",
		application: "dummy-application",
	},
	{
		about:       "destroy an already destroyed application",
		application: "dummy-application",
		err:         `application "dummy-application" not found`,
	},
}

func (s *applicationSuite) TestApplicationDestroy(c *gc.C) {
	s.AddTestingApplication(c, "dummy-application", s.AddTestingCharm(c, "dummy"))
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "remote-application",
		SourceModel: s.Model.ModelTag(),
		Token:       "t0",
	})
	c.Assert(err, jc.ErrorIsNil)

	for i, t := range applicationDestroyTests {
		c.Logf("test %d. %s", i, t.about)
		err := s.applicationAPI.Destroy(params.ApplicationDestroy{t.application})
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	// Now do Destroy on an application with units. Destroy will
	// cause the application to be not-Alive, but will not remove its
	// document.
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	applicationName := "wordpress"
	application, err := s.State.Application(applicationName)
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.Destroy(params.ApplicationDestroy{applicationName})
	c.Assert(err, jc.ErrorIsNil)
	err = application.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func assertLife(c *gc.C, entity state.Living, life state.Life) {
	err := entity.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Life(), gc.Equals, life)
}

func (s *applicationSuite) TestBlockApplicationDestroy(c *gc.C) {
	s.AddTestingApplication(c, "dummy-application", s.AddTestingCharm(c, "dummy"))

	// block remove-objects
	s.BlockRemoveObject(c, "TestBlockApplicationDestroy")
	err := s.applicationAPI.Destroy(params.ApplicationDestroy{"dummy-application"})
	s.AssertBlocked(c, err, "TestBlockApplicationDestroy")
	// Tests may have invalid application names.
	application, err := s.State.Application("dummy-application")
	if err == nil {
		// For valid application names, check that application is alive :-)
		assertLife(c, application, state.Alive)
	}
}

func (s *applicationSuite) TestDestroyPrincipalUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	units := make([]*state.Unit, 5)
	for i := range units {
		unit, err := wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		unit.AssignToNewMachine()
		c.Assert(err, jc.ErrorIsNil)
		now := time.Now()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = unit.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	s.assertDestroyPrincipalUnits(c, units)
}

func (s *applicationSuite) TestDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Try to destroy the subordinate alone; check it fails.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *applicationSuite) assertDestroyPrincipalUnits(c *gc.C, units []*state.Unit) {
	// Destroy 2 of them; check they become Dying.
	err := s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)

	// Try to destroy an Alive one and a Dying one; check
	// it destroys the Alive one and ignores the Dying one.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/2", "wordpress/0"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[2], state.Dying)

	// Try to destroy an Alive one along with a nonexistent one; check that
	// the valid instruction is followed but the invalid one is warned about.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"boojum/123", "wordpress/3"},
	})
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "boojum/123" does not exist`)
	assertLife(c, units[3], state.Dying)

	// Make one Dead, and destroy an Alive one alongside it; check no errors.
	wp0, err := s.State.Unit("wordpress/0")
	c.Assert(err, jc.ErrorIsNil)
	err = wp0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/4"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dead)
	assertLife(c, units[4], state.Dying)
}

func (s *applicationSuite) setupDestroyPrincipalUnits(c *gc.C) []*state.Unit {
	units := make([]*state.Unit, 5)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	for i := range units {
		unit, err := wordpress.AddUnit(state.AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		err = unit.AssignToNewMachine()
		c.Assert(err, jc.ErrorIsNil)
		now := time.Now()
		sInfo := status.StatusInfo{
			Status:  status.Idle,
			Message: "",
			Since:   &now,
		}
		err = unit.SetAgentStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
		units[i] = unit
	}
	return units
}

func (s *applicationSuite) assertBlockedErrorAndLiveliness(
	c *gc.C,
	err error,
	msg string,
	living1 state.Living,
	living2 state.Living,
	living3 state.Living,
	living4 state.Living,
) {
	s.AssertBlocked(c, err, msg)
	assertLife(c, living1, state.Alive)
	assertLife(c, living2, state.Alive)
	assertLife(c, living3, state.Alive)
	assertLife(c, living4, state.Alive)
}

func (s *applicationSuite) TestBlockChangesDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockAllChanges(c, "TestBlockChangesDestroyPrincipalUnits")
	err := s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockChangesDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *applicationSuite) TestBlockRemoveDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockRemoveObject(c, "TestBlockRemoveDestroyPrincipalUnits")
	err := s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	s.assertBlockedErrorAndLiveliness(c, err, "TestBlockRemoveDestroyPrincipalUnits", units[0], units[1], units[2], units[3])
}

func (s *applicationSuite) TestBlockDestroyDestroyPrincipalUnits(c *gc.C) {
	units := s.setupDestroyPrincipalUnits(c)
	s.BlockDestroyModel(c, "TestBlockDestroyDestroyPrincipalUnits")
	err := s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "wordpress/1"},
	})
	c.Assert(err, jc.ErrorIsNil)
	assertLife(c, units[0], state.Dying)
	assertLife(c, units[1], state.Dying)
}

func (s *applicationSuite) assertDestroySubordinateUnits(c *gc.C, wordpress0, logging0 *state.Unit) {
	// Try to destroy the principal and the subordinate together; check it warns
	// about the subordinate, but destroys the one it can. (The principal unit
	// agent will be responsible for destroying the subordinate.)
	err := s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `some units were not destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, wordpress0, state.Dying)
	assertLife(c, logging0, state.Alive)
}

func (s *applicationSuite) TestBlockRemoveDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockRemoveObject(c, "TestBlockRemoveDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockRemoveDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *applicationSuite) TestBlockChangesDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockAllChanges(c, "TestBlockChangesDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, rel, state.Alive)
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)

	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"wordpress/0", "logging/0"},
	})
	s.AssertBlocked(c, err, "TestBlockChangesDestroySubordinateUnits")
	assertLife(c, wordpress0, state.Alive)
	assertLife(c, logging0, state.Alive)
	assertLife(c, rel, state.Alive)
}

func (s *applicationSuite) TestBlockDestroyDestroySubordinateUnits(c *gc.C) {
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	logging0, err := s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	s.BlockDestroyModel(c, "TestBlockDestroyDestroySubordinateUnits")
	// Try to destroy the subordinate alone; check it fails.
	err = s.applicationAPI.DestroyUnits(params.DestroyApplicationUnits{
		UnitNames: []string{"logging/0"},
	})
	c.Assert(err, gc.ErrorMatches, `no units were destroyed: unit "logging/0" is a subordinate`)
	assertLife(c, logging0, state.Alive)

	s.assertDestroySubordinateUnits(c, wordpress0, logging0)
}

func (s *applicationSuite) TestClientSetApplicationConstraints(c *gc.C) {
	application := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))

	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	err = s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the constraints have been correctly updated.
	obtained, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *applicationSuite) setupSetApplicationConstraints(c *gc.C) (*state.Application, constraints.Value) {
	application := s.AddTestingApplication(c, "dummy", s.AddTestingCharm(c, "dummy"))
	// Update constraints for the application.
	cons, err := constraints.Parse("mem=4096", "cores=2")
	c.Assert(err, jc.ErrorIsNil)
	return application, cons
}

func (s *applicationSuite) assertSetApplicationConstraints(c *gc.C, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
	// Ensure the constraints have been correctly updated.
	obtained, err := application.Constraints()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, cons)
}

func (s *applicationSuite) assertSetApplicationConstraintsBlocked(c *gc.C, msg string, application *state.Application, cons constraints.Value) {
	err := s.applicationAPI.SetConstraints(params.SetConstraints{ApplicationName: "dummy", Constraints: cons})
	s.AssertBlocked(c, err, msg)
}

func (s *applicationSuite) TestBlockDestroySetApplicationConstraints(c *gc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockDestroyModel(c, "TestBlockDestroySetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockRemoveSetApplicationConstraints(c *gc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockRemoveObject(c, "TestBlockRemoveSetApplicationConstraints")
	s.assertSetApplicationConstraints(c, app, cons)
}

func (s *applicationSuite) TestBlockChangesSetApplicationConstraints(c *gc.C) {
	app, cons := s.setupSetApplicationConstraints(c)
	s.BlockAllChanges(c, "TestBlockChangesSetApplicationConstraints")
	s.assertSetApplicationConstraintsBlocked(c, "TestBlockChangesSetApplicationConstraints", app, cons)
}

func (s *applicationSuite) TestClientGetApplicationConstraints(c *gc.C) {
	fooConstraints := constraints.MustParse("mem=4G")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "foo",
		Constraints: fooConstraints,
	})
	barConstraints := constraints.MustParse("mem=128G", "cores=64")
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:        "bar",
		Constraints: barConstraints,
	})

	results, err := s.applicationAPI.GetConstraints(params.Entities{
		Entities: []params.Entity{
			{"wat"}, {"machine-0"}, {"user-foo"},
			{"application-foo"}, {"application-bar"}, {"application-wat"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ApplicationGetConstraintsResults{
		Results: []params.ApplicationConstraint{
			{
				Error: &params.Error{Message: `"wat" is not a valid tag`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got machine`},
			}, {
				Error: &params.Error{Message: `unexpected tag type, expected application, got user`},
			}, {
				Constraints: fooConstraints,
			}, {
				Constraints: barConstraints,
			}, {
				Error: &params.Error{Message: `application "wat" not found`, Code: "not found"},
			},
		}})
}

func (s *applicationSuite) checkEndpoints(c *gc.C, mysqlAppName string, endpoints map[string]params.CharmRelation) {
	c.Assert(endpoints["wordpress"], gc.DeepEquals, params.CharmRelation{
		Name:      "db",
		Role:      "requirer",
		Interface: "mysql",
		Optional:  false,
		Limit:     1,
		Scope:     "global",
	})
	ep := params.CharmRelation{
		Name:      "server",
		Role:      "provider",
		Interface: "mysql",
		Scope:     "global",
	}
	// Remote applications don't use scope.
	if mysqlAppName == "hosted-mysql" {
		ep.Scope = ""
	}
	c.Assert(endpoints[mysqlAppName], gc.DeepEquals, ep)
}

func (s *applicationSuite) setupRelationScenario(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *applicationSuite) assertAddRelation(c *gc.C, endpoints, viaCIDRs []string) {
	s.setupRelationScenario(c)

	res, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: viaCIDRs})
	c.Assert(err, jc.ErrorIsNil)
	// Show that the relation was added.
	wpApp, err := s.State.Application("wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rels, err := wpApp.Relations()
	// There are 2 relations - the logging-wordpress one set up in the
	// scenario and the one created in this test.
	c.Assert(len(rels), gc.Equals, 2)

	// We may be related to a local application or a remote offer
	// or an application in another model.
	var mySqlApplication state.ApplicationEntity
	mySqlApplication, err = s.State.RemoteApplication("hosted-mysql")
	if errors.IsNotFound(err) {
		mySqlApplication, err = s.State.RemoteApplication("othermysql")
		if errors.IsNotFound(err) {
			mySqlApplication, err = s.State.Application("mysql")
			c.Assert(err, jc.ErrorIsNil)
			s.checkEndpoints(c, "mysql", res.Endpoints)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			s.checkEndpoints(c, "othermysql", res.Endpoints)
		}
	} else {
		c.Assert(err, jc.ErrorIsNil)
		s.checkEndpoints(c, "hosted-mysql", res.Endpoints)
	}
	c.Assert(err, jc.ErrorIsNil)
	rels, err = mySqlApplication.Relations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(rels), gc.Equals, 1)
}

func (s *applicationSuite) TestSuccessfullyAddRelation(c *gc.C) {
	endpoints := []string{"wordpress", "mysql"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestBlockDestroyAddRelation(c *gc.C) {
	s.BlockDestroyModel(c, "TestBlockDestroyAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}
func (s *applicationSuite) TestBlockRemoveAddRelation(c *gc.C) {
	s.BlockRemoveObject(c, "TestBlockRemoveAddRelation")
	s.assertAddRelation(c, []string{"wordpress", "mysql"}, nil)
}

func (s *applicationSuite) TestBlockChangesAddRelation(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.BlockAllChanges(c, "TestBlockChangesAddRelation")
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: []string{"wordpress", "mysql"}})
	s.AssertBlocked(c, err, "TestBlockChangesAddRelation")
}

func (s *applicationSuite) TestSuccessfullyAddRelationSwapped(c *gc.C) {
	// Show that the order of the applications listed in the AddRelation call
	// does not matter.  This is a repeat of the previous test with the application
	// names swapped.
	endpoints := []string{"mysql", "wordpress"}
	s.assertAddRelation(c, endpoints, nil)
}

func (s *applicationSuite) TestCallWithOnlyOneEndpoint(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestCallWithOneEndpointTooMany(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	endpoints := []string{"wordpress", "mysql", "logging"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "cannot relate 3 endpoints")
}

func (s *applicationSuite) TestAddAlreadyAddedRelation(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	// Add a relation between wordpress and mysql.
	endpoints := []string{"wordpress", "mysql"}
	eps, err := s.State.InferEndpoints(endpoints...)
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	// And try to add it again.
	_, err = s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `cannot add relation "wordpress:db mysql:server": relation wordpress:db mysql:server already exists`)
}

func (s *applicationSuite) setupRemoteApplication(c *gc.C) {
	results, err := s.applicationAPI.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{
			{ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         testing.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "A pretty popular database",
				Endpoints: []params.RemoteEndpoint{
					{Name: "server", Interface: "mysql", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
}

func (s *applicationSuite) TestAddRemoteRelation(c *gc.C) {
	s.setupRemoteApplication(c)
	// There's already a wordpress in the scenario this assertion sets up.
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationWithRelName(c *gc.C) {
	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, nil)
}

func (s *applicationSuite) TestAddRemoteRelationVia(c *gc.C) {
	s.setupRemoteApplication(c)
	s.assertAddRelation(c, []string{"wordpress", "hosted-mysql:server"}, []string{"192.168.0.0/16"})

	rel, err := s.State.KeyRelation("wordpress:db hosted-mysql:server")
	c.Assert(err, jc.ErrorIsNil)
	w := rel.WatchRelationEgressNetworks()
	defer statetesting.AssertStop(c, w)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertChange("192.168.0.0/16")
	wc.AssertNoChange()
}

func (s *applicationSuite) TestAddRemoteRelationOnlyOneEndpoint(c *gc.C) {
	s.setupRemoteApplication(c)
	endpoints := []string{"hosted-mysql"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestAlreadyAddedRemoteRelation(c *gc.C) {
	s.setupRemoteApplication(c)
	endpoints := []string{"wordpress", "hosted-mysql"}
	s.assertAddRelation(c, endpoints, nil)

	// And try to add it again.
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`cannot add relation "wordpress:db hosted-mysql:server": relation wordpress:db hosted-mysql:server already exists`))
}

func (s *applicationSuite) TestRemoteRelationInvalidEndpoint(c *gc.C) {
	s.setupRemoteApplication(c)
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))

	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `remote application "hosted-mysql" has no "nope" relation`)
}

func (s *applicationSuite) TestRemoteRelationNoMatchingEndpoint(c *gc.C) {
	results, err := s.applicationAPI.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{
			{ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag: testing.ModelTag.String(),
				OfferName:      "hosted-db2",
				OfferUUID:      "hosted-db2-uuid",
				Endpoints: []params.RemoteEndpoint{
					{Name: "database", Interface: "db2", Role: "provider"},
				},
			}},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)

	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress", "hosted-db2"}
	_, err = s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *applicationSuite) TestRemoteRelationApplicationNotFound(c *gc.C) {
	s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	endpoints := []string{"wordpress", "unknown"}
	_, err := s.applicationAPI.AddRelation(params.AddRelation{Endpoints: endpoints})
	c.Assert(err, gc.ErrorMatches, `application "unknown" not found`)
}
