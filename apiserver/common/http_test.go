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
