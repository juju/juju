// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type APIContextSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&APIContextSuite{})

func (s *APIContextSuite) TestNewAPIContext(c *tc.C) {
	store := jujuclient.NewFileClientStore()

	ctx, err := modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)

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
	c.Assert(err, tc.ErrorIsNil)

	// Make another APIContext which should
	// get the cookies just saved.
	ctx, err = modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)

	handler = func(w http.ResponseWriter, req *http.Request) {
		c.Check(req.Cookies(), tc.DeepEquals, []*http.Cookie{{
			Name:  "cook",
			Value: "val",
		}})
		w.Write([]byte("goodbye"))
	}
	assertClientGet(c, ctx.NewBakeryClient(), srv.URL, "goodbye")
}

func (s *APIContextSuite) TestDomainCookie(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	s.PatchEnvironment("JUJU_USER_DOMAIN", "something")
	ctx, err := modelcmd.NewAPIContext(nil, nil, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		c.Check(req.Cookies(), tc.DeepEquals, []*http.Cookie{{
			Name:  "domain",
			Value: "something",
		}})
		w.Write([]byte("hello"))
	}))
	defer srv.Close()
	// Check that we can use the client.
	assertClientGet(c, ctx.NewBakeryClient(), srv.URL, "hello")
}

func assertClientGet(c *tc.C, client *httpbakery.Client, url string, expectBody string) {
	req, err := http.NewRequest("GET", url, nil)
	c.Assert(err, tc.ErrorIsNil)
	resp, err := client.Do(req)
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	data, _ := io.ReadAll(resp.Body)
	c.Assert(string(data), tc.Equals, expectBody)
}

func (s *APIContextSuite) TestNewAPIContextEmbedded(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	opts := modelcmd.AuthOpts{Embedded: true}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, tc.Not(tc.IsNil))
	_, err = interactor.Interact(context.Background(), nil, "", nil)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *APIContextSuite) TestNewAPIContextNoBrowser(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	opts := modelcmd.AuthOpts{NoBrowser: true}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, tc.Not(tc.IsNil))
	c.Assert(interactor.Kind(), tc.Equals, "usso_oauth")
}

func (s *APIContextSuite) TestNewAPIContextBrowser(c *tc.C) {
	store := jujuclient.NewFileClientStore()
	cmdCtx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	opts := modelcmd.AuthOpts{}
	ctx, err := modelcmd.NewAPIContext(cmdCtx, &opts, store, "testcontroller")
	c.Assert(err, tc.ErrorIsNil)
	interactor := modelcmd.Interactor(ctx)
	c.Assert(interactor, tc.Not(tc.IsNil))
	c.Assert(interactor.Kind(), tc.Equals, "browser-window")
}
