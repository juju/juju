// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

type apiserverSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&apiserverSuite{})

func (s *apiserverSuite) TestCleanStop(c *gc.C) {
	workertest.CleanKill(c, s.Server)
}

func (s *apiserverSuite) getHealth(c *gc.C) (string, int) {
	uri := s.URL("/health", url.Values{}).String()
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{Method: "GET", URL: uri})
	body, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	result := string(body)
	// Ensure that the last value is a carriage return.
	c.Assert(strings.HasSuffix(result, "\n"), jc.IsTrue)
	return strings.TrimSuffix(result, "\n"), resp.StatusCode
}

func (s *apiserverSuite) TestHealthRunning(c *gc.C) {
	health, statusCode := s.getHealth(c)
	c.Assert(health, gc.Equals, "running")
	c.Assert(statusCode, gc.Equals, http.StatusOK)
}

func (s *apiserverSuite) TestHealthStopping(c *gc.C) {
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
			c.Assert(statusCode, gc.Equals, http.StatusServiceUnavailable)
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
