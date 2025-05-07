// Copyright 2024 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/authentication"
)

type MockAuthenticatorNotFound struct{}

func (m *MockAuthenticatorNotFound) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, errors.NotFound
}

type MockAuthenticatorNotImplemented struct{}

func (m *MockAuthenticatorNotImplemented) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, errors.NotImplemented
}

type MockAuthenticatorError struct{}

func (m *MockAuthenticatorError) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, errors.Errorf("error mock authentication")
}

type MockAuthenticatorNoError struct{}

func (m *MockAuthenticatorNoError) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	return authentication.AuthInfo{}, nil
}

type HTTPAuthenticatorSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&HTTPAuthenticatorSuite{})

func (s *HTTPAuthenticatorSuite) TestHTTPStrategicAuthenticator(c *tc.C) {
	tests := []struct {
		description        string
		httpAuthenticators authentication.HTTPStrategicAuthenticator
		expectedError      string
	}{
		{
			description:        "good",
			httpAuthenticators: authentication.HTTPStrategicAuthenticator{&MockAuthenticatorNoError{}},
			expectedError:      "",
		},
		{
			description: "good",
			httpAuthenticators: authentication.HTTPStrategicAuthenticator{
				&MockAuthenticatorNotFound{}, &MockAuthenticatorNotImplemented{}, &MockAuthenticatorNoError{},
			},
			expectedError: "",
		},
		{
			description: "error is returned",
			httpAuthenticators: authentication.HTTPStrategicAuthenticator{
				&MockAuthenticatorNotFound{}, &MockAuthenticatorError{},
			},
			expectedError: "error mock authentication",
		},
		{
			description: "fallback to default error",
			httpAuthenticators: authentication.HTTPStrategicAuthenticator{
				&MockAuthenticatorNotFound{}, &MockAuthenticatorNotImplemented{},
			},
			expectedError: "authentication not found",
		},
	}

	for _, t := range tests {
		_, err := t.httpAuthenticators.Authenticate(nil)
		if t.expectedError != "" {
			c.Check(err.Error(), tc.Contains, t.expectedError)
		} else {
			c.Assert(err, tc.ErrorIsNil)
		}
	}
}
