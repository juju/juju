// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/apiserver/httpcontext"
	coretesting "github.com/juju/juju/v3/testing"
)

type ModelHandlersSuite struct {
	testing.IsolationSuite
	impliedHandler *httpcontext.ImpliedModelHandler
	queryHandler   *httpcontext.QueryModelHandler
	server         *httptest.Server
}

var _ = gc.Suite(&ModelHandlersSuite{})

func (s *ModelHandlersSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, httpcontext.RequestModelUUID(r))
	})
	s.impliedHandler = &httpcontext.ImpliedModelHandler{
		Handler:   h,
		ModelUUID: coretesting.ModelTag.Id(),
	}
	s.queryHandler = &httpcontext.QueryModelHandler{
		Handler: h,
		Query:   "modeluuid",
	}
	mux := http.NewServeMux()
	mux.Handle("/query", s.queryHandler)
	mux.Handle("/implied", s.impliedHandler)
	s.server = httptest.NewServer(mux)
}

func (s *ModelHandlersSuite) TestRequestModelUUIDNoContext(c *gc.C) {
	uuid := httpcontext.RequestModelUUID(&http.Request{})
	c.Assert(uuid, gc.Equals, "")
}

func (s *ModelHandlersSuite) TestImplied(c *gc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/implied")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQuery(c *gc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=" + coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQueryInvalidModelUUID(c *gc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=zing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusBadRequest)
	defer resp.Body.Close()

	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, `invalid model UUID "zing"`+"\n")
}
