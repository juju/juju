// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sla_test

import (
	stdtesting "testing"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/cmd/juju/romulus/sla"
	jujutesting "github.com/juju/testing"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&supportCommandSuite{})

type supportCommandSuite struct {
	jujutesting.CleanupSuite
	mockAPI       *mockapi
	mockSLAClient *mockSlaClient
	charmURL      string
	modelUUID     string
}

func (s *supportCommandSuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	mockAPI, err := newMockAPI()
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = mockAPI
	s.mockSLAClient = &mockSlaClient{}
	s.modelUUID = utils.MustNewUUID().String()

	s.PatchValue(sla.NewAuthorizationClient, sla.APIClientFnc(s.mockAPI))
	s.PatchValue(sla.NewSLAClient, sla.SLAClientFnc(s.mockSLAClient))
	s.PatchValue(sla.ModelId, sla.ModelIdFnc(s.modelUUID))
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
				s.modelUUID,
				"essential",
				"",
			},
		}},
	}, {
		about:  "all is well with budget",
		level:  "essential",
		budget: "personal:10",
		apiCalls: []testing.StubCall{{
			FuncName: "Authorize",
			Args: []interface{}{
				s.modelUUID,
				"essential",
				"personal:10",
			},
		}},
	},
	}
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
		} else {
			c.Assert(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *supportCommandSuite) TestDiplayCurrentLevel(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, sla.NewSLACommand())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "mock-level\n")
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

type mockSlaClient struct {
	testing.Stub
}

func (m *mockSlaClient) SetSLALevel(level string, creds []byte) error {
	m.AddCall("SetSLALevel", level, creds)
	return nil
}
func (m *mockSlaClient) SLALevel() (string, error) {
	m.AddCall("SLALevel")
	return "mock-level", nil
}
