// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/http"
)

type EndpointsSuite struct {
	BaseSuite
}

var _ = gc.Suite(&EndpointsSuite{})

func (s *EndpointsSuite) TestNewEndpoints(c *gc.C) {
	endpoints := http.NewEndpoints()
	specs := endpoints.Specs()

	c.Check(specs, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestAddBasic(c *gc.C) {
	endpoints := http.NewEndpoints()
	spec, err := http.NewEndpointSpec("/spam", http.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints.Specs(), jc.DeepEquals, []http.EndpointSpec{
		spec,
	})
}

func (s *EndpointsSuite) TestAddZeroValueSpec(c *gc.C) {
	endpoints := http.NewEndpoints()
	var spec http.EndpointSpec

	err := endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `spec missing pattern`)
	c.Check(endpoints.Specs(), gc.HasLen, 0)
}

func (s *EndpointsSuite) TestAddNoCollision(c *gc.C) {
	endpoints := http.NewEndpoints()
	existing, err := http.NewEndpointSpec("/spam", http.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := http.NewEndpointSpec("/eggs", http.HandlerSpec{
		NewHandler: s.newHandler,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints.Specs(), jc.DeepEquals, []http.EndpointSpec{
		existing,
		spec,
	})
}

func (s *EndpointsSuite) TestAddCollisionDisjointMethods(c *gc.C) {
	endpoints := http.NewEndpoints()
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	existing, err := http.NewEndpointSpec("/spam", hSpec, "POST")
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := http.NewEndpointSpec("/spam", hSpec, "PUT")
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `endpoint "/spam" already registered`)
	c.Check(endpoints.Specs(), jc.DeepEquals, []http.EndpointSpec{
		existing,
	})
}

func (s *EndpointsSuite) TestAddCollisionOverlappingMethods(c *gc.C) {
	endpoints := http.NewEndpoints()
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	existing, err := http.NewEndpointSpec("/spam", hSpec, "GET", "POST")
	c.Assert(err, jc.ErrorIsNil)
	err = endpoints.Add(existing)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
	c.Assert(err, jc.ErrorIsNil)

	err = endpoints.Add(spec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `endpoint "/spam" already registered`)
	c.Check(endpoints.Specs(), jc.DeepEquals, []http.EndpointSpec{
		existing,
	})
}

func (s *EndpointsSuite) TestSpecsPreservesOrder(c *gc.C) {
	endpoints := http.NewEndpoints()
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []http.EndpointSpec
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := http.NewEndpointSpec(pat, hSpec)
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
	endpoints := http.NewEndpoints()

	specs := endpoints.Specs()

	c.Check(specs, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveOneMethod(c *gc.C) {
	reg := http.NewEndpoints()
	expected := http.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: s.handler,
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c, "newArgs", "newHandler")
	c.Check(endpoints, jc.DeepEquals, []http.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveEmpty(c *gc.C) {
	reg := http.NewEndpoints()

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveMultipleMethods(c *gc.C) {
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range []string{"GET", "PUT", "DEL"} {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL")
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
	reg := http.NewEndpoints()
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []http.Endpoint
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := http.NewEndpointSpec(pat, hSpec, "GET", "PUT")
		c.Assert(err, jc.ErrorIsNil)
		err = reg.Add(spec)
		c.Assert(err, jc.ErrorIsNil)
		for _, method := range []string{"GET", "PUT"} {
			expected = append(expected, http.Endpoint{
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
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range []string{"GET", "PUT", "DEL"} {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec1, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL", "")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec1)
	c.Assert(err, jc.ErrorIsNil)
	spec2, err := http.NewEndpointSpec("/eggs", hSpec)
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
	reg := http.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	expected := http.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: unsupportedHandler,
	}
	var hSpec http.HandlerSpec // no handler
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c, "newArgs", "newHandler")
	c.Check(endpoints, jc.DeepEquals, []http.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveNoHandler(c *gc.C) {
	reg := http.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	expected := http.Endpoint{
		Pattern: "/spam",
		Method:  "GET",
		Handler: unsupportedHandler,
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newNewHandler(nil),
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)
	err = reg.Add(spec)
	c.Assert(err, jc.ErrorIsNil)

	endpoints := reg.Resolve(s.newArgs)

	s.stub.CheckCallNames(c, "newArgs", "newHandler")
	c.Check(endpoints, jc.DeepEquals, []http.Endpoint{
		expected,
	})
}

func (s *EndpointsSuite) TestResolveForMethodsBasic(c *gc.C) {
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range []string{"GET", "PUT"} {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
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
	reg := http.NewEndpoints()

	endpoints := reg.ResolveForMethods([]string{"GET", "PUT"}, s.newArgs)

	s.stub.CheckNoCalls(c)
	c.Check(endpoints, gc.HasLen, 0)
}

func (s *EndpointsSuite) TestResolveForMethodsPreservesOrder(c *gc.C) {
	reg := http.NewEndpoints()
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	var expected []http.Endpoint
	for _, pat := range []string{"/spam", "/eggs", "/ham"} {
		spec, err := http.NewEndpointSpec(pat, hSpec, "GET", "PUT")
		c.Assert(err, jc.ErrorIsNil)
		err = reg.Add(spec)
		c.Assert(err, jc.ErrorIsNil)
		for _, method := range []string{"GET", "PUT"} {
			expected = append(expected, http.Endpoint{
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
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT", "DEL")
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
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	defaultHandler := &nopHandler{id: "default"}
	expected[2].Handler = defaultHandler
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "HEAD", "PUT") // in a different order
	c.Assert(err, jc.ErrorIsNil)
	err = spec.Add("", http.HandlerSpec{
		NewHandler: s.newNewHandler(defaultHandler),
	})
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
		"newArgs",
		"newHandler",
	)
	c.Check(endpoints, jc.DeepEquals, expected)
}

func (s *EndpointsSuite) TestResolveForMethodsNoDefault(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := http.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	expected[1].Handler = unsupportedHandler
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
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

func (s *EndpointsSuite) TestResolveForMethodsOnlyDefault(c *gc.C) {
	methods := []string{"GET", "PUT", "DEL"}
	reg := http.NewEndpoints()
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: s.handler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec)
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
	reg := http.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: unsupportedHandler,
		})
	}
	var hSpec http.HandlerSpec // no handler
	spec, err := http.NewEndpointSpec("/spam", hSpec, methods...)
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

func (s *EndpointsSuite) TestResolveForMethodsNoHandler(c *gc.C) {
	methods := []string{"GET", "PUT"}
	reg := http.NewEndpoints()
	unsupportedHandler := &nopHandler{id: "unsupported"}
	reg.UnsupportedMethodHandler = unsupportedHandler
	var expected []http.Endpoint
	for _, method := range methods {
		expected = append(expected, http.Endpoint{
			Pattern: "/spam",
			Method:  method,
			Handler: unsupportedHandler,
		})
	}
	hSpec := http.HandlerSpec{
		NewHandler: s.newNewHandler(nil),
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, methods...)
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
