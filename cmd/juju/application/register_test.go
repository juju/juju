// Copyright 2015 Canonical Ltd. All rights reserved.

package application

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	apicharms "github.com/juju/juju/api/charms"
	"github.com/juju/juju/charmstore"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&registrationSuite{})

type registrationSuite struct {
	testing.CleanupSuite
	stub     *testing.Stub
	handler  *testMetricsRegistrationHandler
	server   *httptest.Server
	register DeployStep
	ctx      *cmd.Context
}

func (s *registrationSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.handler = &testMetricsRegistrationHandler{Stub: s.stub}
	s.server = httptest.NewServer(s.handler)
	s.register = &RegisterMeteredCharm{
		Plan:           "someplan",
		RegisterURL:    s.server.URL,
		AllocationSpec: "personal:100",
	}
	s.ctx = coretesting.Context(c)
}

func (s *registrationSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
	s.server.Close()
}

// FAILING
func (s *registrationSuite) TestMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		}},
	})
}

func (s *registrationSuite) TestOptionalPlanMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: false},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		}},
	})
}

func (s *registrationSuite) TestPlanNotSpecifiedCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: nil,
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		}},
	})
}

// FAILING
func (s *registrationSuite) TestMeteredCharmAPIError(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("something failed"))
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `authorization failed: something failed`)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}})
}

func (s *registrationSuite) TestMeteredCharmInvalidAllocation(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	s.register = &RegisterMeteredCharm{
		Plan:           "someplan",
		RegisterURL:    s.server.URL,
		AllocationSpec: "invalid allocation",
	}

	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `invalid allocation, expecting <budget>:<limit>`)
	s.stub.CheckNoCalls(c)
}

// FAILING
func (s *registrationSuite) TestMeteredCharmDeployError(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	deployError := errors.New("deployment failed")
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, deployError)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}})
}

func (s *registrationSuite) TestMeteredLocalCharmWithPlan(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("local:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"local:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "local:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		},
	}})
}

func (s *registrationSuite) TestMeteredLocalCharmNoPlan(c *gc.C) {
	s.register = &RegisterMeteredCharm{
		RegisterURL:    s.server.URL,
		QueryURL:       s.server.URL,
		AllocationSpec: "personal:100",
	}
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("local:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"local:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "local:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		}},
	})
}

func (s *registrationSuite) TestMeteredCharmNoPlanSet(c *gc.C) {
	s.register = &RegisterMeteredCharm{
		AllocationSpec: "personal:100",
		RegisterURL:    s.server.URL,
		QueryURL:       s.server.URL}
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "thisplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}, {
		"SetMetricCredentials", []interface{}{
			"application name",
			authorization,
		},
	}})
}

func (s *registrationSuite) TestMeteredCharmNoDefaultPlan(c *gc.C) {
	s.stub.SetErrors(nil, errors.NotFoundf("default plan"))
	s.register = &RegisterMeteredCharm{
		AllocationSpec: "personal:100",
		RegisterURL:    s.server.URL,
		QueryURL:       s.server.URL}
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `cs:quantal/metered-1 has no default plan. Try "juju deploy --plan <plan-name> with one of thisplan, thisotherplan"`)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}, {
		"ListPlans", []interface{}{"cs:quantal/metered-1"},
	}})
}

// FAILING
func (s *registrationSuite) TestMeteredCharmFailToQueryDefaultCharm(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("something failed"))
	s.register = &RegisterMeteredCharm{
		AllocationSpec: "personal:100",
		RegisterURL:    s.server.URL,
		QueryURL:       s.server.URL}
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `failed to query default plan:.*`)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"DefaultPlan", []interface{}{"cs:quantal/metered-1"},
	}})
}

func (s *registrationSuite) TestUnmeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/unmetered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/unmetered-1"},
	}})
	s.stub.ResetCalls()
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{})
}

// FAILING
func (s *registrationSuite) TestFailedAuth(c *gc.C) {
	s.stub.SetErrors(nil, errors.Errorf("could not authorize"))
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: true},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, gc.ErrorMatches, `authorization failed:.*`)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			ModelUUID:       "model uuid",
			CharmURL:        "cs:quantal/metered-1",
			ApplicationName: "application name",
			PlanURL:         "someplan",
			Budget:          "personal",
			Limit:           "100",
		}},
	}})
}

