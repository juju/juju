// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

type MuxSuite struct {
	testing.IsolationSuite
	mux    *apiserverhttp.Mux
	server *httptest.Server
	client *http.Client
}

var _ = gc.Suite(&MuxSuite{})

func (s *MuxSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mux = apiserverhttp.NewMux()
	s.server = httptest.NewServer(s.mux)
	s.client = s.server.Client()
	s.AddCleanup(func(c *gc.C) {
		s.server.Close()
	})
}

func (s *MuxSuite) TestNotFound(c *gc.C) {
	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *MuxSuite) TestAddHandler(c *gc.C) {
	err := s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.Assert(err, jc.ErrorIsNil)

	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
}

func (s *MuxSuite) TestAddRemoveNotFound(c *gc.C) {
	s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.mux.RemoveHandler("GET", "/")

	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusNotFound)
}

func (s *MuxSuite) TestAddHandlerExists(c *gc.C) {
	s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	err := s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.Assert(err, gc.ErrorMatches, `handler for GET "/" already exists`)
}

func (s *MuxSuite) TestRemoveHandlerMissing(c *gc.C) {
	s.mux.RemoveHandler("GET", "/") // no-op
}

func (s *MuxSuite) TestMethodNotSupported(c *gc.C) {
	s.mux.AddHandler("POST", "/", http.NotFoundHandler())
	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, gc.Equals, http.StatusMethodNotAllowed)
}
