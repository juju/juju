// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apihttp_test

import (
	"net/http"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/apihttp"
)

type EndpointsSuite struct {
	BaseSuite

	resolvedHandler http.Handler
}

var _ = gc.Suite(&EndpointsSuite{})

func (s *EndpointsSuite) resolveHandler(spec apihttp.EndpointSpec, method string, unhandled http.Handler, newArgs func(apihttp.HandlerConstraints) apihttp.NewHandlerArgs) http.Handler {
	s.stub.AddCall("resolveHandler", spec, method, unhandled, newArgs)
	s.stub.NextErr() // Pop one off.

	return s.resolvedHandler
}

func (s *EndpointsSuite) TestNewEndpoints(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	specs := endpoints.Specs()

	c.Check(specs, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestAddBasic(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	spec, err := apihttp.NewEndpointSpec("/spam", apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints.Specs(), jc.DeepEquals, []apihttp.EndpointSpec{
		spec,
	})
}

func (s *EndpointsSuite) TestAddZeroValueSpec(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	var spec apihttp.EndpointSpec

	err := endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `spec missing pattern`)
	c.Check(endpoints.Specs(), gc.HasLen, 0)
}

func (s *EndpointsSuite) TestAddNoCollision(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	existing, err := apihttp.NewEndpointSpec("/spam", apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := apihttp.NewEndpointSpec("/eggs", apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints.Specs(), jc.DeepEquals, []apihttp.EndpointSpec{
		existing,
		spec,
	})
}

func (s *EndpointsSuite) TestAddCollisionDisjointMethods(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	existing, err := apihttp.NewEndpointSpec("/spam", hSpec, "POST")
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "PUT")
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `endpoint "/spam" already registered`)
	c.Check(endpoints.Specs(), jc.DeepEquals, []apihttp.EndpointSpec{
		existing,
	})
}

func (s *EndpointsSuite) TestAddCollisionOverlappingMethods(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	existing, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "POST")
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `endpoint "/spam" already registered`)
	c.Check(endpoints.Specs(), jc.DeepEquals, []apihttp.EndpointSpec{
		existing,
	})
}

func (s *EndpointsSuite) TestSpecsPreservesOrder(c *gc.C) {
	endpoints := apihttp.NewEndpoints()
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []apihttp.EndpointSpec
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := apihttp.NewEndpointSpec(pat, hSpec)
		c.Assert(err, jc.ErrorIsNil)
		err = endpoints.Add(spec)
		c.Assert(err, jc.ErrorIsNil)
		expected = append(expected, spec)
	}

	specs := endpoints.Specs()

	s.stub.CheckNoCalls(c)
	c.Check(specs, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestSpecsEmpty(c *gc.C) {
	endpoints := apihttp.NewEndpoints()

	specs := endpoints.Specs()

	c.Check(specs, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveOneMethod(c *gc.C) {
	reg := apihttp.NewEndpoints()
	expected := apihttp.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: s.handler,
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c, "newArgs", "newHandler")
	c.Check(endpoints, jc.DeepEquals, []apihttp.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveEmpty(c *gc.C) {
	reg := apihttp.NewEndpoints()

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveMultipleMethods(c *gc.C) {
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range []string{"GET", "PUT", "DEL"} {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolvePreservesOrder(c *gc.C) {
	reg := apihttp.NewEndpoints()
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []apihttp.Endpoint
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := apihttp.NewEndpointSpec(pat, hSpec, "GET", "PUT")
		c.Assert(err, jc.ErrorIsNil)
		err = reg.Add(spec)
		c.Assert(err, jc.ErrorIsNil)
		for _, method := range []string{"GET", "PUT"} {
			expected = append(expected, apihttp.Endpoint{
				Pattern: pat,
				Method:  method,
				Handler: s.handler,
			})
		}
	}

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveDefaultIgnored(c *gc.C) {
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range []string{"GET", "PUT", "DEL"} {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec1, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec1)
	c.Assert(err, jc.ErrorIsNil)
	spec2, err := apihttp.NewEndpointSpec("/eggs", hSpec)
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec2)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveMissingNewHandler(c *gc.C) {
	reg := apihttp.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	reg.ResolveHandler = s.resolveHandler
	s.resolvedHandler = unsupportedHandler
	expected := apihttp.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: unsupportedHandler,
	}
	var hSpec apihttp.HandlerSpec // no handler
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c,
		"resolveHandler",
	)
	c.Check(endpoints, jc.DeepEquals, []apihttp.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveNoHandler(c *gc.C) {
	reg := apihttp.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	reg.ResolveHandler = s.resolveHandler
	s.resolvedHandler = unsupportedHandler
	expected := apihttp.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: unsupportedHandler,
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newNewHandler(nil),
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c,
		"resolveHandler",
	)
	c.Check(endpoints, jc.DeepEquals, []apihttp.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveForMethodsBasic(c *gc.C) {
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range []string{"GET", "PUT"} {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods([]string{"GET", "PUT"}, s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsEmpty(c *gc.C) {
	reg := apihttp.NewEndpoints()

	endpoints := reg.ResolveForMethods([]string{"GET", "PUT"}, s.newArgs)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveForMethodsPreservesOrder(c *gc.C) {
	reg := apihttp.NewEndpoints()
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []apihttp.Endpoint
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := apihttp.NewEndpointSpec(pat, hSpec, "GET", "PUT")
		c.Assert(err, jc.ErrorIsNil)
		err = reg.Add(spec)
		c.Assert(err, jc.ErrorIsNil)
		for _, method := range []string{"GET", "PUT"} {
			expected = append(expected, apihttp.Endpoint{
				Pattern: pat,
				Method:  method,
				Handler: s.handler,
			})
		}
	}

	endpoints := reg.ResolveForMethods([]string{"GET", "PUT"}, s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsPartialMatch(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsUseDefault(c *gc.C) {
	methods := []string{"GET", "PUT", "DEL", "HEAD"}
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	defaultHandler := &nopHandler{id: "default"}
	expected[2].Handler = defaultHandler
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", apihttp.HandlerSpec{
		NewHandler: s.newNewHandler(defaultHandler),
	})
	c.Assert(err, jc.ErrorIsNil)
	for _, method := range []string{"GET", "HEAD", "PUT"} { // in a different order
		err := spec.Add(method, hSpec)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"NewHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsNoDefault(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := apihttp.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	//reg.ResolveHandler = s.resolveHandler
	//s.resolvedHandler = unsupportedHandler
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	expected[1].Handler = unsupportedHandler
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		//"resolveHandler",
		//"resolveHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		//"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsOnlyDefault(c *gc.C) {
	methods := []string{"GET", "PUT", "DEL"}
	reg := apihttp.NewEndpoints()
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsMissingNewHandler(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := apihttp.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	reg.ResolveHandler = s.resolveHandler
	s.resolvedHandler = unsupportedHandler
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: unsupportedHandler,
		})
	}
	var hSpec apihttp.HandlerSpec // no handler
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, methods...)
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		"resolveHandler",
		"resolveHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsNoHandler(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := apihttp.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	reg.ResolveHandler = s.resolveHandler
	s.resolvedHandler = unsupportedHandler
	var expected []apihttp.Endpoint
	for _, method := range methods {
		expected = append(expected, apihttp.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: unsupportedHandler,
		})
	}
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newNewHandler(nil),
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, methods...)
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.ResolveForMethods(methods, s.newArgs)

	s.stub.CheckCallNames(c,
		"resolveHandler",
		"resolveHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}
