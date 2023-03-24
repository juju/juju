// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
	jujuversion "github.com/juju/juju/version"
)

type loginTokenSuite struct {
	apiserverBaseSuite
	url string
	tok []byte
}

var _ = gc.Suite(&loginTokenSuite{})

func (s *loginTokenSuite) SetUpTest(c *gc.C) {
	tok, set, err := apitesting.EncodedJWT(apitesting.JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			"controller": "superuser",
			"model":      "write",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.tok = tok

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hdrs := w.Header()
		hdrs.Set(`Content-Type`, `application/json`)
		pub, _ := set.Key(0)
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

	request := &params.LoginRequest{
		Token:         base64.StdEncoding.EncodeToString(s.tok),
		ClientVersion: jujuversion.Current.String(),
	}

	var response params.LoginResult
	err := st.APICall("Admin", 3, "", "Login", request, &response)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(response.UserInfo, gc.NotNil)
	c.Assert(response.UserInfo.Identity, gc.Equals, "user-fred")
	c.Assert(response.UserInfo.ControllerAccess, gc.Equals, "superuser")
	c.Assert(response.UserInfo.ModelAccess, gc.Equals, "write")
}
