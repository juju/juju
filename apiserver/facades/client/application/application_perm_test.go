// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/rpc/params"
)

type permSuite struct {
	baseSuite

	apiFn func(*gc.C)
}

func (s *permSuite) TestAPIConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, nil, s.modelInfo, nil, nil, nil, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestAPIServiceConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, nil, s.modelInfo, nil, nil, nil, nil, nil, nil, nil, nil)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *permSuite) TestDeployPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDeployBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestSetCharmPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetCharmBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestSetCharmBlockIgnoredWithForceUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)

	// The check is not even called if force units is set.

	s.apiFn(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ForceUnits: true,
	})

	// The validation error is returned if the charm origin is empty.

	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *permSuite) TestSetCharmValidOrigin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectAllowBlockChange(c)

	s.backend.EXPECT().Application("foo").Return(nil, errors.NotFound)

	s.apiFn(c)

	// Ensure that a valid origin is required before setting a charm.
	// There will be tests from ensuring correctness of the origin.

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmOrigin: &params.CharmOrigin{
			Source: "local",
		},
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *permSuite) TestGetCharmURLOriginPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.GetCharmURLOrigin(context.Background(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestCharmRelationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.CharmRelations(context.Background(), params.ApplicationCharmRelations{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestExposePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	err := s.api.Expose(context.Background(), params.ApplicationExpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestUnexposePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	err := s.api.Unexpose(context.Background(), params.ApplicationUnexpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDestroyApplicationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDestroyApplicationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockRemoval(c)

	s.apiFn(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestDestroyConsumedApplicationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDestroyConsumedApplicationsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockRemoval(c)

	s.apiFn(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestGetConstraintsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.GetConstraints(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetConstraintsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetConstraintsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestAddRelationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestAddRelationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestDestroyRelationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDestroyRelationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockRemoval(c)

	s.apiFn(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestSetRelationsSuspendedPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetRelationsSuspendedBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestConsumePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestConsumeBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestGetPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.Get(context.Background(), params.ApplicationGet{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestCharmConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestGetConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.GetConfig(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetConfigsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestSetConfigsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestUnsetApplicationsConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestUnsetApplicationsConfigBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestResolveUnitErrorsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestResolveUnitErrorsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestApplicationsInfoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.ApplicationsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestMergeBindingsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestMergeBindingsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuite) TestUnitsInfoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.UnitsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestLeaderPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.Leader(context.Background(), params.Entity{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDeployFromRepositoryPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuite) TestDeployFromRepositoryBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

type permSuiteIAAS struct {
	permSuite
}

var _ = gc.Suite(&permSuiteIAAS{})

func (s *permSuiteIAAS) SetUpTest(c *gc.C) {
	s.permSuite.apiFn = s.newIAASAPI
}

func (s *permSuiteIAAS) TestAddUnitsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestAddUnitsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestDestroyUnitPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestDestroyUnitBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockRemoval(c)

	s.apiFn(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestScaleApplicationsInvalidForIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)

	s.apiFn(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

type permSuiteCAAS struct {
	permSuite
}

var _ = gc.Suite(&permSuiteCAAS{})

func (s *permSuiteCAAS) SetUpTest(c *gc.C) {
	s.permSuite.apiFn = s.newCAASAPI
}

func (s *permSuiteCAAS) TestAddUnitsInvalidForCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)

	s.apiFn(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestDestroyUnitInvalidForCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)

	s.apiFn(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestScaleApplicationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasIncorrectPermission(c)

	s.apiFn(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteCAAS) TestScaleApplicationsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient(c)
	s.expectHasWritePermission(c)
	s.expectDisallowBlockChange(c)

	s.apiFn(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}
