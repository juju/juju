// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/rpc/params"
)

const endpointSeparator = ":"

type AddRemoteRelationSuiteNewAPI struct {
	baseAddRemoteRelationSuite
}

var _ = tc.Suite(&AddRemoteRelationSuiteNewAPI{})

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationNoRemoteApplications(c *tc.C) {
	err := s.runAddRelation(c, "applicationname2", "applicationname")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"applicationname2", "applicationname"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationRemoteApplications(c *tc.C) {
	s.assertFailAddRelationTwoRemoteApplications(c)
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationToOneRemoteApplication(c *tc.C) {
	s.assertAddedRelation(c, "applicationname", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 0, "GetConsumeDetails", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 1, "Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetailsV5{
				OfferName: "hosted-mysql",
				OfferURL:  "arthur:bob/prod.hosted-mysql",
			},
			ApplicationAlias: "applicationname2",
			Macaroon:         s.mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerUUID: testing.ControllerTag.Id(),
				Addrs:          []string{"192.168.1.0"},
				Alias:          "arthur",
				CACert:         testing.CACert,
			},
		})
	s.mockAPI.CheckCall(c, 3, "AddRelation", []string{"applicationname", "applicationname2"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationAnyRemoteApplication(c *tc.C) {
	s.assertAddedRelation(c, "othermodel.applicationname2", "applicationname")
	s.mockAPI.CheckCall(c, 0, "GetConsumeDetails", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 1, "Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetailsV5{
				OfferName: "hosted-mysql",
				OfferURL:  "arthur:bob/prod.hosted-mysql",
			},
			ApplicationAlias: "applicationname2",
			Macaroon:         s.mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerUUID: testing.ControllerTag.Id(),
				Addrs:          []string{"192.168.1.0"},
				Alias:          "arthur",
				CACert:         testing.CACert,
			},
		})
	s.mockAPI.CheckCall(c, 3, "AddRelation", []string{"applicationname2", "applicationname"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationFailure(c *tc.C) {
	msg := "add relation failure"
	s.mockAPI.addRelation = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		return nil, errors.New(msg)
	}

	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname")
	c.Assert(err, tc.ErrorMatches, msg)
	s.mockAPI.CheckCallNames(c, "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationTerminated(c *tc.C) {
	msg := "remote offer applicationname is terminated"
	s.mockAPI.addRelation = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		return nil, errors.New(msg)
	}

	err := s.runAddRelation(c, "kontroll:bob/prod.hosted-mysql", "applicationname")
	c.Assert(err, tc.ErrorMatches, `
Offer "applicationname" has been removed from the remote model.
To integrate with a new offer with the same name, first run
'juju remove-saas applicationname' to remove the SAAS record from this model.`[1:])
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationDying(c *tc.C) {
	msg := "applicationname is not alive"
	s.mockAPI.addRelation = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		return nil, errors.New(msg)
	}

	err := s.runAddRelation(c, "applicationname2", "kontroll:bob/prod.hosted-mysql")
	c.Assert(err, tc.ErrorMatches, `
SAAS application "hosted-mysql" has been removed but termination has not completed.
To integrate with a new offer with the same name, first run
'juju remove-saas hosted-mysql --force' to remove the SAAS record from this model.`[1:])
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddedRelationVia(c *tc.C) {
	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname", "--via", "192.168.1.0/16, 10.0.0.0/16")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 3, "AddRelation",
		[]string{"applicationname2", "applicationname"}, []string{"192.168.1.0/16", "10.0.0.0/16"})
}

func (s *AddRemoteRelationSuiteNewAPI) assertAddedRelation(c *tc.C, args ...string) {
	err := s.runAddRelation(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
}

// AddRelationValidationSuite has input validation tests.
type AddRelationValidationSuite struct {
	baseAddRemoteRelationSuite
}

var _ = tc.Suite(&AddRelationValidationSuite{})

func (s *AddRelationValidationSuite) TestAddRelationInvalidEndpoint(c *tc.C) {
	s.assertInvalidEndpoint(c, "applicationname:inva#lid", `endpoint "applicationname:inva#lid" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationSeparatorFirst(c *tc.C) {
	s.assertInvalidEndpoint(c, ":applicationname", `endpoint ":applicationname" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationSeparatorLast(c *tc.C) {
	s.assertInvalidEndpoint(c, "applicationname:", `endpoint "applicationname:" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationMoreThanOneSeparator(c *tc.C) {
	s.assertInvalidEndpoint(c, "serv:ice:name", `endpoint "serv:ice:name" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationInvalidApplication(c *tc.C) {
	s.assertInvalidEndpoint(c, "applicat@ionname", `application name "applicat@ionname" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationInvalidEndpointApplication(c *tc.C) {
	s.assertInvalidEndpoint(c, "applicat@ionname:endpoint", `application name "applicat@ionname" not valid`)
}

func (s *AddRelationValidationSuite) assertInvalidEndpoint(c *tc.C, endpoint, msg string) {
	err := validateLocalEndpoint(endpoint, endpointSeparator)
	c.Assert(err, tc.ErrorMatches, msg)
}

// baseAddRemoteRelationSuite contains common functionality for integrate cmd tests
// that mock out api client.
type baseAddRemoteRelationSuite struct {
	testing.BaseSuite
	mockAPI *mockAddRelationAPI
	mac     *macaroon.Macaroon
}

func (s *baseAddRemoteRelationSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	var err error
	s.mac, err = jujutesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = &mockAddRelationAPI{
		addRelation: func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
			return nil, nil
		},
		mac: s.mac,
	}
}

func (s *baseAddRemoteRelationSuite) runAddRelation(c *tc.C, args ...string) error {
	cmd := NewAddRelationCommandForTest(s.mockAPI, s.mockAPI)
	cmd.SetClientStore(jujuclienttesting.MinimalStore())
	_, err := cmdtesting.RunCommand(c, cmd, args...)
	return err
}

func (s *baseAddRemoteRelationSuite) assertFailAddRelationTwoRemoteApplications(c *tc.C) {
	err := s.runAddRelation(c, "othermodel.applicationname1", "othermodel.applicationname2")
	c.Assert(err, tc.ErrorMatches, "providing more than one remote endpoints not supported")
}

// mockAddRelationAPI contains a stub api used for integrate cmd tests.
type mockAddRelationAPI struct {
	jtesting.Stub

	// addRelation can be defined by tests to test different integrate outcomes.
	addRelation func(endpoints, viaCidrs []string) (*params.AddRelationResults, error)

	mac *macaroon.Macaroon
}

func (m *mockAddRelationAPI) AddRelation(ctx context.Context, endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	m.AddCall("AddRelation", endpoints, viaCIDRs)
	return m.addRelation(endpoints, viaCIDRs)
}

func (m *mockAddRelationAPI) Close() error {
	m.AddCall("Close")
	return nil
}

func (m *mockAddRelationAPI) Consume(ctx context.Context, arg crossmodel.ConsumeApplicationArgs) (string, error) {
	m.AddCall("Consume", arg)
	return arg.ApplicationAlias, nil
}

func (m *mockAddRelationAPI) GetConsumeDetails(ctx context.Context, url string) (params.ConsumeOfferDetails, error) {
	m.AddCall("GetConsumeDetails", url)
	return params.ConsumeOfferDetails{
		Offer: &params.ApplicationOfferDetailsV5{
			OfferName: "hosted-mysql",
			OfferURL:  "bob/prod.hosted-mysql",
		},
		Macaroon: m.mac,
		ControllerInfo: &params.ExternalControllerInfo{
			ControllerTag: testing.ControllerTag.String(),
			Addrs:         []string{"192.168.1.0"},
			Alias:         "controller-alias",
			CACert:        testing.CACert,
		},
	}, nil
}
