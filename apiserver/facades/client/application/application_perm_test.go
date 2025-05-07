// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/rpc/params"
)

type permBaseSuite struct {
	baseSuite

	newAPI func(*tc.C)
}

func (s *permBaseSuite) TestAPIConstruction(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authorizer.EXPECT().AuthClient().Return(false)

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, s.modelUUID, "", nil, nil, nil, nil, nil, nil, nil, clock.WallClock)
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAPIServiceConstruction(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, s.modelUUID, "", nil, nil, nil, nil, nil, nil, nil, clock.WallClock)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *permBaseSuite) TestDeployPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Deploy(context.Background(), params.ApplicationsDeploy{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetCharmBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetCharm(context.Background(), params.ApplicationSetCharmV2{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmBlockIgnoredWithForceUnits(c *tc.C) {
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

func (s *permBaseSuite) TestSetCharmValidOrigin(c *tc.C) {
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

func (s *permBaseSuite) TestGetCharmURLOriginPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetCharmURLOrigin(context.Background(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmRelationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmRelations(context.Background(), params.ApplicationCharmRelations{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestExposePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Expose(context.Background(), params.ApplicationExpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnexposePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Unexpose(context.Background(), params.ApplicationUnexpose{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(context.Background(), params.DestroyApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(context.Background(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetConstraintsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConstraints(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetConstraints(context.Background(), params.SetConstraints{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestAddRelationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAddRelationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddRelation(context.Background(), params.AddRelation{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyRelationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyRelationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	err := s.api.DestroyRelation(context.Background(), params.DestroyRelation{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetRelationsSuspendedPermission(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetRelationsSuspendedBlocked(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(context.Background(), params.RelationSuspendedArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestConsumePermission(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestConsumeBlocked(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Consume(context.Background(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Get(context.Background(), params.ApplicationGet{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmConfig(context.Background(), params.ApplicationGetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestGetConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConfig(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetConfigs(context.Background(), params.ConfigSetArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnsetApplicationsConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnsetApplicationsConfigBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(context.Background(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestResolveUnitErrorsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestResolveUnitErrorsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(context.Background(), params.UnitsResolved{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestApplicationsInfoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ApplicationsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.MergeBindings(context.Background(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnitsInfoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnitsInfo(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestLeaderPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Leader(context.Background(), params.Entity{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(context.Background(), params.DeployFromRepositoryArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

type permSuiteIAAS struct {
	permBaseSuite
}

var _ = tc.Suite(&permSuiteIAAS{})

func (s *permSuiteIAAS) SetUpTest(c *tc.C) {
	s.permBaseSuite.newAPI = s.newIAASAPI
}

func (s *permSuiteIAAS) TestAddUnitsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestAddUnitsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestDestroyUnitPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestDestroyUnitBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestScaleApplicationsInvalidForIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

type permSuiteCAAS struct {
	permBaseSuite
}

var _ = tc.Suite(&permSuiteCAAS{})

func (s *permSuiteCAAS) SetUpTest(c *tc.C) {
	s.permBaseSuite.newAPI = s.newCAASAPI
}

func (s *permSuiteCAAS) TestAddUnitsInvalidForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.AddUnits(context.Background(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestDestroyUnitInvalidForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(context.Background(), params.DestroyUnitsParams{})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestScaleApplicationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteCAAS) TestScaleApplicationsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(context.Background(), params.ScaleApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}
