// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package support_test

import (
	//"encoding/json"
	stdtesting "testing"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/romulus/support"
	jjjtesting "github.com/juju/juju/juju/testing"
	// "github.com/juju/juju/state" // TODO will need this to assert value is set in model.
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

func (s *supportCommandSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	mockAPI, err := newMockAPI()
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = mockAPI

	s.PatchValue(support.NewAuthorizationClient, support.APIClientFnc(s.mockAPI))
}

func (s supportCommandSuite) TestSupportCommand(c *gc.C) {
	tests := []struct {
		about    string
		level    string
		budget   string
		err      string
		apiErr   error
		apiCalls []testing.StubCall
	}{{
		about: "all is well",
		level: "essential",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				"essential",
			},
		}},
	},
	// TODO Add test for budget args.
	}
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		s.mockAPI.ResetCalls()
		if test.apiErr != nil {
			s.mockAPI.SetErrors(test.apiErr)
		}
		_, err := cmdtesting.RunCommand(c, support.NewSupportCommand(), test.level)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 1)
			s.mockAPI.CheckCalls(c, test.apiCalls)

			/*
				// TODO Check model level and creds are set
				model, err := s.State.Model()
				c.Assert(err, jc.ErrorIsNil)
				svcMacaroon := app.MetricCredentials()
				data, err := json.Marshal(macaroon.Slice{s.mockAPI.macaroon})
				c.Assert(err, jc.ErrorIsNil)
				c.Assert(svcMacaroon, gc.DeepEquals, data)
			*/
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 0)
		}
	}
}

func (s *supportCommandSuite) TestNoArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, support.NewSupportCommand())
	c.Assert(err, gc.ErrorMatches, "need to specify suppot level")
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

func (m *mockapi) Authorize(modelUUID, supportLevel string) (*macaroon.Macaroon, error) {
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
