// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"net/http"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	coretesting "github.com/juju/juju/testing"
)

type HTTPEndpointSpecSuite struct {
	HTTPBaseSuite
}

var _ = gc.Suite(&HTTPEndpointSpecSuite{})

func (s *HTTPEndpointSpecSuite) TestNewHTTPEndpointSpec(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointSpecSuite) TestAdd(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointSpecSuite) TestPattern(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointSpecSuite) TestMethods(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointSpecSuite) TestResolve(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

type HTTPEndpointsSuite struct {
	HTTPBaseSuite
}

var _ = gc.Suite(&HTTPEndpointsSuite{})

func (s *HTTPEndpointsSuite) TestNewHTTPEndpoints(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointsSuite) TestAdd(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointsSuite) TestSpecs(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointsSuite) TestResolve(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

func (s *HTTPEndpointsSuite) TestResolveForMethods(c *gc.C) {
	// TODO(ericsnow) This needs to be implemented ASAP.
}

type HTTPBaseSuite struct {
	coretesting.BaseSuite

	stub    *testing.Stub
	handler http.Handler
	args    common.NewHTTPHandlerArgs
}

func (s *HTTPBaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.handler = &nopHTTPHandler{id: "suite default"}
	s.args = common.NewHTTPHandlerArgs{}
}

func (s *HTTPBaseSuite) newNewHTTPHandler(handler http.Handler) func(common.NewHTTPHandlerArgs) http.Handler {
	return func(args common.NewHTTPHandlerArgs) http.Handler {
		s.stub.AddCall("NewHTTPHandler", args)
		s.stub.NextErr() // pop one off

		return handler
	}
}

func (s *HTTPBaseSuite) newHandler(args common.NewHTTPHandlerArgs) http.Handler {
	s.stub.AddCall("newHandler", args)
	s.stub.NextErr() // pop one off

	return s.handler
}

func (s *HTTPBaseSuite) newArgs(constraints common.HTTPHandlerConstraints) common.NewHTTPHandlerArgs {
	s.stub.AddCall("newArgs", constraints)
	s.stub.NextErr() // pop one off

	return s.args
}

type nopHTTPHandler struct {
	// id uniquely identifies the handler (for when that matters).
	// This is not required.
	id string
}

func (nopHTTPHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

type httpHandlerSpec struct {
	constraints common.HTTPHandlerConstraints
	handler     http.Handler
}

type httpEndpointSpec struct {
	pattern        string
	methodHandlers map[string]httpHandlerSpec
}

func checkSpec(c *gc.C, spec common.HTTPEndpointSpec, expected httpEndpointSpec) {
	// Note that we don't check HTTPHandlerSpec.NewHTTPHandler directly.
	// Go does not support direct comparison of functions.
	actual := httpEndpointSpec{
		pattern:        spec.Pattern(),
		methodHandlers: make(map[string]httpHandlerSpec),
	}
	unhandled := &nopHTTPHandler{id: "unhandled"} // We use this to ensure unhandled mismatches.
	for _, method := range spec.Methods() {
		hSpec := spec.Resolve(method, unhandled)
		handler := hSpec.NewHTTPHandler(common.NewHTTPHandlerArgs{})
		actual.methodHandlers[method] = httpHandlerSpec{
			constraints: hSpec.Constraints,
			handler:     handler,
		}

	}
	c.Check(actual, jc.DeepEquals, expected)
}

func checkSpecs(c *gc.C, specs []common.HTTPEndpointSpec, expected []httpEndpointSpec) {
	comment := gc.Commentf("len(%#v) != len(%#v)", specs, expected)
	if !c.Check(len(specs), gc.Equals, len(expected), comment) {
		return
	}
	for i, expectedSpec := range expected {
		spec := specs[i]
		checkSpec(c, spec, expectedSpec)
	}
}