func (s *registrationSuite) TestPlanArgumentPlanRequiredInteraction(c *gc.C) {
	tests := []struct {
		about         string
		planArgument  string
		planRequired  bool
		noDefaultPlan bool
		apiCalls      []string
		err           string
	}{{
		about:        "deploy with --plan, required false",
		planArgument: "plan",
		planRequired: false,
		apiCalls:     []string{"IsMetered", "Authorize"},
		err:          "",
	}, {
		about:        "deploy with --plan, required true",
		planArgument: "plan",
		planRequired: true,
		apiCalls:     []string{"IsMetered", "Authorize"},
		err:          "",
	}, {
		about:        "deploy without --plan, required false with default plan",
		planRequired: false,
		apiCalls:     []string{"IsMetered"},
		err:          "",
	}, {
		about:        "deploy without --plan, required true with default plan",
		planRequired: true,
		apiCalls:     []string{"IsMetered", "DefaultPlan", "Authorize"},
		err:          "",
	}, {
		about:         "deploy without --plan, required false with no default plan",
		planRequired:  false,
		noDefaultPlan: true,
		apiCalls:      []string{"IsMetered"},
		err:           "",
	}, {
		about:         "deploy without --plan, required true with no default plan",
		planRequired:  true,
		noDefaultPlan: true,
		apiCalls:      []string{"IsMetered", "DefaultPlan", "ListPlans"},
		err:           `cs:quantal/metered-1 has no default plan. Try "juju deploy --plan <plan-name> with one of thisplan, thisotherplan"`,
	},
	}
	for i, test := range tests {
		s.stub.ResetCalls()
		c.Logf("running test %d: %s", i, test.about)
		if test.noDefaultPlan {
			s.stub.SetErrors(nil, errors.NotFoundf("default plan"))
		} else {
			s.stub.SetErrors(nil)
		}
		s.register = &RegisterMeteredCharm{
			Plan:           test.planArgument,
			AllocationSpec: "personal:100",
			RegisterURL:    s.server.URL,
			QueryURL:       s.server.URL,
		}
		client := httpbakery.NewClient()
		d := DeploymentInfo{
			CharmID: charmstore.CharmID{
				URL: charm.MustParseURL("cs:quantal/metered-1"),
			},
			ApplicationName: "application name",
			ModelUUID:       "model uuid",
			CharmInfo: &apicharms.CharmInfo{
				Metrics: &charm.Metrics{
					Plan: &charm.Plan{Required: test.planRequired},
				},
			},
		}

		err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
		if test.err != "" {
			c.Assert(err, gc.ErrorMatches, test.err)
		} else {
			c.Assert(err, jc.ErrorIsNil)
		}

		s.stub.CheckCallNames(c, test.apiCalls...)
	}
}

type testMetricsRegistrationHandler struct {
	*testing.Stub
}

type respErr struct {
	Error string `json:"error"`
}

// ServeHTTP implements http.Handler.
func (c *testMetricsRegistrationHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		var registrationPost metricRegistrationPost
		decoder := json.NewDecoder(req.Body)
		err := decoder.Decode(&registrationPost)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		c.AddCall("Authorize", registrationPost)
		rErr := c.NextErr()
		if rErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			err = json.NewEncoder(w).Encode(respErr{Error: rErr.Error()})
			if err != nil {
				panic(err)
			}
			return
		}
		err = json.NewEncoder(w).Encode([]byte("hello registration"))
		if err != nil {
			panic(err)
		}
	} else if req.Method == "GET" {
		if req.URL.Path == "/default" {
			cURL := req.URL.Query().Get("charm-url")
			c.AddCall("DefaultPlan", cURL)
			rErr := c.NextErr()
			if rErr != nil {
				if errors.IsNotFound(rErr) {
					http.Error(w, rErr.Error(), http.StatusNotFound)
					return
				}
				http.Error(w, rErr.Error(), http.StatusInternalServerError)
				return
			}
			result := struct {
				URL string `json:"url"`
			}{"thisplan"}
			err := json.NewEncoder(w).Encode(result)
			if err != nil {
				panic(err)
			}
			return
		}
		cURL := req.URL.Query().Get("charm-url")
		c.AddCall("ListPlans", cURL)
		rErr := c.NextErr()
		if rErr != nil {
			http.Error(w, rErr.Error(), http.StatusInternalServerError)
			return
		}
		result := []struct {
			URL string `json:"url"`
		}{
			{"thisplan"},
			{"thisotherplan"},
		}
		err := json.NewEncoder(w).Encode(result)
		if err != nil {
			panic(err)
		}
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

var _ = gc.Suite(&noPlanRegistrationSuite{})

type noPlanRegistrationSuite struct {
	testing.CleanupSuite
	stub     *testing.Stub
	handler  *testMetricsRegistrationHandler
	server   *httptest.Server
	register DeployStep
	ctx      *cmd.Context
}

func (s *noPlanRegistrationSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.stub = &testing.Stub{}
	s.handler = &testMetricsRegistrationHandler{Stub: s.stub}
	s.server = httptest.NewServer(s.handler)
	s.register = &RegisterMeteredCharm{
		Plan:           "",
		RegisterURL:    s.server.URL,
		AllocationSpec: "personal:100",
	}
	s.ctx = coretesting.Context(c)
}

func (s *noPlanRegistrationSuite) TearDownTest(c *gc.C) {
	s.CleanupSuite.TearDownTest(c)
	s.server.Close()
}
func (s *noPlanRegistrationSuite) TestOptionalPlanMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: &charm.Plan{Required: false},
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}})
}

func (s *noPlanRegistrationSuite) TestPlanNotSpecifiedCharm(c *gc.C) {
	client := httpbakery.NewClient()
	d := DeploymentInfo{
		CharmID: charmstore.CharmID{
			URL: charm.MustParseURL("cs:quantal/metered-1"),
		},
		ApplicationName: "application name",
		ModelUUID:       "model uuid",
		CharmInfo: &apicharms.CharmInfo{
			Metrics: &charm.Metrics{
				Plan: nil,
			},
		},
	}
	err := s.register.RunPre(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockMeteredDeployAPI{Stub: s.stub}, client, s.ctx, d, nil)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"IsMetered", []interface{}{"cs:quantal/metered-1"},
	}})
}

type mockMeteredDeployAPI struct {
	MeteredDeployAPI
	*testing.Stub
}

func (m *mockMeteredDeployAPI) IsMetered(charmURL string) (bool, error) {
	m.AddCall("IsMetered", charmURL)
	if charmURL == "cs:quantal/metered-1" || charmURL == "local:quantal/metered-1" {
		return true, m.NextErr()
	}
	return false, m.NextErr()

}
func (m *mockMeteredDeployAPI) SetMetricCredentials(service string, credentials []byte) error {
	m.AddCall("SetMetricCredentials", service, credentials)
	return m.NextErr()
}
