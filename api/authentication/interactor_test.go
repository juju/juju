// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery/form"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/authentication"
)

type InteractorSuite struct {
	testing.IsolationSuite

	jar     *cookiejar.Jar
	client  *httpbakery.Client
	server  *httptest.Server
	handler http.Handler
}

var _ = tc.Suite(&InteractorSuite{})

func (s *InteractorSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	var err error
	s.jar, err = cookiejar.New(nil)
	c.Assert(err, tc.ErrorIsNil)
	s.client = httpbakery.NewClient()
	s.client.Jar = s.jar
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handler.ServeHTTP(w, r)
	}))
	s.AddCleanup(func(c *tc.C) { s.server.Close() })
}

func (s *InteractorSuite) TestNotSupportedInteract(c *tc.C) {
	v := authentication.NewNotSupportedInteractor()
	c.Assert(v.Kind(), tc.Equals, "juju_userpass")
	_, err := v.Interact(context.Background(), nil, "", nil)
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *InteractorSuite) TestLegacyInteract(c *tc.C) {
	v := authentication.NewInteractor("bob", func(username string) (string, error) {
		c.Assert(username, tc.Equals, "bob")
		return "hunter2", nil
	})
	lv, ok := v.(httpbakery.LegacyInteractor)
	c.Assert(ok, tc.IsTrue)
	var formUser, formPassword string
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		formUser = r.Form.Get("user")
		formPassword = r.Form.Get("password")
	})
	err := lv.LegacyInteract(context.Background(), s.client, "", mustParseURL(s.server.URL))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(formUser, tc.Equals, "bob")
	c.Assert(formPassword, tc.Equals, "hunter2")
}

func (s *InteractorSuite) TestKind(c *tc.C) {
	v := authentication.NewInteractor("bob", nil)
	c.Assert(v.Kind(), tc.Equals, "juju_userpass")
}

func (s *InteractorSuite) TestLegacyInteractErrorResult(c *tc.C) {
	v := authentication.NewInteractor("bob", func(username string) (string, error) {
		return "hunter2", nil
	})
	lv, ok := v.(httpbakery.LegacyInteractor)
	c.Assert(ok, tc.IsTrue)
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"Message":"bleh"}`, http.StatusInternalServerError)
	})
	err := lv.LegacyInteract(context.Background(), s.client, "", mustParseURL(s.server.URL))
	c.Assert(err, tc.ErrorMatches, "bleh")
}

func (s *InteractorSuite) TestInteract(c *tc.C) {
	v := authentication.NewInteractor("bob", func(username string) (string, error) {
		c.Assert(username, tc.Equals, "bob")
		return "hunter2", nil
	})
	var formUser, formPassword string
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqParams := httprequest.Params{
			Response: w,
			Request:  r,
			Context:  context.Background(),
		}
		loginRequest := form.LoginRequest{}
		err := httprequest.Unmarshal(reqParams, &loginRequest)
		c.Assert(err, tc.ErrorIsNil)
		formUser = loginRequest.Body.Form["user"].(string)
		formPassword = loginRequest.Body.Form["password"].(string)
		loginResponse := form.LoginResponse{
			Token: &httpbakery.DischargeToken{
				Kind:  "juju_userpass",
				Value: []byte("token"),
			},
		}
		httprequest.WriteJSON(w, http.StatusOK, loginResponse)
	})
	info := form.InteractionInfo{
		URL: s.server.URL,
	}
	infoData, err := json.Marshal(info)
	msgData := json.RawMessage(infoData)
	c.Assert(err, tc.ErrorIsNil)
	token, err := v.Interact(context.Background(), s.client, "", &httpbakery.Error{
		Code: httpbakery.ErrInteractionRequired,
		Info: &httpbakery.ErrorInfo{
			InteractionMethods: map[string]*json.RawMessage{
				"juju_userpass": &msgData,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(formUser, tc.Equals, "bob")
	c.Assert(formPassword, tc.Equals, "hunter2")
	c.Assert(token.Kind, tc.Equals, "juju_userpass")
	c.Assert(string(token.Value), tc.Equals, "token")
}

func (s *InteractorSuite) TestInteractErrorResult(c *tc.C) {
	v := authentication.NewInteractor("bob", func(username string) (string, error) {
		return "hunter2", nil
	})
	s.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"Message":"bleh"}`, http.StatusInternalServerError)
	})
	info := form.InteractionInfo{
		URL: s.server.URL,
	}
	infoData, err := json.Marshal(info)
	c.Assert(err, tc.ErrorIsNil)
	msgData := json.RawMessage(infoData)
	_, err = v.Interact(context.Background(), s.client, "", &httpbakery.Error{
		Code: httpbakery.ErrInteractionRequired,
		Info: &httpbakery.ErrorInfo{
			InteractionMethods: map[string]*json.RawMessage{
				"juju_userpass": &msgData,
			},
		},
	})
	c.Assert(err, tc.ErrorMatches, ".*bleh.*")
}

func mustParseURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
