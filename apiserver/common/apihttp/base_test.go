// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apihttp_test

import (
	"net/http"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/apihttp"
	coretesting "github.com/juju/juju/testing"
)

type BaseSuite struct {
	coretesting.BaseSuite

	stub    *testing.Stub
	handler http.Handler
	args    apihttp.NewHandlerArgs
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.handler = &nopHandler{id: "suite default"}
	s.args = apihttp.NewHandlerArgs{}
}

func (s *BaseSuite) newNewHandler(handler http.Handler) func(apihttp.NewHandlerArgs) http.Handler {
	return func(args apihttp.NewHandlerArgs) http.Handler {
		s.stub.AddCall("NewHandler", args)
		s.stub.NextErr() // pop one off

		return handler
	}
}

func (s *BaseSuite) newHandler(args apihttp.NewHandlerArgs) http.Handler {
	s.stub.AddCall("newHandler", args)
	s.stub.NextErr() // pop one off

	return s.handler
}

func (s *BaseSuite) newArgs(constraints apihttp.HandlerConstraints) apihttp.NewHandlerArgs {
	s.stub.AddCall("newArgs", constraints)
	s.stub.NextErr() // pop one off

	return s.args
}

type nopHandler struct {
	// id uniquely identifies the handler (for when that matters).
	// This is not required.
	id string
}

func (nopHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

// TODO(ericsnow) Drop these...

type httpHandlerSpec struct {
	constraints apihttp.HandlerConstraints
	handler     http.Handler
}

type httpEndpointSpec struct {
	pattern        string
	methodHandlers map[string]httpHandlerSpec
}

func checkSpec(c *gc.C, spec apihttp.EndpointSpec, expected httpEndpointSpec) {
	// Note that we don't check HandlerSpec.NewHandler directly.
	// Go does not support direct comparison of functions.
	actual := httpEndpointSpec{
		pattern:        spec.Pattern(),
		methodHandlers: make(map[string]httpHandlerSpec),
	}
	unhandled := &nopHandler{id: "unhandled"} // We use this to ensure unhandled mismatches.
	for _, method := range spec.Methods() {
		hSpec := spec.Resolve(method, unhandled)
		handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
		actual.methodHandlers[method] = httpHandlerSpec{
			constraints: hSpec.Constraints,
			handler:     handler,
		}

	}
	c.Check(actual, jc.DeepEquals, expected)
}

func checkSpecs(c *gc.C, specs []apihttp.EndpointSpec, expected []httpEndpointSpec) {
	comment := gc.Commentf("len(%#v) != len(%#v)", specs, expected)
	if !c.Check(len(specs), gc.Equals, len(expected), comment) {
		return
	}
	for i, expectedSpec := range expected {
		spec := specs[i]
		checkSpec(c, spec, expectedSpec)
	}
}
