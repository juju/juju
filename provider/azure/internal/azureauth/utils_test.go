// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"io"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
)

type TokenResourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TokenResourceSuite{})

func (s *TokenResourceSuite) TestTokenResource(c *gc.C) {
	out := azureauth.TokenResource("https://graph.windows.net")
	c.Assert(out, gc.Equals, "https://graph.windows.net/")
	out = azureauth.TokenResource("https://graph.windows.net/")
	c.Assert(out, gc.Equals, "https://graph.windows.net/")
}

var checkForGraphErrorTests = []struct {
	about        string
	responseBody string
	expectError  string
}{{
	about:        "error body",
	responseBody: `{"odata.error":{"code":"ErrorCode","message":{"value": "error message"}}}`,
	expectError:  "ErrorCode: error message",
}, {
	about:        "not error",
	responseBody: `{}`,
	expectError:  "",
}, {
	about:        "error body with unicode BOM",
	responseBody: "\ufeff" + `{"odata.error":{"code":"ErrorCode","message":{"value": "error message"}}}`,
	expectError:  "ErrorCode: error message",
}, {
	about:        "not error with unicode BOM",
	responseBody: "\ufeff{}",
	expectError:  "",
}}

type ErrorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorSuite{})

func (ErrorSuite) TestCheckForGraphError(c *gc.C) {
	for i, test := range checkForGraphErrorTests {
		c.Logf("test %d. %s", i, test.about)

		var nextResponderCalled bool
		var r autorest.Responder
		r = autorest.ResponderFunc(func(resp *http.Response) error {
			nextResponderCalled = true

			// If the next in the chain is called then the response body should be unchanged.
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			c.Assert(err, jc.ErrorIsNil)
			c.Check(string(b), gc.Equals, test.responseBody)
			return nil
		})

		r = azureauth.CheckForGraphError(r)

		err := r.Respond(&http.Response{Body: io.NopCloser(strings.NewReader(test.responseBody))})
		if test.expectError == "" {
			c.Check(nextResponderCalled, gc.Equals, true)
			continue
		}
		c.Assert(err, gc.Not(jc.ErrorIsNil))
		ge := azureauth.AsGraphError(err)
		c.Check(ge, gc.Not(gc.IsNil))
		c.Check(err.Error(), gc.Equals, test.expectError)
	}
}

var asGraphErrorTests = []struct {
	about            string
	err              error
	expectGraphError string
}{{
	about: "graph error",
	err: &azureauth.GraphError{
		GraphError: graphrbac.GraphError{
			OdataError: &graphrbac.OdataError{
				Code: to.Ptr("ErrorCode"),
				ErrorMessage: &graphrbac.ErrorMessage{
					Message: to.Ptr("error message"),
				},
			},
		},
	},
	expectGraphError: "ErrorCode: error message",
}, {
	about: "nil error",
	err:   nil,
}, {
	about: "unrelated error",
	err:   errors.New("test error"),
}, {
	about: "in autorest.DetailedError",
	err: autorest.DetailedError{
		Original: &azureauth.GraphError{
			GraphError: graphrbac.GraphError{
				OdataError: &graphrbac.OdataError{
					Code: to.Ptr("ErrorCode"),
					ErrorMessage: &graphrbac.ErrorMessage{
						Message: to.Ptr("error message"),
					},
				},
			},
		},
	},
	expectGraphError: "ErrorCode: error message",
}, {
	about: "traced graph error",
	err: errors.Trace(&azureauth.GraphError{
		GraphError: graphrbac.GraphError{
			OdataError: &graphrbac.OdataError{
				Code: to.Ptr("ErrorCode"),
				ErrorMessage: &graphrbac.ErrorMessage{
					Message: to.Ptr("error message"),
				},
			},
		},
	}),
	expectGraphError: "ErrorCode: error message",
}}

func (ErrorSuite) TestAsGraphError(c *gc.C) {
	for i, test := range asGraphErrorTests {
		c.Logf("test %d. %s", i, test.about)
		ge := azureauth.AsGraphError(test.err)
		if test.expectGraphError == "" {
			c.Check(ge, gc.IsNil)
			continue
		}
		c.Assert(ge, gc.Not(gc.IsNil))
		c.Check(ge.Error(), gc.Equals, test.expectGraphError)
	}
}
