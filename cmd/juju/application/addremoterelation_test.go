// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/crossmodel"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"
)

const endpointSeparator = ":"

type AddRemoteRelationSuiteNewAPI struct {
	baseAddRemoteRelationSuite
}

var _ = gc.Suite(&AddRemoteRelationSuiteNewAPI{})

func (s *AddRemoteRelationSuiteNewAPI) SetUpTest(c *gc.C) {
	s.baseAddRemoteRelationSuite.SetUpTest(c)
	s.mockAPI.version = 5
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationNoRemoteApplications(c *gc.C) {
	err := s.runAddRelation(c, "applicationname2", "applicationname")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"applicationname2", "applicationname"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationRemoteApplications(c *gc.C) {
	s.assertFailAddRelationTwoRemoteApplications(c)
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationToOneRemoteApplication(c *gc.C) {
	s.assertAddedRelation(c, "applicationname", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 1, "GetConsumeDetails", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 2, "Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetails{
				OfferName: "hosted-mysql",
				OfferURL:  "kontroll:bob/prod.hosted-mysql",
			},
			ApplicationAlias: "applicationname2",
			Macaroon:         s.mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerTag: testing.ControllerTag,
				Addrs:         []string{"192.168.1.0"},
				Alias:         "kontroll",
				CACert:        testing.CACert,
			},
		})
	s.mockAPI.CheckCall(c, 4, "AddRelation", []string{"applicationname", "applicationname2"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationAnyRemoteApplication(c *gc.C) {
	s.assertAddedRelation(c, "othermodel.applicationname2", "applicationname")
	s.mockAPI.CheckCall(c, 1, "GetConsumeDetails", "othermodel.applicationname2")
	s.mockAPI.CheckCall(c, 2, "Consume",
		crossmodel.ConsumeApplicationArgs{
			Offer: params.ApplicationOfferDetails{
				OfferName: "hosted-mysql",
				OfferURL:  "kontroll:bob/prod.hosted-mysql",
			},
			ApplicationAlias: "applicationname2",
			Macaroon:         s.mac,
			ControllerInfo: &crossmodel.ControllerInfo{
				ControllerTag: testing.ControllerTag,
				Addrs:         []string{"192.168.1.0"},
				Alias:         "kontroll",
				CACert:        testing.CACert,
			},
		})
	s.mockAPI.CheckCall(c, 4, "AddRelation", []string{"applicationname2", "applicationname"}, []string(nil))
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationFailure(c *gc.C) {
	msg := "add relation failure"
	s.mockAPI.addRelation = func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
		return nil, errors.New(msg)
	}

	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCallNames(c, "BestAPIVersion", "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddedRelationVia(c *gc.C) {
	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname", "--via", "192.168.1.0/16, 10.0.0.0/16")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "BestAPIVersion", "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 4, "AddRelation",
		[]string{"applicationname2", "applicationname"}, []string{"192.168.1.0/16", "10.0.0.0/16"})
}

func (s *AddRemoteRelationSuiteNewAPI) assertAddedRelation(c *gc.C, args ...string) {
	err := s.runAddRelation(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "BestAPIVersion", "GetConsumeDetails", "Consume", "Close", "AddRelation", "Close")
}

// AddRemoteRelationSuiteOldAPI only needs to check that we have fallen through to the old api
// since best facade version is 0...
// This old api is tested in another suite.
type AddRemoteRelationSuiteOldAPI struct {
	baseAddRemoteRelationSuite
}

var _ = gc.Suite(&AddRemoteRelationSuiteOldAPI{})

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationRemoteApplications(c *gc.C) {
	s.assertFailAddRelationTwoRemoteApplications(c)
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationToOneRemoteApplication(c *gc.C) {
	err := s.runAddRelation(c, "applicationname", "othermodel.applicationname2")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("cannot add relation to othermodel.applicationname2: remote endpoints not supported"))
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationNoRemoteApplicationsVia(c *gc.C) {
	err := s.runAddRelation(c, "applicationname", "applicationname2", "--via", "192.168.0.0/16")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("the --via option can only be used when relating to offers in a different model"))
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationViaBadCidr(c *gc.C) {
	err := s.runAddRelation(c, "applicationname", "othermodel.applicationname2", "--via", "bad.cidr")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`invalid CIDR address: bad.cidr`))
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationViaDisallowedCidr(c *gc.C) {
	err := s.runAddRelation(c, "applicationname", "othermodel.applicationname2", "--via", "0.0.0.0/0")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta(`CIDR "0.0.0.0/0" not allowed`))
}

// AddRelationValidationSuite has input validation tests.
type AddRelationValidationSuite struct {
	baseAddRemoteRelationSuite
}

var _ = gc.Suite(&AddRelationValidationSuite{})

func (s *AddRelationValidationSuite) TestAddRelationInvalidEndpoint(c *gc.C) {
	s.assertInvalidEndpoint(c, "applicationname:inva#lid", `endpoint "applicationname:inva#lid" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationSeparatorFirst(c *gc.C) {
	s.assertInvalidEndpoint(c, ":applicationname", `endpoint ":applicationname" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationSeparatorLast(c *gc.C) {
	s.assertInvalidEndpoint(c, "applicationname:", `endpoint "applicationname:" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationMoreThanOneSeparator(c *gc.C) {
	s.assertInvalidEndpoint(c, "serv:ice:name", `endpoint "serv:ice:name" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationInvalidApplication(c *gc.C) {
	s.assertInvalidEndpoint(c, "applicat@ionname", `application name "applicat@ionname" not valid`)
}

func (s *AddRelationValidationSuite) TestAddRelationInvalidEndpointApplication(c *gc.C) {
	s.assertInvalidEndpoint(c, "applicat@ionname:endpoint", `application name "applicat@ionname" not valid`)
}

func (s *AddRelationValidationSuite) assertInvalidEndpoint(c *gc.C, endpoint, msg string) {
	err := validateLocalEndpoint(endpoint, endpointSeparator)
	c.Assert(err, gc.ErrorMatches, msg)
}

// baseAddRemoteRelationSuite contains common functionality for add-relation cmd tests
// that mock out api client.
type baseAddRemoteRelationSuite struct {
	jujutesting.RepoSuite

	mockAPI *mockAddRelationAPI
	mac     *macaroon.Macaroon
}

func (s *baseAddRemoteRelationSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)
	var err error
	s.mac, err = apitesting.NewMacaroon("id")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI = &mockAddRelationAPI{
		addRelation: func(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
			return nil, nil
		},
		mac: s.mac,
	}
}

func (s *baseAddRemoteRelationSuite) TearDownTest(c *gc.C) {
	s.RepoSuite.TearDownTest(c)
}

func (s *baseAddRemoteRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.addRelationAPI = s.mockAPI
	addRelationCmd.consumeDetailsAPI = s.mockAPI
	_, err := cmdtesting.RunCommand(c, modelcmd.Wrap(addRelationCmd), args...)
	return err
}

func (s *baseAddRemoteRelationSuite) assertFailAddRelationTwoRemoteApplications(c *gc.C) {
	err := s.runAddRelation(c, "othermodel.applicationname1", "othermodel.applicationname2")
	c.Assert(err, gc.ErrorMatches, "providing more than one remote endpoints not supported")
}

// mockAddRelationAPI contains a stub api used for add-relation cmd tests.
type mockAddRelationAPI struct {
	jtesting.Stub

	// addRelation can be defined by tests to test different add-relation outcomes.
	addRelation func(endpoints, viaCidrs []string) (*params.AddRelationResults, error)

	// version can be overwritten by tests interested in different behaviour based on client version.
	version int

	mac *macaroon.Macaroon
}

func (m *mockAddRelationAPI) AddRelation(endpoints, viaCIDRs []string) (*params.AddRelationResults, error) {
	m.AddCall("AddRelation", endpoints, viaCIDRs)
	return m.addRelation(endpoints, viaCIDRs)
}

func (m *mockAddRelationAPI) Close() error {
	m.AddCall("Close")
	return nil
}

func (m *mockAddRelationAPI) BestAPIVersion() int {
	m.AddCall("BestAPIVersion")
	return m.version
}

func (m *mockAddRelationAPI) Consume(arg crossmodel.ConsumeApplicationArgs) (string, error) {
	m.AddCall("Consume", arg)
	return arg.ApplicationAlias, nil
}

func (m *mockAddRelationAPI) GetConsumeDetails(url string) (params.ConsumeOfferDetails, error) {
	m.AddCall("GetConsumeDetails", url)
	return params.ConsumeOfferDetails{
		Offer: &params.ApplicationOfferDetails{
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
