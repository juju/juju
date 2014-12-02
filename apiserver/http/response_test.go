// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"net/http"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/apiserver/http"
	apihttptesting "github.com/juju/juju/apiserver/http/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing"
)

type responseSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&responseSuite{})

func (s *responseSuite) TestExtractAPIErrorFailure(c *gc.C) {
	original := &params.Error{
		Message: "something went wrong!",
	}
	response := apihttptesting.NewFailureResponse(original)
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure, gc.Not(gc.Equals), original)
	c.Check(failure, gc.DeepEquals, original)
}

func (s *responseSuite) TestExtractAPIErrorWrongContentType(c *gc.C) {
	original := &params.Error{
		Message: "something went wrong!",
	}
	response := apihttptesting.NewFailureResponse(original)
	response.Header.Del("Content-Type")
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure.Message, gc.Equals, `{"Message":"something went wrong!","Code":""}`+"\n")
	c.Check(failure.Code, gc.Equals, "")
}

func (s *responseSuite) TestExtractAPIErrorString(c *gc.C) {
	response := apihttptesting.NewErrorResponse(http.StatusInternalServerError, "something went wrong!")
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure.Message, gc.Equals, "something went wrong!")
	c.Check(failure.Code, gc.Equals, "")
}

func (s *responseSuite) TestExtractAPIErrorNotFound(c *gc.C) {
	response := apihttptesting.NewErrorResponse(http.StatusNotFound, "something went wrong!")
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure.Message, gc.Equals, "something went wrong!")
	c.Check(failure.Code, gc.Equals, params.CodeNotImplemented)
}

func (s *responseSuite) TestExtractAPIErrorMethodNotAllowed(c *gc.C) {
	response := apihttptesting.NewErrorResponse(http.StatusMethodNotAllowed, "something went wrong!")
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure.Message, gc.Equals, "something went wrong!")
	c.Check(failure.Code, gc.Equals, params.CodeNotImplemented)
}

func (s *responseSuite) TestExtractAPIErrorOK(c *gc.C) {
	response := apihttptesting.NewHTTPResponse()
	failure, err := apihttp.ExtractAPIError(&response.Response)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(failure, gc.IsNil)
}
