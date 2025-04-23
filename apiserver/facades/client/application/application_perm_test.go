// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/rpc/params"
)

type permBaseSuite struct {
	baseSuite

	newAPI func(*gc.C)
}

func (s *permBaseSuite) TestAPIConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, s.modelUUID, "", nil, nil, nil, nil, nil, nil, nil, nil, clock.WallClock)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAPIServiceConstruction(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, s.modelUUID, "", nil, nil, nil, nil, nil, nil, nil, nil, clock.WallClock)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *permBaseSuite) TestDeployPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetCharmBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmBlockIgnoredWithForceUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()

	// The check is not even called if force units is set.

	s.newAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{
		ForceUnits: true,
	})

	// The validation error is returned if the charm origin is empty.

	c.Assert(err, jc.ErrorIs, errors.BadRequest)
}

func (s *permBaseSuite) TestSetCharmValidOrigin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectAllowBlockChange()

	s.backend.EXPECT().Application("foo").Return(nil, errors.NotFound)

	s.newAPI(c)

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

func (s *permBaseSuite) TestGetCharmURLOriginPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetCharmURLOrigin(context.Background(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmRelationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmRelations(context.Background(), params.ApplicationCharmRelations{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestExposePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Expose(context.Background(), params.ApplicationExpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnexposePermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Unexpose(context.Background(), params.ApplicationUnexpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetConstraintsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConstraints(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestAddRelationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAddRelationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyRelationPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyRelationBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetRelationsSuspendedPermission(c *gc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetRelationsSuspendedBlocked(c *gc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestConsumePermission(c *gc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestConsumeBlocked(c *gc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Get(context.Background(), params.ApplicationGet{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestGetConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConfig(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnsetApplicationsConfigPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnsetApplicationsConfigBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestResolveUnitErrorsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestResolveUnitErrorsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestApplicationsInfoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ApplicationsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnitsInfoPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnitsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestLeaderPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Leader(context.Background(), params.Entity{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

type permSuiteIAAS struct {
	permBaseSuite
}

var _ = gc.Suite(&permSuiteIAAS{})

func (s *permSuiteIAAS) SetUpTest(c *gc.C) {
	s.permBaseSuite.newAPI = s.newIAASAPI
}

func (s *permSuiteIAAS) TestAddUnitsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestAddUnitsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestDestroyUnitPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestDestroyUnitBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestScaleApplicationsInvalidForIAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

type permSuiteCAAS struct {
	permBaseSuite
}

var _ = gc.Suite(&permSuiteCAAS{})

func (s *permSuiteCAAS) SetUpTest(c *gc.C) {
	s.permBaseSuite.newAPI = s.newCAASAPI
}

func (s *permSuiteCAAS) TestAddUnitsInvalidForCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestDestroyUnitInvalidForCAAS(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestScaleApplicationsPermission(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteCAAS) TestScaleApplicationsBlocked(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, gc.ErrorMatches, "blocked")
}
