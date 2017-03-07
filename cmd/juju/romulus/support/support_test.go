// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package support_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	stdtesting "testing"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/romulus/support"
	jjjtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
	jjtesting "github.com/juju/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	jjtesting.MgoTestPackage(t)
}

var _ = gc.Suite(&supportCommandSuite{})

type supportCommandSuite struct {
	jjjtesting.JujuConnSuite

	mockAPI  *mockapi
	charmURL string
}

func (s *setPlanCommandSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	mockAPI, err := newMockAPI()
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = mockAPI

	s.PatchValue(support.NewAuthorizationClient, support.APIClientFnc(s.mockAPI))
}

func (s setPlanCommandSuite) TestSupportCommand(c *gc.C) {
	c.Skip("not ready")
	tests := []struct {
		about    string
		level    string
		budget   string
		err      string
		apiErr   error
		apiCalls []testing.StubCall
	}{{
		about:       "all is well",
		plan:        "bob/default",
		application: "mysql",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				s.charmURL,
				"mysql",
			},
		}},
	}, {
		about:       "invalid application name",
		plan:        "bob/default",
		application: "mysql-0",
		err:         "invalid application name \"mysql-0\"",
	}, {
		about:       "unknown application",
		plan:        "bob/default",
		application: "wordpress",
		err:         "application \"wordpress\" not found.*",
	}, {
		about:       "unknown application",
		plan:        "bob/default",
		application: "mysql",
		apiErr:      errors.New("some strange error"),
		err:         "some strange error",
	},
	}
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		s.mockAPI.ResetCalls()
		if test.apiErr != nil {
			s.mockAPI.SetErrors(test.apiErr)
		}
		_, err := cmdtesting.RunCommand(c, setplan.NewSetPlanCommand(), test.application, test.plan)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 1)
			s.mockAPI.CheckCalls(c, test.apiCalls)

			app, err := s.State.Application("mysql")
			c.Assert(err, jc.ErrorIsNil)
			svcMacaroon := app.MetricCredentials()
			data, err := json.Marshal(macaroon.Slice{s.mockAPI.macaroon})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(svcMacaroon, gc.DeepEquals, data)
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 0)
		}
	}
}

func (s *setPlanCommandSuite) TestNoArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, setplan.NewSetPlanCommand())
	c.Assert(err, gc.ErrorMatches, "need to specify application name and plan url")
}

func newMockAPI() (*mockapi, error) {
	kp, err := bakery.GenerateKey()
	if err != nil {
		return nil, errors.Trace(err)
	}
	svc, err := bakery.NewService(bakery.NewServiceParams{
		Location: "omnibus",
		Key:      kp,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &mockapi{
		service: svc,
	}, nil
}

type mockapi struct {
	testing.Stub

	service  *bakery.Service
	macaroon *macaroon.Macaroon
}

func (m *mockapi) Authorize(modelUUID, supportLevel string, visitWebPage func(*url.URL) error) (*macaroon.Macaroon, error) {
	err := m.NextErr()
	if err != nil {
		return nil, errors.Trace(err)
	}
	m.AddCall("Authorize", modelUUID, supportLevel)
	macaroon, err := m.service.NewMacaroon(
		"foobar",
		nil,
		[]checkers.Caveat{},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	m.macaroon = macaroon
	return m.macaroon, nil
}
