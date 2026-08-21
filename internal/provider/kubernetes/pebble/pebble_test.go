// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pebble_test

import (
	"testing"

	"github.com/juju/tc"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/juju/juju/internal/provider/kubernetes/pebble"
)

type suite struct{}

func TestSuite(t *testing.T) {
	tc.Run(t, &suite{})
}

func (s *suite) TestStartupHandler(c *tc.C) {
	h := pebble.StartupHandler("38811")
	c.Check(h.HTTPGet, tc.NotNil)
	c.Check(h.HTTPGet.Path, tc.Equals, "/v1/health?level=alive")
	c.Check(h.HTTPGet.Port, tc.Equals, intstr.Parse("38811"))
}

func (s *suite) TestLivenessHandler(c *tc.C) {
	h := pebble.LivenessHandler("38811")
	c.Check(h.HTTPGet, tc.NotNil)
	c.Check(h.HTTPGet.Path, tc.Equals, "/v1/health?level=alive")
	c.Check(h.HTTPGet.Port, tc.Equals, intstr.Parse("38811"))
}

func (s *suite) TestReadinessHandler(c *tc.C) {
	h := pebble.ReadinessHandler("38812")
	c.Check(h.HTTPGet, tc.NotNil)
	c.Check(h.HTTPGet.Path, tc.Equals, "/v1/health?level=ready")
	c.Check(h.HTTPGet.Port, tc.Equals, intstr.Parse("38812"))
}

func (s *suite) TestAPIServerReadinessHandler(c *tc.C) {
	h := pebble.APIServerReadinessHandler(17070)
	c.Check(h.TCPSocket, tc.NotNil)
	c.Check(h.TCPSocket.Port, tc.Equals, intstr.FromInt(17070))
}

func (s *suite) TestAPIServerReadinessHandlerDifferentPort(c *tc.C) {
	h := pebble.APIServerReadinessHandler(17777)
	c.Check(h.TCPSocket, tc.NotNil)
	c.Check(h.TCPSocket.Port, tc.Equals, intstr.FromInt(17777))
}

func (s *suite) TestWorkloadHealthCheckPort(c *tc.C) {
	c.Check(pebble.WorkloadHealthCheckPort(0), tc.Equals, "38813")
	c.Check(pebble.WorkloadHealthCheckPort(1), tc.Equals, "38814")
	c.Check(pebble.WorkloadHealthCheckPort(2), tc.Equals, "38815")
	c.Check(pebble.WorkloadHealthCheckPort(10), tc.Equals, "38823")
}

func (s *suite) TestStartupHandlerNilGRPC(c *tc.C) {
	h := pebble.StartupHandler("38811")
	c.Check(h.Exec, tc.IsNil)
	c.Check(h.GRPC, tc.IsNil)
	c.Check(h.TCPSocket, tc.IsNil)
}

func (s *suite) TestAPIServerReadinessHandlerNilOthers(c *tc.C) {
	h := pebble.APIServerReadinessHandler(17070)
	c.Check(h.Exec, tc.IsNil)
	c.Check(h.GRPC, tc.IsNil)
	c.Check(h.HTTPGet, tc.IsNil)
}

func (s *suite) TestProbeHandlerFieldsConsistent(c *tc.C) {
	// HTTP-based handlers use only HTTPGet.
	checkHTTPOnly := func(h corev1.ProbeHandler) {
		c.Check(h.HTTPGet, tc.NotNil)
		c.Check(h.Exec, tc.IsNil)
		c.Check(h.TCPSocket, tc.IsNil)
		c.Check(h.GRPC, tc.IsNil)
	}

	checkHTTPOnly(pebble.StartupHandler("38811"))
	checkHTTPOnly(pebble.LivenessHandler("38811"))
	checkHTTPOnly(pebble.ReadinessHandler("38811"))

	// TCP-based handler uses only TCPSocket.
	h := pebble.APIServerReadinessHandler(17070)
	c.Check(h.TCPSocket, tc.NotNil)
	c.Check(h.Exec, tc.IsNil)
	c.Check(h.HTTPGet, tc.IsNil)
	c.Check(h.GRPC, tc.IsNil)
}
