// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext_test

import (
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/apiserver/httpcontext"
	coremodel "github.com/juju/juju/core/model"
	coretesting "github.com/juju/juju/internal/testing"
)

type ModelHandlersSuite struct {
	testing.IsolationSuite

	controllerModelHandler *httpcontext.ControllerModelHandler
	queryHandler           *httpcontext.QueryModelHandler
	bucketHandler          *httpcontext.BucketModelHandler

	server *httptest.Server
}

var _ = tc.Suite(&ModelHandlersSuite{})

func (s *ModelHandlersSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modelUUID, _ := httpcontext.RequestModelUUID(r.Context())
		io.WriteString(w, modelUUID)
	})
	s.controllerModelHandler = &httpcontext.ControllerModelHandler{
		Handler:             h,
		ControllerModelUUID: coremodel.UUID(coretesting.ModelTag.Id()),
	}
	s.queryHandler = &httpcontext.QueryModelHandler{
		Handler: h,
		Query:   "modeluuid",
	}
	s.bucketHandler = &httpcontext.BucketModelHandler{
		Handler: h,
		Query:   ":modeluuid",
	}
	mux := apiserverhttp.NewMux()
	mux.AddHandler("GET", "/query", s.queryHandler)
	mux.AddHandler("GET", "/controller", s.controllerModelHandler)
	mux.AddHandler("GET", "/model-:modeluuid/charms/:object", s.bucketHandler)
	s.server = httptest.NewServer(mux)
}

func (s *ModelHandlersSuite) TestControllerUUID(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/controller")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQuery(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=" + coretesting.ModelTag.Id())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestQueryInvalidModelUUID(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/query?modeluuid=zing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusBadRequest)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, `invalid model UUID "zing"`+"\n")
}

func (s *ModelHandlersSuite) TestBucket(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/model-" + coretesting.ModelTag.Id() + "/charms/somecharm-abcd0123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, coretesting.ModelTag.Id())
}

func (s *ModelHandlersSuite) TestInvalidBucket(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/modelwrongbucket/charms/somecharm-abcd0123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "404 page not found\n")
}

func (s *ModelHandlersSuite) TestBucketInvalidModelUUID(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL + "/model-wrongbucket/charms/somecharm-abcd0123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusBadRequest)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, `invalid model UUID "wrongbucket"`+"\n")
}
