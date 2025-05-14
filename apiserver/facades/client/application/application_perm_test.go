// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/clock"
	"github.com/juju/tc"

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
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAPIServiceConstruction(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	_, err := NewAPIBase(nil, Services{}, nil, s.authorizer, nil, s.modelUUID, "", nil, nil, nil, nil, nil, nil, nil, clock.WallClock)
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *permBaseSuite) TestDeployPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Deploy(c.Context(), params.ApplicationsDeploy{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetCharm(c.Context(), params.ApplicationSetCharmV2{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetCharmBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetCharm(c.Context(), params.ApplicationSetCharmV2{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetCharmBlockIgnoredWithForceUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()

	// The check is not even called if force units is set.

	s.newAPI(c)

	err := s.api.SetCharm(c.Context(), params.ApplicationSetCharmV2{
		ForceUnits: true,
	})

	// The validation error is returned if the charm origin is empty.

	c.Assert(err, tc.ErrorIs, errors.BadRequest)
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

	err := s.api.SetCharm(c.Context(), params.ApplicationSetCharmV2{
		ApplicationName: "foo",
		CharmOrigin: &params.CharmOrigin{
			Source: "local",
		},
	})

	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *permBaseSuite) TestGetCharmURLOriginPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetCharmURLOrigin(c.Context(), params.ApplicationGet{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmRelationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmRelations(c.Context(), params.ApplicationCharmRelations{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestExposePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Expose(c.Context(), params.ApplicationExpose{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnexposePermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.Unexpose(c.Context(), params.ApplicationUnexpose{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyApplicationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyApplication(c.Context(), params.DestroyApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(c.Context(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyConsumedApplicationsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyConsumedApplications(c.Context(), params.DestroyConsumedApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetConstraintsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConstraints(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.SetConstraints(c.Context(), params.SetConstraints{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConstraintsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	err := s.api.SetConstraints(c.Context(), params.SetConstraints{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestAddRelationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.AddRelation(c.Context(), params.AddRelation{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestAddRelationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddRelation(c.Context(), params.AddRelation{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestDestroyRelationPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	err := s.api.DestroyRelation(c.Context(), params.DestroyRelation{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDestroyRelationBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	err := s.api.DestroyRelation(c.Context(), params.DestroyRelation{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestSetRelationsSuspendedPermission(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(c.Context(), params.RelationSuspendedArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetRelationsSuspendedBlocked(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetRelationsSuspended(c.Context(), params.RelationSuspendedArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestConsumePermission(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Consume(c.Context(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestConsumeBlocked(c *tc.C) {
	c.Skip("cross model relations are disabled until backend functionality is moved to domain")
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.Consume(c.Context(), params.ConsumeApplicationArgsV5{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestGetPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Get(c.Context(), params.ApplicationGet{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestCharmConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.CharmConfig(c.Context(), params.ApplicationGetArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestGetConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.GetConfig(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestSetConfigsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.SetConfigs(c.Context(), params.ConfigSetArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnsetApplicationsConfigPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(c.Context(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestUnsetApplicationsConfigBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.UnsetApplicationsConfig(c.Context(), params.ApplicationConfigUnsetArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestResolveUnitErrorsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestResolveUnitErrorsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ResolveUnitErrors(c.Context(), params.UnitsResolved{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestApplicationsInfoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ApplicationsInfo(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.MergeBindings(c.Context(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestMergeBindingsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.MergeBindings(c.Context(), params.ApplicationMergeBindingsArgs{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permBaseSuite) TestUnitsInfoPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.UnitsInfo(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestLeaderPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.Leader(c.Context(), params.Entity{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(c.Context(), params.DeployFromRepositoryArgs{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permBaseSuite) TestDeployFromRepositoryBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.DeployFromRepository(c.Context(), params.DeployFromRepositoryArgs{})
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

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestAddUnitsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestDestroyUnitPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteIAAS) TestDestroyUnitBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockRemoval()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}

func (s *permSuiteIAAS) TestScaleApplicationsInvalidForIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(c.Context(), params.ScaleApplicationsParams{})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
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

	_, err := s.api.AddUnits(c.Context(), params.AddApplicationUnits{
		ApplicationName: "foo",
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestDestroyUnitInvalidForCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()

	s.newAPI(c)

	_, err := s.api.DestroyUnit(c.Context(), params.DestroyUnitsParams{})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *permSuiteCAAS) TestScaleApplicationsPermission(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasIncorrectPermission()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(c.Context(), params.ScaleApplicationsParams{})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *permSuiteCAAS) TestScaleApplicationsBlocked(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAuthClient()
	s.expectHasWritePermission()
	s.expectDisallowBlockChange()

	s.newAPI(c)

	_, err := s.api.ScaleApplications(c.Context(), params.ScaleApplicationsParams{})
	c.Assert(err, tc.ErrorMatches, "blocked")
}
