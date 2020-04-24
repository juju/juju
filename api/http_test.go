// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/httprequest.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type httpSuite struct {
	jujutesting.JujuConnSuite

	client *httprequest.Client
}

var _ = gc.Suite(&httpSuite{})

func (s *httpSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	client, err := s.APIState.HTTPClient()
	c.Assert(err, gc.IsNil)
	s.client = client
}

var httpClientTests = []struct {
	about           string
	handler         http.HandlerFunc
	expectResponse  interface{}
	expectError     string
	expectErrorCode string
	expectErrorInfo map[string]interface{}
}{{
	about: "success",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusOK, "hello, world")
	},
	expectResponse: newString("hello, world"),
}, {
	about: "unauthorized status without discharge-required error",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
			Message: "something",
		})
	},
	expectError: `Get http://.*/: something`,
}, {
	about:       "non-JSON error response",
	handler:     http.NotFound,
	expectError: `Get http://.*/: unexpected content type text/plain; want application/json; content: 404 page not found`,
}, {
	about: "bad error response",
	handler: func(w http.ResponseWriter, req *http.Request) {
		type badResponse struct {
			Message map[string]int
		}
		httprequest.WriteJSON(w, http.StatusUnauthorized, badResponse{
			Message: make(map[string]int),
		})
	},
	expectError: `Get http://.*/: incompatible error response: json: cannot unmarshal object into Go .+`,
}, {
	about: "bad charms error response",
	handler: func(w http.ResponseWriter, req *http.Request) {
		type badResponse struct {
			Error    string         `json:"error"`
			CharmURL map[string]int `json:"charm-url"`
		}
		httprequest.WriteJSON(w, http.StatusUnauthorized, badResponse{
			Error:    "something",
			CharmURL: make(map[string]int),
		})
	},
	expectError: `Get http://.*/: incompatible error response: json: cannot unmarshal object into Go .+`,
}, {
	about: "no message in ErrorResponse",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.ErrorResult{
			Error: &params.Error{},
		})
	},
	expectError: `Get http://.*/: error response with no message`,
}, {
	about: "no message in Error",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{})
	},
	expectError: `Get http://.*/: error response with no message`,
}, {
	about: "charms error response",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusBadRequest, params.CharmsResponse{
			Error:     "some error",
			ErrorCode: params.CodeBadRequest,
			ErrorInfo: params.DischargeRequiredErrorInfo{
				MacaroonPath: "foo",
			}.AsMap(),
		})
	},
	expectError:     `.*some error$`,
	expectErrorCode: params.CodeBadRequest,
	expectErrorInfo: params.DischargeRequiredErrorInfo{
		MacaroonPath: "foo",
	}.AsMap(),
}, {
	about: "discharge-required response with no attached info",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
			Message: "some error",
			Code:    params.CodeDischargeRequired,
		})
	},
	expectError:     `Get http://.*/: no error info found in discharge-required response error: some error`,
	expectErrorCode: params.CodeDischargeRequired,
}, {
	about: "discharge-required response with no macaroon",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
			Message: "some error",
			Code:    params.CodeDischargeRequired,
			Info: params.DischargeRequiredErrorInfo{
				MacaroonPath: "/",
			}.AsMap(),
		})
	},
	expectError: `Get http://.*/: no macaroon found in discharge-required response`,
}}

func (s *httpSuite) TestHTTPClient(c *gc.C) {
	var handler http.HandlerFunc
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handler(w, req)
	}))
	defer srv.Close()
	s.client.BaseURL = srv.URL
	for i, test := range httpClientTests {
		c.Logf("test %d: %s", i, test.about)
		handler = test.handler
		var resp interface{}
		if test.expectResponse != nil {
			resp = reflect.New(reflect.TypeOf(test.expectResponse).Elem()).Interface()
		}
		err := s.client.Get(context.Background(), "/", resp)
		if test.expectError != "" {
			c.Check(err, gc.ErrorMatches, test.expectError)
			c.Check(params.ErrCode(err), gc.Equals, test.expectErrorCode)
			if err, ok := errors.Cause(err).(*params.Error); ok {
				c.Check(err.Info, jc.DeepEquals, test.expectErrorInfo)
			} else if test.expectErrorInfo != nil {
				c.Fatalf("no error info found in error")
			}
			continue
		}
		c.Check(err, gc.IsNil)
		c.Check(resp, jc.DeepEquals, test.expectResponse)
	}
}

