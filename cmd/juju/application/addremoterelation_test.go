// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"regexp"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/feature"
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
	s.mockAPI.version = 3
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationNoRemoteApplications(c *gc.C) {
	err := s.runAddRelation(c, "applicationname2", "applicationname")
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 0, "AddRelation", []string{"applicationname2", "applicationname"})
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationRemoteApplications(c *gc.C) {
	s.assertFailAddRelationTwoRemoteApplications(c)
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationToOneRemoteApplication(c *gc.C) {
	s.assertAddedRelation(c, "applicationname", "othermodel.applicationname2")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationAnyRemoteApplication(c *gc.C) {
	s.assertAddedRelation(c, "othermodel.applicationname2", "applicationname")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationFailure(c *gc.C) {
	msg := "add relation failure"
	s.mockAPI.addRelation = func(endpoints ...string) (*params.AddRelationResults, error) {
		return nil, errors.New(msg)
	}

	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname")
	c.Assert(err, gc.ErrorMatches, msg)
	s.mockAPI.CheckCallNames(c, "BestAPIVersion", "AddRelation", "Close")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationClientRetrievalFailure(c *gc.C) {
	msg := "where is my client"

	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (ApplicationAddRelationAPI, error) {
		return nil, errors.New(msg)
	}

	_, err := testing.RunCommand(c, modelcmd.Wrap(addRelationCmd), "othermodel.applicationname2", "applicationname")
	c.Assert(err, gc.ErrorMatches, msg)
}

func (s *AddRemoteRelationSuiteNewAPI) assertAddedRelation(c *gc.C, args ...string) {
	err := s.runAddRelation(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	s.mockAPI.CheckCallNames(c, "BestAPIVersion", "AddRelation", "Close")
	s.mockAPI.CheckCall(c, 1, "AddRelation", args)
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
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("cannot add relation between [applicationname othermodel.applicationname2]: remote endpoints not supported"))
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationAnyRemoteApplication(c *gc.C) {
	err := s.runAddRelation(c, "othermodel.applicationname2", "applicationname")
	c.Assert(err, gc.ErrorMatches, regexp.QuoteMeta("cannot add relation between [othermodel.applicationname2 applicationname]: remote endpoints not supported"))
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
}

func (s *baseAddRemoteRelationSuite) SetUpTest(c *gc.C) {
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.RepoSuite.SetUpTest(c)
	s.mockAPI = &mockAddRelationAPI{
		addRelation: func(endpoints ...string) (*params.AddRelationResults, error) {
			return nil, nil
		},
	}
}

func (s *baseAddRemoteRelationSuite) TearDownTest(c *gc.C) {
	s.RepoSuite.TearDownTest(c)
}

func (s *baseAddRemoteRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (ApplicationAddRelationAPI, error) {
		return s.mockAPI, nil
	}
	_, err := testing.RunCommand(c, modelcmd.Wrap(addRelationCmd), args...)
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
	addRelation func(endpoints ...string) (*params.AddRelationResults, error)

	// version can be overwritten by tests interested in different behaviour based on client version.
	version int
}

func (m *mockAddRelationAPI) AddRelation(endpoints ...string) (*params.AddRelationResults, error) {
	m.AddCall("AddRelation", endpoints)
	return m.addRelation(endpoints...)
}

func (m *mockAddRelationAPI) Close() error {
	m.AddCall("Close")
	return nil
}

func (m *mockAddRelationAPI) BestAPIVersion() int {
	m.AddCall("BestAPIVersion")
	return m.version
}
