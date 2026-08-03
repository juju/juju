// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/tracer"
	"github.com/juju/juju/api/base/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type tracerSuite struct {
	coretesting.BaseSuite
}

func TestTracerSuite(t *stdtesting.T) {
	tc.Run(t, &tracerSuite{})
}

func (s *tracerSuite) TestGetControllerTracingConfig(c *tc.C) {
	caCert := "ca-cert"
	insecure := false
	stackTraces := true
	sampleRatio := 0.5
	tailSamplingThreshold := "1s"
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "Tracer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "GetControllerTracingConfig")
		c.Check(arg, tc.DeepEquals, params.Entity{
			Tag: "machine-666",
		})
		c.Assert(result, tc.FitsTypeOf, &params.TracingConfigResult{})
		*(result.(*params.TracingConfigResult)) = params.TracingConfigResult{
			HTTPEndpoint:          "https://otel.example.com",
			GRPCEndpoint:          "otel.example.com:4317",
			CACert:                &caCert,
			InsecureSkipVerify:    &insecure,
			StackTraces:           &stackTraces,
			SampleRatio:           &sampleRatio,
			TailSamplingThreshold: &tailSamplingThreshold,
		}
		return nil
	})

	client := tracer.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	result, err := client.GetControllerTracingConfig(c.Context(), tag)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.HTTPEndpoint, tc.Equals, "https://otel.example.com")
	c.Check(result.GRPCEndpoint, tc.Equals, "otel.example.com:4317")
	c.Check(result.CACert, tc.Equals, "ca-cert")
	c.Check(result.InsecureSkipVerify, tc.NotNil)
	c.Check(*result.InsecureSkipVerify, tc.Equals, false)
	c.Check(result.StackTraces, tc.NotNil)
	c.Check(*result.StackTraces, tc.Equals, true)
	c.Check(result.SampleRatio, tc.NotNil)
	c.Check(*result.SampleRatio, tc.Equals, 0.5)
	c.Check(result.TailSamplingThreshold, tc.NotNil)
	c.Check(*result.TailSamplingThreshold, tc.Equals, "1s")
}

func (s *tracerSuite) TestGetControllerTracingConfigError(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Assert(result, tc.FitsTypeOf, &params.TracingConfigResult{})
		*(result.(*params.TracingConfigResult)) = params.TracingConfigResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := tracer.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	_, err := client.GetControllerTracingConfig(c.Context(), tag)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}

func (s *tracerSuite) TestWatchControllerTracingConfig(c *tc.C) {
	apiCaller := testing.APICallerFunc(func(objType string, version int, id, request string, arg, result any) error {
		c.Check(objType, tc.Equals, "Tracer")
		c.Check(version, tc.Equals, 0)
		c.Check(id, tc.Equals, "")
		c.Check(request, tc.Equals, "WatchControllerTracingConfig")
		c.Check(arg, tc.DeepEquals, params.Entity{
			Tag: "machine-666",
		})
		c.Assert(result, tc.FitsTypeOf, &params.NotifyWatchResult{})
		*(result.(*params.NotifyWatchResult)) = params.NotifyWatchResult{
			Error: &params.Error{Message: "FAIL"},
		}
		return nil
	})

	client := tracer.NewClient(apiCaller)
	tag := names.NewMachineTag("666")
	_, err := client.WatchControllerTracingConfig(c.Context(), tag)
	c.Assert(err, tc.ErrorMatches, "FAIL")
}
