// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils_test

import (
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/testing"
)

type ErrorSuite struct {
	testing.BaseSuite

	azureError autorest.DetailedError
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.azureError = autorest.DetailedError{
		StatusCode: http.StatusUnauthorized,
	}
}

func (s *ErrorSuite) TestNilContext(c *gc.C) {
	err := errorutils.HandleCredentialError(s.azureError, nil)
	c.Assert(err, gc.DeepEquals, s.azureError)

	invalidated := errorutils.MaybeInvalidateCredential(s.azureError, nil)
	c.Assert(invalidated, jc.IsFalse)

	c.Assert(c.GetTestLog(), jc.DeepEquals, "")
}

func (s *ErrorSuite) TestInvalidationCallbackErrorOnlyLogs(c *gc.C) {
	ctx := context.NewCloudCallContext()
	ctx.InvalidateCredentialFunc = func(msg string) error {
		return errors.New("kaboom")
	}
	errorutils.MaybeInvalidateCredential(s.azureError, ctx)
	c.Assert(c.GetTestLog(), jc.Contains, "could not invalidate stored azure cloud credential on the controller")
}

func (s *ErrorSuite) TestAuthRelatedStatusCodes(c *gc.C) {
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		c.Assert(msg, gc.DeepEquals, "azure cloud denied access")
		called = true
		return nil
	}

	// First test another status code.
	s.azureError.StatusCode = http.StatusAccepted
	errorutils.HandleCredentialError(s.azureError, ctx)
	c.Assert(called, jc.IsFalse)

	for t := range common.AuthorisationFailureStatusCodes {
		called = false
		s.azureError.StatusCode = t
		errorutils.HandleCredentialError(s.azureError, ctx)
		c.Assert(called, jc.IsTrue)
	}
}

func (*ErrorSuite) TestNilAzureError(c *gc.C) {
	ctx := context.NewCloudCallContext()
	called := false
	ctx.InvalidateCredentialFunc = func(msg string) error {
		called = true
		return nil
	}
	returnedErr := errorutils.HandleCredentialError(nil, ctx)
	c.Assert(called, jc.IsFalse)
	c.Assert(returnedErr, jc.ErrorIsNil)
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

func (ErrorSuite) TestCheckForGraphError(c *gc.C) {
	for i, test := range checkForGraphErrorTests {
		c.Logf("test %d. %s", i, test.about)

		var nextResponderCalled bool
		var r autorest.Responder
		r = autorest.ResponderFunc(func(resp *http.Response) error {
			nextResponderCalled = true

			// If the next in the chain is called then the response body should be unchanged.
			defer resp.Body.Close()
			b, err := ioutil.ReadAll(resp.Body)
			c.Assert(err, jc.ErrorIsNil)
			c.Check(string(b), gc.Equals, test.responseBody)
			return nil
		})

		r = errorutils.CheckForGraphError(r)

		err := r.Respond(&http.Response{Body: ioutil.NopCloser(strings.NewReader(test.responseBody))})
		if test.expectError == "" {
			c.Check(nextResponderCalled, gc.Equals, true)
			continue
		}
		c.Assert(err, gc.Not(jc.ErrorIsNil))
		ge := errorutils.AsGraphError(err)
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
	err: &errorutils.GraphError{
		GraphError: graphrbac.GraphError{
			OdataError: &graphrbac.OdataError{
				Code: to.StringPtr("ErrorCode"),
				ErrorMessage: &graphrbac.ErrorMessage{
					Message: to.StringPtr("error message"),
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
		Original: &errorutils.GraphError{
			GraphError: graphrbac.GraphError{
				OdataError: &graphrbac.OdataError{
					Code: to.StringPtr("ErrorCode"),
					ErrorMessage: &graphrbac.ErrorMessage{
						Message: to.StringPtr("error message"),
					},
				},
			},
		},
	},
	expectGraphError: "ErrorCode: error message",
}, {
	about: "traced graph error",
	err: errors.Trace(&errorutils.GraphError{
		GraphError: graphrbac.GraphError{
			OdataError: &graphrbac.OdataError{
				Code: to.StringPtr("ErrorCode"),
				ErrorMessage: &graphrbac.ErrorMessage{
					Message: to.StringPtr("error message"),
				},
			},
		},
	}),
	expectGraphError: "ErrorCode: error message",
}}

func (ErrorSuite) TestAsGraphError(c *gc.C) {
	for i, test := range asGraphErrorTests {
		c.Logf("test %d. %s", i, test.about)
		ge := errorutils.AsGraphError(test.err)
		if test.expectGraphError == "" {
			c.Check(ge, gc.IsNil)
			continue
		}
		c.Assert(ge, gc.Not(gc.IsNil))
		c.Check(ge.Error(), gc.Equals, test.expectGraphError)
	}
}
