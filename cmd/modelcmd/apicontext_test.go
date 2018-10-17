// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type APIContextSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&APIContextSuite{})

func (s *APIContextSuite) TestNewAPIContext(c *gc.C) {
	store := jujuclient.NewFileClientStore()

	ctx, err := modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)

	handler := func(w http.ResponseWriter, req *http.Request) {
		// Set a cookie so we can check that cookies are
		// saved.
		http.SetCookie(w, &http.Cookie{
			Name:   "cook",
			Value:  "val",
			MaxAge: 1000,
		})
		w.Write([]byte("hello"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
	}))
	defer srv.Close()
	// Check that we can use the client.
	assertClientGet(c, ctx.NewBakeryClient(), srv.URL, "hello")

	// Close the context, which should save the cookies.
	err = ctx.Close()
	c.Assert(err, jc.ErrorIsNil)

	// Make another APIContext which should
	// get the cookies just saved.
	ctx, err = modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)

	handler = func(w http.ResponseWriter, req *http.Request) {
		c.Check(req.Cookies(), jc.DeepEquals, []*http.Cookie{{
			Name:  "cook",
			Value: "val",
		}})
		w.Write([]byte("goodbye"))
	}
	assertClientGet(c, ctx.NewBakeryClient(), srv.URL, "goodbye")
}

func (s *APIContextSuite) TestDomainCookie(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	s.PatchEnvironment("JUJU_USER_DOMAIN", "something")
	ctx, err := modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c.Check(req.Cookies(), jc.DeepEquals, []*http.Cookie{{
			Name:  "domain",
			Value: "something",
		}})
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	// Check that we can use the client.
	assertClientGet(c, ctx.NewBakeryClient(), srv.URL, "hello")
}

func assertClientGet(c *gc.C, client *httpbakery.Client, url string, expectBody string) {
	req, err := http.NewRequest("GET", url, nil)
	c.Assert(err, jc.ErrorIsNil)
	resp, err := client.Do(req)
	c.Assert(err, jc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	data, _ := ioutil.ReadAll(resp.Body)
	c.Assert(string(data), gc.Equals, expectBody)
}
