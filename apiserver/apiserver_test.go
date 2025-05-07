// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type apiserverSuite struct {
	jujutesting.ApiServerSuite
}

var _ = tc.Suite(&apiserverSuite{})

func (s *apiserverSuite) TestCleanStop(c *tc.C) {
	workertest.CleanKill(c, s.Server)
}

func (s *apiserverSuite) getHealth(c *tc.C) (string, int) {
	uri := s.URL("/health", url.Values{}).String()
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	result := string(body)
	// Ensure that the last value is a carriage return.
	c.Assert(strings.HasSuffix(result, "\n"), tc.IsTrue)
	return strings.TrimSuffix(result, "\n"), resp.StatusCode
}

func (s *apiserverSuite) TestHealthRunning(c *tc.C) {
	health, statusCode := s.getHealth(c)
	c.Assert(health, tc.Equals, "running")
	c.Assert(statusCode, tc.Equals, http.StatusOK)
}

func (s *apiserverSuite) TestHealthStopping(c *tc.C) {
	wg := apiserver.ServerWaitGroup(s.Server)
	wg.Add(1)

	s.Server.Kill()
	// There is a race here between the test and the goroutine setting
	// the value, so loop until we see the right health, then exit.
	timeout := time.After(testing.LongWait)
	for {
		health, statusCode := s.getHealth(c)
		if health == "stopping" {
			// Expected, we're done.
			c.Assert(statusCode, tc.Equals, http.StatusServiceUnavailable)
			wg.Done()
			return
		}
		select {
		case <-timeout:
			c.Fatalf("health not set to stopping")
		case <-time.After(testing.ShortWait):
			// Look again.
		}
	}
}
