// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package connector

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/testing"
)

type simpleConnectorSuite struct {
	testing.BaseSuite
}

func TestSimpleConnectorSuite(t *stdtesting.T) { tc.Run(t, &simpleConnectorSuite{}) }
func (s *simpleConnectorSuite) TestNewSimpleRespectsClientCredentials(c *tc.C) {
	tests := []struct {
		name                    string
		opts                    SimpleConfig
		expectedError           string
		expectedAPIInfo         api.Info
		expectedDefaultDialOpts func() api.DialOpts
	}{
		{
			name: "with username/password",
			opts: SimpleConfig{
				ControllerAddresses: []string{"some.host:9999"},
				ModelUUID:           "some-uuid",
				Username:            "some-username",
				Password:            "some-password",
			},
			expectedAPIInfo: api.Info{
				Addrs:    []string{"some.host:9999"},
				ModelTag: names.NewModelTag("some-uuid"),
				Tag:      names.NewUserTag("some-username"),
				Password: "some-password",
			},
		},
		{
			name: "with client credentials",
			opts: SimpleConfig{
				ControllerAddresses: []string{"some.host:9999"},
				ModelUUID:           "some-uuid",
				ClientID:            "some-client-id",
				ClientSecret:        "some-client-secret",
			},
			expectedAPIInfo: api.Info{
				Addrs:    []string{"some.host:9999"},
				ModelTag: names.NewModelTag("some-uuid"),
			},
			expectedDefaultDialOpts: func() api.DialOpts {
				expected := api.DefaultDialOpts()
				expected.LoginProvider = api.NewClientCredentialsLoginProvider("some-client-id", "some-client-secret")
				return expected
			},
		},
		{
			name: "with both username and client ID",
			opts: SimpleConfig{
				ControllerAddresses: []string{"some.host:9999"},
				ModelUUID:           "some-uuid",
				Username:            "some-username",
				Password:            "some-password",
				ClientID:            "some-client-id",
				ClientSecret:        "some-client-secret",
			},
			expectedError: "only one of Username or ClientID should be set",
		},
		{
			name: "with neither username nor client ID",
			opts: SimpleConfig{
				ControllerAddresses: []string{"some.host:9999"},
				ModelUUID:           "some-uuid",
			},
			expectedError: "one of Username or ClientID must be set",
		},
	}

	for _, test := range tests {
		c.Logf("running test %s", test.name)

		connector, err := NewSimple(test.opts)

		if test.expectedError != "" {
			c.Assert(err, tc.ErrorMatches, test.expectedError)
			c.Assert(connector, tc.IsNil)
		} else {
			c.Assert(err, tc.IsNil)
			c.Assert(connector.info, tc.DeepEquals, test.expectedAPIInfo)

			expectedDefaultDialOpts := api.DefaultDialOpts()
			if test.expectedDefaultDialOpts != nil {
				expectedDefaultDialOpts = test.expectedDefaultDialOpts()
			}
			c.Assert(connector.defaultDialOpts, tc.DeepEquals, expectedDefaultDialOpts)
		}
	}
}

func (s *simpleConnectorSuite) TestSimpleConnectorConnect(c *tc.C) {
	connector, err := NewSimple(SimpleConfig{
		Username:            "alice@canonical.com",
		ControllerAddresses: []string{"localhost:17080"},
	})
	c.Assert(err, tc.IsNil)

	var called bool

	s.PatchValue(&apiOpen, func(_ context.Context, i *api.Info, do api.DialOpts) (api.Connection, error) {
		called = true

		// Zeros to false, ensure it is true after Connect dial opt.
		c.Assert(do.InsecureSkipVerify, tc.Equals, true)

		// Defaults to 10 * time.Minute, ensure it is overwritten after Connect dial opt.
		c.Assert(do.Timeout, tc.Equals, 5*time.Minute)
		return nil, nil
	})

	_, err = connector.Connect(c.Context(),
		func(do *api.DialOpts) {
			do.InsecureSkipVerify = true
			do.Timeout = 5 * time.Minute
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(called, tc.Equals, true)
}
