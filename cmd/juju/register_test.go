// Copyright 2015 Canonical Ltd. All rights reserved.

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"

	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v1"
)

var _ = gc.Suite(&registrationSuite{})

type registrationSuite struct {
	handler *testMetricsRegistrationHandler
	server  *httptest.Server
}

func (s *registrationSuite) SetUpTest(c *gc.C) {
	s.handler = &testMetricsRegistrationHandler{}
	s.server = httptest.NewServer(s.handler)
}

func (s *registrationSuite) TearDownTest(c *gc.C) {
	s.server.Close()
}

/*
func (s *registrationSuite) TestNilMetricsRegistrar(c *gc.C) {
	data, err := nilMetricsRegistrar("environment uuid", "charm url", "service name", &http.Client{}, func(*url.URL) error { return nil })
	c.Assert(err, gc.IsNil)
	c.Assert(data, gc.DeepEquals, []byte{})
}
*/

func (s *registrationSuite) TestHttpMetricsRegistrar(c *gc.C) {
	cleanup := jujutesting.PatchValue(&registerMetricsURL, s.server.URL)
	defer cleanup()

	data, err := registerMetrics("environment uuid", "charm url", "service name", &http.Client{}, func(*url.URL) error { return nil })
	c.Assert(err, gc.IsNil)
	var ms macaroon.Slice
	err = json.Unmarshal(data, &ms)
	c.Assert(err, gc.IsNil)
	c.Assert(ms, gc.HasLen, 1)
	c.Assert(ms[0].Id(), gc.Equals, "hello registration")
	c.Assert(s.handler.registrationCalls, gc.HasLen, 1)
	c.Assert(s.handler.registrationCalls[0], gc.DeepEquals, metricRegistrationPost{EnvironmentUUID: "environment uuid", CharmURL: "charm url", ServiceName: "service name"})
}

type testMetricsRegistrationHandler struct {
	registrationCalls []metricRegistrationPost
}

// ServeHTTP implements http.Handler.
func (c *testMetricsRegistrationHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var registrationPost metricRegistrationPost
	decoder := json.NewDecoder(req.Body)
	err := decoder.Decode(&registrationPost)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m, err := macaroon.New(nil, "hello registration", "test")
	if err != nil {
		panic(err)
	}
	err = json.NewEncoder(w).Encode(m)
	if err != nil {
		panic(err)
	}

	c.registrationCalls = append(c.registrationCalls, registrationPost)
}
