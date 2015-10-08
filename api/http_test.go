// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/httprequest"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
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
	expectErrorInfo *params.ErrorInfo
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
	expectError: `GET http://.*/: something`,
}, {
	about:       "non-JSON error response",
	handler:     http.NotFound,
	expectError: `GET http://.*/: unexpected content type text/plain; want application/json; content: 404 page not found`,
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
	expectError: `GET http://.*/: incompatible error response: json: cannot unmarshal object into Go value of type string`,
}, {
	about: "bad charms error response",
	handler: func(w http.ResponseWriter, req *http.Request) {
		type badResponse struct {
			Error    string
			CharmURL map[string]int
		}
		httprequest.WriteJSON(w, http.StatusUnauthorized, badResponse{
			Error:    "something",
			CharmURL: make(map[string]int),
		})
	},
	expectError: `GET http://.*/: incompatible error response: json: cannot unmarshal object into Go value of type string`,
}, {
	about: "no message in ErrorResponse",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.ErrorResult{
			Error: &params.Error{},
		})
	},
	expectError: `GET http://.*/: error response with no message`,
}, {
	about: "no message in Error",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{})
	},
	expectError: `GET http://.*/: error response with no message`,
}, {
	about: "charms error response",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusBadRequest, params.CharmsResponse{
			Error:     "some error",
			ErrorCode: params.CodeBadRequest,
			ErrorInfo: &params.ErrorInfo{
				MacaroonPath: "foo",
			},
		})
	},
	expectError:     `GET http://.*/: some error`,
	expectErrorCode: params.CodeBadRequest,
	expectErrorInfo: &params.ErrorInfo{
		MacaroonPath: "foo",
	},
}, {
	about: "discharge-required response with no error info",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
			Message: "some error",
			Code:    params.CodeDischargeRequired,
		})
	},
	expectError:     `GET http://.*/: no error info found in discharge-required response error: some error`,
	expectErrorCode: params.CodeDischargeRequired,
}, {
	about: "discharge-required response with no macaroon",
	handler: func(w http.ResponseWriter, req *http.Request) {
		httprequest.WriteJSON(w, http.StatusUnauthorized, params.Error{
			Message: "some error",
			Code:    params.CodeDischargeRequired,
			Info:    &params.ErrorInfo{},
		})
	},
	expectError: `GET http://.*/: no macaroon found in discharge-required response`,
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
		err := s.client.Get("/", resp)
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(params.ErrCode(err), gc.Equals, test.expectErrorCode)
			if err, ok := errors.Cause(err).(*params.Error); ok {
				c.Assert(err.Info, jc.DeepEquals, test.expectErrorInfo)
			} else if test.expectErrorInfo != nil {
				c.Fatalf("no error info found in error")
			}
			continue
		}
		c.Assert(err, gc.IsNil)
		c.Assert(resp, jc.DeepEquals, test.expectResponse)
	}
}

// Note: the fact that the code works against the actual API server is
// well tested by some of the other API tests.
// This suite focuses on less reachable paths by changing
// the BaseURL of the httprequest.Client so that
// we can use our own custom servers.

func newString(s string) *string {
	return &s
}
