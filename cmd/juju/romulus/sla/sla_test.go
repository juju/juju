// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sla_test

import (
	"encoding/json"
	stdtesting "testing"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/romulus/sla"
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

	s.PatchValue(sla.NewAuthorizationClient, sla.APIClientFnc(s.mockAPI))
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
				"",
			},
		}},
	}, {
		about: "invalid level",
		level: "invalid",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				"invalid",
				"",
			},
		}},
		err: `SLA level "invalid" not valid`,
	}, {
		about:  "all is well with budget",
		level:  "essential",
		budget: "personal:10",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				"essential",
				"personal:10",
			},
		}},
	}, {
		about: "invalid level",
		level: "invalid",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				"invalid",
				"",
			},
		}},
		err: `SLA level "invalid" not valid`,
	}, {
		about:  "all is well with budget",
		level:  "essential",
		budget: "personal:10",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.State.ModelUUID(),
				"essential",
				"personal:10",
			},
		}},
	}}
	for i, test := range tests {
		c.Logf("running test %d: %v", i, test.about)
		s.mockAPI.ResetCalls()
		if test.apiErr != nil {
			s.mockAPI.SetErrors(test.apiErr)
		}
		_, err := cmdtesting.RunCommand(c, sla.NewSLACommand(), test.level, "--budget", test.budget)
		if test.err == "" {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(s.mockAPI.Calls(), gc.HasLen, 1)
			s.mockAPI.CheckCalls(c, test.apiCalls)

			// TODO Check model level and creds are set
			model, err := s.State.Model()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(model.SLALevel(), gc.Equals, test.level)
			data, err := json.Marshal(macaroon.Slice{s.mockAPI.macaroon})
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(data, jc.DeepEquals, model.SLACredential())
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *supportCommandSuite) TestDiplayCurrentLevel(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, sla.NewSLACommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "unsupported\n")
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

func (m *mockapi) Authorize(modelUUID, supportLevel, budget string) (*macaroon.Macaroon, error) {
	err := m.NextErr()
	if err != nil {
		return nil, errors.Trace(err)
	}
	m.AddCall("Authorize", modelUUID, supportLevel, budget)
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