func (s *httpSuite) TestControllerMachineAuthForHostedModel(c *gc.C) {
	// Create a controller machine & hosted model.
	const nonce = "gary"
	m, password := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Jobs:  []state.MachineJob{state.JobManageModel},
		Nonce: nonce,
	})
	hostedState := s.Factory.MakeModel(c, nil)
	defer hostedState.Close()

	// Connect to the hosted model using the credentials of the
	// controller machine.
	apiInfo := s.APIInfo(c)
	apiInfo.Tag = m.Tag()
	apiInfo.Password = password
	hostedModel, err := hostedState.Model()
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.ModelTag = hostedModel.ModelTag()
	apiInfo.Nonce = nonce
	conn, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	httpClient, err := conn.HTTPClient()
	c.Assert(err, jc.ErrorIsNil)

	// Test with a dummy HTTP server returns the auth related headers used.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		username, password, ok := req.BasicAuth()
		if ok {
			httprequest.WriteJSON(w, http.StatusOK, map[string]string{
				"username": username,
				"password": password,
				"nonce":    req.Header.Get(params.MachineNonceHeader),
			})
		} else {
			httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
				Message: "no auth header",
			})
		}
	}))
	defer srv.Close()
	httpClient.BaseURL = srv.URL
	var out map[string]string
	c.Assert(httpClient.Get(context.Background(), "/", &out), jc.ErrorIsNil)
	c.Assert(out, gc.DeepEquals, map[string]string{
		"username": m.Tag().String(),
		"password": password,
		"nonce":    nonce,
	})
}

func (s *httpSuite) TestAuthHTTPRequest(c *gc.C) {
	apiInfo := &api.Info{}

	req := s.authHTTPRequest(c, apiInfo)
	_, _, ok := req.BasicAuth()
	c.Assert(ok, jc.IsFalse)
	c.Assert(req.Header, gc.HasLen, 1)
	c.Assert(req.Header.Get(httpbakery.BakeryProtocolHeader), gc.Equals, "3")

	apiInfo.Nonce = "foo"
	req = s.authHTTPRequest(c, apiInfo)
	_, _, ok = req.BasicAuth()
	c.Assert(ok, jc.IsFalse)
	c.Assert(req.Header.Get(params.MachineNonceHeader), gc.Equals, "foo")
	c.Assert(req.Header.Get(httpbakery.BakeryProtocolHeader), gc.Equals, "3")

	apiInfo.Tag = names.NewMachineTag("123")
	apiInfo.Password = "password"
	req = s.authHTTPRequest(c, apiInfo)
	user, pass, ok := req.BasicAuth()
	c.Assert(ok, jc.IsTrue)
	c.Assert(user, gc.Equals, "machine-123")
	c.Assert(pass, gc.Equals, "password")
	c.Assert(req.Header.Get(params.MachineNonceHeader), gc.Equals, "foo")
	c.Assert(req.Header.Get(httpbakery.BakeryProtocolHeader), gc.Equals, "3")

	mac, err := apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Macaroons = []macaroon.Slice{{mac}}
	req = s.authHTTPRequest(c, apiInfo)
	c.Assert(req.Header.Get(params.MachineNonceHeader), gc.Equals, "foo")
	c.Assert(req.Header.Get(httpbakery.BakeryProtocolHeader), gc.Equals, "3")
	macaroons := httpbakery.RequestMacaroons(req)
	apitesting.MacaroonsEqual(c, macaroons, apiInfo.Macaroons)
}

func (s *httpSuite) authHTTPRequest(c *gc.C, info *api.Info) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = api.AuthHTTPRequest(req, info)
	c.Assert(err, jc.ErrorIsNil)
	return req
}

// Note: the fact that the code works against the actual API server is
// well tested by some of the other API tests.
// This suite focuses on less reachable paths by changing
// the BaseURL of the httprequest.Client so that
// we can use our own custom servers.

func newString(s string) *string {
	return &s
}
