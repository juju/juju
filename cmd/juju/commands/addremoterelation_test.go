// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/model/crossmodel"
	"github.com/juju/juju/testing"
)

type AddRemoteRelationSuiteNewAPI struct {
	baseAddRemoteRelationSuite
}

func (s *AddRemoteRelationSuiteNewAPI) SetUpTest(c *gc.C) {
	s.baseAddRemoteRelationSuite.SetUpTest(c)
	s.mockAPI.version = 2
}

var _ = gc.Suite(&AddRemoteRelationSuiteNewAPI{})

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationNoRemoteServices(c *gc.C) {
	s.assertAddedRelation(c, "servicename2", "servicename")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationRemoteServices(c *gc.C) {
	s.assertAddedRelation(c, "local:/u/user/servicename1", "local:/u/user/servicename2")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationToOneRemoteService(c *gc.C) {
	s.assertAddedRelation(c, "servicename", "local:/u/user/servicename2")
}

func (s *AddRemoteRelationSuiteNewAPI) TestAddRelationAnyRemoteService(c *gc.C) {
	s.assertAddedRelation(c, "local:/u/user/servicename2", "servicename")
}

// AddRemoteRelationSuiteOldAPI only needs to check that we have fallen through to the old api
// since best facade version is 0...
// This old api is tested in another suite.
type AddRemoteRelationSuiteOldAPI struct {
	baseAddRemoteRelationSuite
}

var _ = gc.Suite(&AddRemoteRelationSuiteOldAPI{})

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationNoRemoteServices(c *gc.C) {
	err := s.runAddRelation(c, "servicename2", "servicename")
	c.Assert(err, gc.ErrorMatches, ".*not found.*")
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationRemoteServices(c *gc.C) {
	err := s.runAddRelation(c, "local:/u/user/servicename1", "local:/u/user/servicename2")
	c.Assert(err, gc.ErrorMatches, ".*remote services not supported.*")
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationToOneRemoteService(c *gc.C) {
	err := s.runAddRelation(c, "servicename", "local:/u/user/servicename2")
	c.Assert(err, gc.ErrorMatches, ".*remote services not supported.*")
}

func (s *AddRemoteRelationSuiteOldAPI) TestAddRelationAnyRemoteService(c *gc.C) {
	err := s.runAddRelation(c, "local:/u/user/servicename2", "servicename")
	c.Assert(err, gc.ErrorMatches, ".*remote services not supported.*")
}

type baseAddRemoteRelationSuite struct {
	jujutesting.RepoSuite

	mockAPI *mockAddRelationAPI

	endpoints []string
}

func (s *baseAddRemoteRelationSuite) TearDownTest(c *gc.C) {
	s.RepoSuite.TearDownTest(c)
}

func (s *baseAddRemoteRelationSuite) SetUpTest(c *gc.C) {
	s.RepoSuite.SetUpTest(c)

	s.endpoints = []string{}
	s.mockAPI = &mockAddRelationAPI{
		addRelation: func(endpoints ...string) (crossmodel.AddRelationResults, error) {
			s.endpoints = endpoints
			return crossmodel.AddRelationResults{}, nil
		},
	}
}

func (s *baseAddRemoteRelationSuite) assertAddedRelation(c *gc.C, args ...string) {
	err := s.runAddRelation(c, args...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.endpoints, gc.DeepEquals, args)
}

func (s *baseAddRemoteRelationSuite) runAddRelation(c *gc.C, args ...string) error {
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (AddRelationAPI, error) {
		return s.mockAPI, nil
	}
	_, err := testing.RunCommand(c, envcmd.Wrap(addRelationCmd), args...)
	return err
}

type mockAddRelationAPI struct {
	addRelation func(endpoints ...string) (crossmodel.AddRelationResults, error)
	version     int
}

func (m *mockAddRelationAPI) AddRelation(endpoints ...string) (crossmodel.AddRelationResults, error) {
	return m.addRelation(endpoints...)
}

func (m *mockAddRelationAPI) Close() error {
	return nil
}

func (m *mockAddRelationAPI) BestAPIVersion() int {
	return m.version
}
