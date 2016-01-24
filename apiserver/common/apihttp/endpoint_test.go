// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apihttp_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/apihttp"
)

type EndpointSpecSuite struct {
	BaseSuite
}

var _ = gc.Suite(&EndpointSpecSuite{})

func (s *EndpointSpecSuite) TestNewEndpointSpecBasic(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestNewEndpointSpecNoMethods(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), gc.HasLen, 0)
	_, ok := spec.Default()
	c.Check(ok, jc.IsTrue)
	// We don't check the actual default because Go doesn't let us
	// compare functions.
}

func (s *EndpointSpecSuite) TestNewEndpointSpecLowerCaseMethod(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "get")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestNewEndpointSpecEmptyMethod(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	_, err := apihttp.NewEndpointSpec("/spam", hSpec, "")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing method`)
}

func (s *EndpointSpecSuite) TestNewEndpointSpecUnrecognizedMethod(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "<NOT VALID>")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"<NOT VALID>"})
}

func (s *EndpointSpecSuite) TestNewEndpointSpecTrailingSlash(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("/spam/", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecRelativePattern(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecMissingPattern(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := apihttp.NewEndpointSpec("", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecMissingNewHandler(c *gc.C) {
	var hSpec apihttp.HandlerSpec

	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestAddOkay(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestAddLowerCase(c *gc.C) {
	var hSpec apihttp.HandlerSpec
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("put", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
}

func (s *EndpointSpecSuite) TestAddWithDefault(c *gc.C) {
	var hSpec apihttp.HandlerSpec
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsTrue)
}

func (s *EndpointSpecSuite) TestAddMissingMethod(c *gc.C) {
	var hSpec apihttp.HandlerSpec
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("", hSpec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing method`)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestAddCollision(c *gc.C) {
	var hSpec apihttp.HandlerSpec
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("GET", hSpec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `HTTP method "GET" already added`)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestAddZeroValueHandlerSpec(c *gc.C) {
	var hSpec apihttp.HandlerSpec
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestDefaultWithDefault(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	_, ok := spec.Default()

	c.Check(ok, jc.IsTrue)
	// We can't compare the returned spec because Go can't compare functions.
}

func (s *EndpointSpecSuite) TestDefaultWithoutDefault(c *gc.C) {
	hSpec := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	_, ok := spec.Default()

	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestResolveOkay(c *gc.C) {
	constraints := apihttp.HandlerConstraints{
		AuthKind: "user",
	}
	orig := apihttp.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, apihttp.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveMissingUnhandled(c *gc.C) {
	constraints := apihttp.HandlerConstraints{
		AuthKind: "user",
	}
	orig := apihttp.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)

	hSpec := spec.Resolve("GET", nil)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, apihttp.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveMissingNewHandler(c *gc.C) {
	constraints := apihttp.HandlerConstraints{
		AuthKind: "user",
	}
	orig := apihttp.HandlerSpec{
		Constraints: constraints,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, apihttp.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, unhandled)
}

func (s *EndpointSpecSuite) TestResolveNoHandler(c *gc.C) {
	constraints := apihttp.HandlerConstraints{
		AuthKind: "user",
	}
	orig := apihttp.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newNewHandler(nil),
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, apihttp.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, unhandled)
}

func (s *EndpointSpecSuite) TestResolveNotFoundWithDefault(c *gc.C) {
	orig := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig)
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveNotFoundWithoutDefault(c *gc.C) {
	orig := apihttp.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := apihttp.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("PUT", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(apihttp.NewHandlerArgs{})
	c.Check(handler, gc.Equals, unhandled)
}
