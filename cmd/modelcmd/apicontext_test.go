// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/cmd/modelcmd"
	"github.com/juju/juju/v3/jujuclient"
	"github.com/juju/juju/v3/testing"
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

func (s *APIContextSuite) TestNewAPIContextEmbedded(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	opts := modelcmd.AuthOpts{Embedded: true}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, gc.Not(gc.IsNil))
	_, err = interactor.Interact(context.TODO(), nil, "", nil)
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *APIContextSuite) TestNewAPIContextNoBrowser(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	opts := modelcmd.AuthOpts{NoBrowser: true}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, gc.Not(gc.IsNil))
	c.Assert(interactor.Kind(), gc.Equals, "usso_oauth")
}

func (s *APIContextSuite) TestNewAPIContextBrowser(c *gc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	opts := modelcmd.AuthOpts{}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, jc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, gc.Not(gc.IsNil))
	c.Assert(interactor.Kind(), gc.Equals, "browser-window")
}
