// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	applicationservice "github.com/juju/juju/domain/application/service"
	machineservice "github.com/juju/juju/domain/machine/service"
	portservice "github.com/juju/juju/domain/port/service"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/rpc/params"
)

type uniterLegacySuite struct {
	uniterSuiteBase
	domainServices     services.DomainServices
	machineService     *machineservice.WatchableService
	applicationService *applicationservice.WatchableService
	portService        *portservice.WatchableService
}

func TestUniterLegacySuite(t *testing.T) {
	tc.Run(t, &uniterLegacySuite{})
}

func (s *uniterLegacySuite) SetUpSuite(c *tc.C) {
	c.Skip("Skip factory-based uniter tests. TODO: Re-write without factories")
}

func (s *uniterLegacySuite) SetUpTest(c *tc.C) {
	s.uniterSuiteBase.SetUpTest(c)
	s.domainServices = s.ControllerDomainServices(c)

	s.machineService = s.domainServices.Machine()
	s.applicationService = s.domainServices.Application()
	s.portService = s.domainServices.Port()
}

func (s *uniterLegacySuite) TestUniterFailsWithNonUnitAgentUser(c *tc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	ctx := s.facadeContext(c)
	ctx.Auth_ = anAuthorizer
	_, err := uniter.NewUniterAPI(c.Context(), ctx)
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *uniterLegacySuite) TestLife(c *tc.C) {
	// Add a relation wordpress-mysql.
	// Make the wordpressUnit dead.
	// Add another unit, so the service will stay dying when we destroy it later.
	// Make the wordpress application dying.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "application-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
		// {Tag: rel.Tag().String()},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
		// {Tag: "relation-blah"},
	}}
	result, err := s.uniter.Life(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// TODO(dfc) see above
			// {Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterLegacySuite) TestEnsureDead(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPIv19(c, s.authorizer)
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
	}}
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("2", nil)
	result, err := uniterAPI.Watch(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "2"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterLegacySuite) TestWatchNoArgsNoError(c *tc.C) {
	uniterAPI := s.newUniterAPIv19(c, s.authorizer)
	result, err := uniterAPI.Watch(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *uniterLegacySuite) TestApplicationWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	args := params.Entity{Tag: "application-wordpress"}
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchApplication(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchApplicationBadTag(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	result, err := uniterAPI.WatchApplication(c.Context(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchApplicationNoPermission(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	// Permissions for mysql will be denied by the accessApplication function
	// defined in test set up.
	result, err := uniterAPI.WatchApplication(c.Context(), params.Entity{Tag: "application-mysql"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestUnitWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	// Recreate the uniter API with the mocks initialized.
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	args := params.Entity{Tag: "unit-wordpress-0"}
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("1", nil)
	result, err := uniterAPI.WatchUnit(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "1",
	})
}

func (s *uniterLegacySuite) TestWatchUnitBadTag(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	result, err := uniterAPI.WatchUnit(c.Context(), params.Entity{Tag: "bad-tag"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestWatchUnitNoPermission(c *tc.C) {
	uniterAPI := s.newUniterAPI(c, s.authorizer)
	// Permissions for mysql will be denied by the accessUnit function
	// defined in test set up.
	result, err := uniterAPI.WatchUnit(c.Context(), params.Entity{Tag: "unit-mysql-0"})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{Error: &params.Error{
		Code:    params.CodeUnauthorized,
		Message: "permission denied",
	}})
}

func (s *uniterLegacySuite) TestPublicAddress(c *tc.C) {
}

func (s *uniterLegacySuite) TestPrivateAddress(c *tc.C) {
}

func (s *uniterLegacySuite) TestResolvedAPIV6(c *tc.C) {
}

func (s *uniterLegacySuite) TestClearResolved(c *tc.C) {
}

func (s *uniterLegacySuite) TestGetPrincipal(c *tc.C) {
	// Add a subordinate to wordpressUnit.
	// First try it as wordpressUnit's agent.
	// Now try as subordinate's agent.
}

func (s *uniterLegacySuite) TestDestroy(c *tc.C) {
	// Verify wordpressUnit is destroyed and removed.
}

func (s *uniterLegacySuite) TestDestroyAllSubordinates(c *tc.C) {
	// Add two subordinates to wordpressUnit.
	// Verify wordpressUnit's subordinates were destroyed.
}

func (s *uniterLegacySuite) TestWorkloadVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestSetWorkloadVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestCharmModifiedVersion(c *tc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessage(c *tc.C) {
}

func (s *uniterLegacySuite) TestLogActionMessageAborting(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotifications(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchPreexistingActions(c *tc.C) {
}

func (s *uniterLegacySuite) TestWatchActionNotificationsPermissionDenied(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.NotNil)
	c.Assert(len(results.Results), tc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, tc.NotNil)
	c.Assert(result.Error.Message, tc.Equals, "permission denied")
}

func (s *uniterLegacySuite) TestCurrentModel(c *tc.C) {
	result, err := s.uniter.CurrentModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	expected := params.ModelResult{
		Name: modelInfo.Name,
		UUID: modelInfo.UUID.String(),
		Type: "iaas",
	}
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *uniterLegacySuite) TestProviderType(c *tc.C) {
	modelInfo, err := s.ControllerDomainServices(c).ModelInfo().GetModelInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	result, err := s.uniter.ProviderType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResult{Result: modelInfo.CloudType})
}

func (s *uniterLegacySuite) TestWatchRelationUnits(c *tc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.

	// UnitSettings versions are volatile, so we don't check them.
	// We just make sure the keys of the Changed field are as
	// expected.

	// Verify the resource was registered and stop when done

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)

	// Leave scope with mysqlUnit and check it's detected.

	// TODO(jam): 2019-10-21 this test is getting a bit unweildy, but maybe we
	//  should test that changing application data triggers a change here
}

func (s *uniterLegacySuite) TestWatchUnitAddressesHash(c *tc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-wordpress"},
	}}
	result, err := s.uniter.WatchUnitAddressesHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// The unit's machine has no network addresses
				// so the expected hash only contains the
				// sorted endpoint to space ID bindings for the
				// wordpress application.
				Changes: []string{"6048d9d417c851eddf006fa5b5435549313ee3046cf45a8223f47244d8c73e03"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	resource, err := s.watcherRegistry.Get("1")
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := watchertest.NewStringsWatcherC(c, resource.(watcher.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterLegacySuite) TestStorageAttachments(c *tc.C) {
	// We need to set up a unit that has storage metadata defined.
}

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelation(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR preferring private addresses. ")
}

func (s *uniterLegacySuite) TestPrivateAddressWithRemoteRelationNoPublic(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR when unit does not have " +
		"a public addresses. ")
}

func (s *uniterLegacySuite) TestRelationEgressSubnets(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Use NewRelationEgressNetworks " +
		"to set different egress networks from the model config. ")
}

func (s *uniterLegacySuite) TestModelEgressSubnets(c *tc.C) {
	c.Skip("Reimplement with CMR domain work, JUJU-4855\n" +
		"This test asserts that a relation unit's settings include: " +
		"private-address, ingress-address, and egress-subnets keywords " +
		"when the relation is in scope and CMR. Egress networks are set " +
		"via model config.")
}

func (s *uniterLegacySuite) TestRefresh(c *tc.C) {
}

func (s *uniterLegacySuite) TestRefreshNoArgs(c *tc.C) {
	results, err := s.uniter.Refresh(c.Context(), params.Entities{Entities: []params.Entity{}})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UnitRefreshResults{Results: []params.UnitRefreshResult{}})
}

func (s *uniterLegacySuite) TestCommitHookChangesWithSecrets(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")
	// See commitHookChangesSuite
}

func (s *uniterLegacySuite) TestCommitHookChangesWithStorage(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite once other hook commit concerns are in Dqlite")

	// Test-suite uses an older API version. Create a new one and override
	// authorizer to allow access to the unit we just created.

	// Verify state
}

func (s *uniterLegacySuite) TestCommitHookChangesWithPortsSidecarApplication(c *tc.C) {
	c.Skip("Rewrite this in the commitHookChangesSuite other hook commit concerns are in Dqlite")
}

func (s *uniterNetworkInfoSuite) TestCommitHookChangesCAAS(c *tc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelRelation(c *tc.C) {
}

func (s *uniterLegacySuite) TestNetworkInfoCAASModelNoRelation(c *tc.C) {
}

func (s *uniterLegacySuite) TestGetCloudSpecDeniesAccessWhenNotTrusted(c *tc.C) {
	result, err := s.uniter.CloudSpec(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.CloudSpecResult{Error: apiservertesting.ErrUnauthorized})
}
