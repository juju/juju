// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http_test

import (
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/apiserver/http"
	apihttptesting "github.com/juju/juju/apiserver/http/testing"
	"github.com/juju/juju/testing"
)

type requestSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&requestSuite{})

func (s *requestSuite) TestNewRequestSuccess(c *gc.C) {
	baseURL, err := url.Parse("https://localhost:8080/")
	c.Assert(err, jc.ErrorIsNil)
	uuid := "abcd-efedcb-012345-6789"
	tag := "machine-0"
	pw := "secure"
	req, err := apihttp.NewRequest("GET", baseURL, "somefacade", uuid, tag, pw)
	c.Assert(err, jc.ErrorIsNil)

	apihttptesting.CheckRequest(c, req, "GET", tag, pw, "localhost", "somefacade")
}
