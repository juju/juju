// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/http"
)

type EndpointSpecSuite struct {
	BaseSuite
}

var _ = gc.Suite(&EndpointSpecSuite{})

func (s *EndpointSpecSuite) TestNewEndpointSpecBasic(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET", "PUT")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), gc.Equals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestNewEndpointSpecNoMethods(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), gc.HasLen, 0)
	dflt, ok := spec.Default()
	c.Check(ok, jc.IsTrue)
	c.Check(dflt, jc.DeepEquals, hSpec)
}

func (s *EndpointSpecSuite) TestNewEndpointSpecLowerCaseMethod(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("/spam", hSpec, "get")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), gc.Equals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestNewEndpointSpecEmptyMethod(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	_, err := http.NewEndpointSpec("/spam", hSpec, "")

	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing method`)
}

func (s *EndpointSpecSuite) TestNewEndpointSpecUnrecognizedMethod(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("/spam", hSpec, "<NOT VALID>")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), gc.Equals, []string{"<NOT VALID>"})
}

func (s *EndpointSpecSuite) TestNewEndpointSpecTrailingSlash(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("/spam/", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecRelativePattern(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecMissingPattern(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}

	spec, err := http.NewEndpointSpec("", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/")
}

func (s *EndpointSpecSuite) TestNewEndpointSpecMissingNewHandler(c *gc.C) {
	var hSpec http.HandlerSpec

	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Pattern(), gc.Equals, "/spam")
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
	dflt, ok := spec.Default()
	c.Check(ok, jc.IsTrue)
	c.Check(dflt, jc.DeepEquals, hSpec)
	// We don't actually check the handler spec since we can't
	// compare functions.
}

func (s *EndpointSpecSuite) TestAddOkay(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestAddLowerCase(c *gc.C) {
	var hSpec http.HandlerSpec
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("put", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
}

func (s *EndpointSpecSuite) TestAddWithDefault(c *gc.C) {
	var hSpec http.HandlerSpec
	spec, err := http.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsTrue)
}

func (s *EndpointSpecSuite) TestAddMissingMethod(c *gc.C) {
	var hSpec http.HandlerSpec
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("", hSpec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
	c.Check(err, gc.ErrorMatches, `missing method`)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestAddCollision(c *gc.C) {
	var hSpec http.HandlerSpec
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("GET", hSpec)

	s.stub.CheckNoCalls(c)
	c.Check(err, jc.Satisfies, errors.IsAlreadyExists)
	c.Check(err, gc.ErrorMatches, `HTTP method "GET" already added`)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET"})
}

func (s *EndpointSpecSuite) TestAddZeroValueHandlerSpec(c *gc.C) {
	var hSpec http.HandlerSpec
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	err = spec.Add("PUT", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckNoCalls(c)
	c.Check(spec.Methods(), jc.DeepEquals, []string{"GET", "PUT"})
	_, ok := spec.Default()
	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestDefaultWithDefault(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec)
	c.Assert(err, jc.ErrorIsNil)

	_, ok := spec.Default()

	c.Check(ok, jc.IsTrue)
	// We can't compare the returned spec because Go can't compare functions.
}

func (s *EndpointSpecSuite) TestDefaultWithoutDefault(c *gc.C) {
	hSpec := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", hSpec, "GET")
	c.Assert(err, jc.ErrorIsNil)

	_, ok := spec.Default()

	c.Check(ok, jc.IsFalse)
}

func (s *EndpointSpecSuite) TestResolveOkay(c *gc.C) {
	constraints := http.HandlerConstraints{
		AuthKind: "user",
	}
	orig := http.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, http.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveMissingUnhandled(c *gc.C) {
	constraints := http.HandlerConstraints{
		AuthKind: "user",
	}
	orig := http.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)

	hSpec := spec.Resolve("GET", nil)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, http.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveMissingNewHandler(c *gc.C) {
	constraints := http.HandlerConstraints{
		AuthKind: "user",
	}
	orig := http.HandlerSpec{
		Constraints: constraints,
	}
	spec, err := http.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, http.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, unhandled)
}

func (s *EndpointSpecSuite) TestResolveNoHandler(c *gc.C) {
	constraints := http.HandlerConstraints{
		AuthKind: "user",
	}
	orig := http.HandlerSpec{
		Constraints: constraints,
		NewHandler:  s.newNewHandler(nil),
	}
	spec, err := http.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	hSpec.NewHandler = nil
	c.Check(hSpec, jc.DeepEquals, http.HandlerSpec{
		Constraints: constraints,
	})
	c.Check(handler, gc.Equals, unhandled)
}

func (s *EndpointSpecSuite) TestResolveNotFoundWithDefault(c *gc.C) {
	orig := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", orig)
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("GET", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	c.Check(handler, gc.Equals, s.handler)
}

func (s *EndpointSpecSuite) TestResolveNotFoundWithoutDefault(c *gc.C) {
	orig := http.HandlerSpec{
		NewHandler: s.newHandler,
	}
	spec, err := http.NewEndpointSpec("/spam", orig, "GET")
	c.Assert(err, jc.ErrorIsNil)
	unhandled := &nopHandler{id: "unhandled"}

	hSpec := spec.Resolve("PUT", unhandled)

	s.stub.CheckNoCalls(c)
	handler := hSpec.NewHandler(http.NewHandlerArgs{})
	c.Check(handler, gc.Equals, unhandled)
}
