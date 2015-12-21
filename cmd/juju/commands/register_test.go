// Copyright 2015 Canonical Ltd. All rights reserved.

package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
)

var _ = gc.Suite(&registrationSuite{})

type registrationSuite struct {
	stub     *testing.Stub
	handler  *testMetricsRegistrationHandler
	server   *httptest.Server
	register DeployStep
}

func (s *registrationSuite) SetUpTest(c *gc.C) {
	s.stub = &testing.Stub{}
	s.handler = &testMetricsRegistrationHandler{Stub: s.stub}
	s.server = httptest.NewServer(s.handler)
	s.register = &RegisterMeteredCharm{Plan: "someplan", RegisterURL: s.server.URL}
}

func (s *registrationSuite) TearDownTest(c *gc.C) {
	s.server.Close()
}

func (s *registrationSuite) TestMeteredCharm(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			EnvironmentUUID: "environment uuid",
			CharmURL:        "local:quantal/metered-1",
			ServiceName:     "service name",
			PlanURL:         "someplan",
		}},
	}, {
		"APICall", []interface{}{"Service", "SetMetricCredentials", params.ServiceMetricCredentials{
			Creds: []params.ServiceMetricCredential{params.ServiceMetricCredential{
				ServiceName:       "service name",
				MetricCredentials: authorization,
			}},
		}},
	}})
}

func (s *registrationSuite) TestMeteredCharmNoPlanSet(c *gc.C) {
	s.register = &RegisterMeteredCharm{RegisterURL: s.server.URL, QueryURL: s.server.URL}
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	err = s.register.RunPost(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"DefaultPlan", []interface{}{"local:quantal/metered-1"},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			EnvironmentUUID: "environment uuid",
			CharmURL:        "local:quantal/metered-1",
			ServiceName:     "service name",
			PlanURL:         "thisplan",
		}},
	}, {
		"APICall", []interface{}{"Service", "SetMetricCredentials", params.ServiceMetricCredentials{
			Creds: []params.ServiceMetricCredential{params.ServiceMetricCredential{
				ServiceName:       "service name",
				MetricCredentials: authorization,
			}},
		}},
	}})
}

func (s *registrationSuite) TestMeteredCharmNoDefaultPlan(c *gc.C) {
	s.stub.SetErrors(nil, errors.NotFoundf("default charm"))
	s.register = &RegisterMeteredCharm{RegisterURL: s.server.URL, QueryURL: s.server.URL}
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, gc.ErrorMatches, `local:quantal/metered-1 has no default plan. Try "juju deploy --plan <plan-name> with one of thisplan, thisotherplan"`)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"DefaultPlan", []interface{}{"local:quantal/metered-1"},
	}, {
		"ListPlans", []interface{}{"local:quantal/metered-1"},
	}})
}

func (s *registrationSuite) TestMeteredCharmFailToQueryDefaultCharm(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("something failed"))
	s.register = &RegisterMeteredCharm{RegisterURL: s.server.URL, QueryURL: s.server.URL}
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, gc.ErrorMatches, `failed to query default plan:.*`)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"DefaultPlan", []interface{}{"local:quantal/metered-1"},
	}})
}

func (s *registrationSuite) TestUnmeteredCharm(c *gc.C) {
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/unmetered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/unmetered-1"}},
	}})
	s.stub.ResetCalls()
	err = s.register.RunPost(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{})
}

func (s *registrationSuite) TestFailedAuth(c *gc.C) {
	s.stub.SetErrors(nil, fmt.Errorf("could not authorize"))
	client := httpbakery.NewClient().Client
	d := DeploymentInfo{
		CharmURL:    charm.MustParseURL("local:quantal/metered-1"),
		ServiceName: "service name",
		EnvUUID:     "environment uuid",
	}
	err := s.register.RunPre(&mockAPIConnection{Stub: s.stub}, client, d)
	c.Assert(err, gc.ErrorMatches, `failed to register metrics:.*`)
	authorization, err := json.Marshal([]byte("hello registration"))
	authorization = append(authorization, byte(0xa))
	c.Assert(err, jc.ErrorIsNil)
	s.stub.CheckCalls(c, []testing.StubCall{{
		"APICall", []interface{}{"Charms", "IsMetered", params.CharmInfo{CharmURL: "local:quantal/metered-1"}},
	}, {
		"Authorize", []interface{}{metricRegistrationPost{
			EnvironmentUUID: "environment uuid",
			CharmURL:        "local:quantal/metered-1",
			ServiceName:     "service name",
			PlanURL:         "someplan",
		}},
	}})
}

type testMetricsRegistrationHandler struct {
	*testing.Stub
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
			http.Error(w, rErr.Error(), http.StatusInternalServerError)
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

type mockAPIConnection struct {
	api.Connection
	*testing.Stub
}

func (*mockAPIConnection) BestFacadeVersion(facade string) int {
	return 42
}

func (*mockAPIConnection) Close() error {
	return nil
}

func (m *mockAPIConnection) APICall(objType string, version int, id, request string, parameters, response interface{}) error {
	m.MethodCall(m, "APICall", objType, request, parameters)

	switch request {
	case "IsMetered":
		parameters := parameters.(params.CharmInfo)
		response := response.(*params.IsMeteredResult)
		if parameters.CharmURL == "local:quantal/metered-1" {
			response.Metered = true
		}
	case "SetMetricCredentials":
		response := response.(*params.ErrorResults)
		response.Results = append(response.Results, params.ErrorResult{Error: nil})
	}
	return m.NextErr()
}
