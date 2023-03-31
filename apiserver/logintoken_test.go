// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwk"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type loginTokenSuite struct {
	apiserverBaseSuite
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
}

var _ = gc.Suite(&loginTokenSuite{})

func (s *loginTokenSuite) SetUpTest(c *gc.C) {
	keySet, signingKey, err := apitesting.NewJWKSet()
	c.Assert(err, jc.ErrorIsNil)
	s.keySet = keySet
	s.signingKey = signingKey

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hdrs := w.Header()
		hdrs.Set(`Content-Type`, `application/json`)
		pub, _ := s.keySet.Key(0)
		_ = json.NewEncoder(w).Encode(pub)
	}))

	s.ControllerConfig = testing.FakeControllerConfig()
	s.ControllerConfig["login-token-refresh-url"] = srv.URL
	s.url = srv.URL + "/.well-known/jwks.json"
	s.apiserverBaseSuite.SetUpTest(c)
	s.AddCleanup(func(_ *gc.C) { srv.Close() })
}

func (s *loginTokenSuite) TestLoginTokenRefreshConfig(c *gc.C) {
	srv := s.newServer(c, s.config)
	set := apiserver.TokenPublicKey(c, srv, s.url)
	c.Assert(set.Len(), gc.Equals, 1)
}

func (s *loginTokenSuite) TestUsesLoginToken(c *gc.C) {
	srv := s.newServer(c, s.config)
	st := s.openAPINoLogin(c, srv, false)

	modelTag, ok := st.ModelTag()
	c.Assert(ok, jc.IsTrue)
	tok, err := apitesting.EncodedJWT(apitesting.JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)
	request := &params.LoginRequest{
		Token:         base64.StdEncoding.EncodeToString(tok),
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err = st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.UserInfo, gc.NotNil)
	c.Assert(response.UserInfo.Identity, gc.Equals, "user-fred")
	c.Assert(response.UserInfo.ControllerAccess, gc.Equals, "login")
	c.Assert(response.UserInfo.ModelAccess, gc.Equals, "write")
}

func (s *loginTokenSuite) TestControllerSuperuser(c *gc.C) {
	srv := s.newServer(c, s.config)
	st := s.openAPINoLogin(c, srv, false)

	tok, err := apitesting.EncodedJWT(apitesting.JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "superuser",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)
	request := &params.LoginRequest{
		Token:         base64.StdEncoding.EncodeToString(tok),
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err = st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.UserInfo, gc.NotNil)
	c.Assert(response.UserInfo.Identity, gc.Equals, "user-fred")
	c.Assert(response.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Assert(response.UserInfo.ModelAccess, gc.Equals, "admin")
}

func (s *loginTokenSuite) TestLoginInvalidUser(c *gc.C) {
	srv := s.newServer(c, s.config)
	st := s.openAPINoLogin(c, srv, false)

	tok, err := apitesting.EncodedJWT(apitesting.JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "machine-0",
		Access: map[string]string{
			testing.ControllerTag.String(): "superuser",
			testing.ModelTag.String():      "write",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)
	request := &params.LoginRequest{
		Token:         base64.StdEncoding.EncodeToString(tok),
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err = st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, gc.ErrorMatches, `parsing request authToken: invalid user tag in authToken: "machine-0" is not a valid user tag`)
}

func (s *loginTokenSuite) TestLoginInvalidPermission(c *gc.C) {
	srv := s.newServer(c, s.config)
	st := s.openAPINoLogin(c, srv, false)

	tok, err := apitesting.EncodedJWT(apitesting.JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "admin",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)
	request := &params.LoginRequest{
		Token:         base64.StdEncoding.EncodeToString(tok),
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err = st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, gc.ErrorMatches, `"admin" controller access not valid .*`)
}
