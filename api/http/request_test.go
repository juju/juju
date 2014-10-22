// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"net/url"

	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/api/http"
	apihttptesting "github.com/juju/juju/api/http/testing"
)

type requestSuite struct {
	apihttptesting.BaseSuite
}

var _ = gc.Suite(&requestSuite{})

func (s *requestSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *requestSuite) TestNewRequestSuccess(c *gc.C) {
	baseURL, err := url.Parse("https://localhost:8080/")
	c.Assert(err, gc.IsNil)
	uuid := "abcd-efedcb-012345-6789"
	tag := "machine-0"
	pw := "secure"
	req, err := apihttp.NewRequest("GET", baseURL, "somefacade", uuid, tag, pw)
	c.Assert(err, gc.IsNil)

	s.CheckRequest(c, req, "GET", tag, pw, "localhost", "somefacade")
}
