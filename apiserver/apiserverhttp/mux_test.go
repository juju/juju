// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserverhttp_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/apiserverhttp"
	coretesting "github.com/juju/juju/internal/testing"
)

type MuxSuite struct {
	testing.IsolationSuite
	mux    *apiserverhttp.Mux
	server *httptest.Server
	client *http.Client
}

var _ = tc.Suite(&MuxSuite{})

func (s *MuxSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.mux = apiserverhttp.NewMux()
	s.server = httptest.NewServer(s.mux)
	s.client = s.server.Client()
	s.AddCleanup(func(c *tc.C) {
		s.server.Close()
	})
}

func (s *MuxSuite) TestNotFound(c *tc.C) {
	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
}

func (s *MuxSuite) TestAddHandler(c *tc.C) {
	err := s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.Assert(err, tc.ErrorIsNil)

	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *MuxSuite) TestAddRemoveNotFound(c *tc.C) {
	s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	s.mux.RemoveHandler("GET", "/")

	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusNotFound)
}

func (s *MuxSuite) TestAddHandlerExists(c *tc.C) {
	s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	err := s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.Assert(err, tc.ErrorMatches, `handler for GET "/" already exists`)
}

func (s *MuxSuite) TestRemoveHandlerMissing(c *tc.C) {
	s.mux.RemoveHandler("GET", "/") // no-op
}

func (s *MuxSuite) TestMethodNotSupported(c *tc.C) {
	s.mux.AddHandler("POST", "/", http.NotFoundHandler())
	resp, err := s.client.Get(s.server.URL + "/")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()

	c.Assert(resp.StatusCode, tc.Equals, http.StatusMethodNotAllowed)
}

func (s *MuxSuite) TestConcurrentAddHandler(c *tc.C) {
	err := s.mux.AddHandler("GET", "/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c.Assert(err, tc.ErrorIsNil)

	// Concurrently add and remove another handler to show that
	// adding and removing handlers will not race with request
	// handling.

	// bN is the number of add and remove handlers to make.
	const bN = 1000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < bN; i++ {
			s.mux.AddHandler("POST", "/", http.NotFoundHandler())
			s.mux.RemoveHandler("POST", "/")
		}
	}()
	defer wg.Wait()

	for i := 0; i < bN; i++ {
		resp, err := s.client.Get(s.server.URL + "/")
		c.Assert(err, tc.ErrorIsNil)
		resp.Body.Close()
		c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	}
}

func (s *MuxSuite) TestConcurrentRemoveHandler(c *tc.C) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// Concurrently add and remove another handler to show that
	// adding and removing handlers will not race with request
	// handling.

	// bN is the number of add and remove handlers to make.
	const bN = 500
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		defer close(done)
		for i := 0; i < bN; i++ {
			s.mux.AddHandler("GET", "/", h)
			// Sleep to give the client a
			// chance to hit the endpoint.
			time.Sleep(time.Millisecond)
			s.mux.RemoveHandler("GET", "/")
			time.Sleep(time.Millisecond)
		}
	}()
	defer wg.Wait()

	var ok, notfound int
out:
	for {
		select {
		case <-done:
			break out
		default:
		}
		resp, err := s.client.Get(s.server.URL + "/")
		c.Assert(err, tc.ErrorIsNil)
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusOK:
			ok++
		case http.StatusNotFound:
			notfound++
		default:
			c.Fatalf(
				"got status %d, expected %d or %d",
				resp.StatusCode,
				http.StatusOK,
				http.StatusNotFound,
			)
		}
	}
	c.Assert(ok, tc.Not(tc.Equals), 0)
	c.Assert(notfound, tc.Not(tc.Equals), 0)
}

func (s *MuxSuite) TestWait(c *tc.C) {
	// Check that mux.Wait() blocks until clients are all finished
	// with it.
	s.mux.AddClient()
	s.mux.AddClient()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.mux.Wait()
	}()

	select {
	case <-finished:
		c.Fatalf("should wait when there are clients")
	case <-time.After(coretesting.ShortWait):
	}

	s.mux.ClientDone()
	select {
	case <-finished:
		c.Fatalf("should wait when there is still a client")
	case <-time.After(coretesting.ShortWait):
	}

	s.mux.ClientDone()
	select {
	case <-finished:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("should finish once clients are done")
	}
}
