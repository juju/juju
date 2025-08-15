// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/collections/transform"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	coremachinetesting "github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/application/architecture"
	domaincharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/relation"
	relationerrors "github.com/juju/juju/domain/relation/errors"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/resolve"
	resolveerrors "github.com/juju/juju/domain/resolve/errors"
	"github.com/juju/juju/internal/charm"
	internalerrors "github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type uniterSuite struct {
	testhelpers.IsolationSuite

	badTag names.Tag

	applicationService *MockApplicationService
	machineService     *MockMachineService
	networkService     *MockNetworkService
	resolveService     *MockResolveService
	removalService     *MockRemovalService
	watcherRegistry    *MockWatcherRegistry

	uniter *UniterAPI
}

func TestUniterSuite(t *testing.T) {
	tc.Run(t, &uniterSuite{})
}

func (s *uniterSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.badTag = nil
}

func (s *uniterSuite) TestEnsureDeadUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.badTag = names.NewUnitTag("foo/0")

	// Act
	res, err := s.uniter.EnsureDead(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: s.badTag.String(),
			},
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestEnsureDead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().MarkUnitAsDead(gomock.Any(), unitUUID).Return(nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, time.Duration(0)).Return("", nil)

	// Act
	res, err := s.uniter.EnsureDead(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewUnitTag(unitName.String()).String(),
			},
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *uniterSuite) TestEnsureDeadNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.applicationService.EXPECT().GetUnitUUID(c.Context(), coreunit.Name("foo/0")).Return("", applicationerrors.UnitNotFound)

	// Act
	res, err := s.uniter.EnsureDead(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewUnitTag("foo/0").String(),
			},
		},
	})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestDestroyUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.badTag = names.NewUnitTag("foo/0")

	// Act:
	res, err := s.uniter.Destroy(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: s.badTag.String(),
			},
		},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestDestroy(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	unitName := coreunit.Name("foo/0")
	unitUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), unitUUID, false, time.Duration(0)).Return("", nil)

	// Act
	res, err := s.uniter.Destroy(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewUnitTag(unitName.String()).String(),
			},
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *uniterSuite) TestDestroyNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.applicationService.EXPECT().GetUnitUUID(c.Context(), coreunit.Name("foo/0")).Return("", applicationerrors.UnitNotFound)

	// Act
	res, err := s.uniter.Destroy(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: names.NewUnitTag("foo/0").String(),
			},
		},
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestDestroyAllSubordinatesUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	s.badTag = names.NewUnitTag("foo/0")

	// Act:
	res, err := s.uniter.DestroyAllSubordinates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: s.badTag.String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestDestroyAllSubordinates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange: a unit with some suboridnates
	principalUnitName := coreunit.Name("foo/0")

	subordinateUnitName1 := coreunit.Name("bar/1")
	subordinateUnitUUID1 := unittesting.GenUnitUUID(c)
	subordinateUnitName2 := coreunit.Name("bar/2")
	subordinateUnitUUID2 := unittesting.GenUnitUUID(c)

	s.applicationService.EXPECT().GetUnitSubordinates(gomock.Any(), principalUnitName).Return([]coreunit.Name{subordinateUnitName1, subordinateUnitName2}, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), subordinateUnitName1).Return(subordinateUnitUUID1, nil)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), subordinateUnitName2).Return(subordinateUnitUUID2, nil)

	s.removalService.EXPECT().RemoveUnit(gomock.Any(), subordinateUnitUUID1, false, time.Duration(0)).Return(removal.UUID(""), nil)
	s.removalService.EXPECT().RemoveUnit(gomock.Any(), subordinateUnitUUID2, false, time.Duration(0)).Return(removal.UUID(""), nil)

	// Act:
	res, err := s.uniter.DestroyAllSubordinates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(principalUnitName.String()).String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.IsNil)
}

func (s *uniterSuite) TestDestroyAllSubordinatesNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	unitName := coreunit.Name("foo/0")
	s.applicationService.EXPECT().GetUnitSubordinates(gomock.Any(), unitName).Return(nil, applicationerrors.UnitNotFound)

	// Act:
	res, err := s.uniter.DestroyAllSubordinates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Results, tc.HasLen, 1)
	c.Check(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestWatchUnitResolveModeUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/0")

	res, err := s.uniter.WatchUnitResolveMode(c.Context(), params.Entity{
		Tag: names.NewUnitTag("foo/0").String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestWatchUnitResolveModeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().WatchUnitResolveMode(gomock.Any(), unitName).Return(nil, resolveerrors.UnitNotFound)

	res, err := s.uniter.WatchUnitResolveMode(c.Context(), params.Entity{
		Tag: names.NewUnitTag(unitName.String()).String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestWatchUnitResolveMode(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	unitName := coreunit.Name("foo/0")
	s.expectWatchUnitResolveMode(ctrl, unitName, "1")

	res, err := s.uniter.WatchUnitResolveMode(c.Context(), params.Entity{
		Tag: names.NewUnitTag(unitName.String()).String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.Error, tc.IsNil)
	c.Check(res.NotifyWatcherId, tc.Equals, "1")
}

func (s *uniterSuite) TestResolvedUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/0")
	res, err := s.uniter.Resolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestResolvedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().UnitResolveMode(gomock.Any(), unitName).Return("", resolveerrors.UnitNotFound)

	res, err := s.uniter.Resolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestResolvedNotResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().UnitResolveMode(gomock.Any(), unitName).Return("", resolveerrors.UnitNotResolved)

	res, err := s.uniter.Resolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Mode, tc.Equals, params.ResolvedNone)
}

func (s *uniterSuite) TestResolvedRetryHooks(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().UnitResolveMode(gomock.Any(), unitName).Return(resolve.ResolveModeRetryHooks, nil)

	res, err := s.uniter.Resolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Mode, tc.Equals, params.ResolvedRetryHooks)
}

func (s *uniterSuite) TestResolvedNoRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().UnitResolveMode(gomock.Any(), unitName).Return(resolve.ResolveModeNoHooks, nil)

	res, err := s.uniter.Resolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Mode, tc.Equals, params.ResolvedNoHooks)
}

func (s *uniterSuite) TestClearResolvedUnauthorised(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/0")
	res, err := s.uniter.ClearResolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestClearResolvedNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().ClearResolved(gomock.Any(), unitName).Return(resolveerrors.UnitNotFound)

	res, err := s.uniter.ClearResolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestClearResolved(c *tc.C) {
	defer s.setupMocks(c).Finish()

	unitName := coreunit.Name("foo/0")
	s.resolveService.EXPECT().ClearResolved(gomock.Any(), unitName).Return(nil)

	res, err := s.uniter.ClearResolved(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag(unitName.String()).String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *uniterSuite) TestCharmArchiveSha256Local(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.LocalSource,
		Name:     "foo",
		Revision: 1,
	}).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(c.Context(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "local:foo-1"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Charmhub(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 1,
	}).Return("sha256:foo", nil)

	results, err := s.uniter.CharmArchiveSha256(c.Context(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{{
			Result: "sha256:foo",
		}},
	})
}

func (s *uniterSuite) TestCharmArchiveSha256Errors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 1,
	}).Return("", applicationerrors.CharmNotFound)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 2,
	}).Return("", applicationerrors.CharmNotFound)
	s.applicationService.EXPECT().GetAvailableCharmArchiveSHA256(gomock.Any(), domaincharm.CharmLocator{
		Source:   domaincharm.CharmHubSource,
		Name:     "foo",
		Revision: 3,
	}).Return("", applicationerrors.CharmNotResolved)

	results, err := s.uniter.CharmArchiveSha256(c.Context(), params.CharmURLs{
		URLs: []params.CharmURL{
			{URL: "foo-1"},
			{URL: "ch:foo-2"},
			{URL: "ch:foo-3"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: &params.Error{Message: `charm "foo-1" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-2" not found`, Code: params.CodeNotFound}},
			{Error: &params.Error{Message: `charm "ch:foo-3" not available`, Code: params.CodeNotYetAvailable}},
		},
	})
}

func (s *uniterSuite) TestLeadershipSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.uniter.Merge(c.Context(), struct{}{}, struct{}{})
	s.uniter.Read(c.Context(), struct{}{}, struct{}{})
	s.uniter.WatchLeadershipSettings(c.Context(), struct{}{}, struct{}{})
}

func (s *uniterSuite) TestGetPrincipal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("mysql/0")
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-subordinate-0"},
		{Tag: "unit-foo-42"},
	}}

	boom := internalerrors.New("boom")
	s.expectGetUnitPrincipal("wordpress/0", "", false, nil)
	s.expectGetUnitPrincipal("subordinate/0", "principal/0", true, nil)
	s.expectGetUnitPrincipal("foo/42", "", false, boom)

	result, err := s.uniter.GetPrincipal(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "", Ok: false, Error: nil},
			{Result: "unit-principal-0", Ok: true, Error: nil},
			{Result: "", Ok: false, Error: apiservererrors.ServerError(boom)},
		},
	})
}

func (s *uniterSuite) TestAvailabilityZone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-mysql-0"},
		{Tag: "unit-postgresql-0"},
		{Tag: "unit-riak-0"},
		{Tag: "unit-foo-0"},
	}}

	machineUUID := coremachinetesting.GenUUID(c)
	s.expectGetUnitMachineUUID("wordpress/0", machineUUID, nil)
	s.expectedGetAvailabilityZone(machineUUID, "a_zone", nil)

	s.expectGetUnitMachineUUID("mysql/0", machineUUID, applicationerrors.UnitMachineNotAssigned)

	s.expectGetUnitMachineUUID("postgresql/0", machineUUID, applicationerrors.UnitNotFound)

	s.expectGetUnitMachineUUID("riak/0", machineUUID, nil)
	s.expectedGetAvailabilityZone(machineUUID, "a_zone", machineerrors.AvailabilityZoneNotFound)

	s.badTag = names.NewUnitTag("foo/0")

	// Act:
	result, err := s.uniter.AvailabilityZone(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "a_zone"},
			{Error: apiservererrors.ServerError(applicationerrors.UnitMachineNotAssigned)},
			{Error: apiservertesting.NotFoundError(`unit "postgresql/0"`)},
			{Error: apiservererrors.ServerError(jujuerrors.NotProvisioned)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestAssignedMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-postgresql-0"},
		{Tag: "unit-foo-42"},
	}}

	machineName := coremachine.Name("0")
	s.expectGetUnitMachineName("mysql/0", machineName, nil)
	s.expectGetUnitMachineName("wordpress/0", "", applicationerrors.UnitMachineNotAssigned)
	s.expectGetUnitMachineName("postgresql/0", "", applicationerrors.UnitNotFound)
	s.badTag = names.NewUnitTag("foo/42")

	// Act:
	result, err := s.uniter.AssignedMachine(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "machine-0"},
			{Error: &params.Error{
				Code:    params.CodeNotAssigned,
				Message: applicationerrors.UnitMachineNotAssigned.Error(),
			}},
			{Error: apiservertesting.NotFoundError(`unit "postgresql/0"`)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestWatchConfiSettingsHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-postgresql-0"},
	}}

	// Arrange: expect a watcher for mysql
	ch := make(chan []string, 1)
	w := watchertest.NewMockStringsWatcher(ch)
	s.applicationService.EXPECT().WatchApplicationConfigHash(gomock.Any(), "mysql").Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), w).Return("1", nil)
	ch <- []string{"change1"}

	// Arrange: wordpress/0 is unauthorised.
	s.badTag = names.NewUnitTag("wordpress/0")

	// Arrange: expect a state error for postgresql
	s.applicationService.EXPECT().WatchApplicationConfigHash(gomock.Any(), "postgresql").Return(nil, applicationerrors.UnitNotFound)

	result, err := s.uniter.WatchConfigSettingsHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: "1",
				Changes:          []string{"change1"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "postgresql/0"`)},
		},
	})
}

func (s *uniterSuite) TestWatchTrustConfiSettingsHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-postgresql-0"},
	}}

	// Arrange: expect a watcher for mysql
	ch := make(chan []string, 1)
	w := watchertest.NewMockStringsWatcher(ch)
	s.applicationService.EXPECT().WatchApplicationConfigHash(gomock.Any(), "mysql").Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), w).Return("1", nil)
	ch <- []string{"change1"}

	// Arrange: wordpress/0 is unauthorised.
	s.badTag = names.NewUnitTag("wordpress/0")

	// Arrange: expect a state error for postgresql
	s.applicationService.EXPECT().WatchApplicationConfigHash(gomock.Any(), "postgresql").Return(nil, applicationerrors.UnitNotFound)

	result, err := s.uniter.WatchTrustConfigSettingsHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: "1",
				Changes:          []string{"change1"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "postgresql/0"`)},
		},
	})
}

func (s *uniterSuite) TestWatchUnitAddressesHash(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-postgresql-0"},
	}}

	// Arrange: expect a watcher for mysql/0
	ch := make(chan []string, 1)
	w := watchertest.NewMockStringsWatcher(ch)
	s.applicationService.EXPECT().WatchUnitAddressesHash(gomock.Any(), coreunit.Name("mysql/0")).Return(w, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), w).Return("1", nil)
	ch <- []string{"change1"}

	// Arrange: wordpress/0 is unauthorised.
	s.badTag = names.NewUnitTag("wordpress/0")

	// Arrange: expect a state error for postgresql/0
	s.applicationService.EXPECT().WatchUnitAddressesHash(gomock.Any(), coreunit.Name("postgresql/0")).Return(nil, applicationerrors.UnitNotFound)

	result, err := s.uniter.WatchUnitAddressesHash(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: "1",
				Changes:          []string{"change1"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(`unit "postgresql/0"`)},
		},
	})
}

func (s *uniterSuite) TestCharmURL(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
		{Tag: "application-bar"},
	}}
	locator := domaincharm.CharmLocator{
		Source:       domaincharm.CharmHubSource,
		Revision:     42,
		Architecture: architecture.AMD64,
	}
	// Arrange: expected unit calls
	s.expectGetCharmLocatorByApplicationName(c, "mysql", locator, nil)

	s.expectGetCharmLocatorByApplicationName(c, "wordpress", locator, nil)

	boom := internalerrors.New("boom")
	s.expectGetCharmLocatorByApplicationName(c, "foo", locator, boom)

	// Arrange: expected application calls
	s.expectShouldAllowCharmUpgradeOnError(c, "mysql", true, nil)
	s.expectGetCharmLocatorByApplicationName(c, "mysql", locator, nil)

	s.expectShouldAllowCharmUpgradeOnError(c, "wordpress", false, nil)
	s.expectGetCharmLocatorByApplicationName(c, "wordpress", locator, nil)

	s.expectShouldAllowCharmUpgradeOnError(c, "foo", false, boom)
	s.badTag = names.NewApplicationTag("bar")

	// Act:
	result, err := s.uniter.CharmURL(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Result: "ch:amd64/-42", Ok: true},
			{Result: "ch:amd64/-42", Ok: true},
			{Error: apiservererrors.ServerError(boom)},
			{Result: "ch:amd64/-42", Ok: true},
			{Result: "ch:amd64/-42", Ok: false},
			{Error: apiservererrors.ServerError(boom)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetCharm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmURL := "local:foo-43"
	locator := domaincharm.CharmLocator{
		Name:     "foo",
		Source:   domaincharm.LocalSource,
		Revision: 43,
	}
	unitName := coreunit.Name("foo/0")

	s.applicationService.EXPECT().UpdateUnitCharm(gomock.Any(), unitName, locator).Return(nil)

	res, err := s.uniter.SetCharm(c.Context(), params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{{
			Tag:      names.NewUnitTag(unitName.String()).String(),
			CharmURL: charmURL,
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.IsNil)
}

func (s *uniterSuite) TestSetCharmUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmURL := "local:foo-43"
	locator := domaincharm.CharmLocator{
		Name:     "foo",
		Source:   domaincharm.LocalSource,
		Revision: 43,
	}
	unitName := coreunit.Name("foo/0")

	s.applicationService.EXPECT().UpdateUnitCharm(gomock.Any(), unitName, locator).Return(applicationerrors.UnitNotFound)

	res, err := s.uniter.SetCharm(c.Context(), params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{{
			Tag:      names.NewUnitTag(unitName.String()).String(),
			CharmURL: charmURL,
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) TestSetCharmCharmNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	charmURL := "local:foo-43"
	locator := domaincharm.CharmLocator{
		Name:     "foo",
		Source:   domaincharm.LocalSource,
		Revision: 43,
	}
	unitName := coreunit.Name("foo/0")

	s.applicationService.EXPECT().UpdateUnitCharm(gomock.Any(), unitName, locator).Return(applicationerrors.CharmNotFound)

	res, err := s.uniter.SetCharm(c.Context(), params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{{
			Tag:      names.NewUnitTag(unitName.String()).String(),
			CharmURL: charmURL,
		}},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.Results, tc.HasLen, 1)
	c.Assert(res.Results[0].Error, tc.Satisfies, params.IsCodeNotFound)
}

func (s *uniterSuite) expectGetCharmLocatorByApplicationName(c *tc.C, appName string, charmLocator domaincharm.CharmLocator, err error) {
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(gomock.Any(), appName).Return(charmLocator, err)
}

func (s *uniterSuite) expectShouldAllowCharmUpgradeOnError(c *tc.C, appName string, v bool, err error) {
	s.applicationService.EXPECT().ShouldAllowCharmUpgradeOnError(gomock.Any(), appName).Return(v, err)
}

func (s *uniterSuite) TestConfigSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange:
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-postgresql-0"},
		{Tag: "unit-foo-42"},
	}}

	settings := map[string]any{
		"foo": "bar",
	}
	s.expectedGetConfigSettings("mysql/0", settings, nil)
	s.expectedGetConfigSettings("wordpress/0", nil, nil)
	s.expectedGetConfigSettings("postgresql/0", nil, applicationerrors.UnitNotFound)
	s.badTag = names.NewUnitTag("foo/42")

	// Act:
	result, err := s.uniter.ConfigSettings(c.Context(), args)

	// Assert:
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.ConfigSettingsResults{
		Results: []params.ConfigSettingsResult{
			{Settings: settings},
			{Settings: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestHasSubordinates(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-subordinate-0"},
		{Tag: "unit-foo-42"},
	}}

	s.badTag = names.NewUnitTag("mysql/0")
	s.expectGetHasSubordinates(c, "wordpress/0", []coreunit.Name{"sub0/0", "sub1/0"}, nil)
	s.expectGetHasSubordinates(c, "subordinate/0", nil, nil)
	boom := internalerrors.New("boom")
	s.expectGetHasSubordinates(c, "foo/42", nil, boom)

	result, err := s.uniter.HasSubordinates(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: true},
			{Result: false},
			{Error: apiservererrors.ServerError(boom)},
		},
	})
}

func (s *uniterSuite) TestPublicAddressFailCanAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/42")
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.PublicAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestPublicAddressErrorFromDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	boom := internalerrors.New("boom")
	s.networkService.EXPECT().GetUnitPublicAddress(gomock.Any(), coreunit.Name("mysql/0")).Return(network.SpaceAddress{}, boom)

	result, err := s.uniter.PublicAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservererrors.ServerError(boom)},
		},
	})
}

func (s *uniterSuite) TestPublicAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	addr := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "192.168.0.1",
		},
	}
	s.networkService.EXPECT().GetUnitPublicAddress(gomock.Any(), coreunit.Name("mysql/0")).Return(addr, nil)

	result, err := s.uniter.PublicAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "192.168.0.1"},
		},
	})
}

func (s *uniterSuite) TestPrivateAddressFailCanAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/42")
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.PrivateAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestPrivateAddressErrorFromDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	boom := internalerrors.New("boom")
	s.networkService.EXPECT().GetUnitPrivateAddress(gomock.Any(), coreunit.Name("mysql/0")).Return(network.SpaceAddress{}, boom)

	result, err := s.uniter.PrivateAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservererrors.ServerError(boom)},
		},
	})
}

func (s *uniterSuite) TestPrivateAddress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
	}}
	addr := network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: "192.168.0.1/24",
		},
	}
	s.networkService.EXPECT().GetUnitPrivateAddress(gomock.Any(), coreunit.Name("mysql/0")).Return(addr, nil)

	result, err := s.uniter.PrivateAddress(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: addr.IP().String()},
		},
	})
}

func (s *uniterSuite) TestNetworkInfoFailCanAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/42")
	args := params.NetworkInfoParams{
		Unit:      "unit-foo-42",
		Endpoints: []string{"endpoint-0", "endpoint-1"},
	}

	_, err := s.uniter.NetworkInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *uniterSuite) TestNetworkInfoErrorFromDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.NetworkInfoParams{
		Unit:      "unit-foo-42",
		Endpoints: []string{"endpoint-0", "endpoint-1"},
	}
	boom := internalerrors.New("boom")

	s.networkService.EXPECT().GetUnitEndpointNetworks(gomock.Any(), coreunit.Name("foo/42"),
		args.Endpoints).Return(nil, boom)

	_, err := s.uniter.NetworkInfo(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *uniterSuite) TestNetworkInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := params.NetworkInfoParams{
		Unit:      "unit-foo-42",
		Endpoints: []string{"endpoint-0", "endpoint-1"},
	}
	addr := "192.168.0.1"

	s.networkService.EXPECT().GetUnitEndpointNetworks(gomock.Any(), coreunit.Name("foo/42"),
		args.Endpoints).Return([]domainnetwork.UnitNetwork{{
		EndpointName:     "endpoint-0",
		IngressAddresses: []string{addr},
	}, {
		EndpointName:     "endpoint-1",
		IngressAddresses: []string{addr},
	}}, nil)

	result, err := s.uniter.NetworkInfo(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"endpoint-0": {IngressAddresses: []string{addr}},
			"endpoint-1": {IngressAddresses: []string{addr}},
		},
	})
}

func (s *uniterSuite) TestGoalStatesUnauthorized(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.badTag = names.NewUnitTag("foo/0")
	res, err := s.uniter.GoalStates(c.Context(), params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewUnitTag("foo/0").String(),
		}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, params.GoalStateResults{
		Results: []params.GoalStateResult{{Error: apiservertesting.ErrUnauthorized}},
	})
}

func (s *uniterSuite) expectedGetConfigSettings(unitName coreunit.Name, settings map[string]any, err error) {
	s.applicationService.EXPECT().GetApplicationIDByUnitName(gomock.Any(), unitName).Return(coreapplication.ID(unitName.Application()), err)
	if err == nil {
		s.applicationService.EXPECT().GetApplicationConfigWithDefaults(
			gomock.Any(), coreapplication.ID(unitName.Application()),
		).Return(settings, nil)
	}
}

func (s *uniterSuite) expectGetUnitPrincipal(unitName, principalName coreunit.Name, ok bool, err error) {
	s.applicationService.EXPECT().GetUnitPrincipal(gomock.Any(), unitName).Return(principalName, ok, err)
}

func (s *uniterSuite) expectGetUnitMachineUUID(unitName coreunit.Name, machineUUID coremachine.UUID, err error) {
	s.applicationService.EXPECT().GetUnitMachineUUID(gomock.Any(), unitName).Return(machineUUID, err)
}

func (s *uniterSuite) expectGetUnitMachineName(unitName coreunit.Name, machineName coremachine.Name, err error) {
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), unitName).Return(machineName, err)
}

func (s *uniterSuite) expectedGetAvailabilityZone(machineUUID coremachine.UUID, az string, err error) {
	s.machineService.EXPECT().AvailabilityZone(gomock.Any(), machineUUID).Return(az, err)
}

func (s *uniterSuite) expectGetHasSubordinates(c *tc.C, unitName coreunit.Name, subordinateNames []coreunit.Name, err error) {
	s.applicationService.EXPECT().GetUnitSubordinates(gomock.Any(), unitName).Return(subordinateNames, err)
}

func (s *uniterSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.resolveService = NewMockResolveService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	authFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag != s.badTag
		}, nil
	}
	s.uniter = &UniterAPI{
		applicationService: s.applicationService,
		machineService:     s.machineService,
		networkService:     s.networkService,
		resolveService:     s.resolveService,
		removalService:     s.removalService,
		accessUnit:         authFunc,
		accessApplication:  authFunc,
		watcherRegistry:    s.watcherRegistry,
	}

	c.Cleanup(func() {
		s.applicationService = nil
		s.machineService = nil
		s.networkService = nil
		s.resolveService = nil
		s.removalService = nil
		s.watcherRegistry = nil
	})

	return ctrl
}

func (s *uniterSuite) expectWatchUnitResolveMode(
	ctrl *gomock.Controller,
	unitName coreunit.Name,
	watcherID string,
) {
	mockWatcher := NewMockNotifyWatcher(ctrl)
	channel := make(chan struct{}, 1)
	channel <- struct{}{}
	mockWatcher.EXPECT().Changes().Return(channel).AnyTimes()
	s.resolveService.EXPECT().WatchUnitResolveMode(gomock.Any(), unitName).Return(mockWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return(watcherID, nil).AnyTimes()
}

type leadershipSettings interface {
	// Merge merges in the provided leadership settings. Only leaders for
	// the given service may perform this operation.
	Merge(ctx context.Context, bulkArgs params.MergeLeadershipSettingsBulkParams) (params.ErrorResults, error)

	// Read reads leadership settings for the provided service ID. Any
	// unit of the service may perform this operation.
	Read(ctx context.Context, bulkArgs params.Entities) (params.GetLeadershipSettingsBulkResults, error)

	// WatchLeadershipSettings will block the caller until leadership settings
	// for the given service ID change.
	WatchLeadershipSettings(ctx context.Context, bulkArgs params.Entities) (params.NotifyWatchResults, error)
}

type leadershipUniterSuite struct {
	testhelpers.IsolationSuite

	watcherRegistry *MockWatcherRegistry

	uniter leadershipSettings

	setupMocks func(c *tc.C) *gomock.Controller
}

func (s *leadershipUniterSuite) TestLeadershipSettingsMerge(c *tc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.Merge(c.Context(), params.MergeLeadershipSettingsBulkParams{
		Params: []params.MergeLeadershipSettingsParam{
			{
				ApplicationTag: "app1",
				Settings: params.Settings{
					"key1": "value1",
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{}},
	})
}

func (s *leadershipUniterSuite) TestLeadershipSettingsRead(c *tc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.Read(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: "app1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.GetLeadershipSettingsBulkResults{
		Results: []params.GetLeadershipSettingsResult{{}},
	})
}

func (s *leadershipUniterSuite) TestLeadershipSettingsWatchLeadershipSettings(c *tc.C) {
	defer s.setupMocks(c).Finish()

	results, err := s.uniter.WatchLeadershipSettings(c.Context(), params.Entities{
		Entities: []params.Entity{
			{
				Tag: "app1",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{{
			NotifyWatcherId: "watcher1",
		}},
	})
}

type uniterv19Suite struct {
	leadershipUniterSuite
}

func TestUniterv19Suite(t *testing.T) {
	tc.Run(t, &uniterv19Suite{})
}

func (s *uniterv19Suite) SetUpTest(c *tc.C) {
	s.setupMocks = func(c *tc.C) *gomock.Controller {
		ctrl := gomock.NewController(c)

		s.watcherRegistry = NewMockWatcherRegistry(ctrl)
		s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("watcher1", nil).AnyTimes()

		s.uniter = &UniterAPIv19{
			UniterAPIv20: &UniterAPIv20{
				UniterAPI: &UniterAPI{
					watcherRegistry: s.watcherRegistry,
				},
			},
		}

		return ctrl
	}
}

type uniterv20Suite struct {
	leadershipUniterSuite
}

func TestUniterv20Suite(t *testing.T) {
	tc.Run(t, &uniterv20Suite{})
}

func (s *uniterv20Suite) SetUpTest(c *tc.C) {
	s.setupMocks = func(c *tc.C) *gomock.Controller {
		ctrl := gomock.NewController(c)

		s.watcherRegistry = NewMockWatcherRegistry(ctrl)
		s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return("watcher1", nil).AnyTimes()

		s.uniter = &UniterAPIv20{
			UniterAPI: &UniterAPI{
				modelUUID:       model.UUID(coretesting.ModelTag.Id()),
				modelType:       model.IAAS,
				watcherRegistry: s.watcherRegistry,
			},
		}

		return ctrl
	}
}

type uniterRelationSuite struct {
	testhelpers.IsolationSuite

	wordpressAppTag  names.ApplicationTag
	authTag          names.Tag
	wordpressUnitTag names.UnitTag

	applicationService *MockApplicationService
	networkService     *MockNetworkService
	relationService    *MockRelationService
	statusService      *MockStatusService
	watcherRegistry    *MockWatcherRegistry

	uniter *UniterAPI
}

func TestUniterRelationSuite(t *testing.T) {
	tc.Run(t, &uniterRelationSuite{})
}

func (s *uniterRelationSuite) SetUpSuite(c *tc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.wordpressAppTag = names.NewApplicationTag("wordpress")
	s.wordpressUnitTag = names.NewUnitTag("wordpress/0")
	s.authTag = s.wordpressUnitTag
}

func (s *uniterRelationSuite) TestRelation(c *tc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relKey := relationtesting.GenNewKey(c, relTag.Id())

	relUUID := relationtesting.GenRelationUUID(c)
	relID := 42

	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	s.expectGetRelationDetails(c, relUUID, relID, relTag)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.Relation(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{
				Id:   relID,
				Key:  relTag.Id(),
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation: params.CharmRelation{
						Name:      "database",
						Role:      string(charm.RoleRequirer),
						Interface: "mysql",
						Optional:  false,
						Limit:     0,
						Scope:     string(charm.ScopeGlobal),
					},
				},
				OtherApplication: params.RelatedApplicationDetails{
					ApplicationName: "mysql",
					ModelUUID:       coretesting.ModelTag.Id(),
				},
			},
		},
	})
}

// TestRelationUnauthorized tests the different scenarios where
// ErrUnauthorized will be returned. It also tests the bulk
// functionality of the Relation facade method.
func (s *uniterRelationSuite) TestRelationUnauthorized(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// arrange
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relTagFail := names.NewRelationTag("foo:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTagFail.Id()), "", relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{
		RelationUnits: []params.RelationUnit{
			// "relation-42" is not a valid relation key.
			{Relation: "relation-42", Unit: "unit-wordpress-0"},
			// "user-foo" is not a parsable unit tag.
			{Unit: "user-foo"},
			// "unit-mysql-0" is not the authorizing tag, though
			// is part of the relation.
			{Relation: relTag.String(), Unit: "unit-mysql-0"},
			// Not found relation with correct unit.
			{Relation: relTagFail.String(), Unit: "unit-wordpress-0"},
		},
	}
	result, err := s.uniter.Relation(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestRelationById(c *tc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relIDNotFound := -1
	relID := 31
	relIDUnexpectedAppName := 42

	s.expectGetRelationUUIDByID(relIDNotFound, relUUID, nil)
	s.expectGetRelationDetailsNotFound(relUUID)

	s.expectGetRelationUUIDByID(relID, relUUID, nil)
	s.expectGetRelationDetails(c, relUUID, relID, relTag)

	s.expectGetRelationUUIDByID(relIDUnexpectedAppName, relUUID, nil)
	s.expectGetRelationDetailsUnexpectedAppName(c, relUUID)

	args := params.RelationIds{
		RelationIds: []int{
			// The relation ID does not exist: ErrUnauthorized.
			relIDNotFound,
			// Successful result.
			relID,
			// The auth application is not part of the relation: ErrUnauthorized.
			relIDUnexpectedAppName,
		},
	}
	result, err := s.uniter.RelationById(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:   relID,
				Key:  relTag.Id(),
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation: params.CharmRelation{
						Name:      "database",
						Role:      string(charm.RoleRequirer),
						Interface: "mysql",
						Optional:  false,
						Limit:     0,
						Scope:     string(charm.ScopeGlobal),
					},
				},
				OtherApplication: params.RelatedApplicationDetails{
					ApplicationName: "mysql",
					ModelUUID:       coretesting.ModelTag.Id(),
				},
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsApplication(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettingsWithLeader(coreunit.Name(s.wordpressUnitTag.Id()), relUUID, appID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsUnit(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetRelationUnit(relUUID, relUnitUUID, s.wordpressUnitTag.Id())
	s.expectGetRelationUnitSettings(relUnitUUID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.ReadSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadSettingsErrUnauthorized(c *tc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)

	errAuthTests := []struct {
		description string
		arg         params.RelationUnit
		arrange     func()
	}{
		{
			description: "unauthorized unit",
			arg:         params.RelationUnit{Relation: "relation-42", Unit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "remote unit, valid in relation, not this call",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "unit-mysql-0"},
			arrange: func() {
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
			},
		}, {
			description: "relation tag parsing fail",
			arg:         params.RelationUnit{Relation: "application-wordpress", Unit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "unit arg not unit nor application",
			arg:         params.RelationUnit{Relation: relTag.String(), Unit: "user-foo"},
			arrange: func() {
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
			},
		},
	}

	for i, testCase := range errAuthTests {
		c.Logf("test %d: %s", i, testCase.description)
		testCase.arrange()
		args := params.RelationUnits{RelationUnits: []params.RelationUnit{testCase.arg}}
		result, err := s.uniter.ReadSettings(c.Context(), args)
		if c.Check(err, tc.ErrorIsNil) {
			if !c.Check(result.Results, tc.HasLen, 1) {
				continue
			}
			c.Check(result.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
		}
	}
}

func (s *uniterRelationSuite) TestReadSettingsForLocalApplication(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettingsWithLeader(coreunit.Name(s.wordpressUnitTag.Id()), relUUID, appID, settings)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestReadRemoteSettingsErrUnauthorized(c *tc.C) {
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)

	errAuthTests := []struct {
		description string
		arg         params.RelationUnitPair
		arrange     func()
	}{
		{
			description: "local unit fails parsing",
			arg:         params.RelationUnitPair{LocalUnit: "foo-0"},
			arrange:     func() {},
		}, {
			description: "remote unit fails parsing",
			arg:         params.RelationUnitPair{LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: ""},
			arrange:     func() {},
		}, {
			description: "local unit cannot access",
			arg:         params.RelationUnitPair{LocalUnit: "unit-foo-0"},
			arrange:     func() {},
		}, {
			description: "bad relation tag",
			arg:         params.RelationUnitPair{Relation: "failme-76", LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: "unit-one-2"},
			arrange:     func() {},
		}, {
			description: "remote unit tag not unit nor application kinds",
			arg:         params.RelationUnitPair{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: "machine-2"},
			arrange: func() {
				s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
			},
		},
	}

	for i, testCase := range errAuthTests {
		c.Logf("test %d: %s", i, testCase.description)
		testCase.arrange()
		args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{testCase.arg}}
		result, err := s.uniter.ReadRemoteSettings(c.Context(), args)
		if c.Check(err, tc.ErrorIsNil) {
			if !c.Check(result.Results, tc.HasLen, 1) {
				continue
			}
			c.Check(result.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
		}
	}
}

// TestReadRemoteSettingsForUnit tests a local unit's ability to read the
// unit settings from the unit at the other end of the relation.
// local = wordpress
// remote = mysql
func (s *uniterRelationSuite) TestReadRemoteSettingsForUnit(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	remoteUnitTag := names.NewUnitTag("mysql/2")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetRelationUnit(relUUID, relUnitUUID, remoteUnitTag.Id())
	s.expectGetRelationUnitSettings(relUnitUUID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: remoteUnitTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

// TestReadRemoteSettingsForApplication tests a local unit's ability to read the
// application settings from the application at the other end of the relation.
// local = wordpress
// remote = mysql
func (s *uniterRelationSuite) TestReadRemoteSettingsForApplication(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	remoteAppTag := names.NewApplicationTag("mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(remoteAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: remoteAppTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

// TestReadRemoteApplicationSettingsWithLocalApplication tests a local unit's
// ability to read the application settings of its own application via the
// ReadRemoteSettings method .
// local = wordpress
func (s *uniterRelationSuite) TestReadRemoteApplicationSettingsWithLocalApplication(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	appID := applicationtesting.GenApplicationUUID(c)
	settings := map[string]string{"wanda": "firebaugh"}

	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, relTag.Id()), relUUID, nil)
	s.expectGetApplicationIDByName(s.wordpressAppTag.Id(), appID)
	s.expectGetRelationApplicationSettings(relUUID, appID, settings)

	// act
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: relTag.String(), LocalUnit: s.wordpressUnitTag.String(), RemoteUnit: s.wordpressAppTag.String()},
	}}
	result, err := s.uniter.ReadRemoteSettings(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
		},
	})
}

func (s *uniterRelationSuite) TestRelationStatus(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()

	unitUUID := unittesting.GenUnitUUID(c)
	s.expectGetUnitUUID(s.wordpressUnitTag.Id(), unitUUID, nil)
	relTagOne := names.NewRelationTag("mysql:database wordpress:mysql")
	relTagTwo := names.NewRelationTag("redis:endpoint wordpress:endpoint")
	expectedRelationUnitStatus := []params.RelationUnitStatus{
		{
			RelationTag: relTagOne.String(),
			InScope:     true,
			Suspended:   false,
		}, {
			RelationTag: relTagTwo.String(),
			InScope:     true,
			Suspended:   true,
		},
	}
	s.expectedGetRelationsStatusForUnit(c, unitUUID, expectedRelationUnitStatus)

	// act
	args := params.Entities{Entities: []params.Entity{{Tag: s.wordpressUnitTag.String()}}}
	result, err := s.uniter.RelationsStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.RelationUnitStatusResults{
		Results: []params.RelationUnitStatusResult{
			{RelationResults: expectedRelationUnitStatus},
		},
	})
}

// TestRelationsStatusUnitTagNotUnitNorApplication test that a valid tag not of
// the type application nor unit fails with unauthorized.
func (s *uniterRelationSuite) TestRelationsStatusUnitTagNotUnitNorApplication(c *tc.C) {
	// act
	args := params.Entities{Entities: []params.Entity{{Tag: "machine-0"}}}
	result, err := s.uniter.RelationsStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

// TestRelationsStatusUnitTagCannotAccess tests that a valid unit tag which is not
// the authorized one will fail.
func (s *uniterRelationSuite) TestRelationsStatusUnitTagCannotAccess(c *tc.C) {
	// act
	args := params.Entities{Entities: []params.Entity{{Tag: "unit-mysql-0"}}}
	result, err := s.uniter.RelationsStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *uniterRelationSuite) TestSetRelationStatus(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relID := 42
	relationUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByID(relID, relationUUID, nil)
	relStatus := status.StatusInfo{
		Status: status.Joined,
		Since:  ptr(s.uniter.clock.Now()),
	}
	s.expectSetRelationStatus(s.wordpressUnitTag.Id(), relationUUID, relStatus)

	// act
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{
			{UnitTag: s.wordpressUnitTag.String(), RelationId: relID, Status: params.Joined},
		},
	}
	result, err := s.uniter.SetRelationStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, tc.DeepEquals, emptyErrorResults)
}

func (s *uniterRelationSuite) TestSetRelationStatusUnitTagNotValid(c *tc.C) {
	// act
	args := params.RelationStatusArgs{Args: []params.RelationStatusArg{{UnitTag: "foo"}}}
	result, err := s.uniter.SetRelationStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.ErrorMatches, "\"foo\" is not a valid tag")
}

func (s *uniterRelationSuite) TestSetRelationStatusRelationNotFound(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relID := 42
	relationUUID := relationtesting.GenRelationUUID(c)
	s.expectGetRelationUUIDByID(relID, relationUUID, relationerrors.RelationNotFound)

	// act
	args := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    s.wordpressUnitTag.String(),
		RelationId: relID,
		Status:     params.Joined,
	}}}
	result, err := s.uniter.SetRelationStatus(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.DeepEquals, apiservertesting.ErrUnauthorized)
}

func (s *uniterRelationSuite) TestEnterScopeErrUnauthorized(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	failRelTag := names.NewRelationTag("postgresql:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, failRelTag.Id()), "", relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// relation tag not parsable
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		// not found relation key
		{Relation: failRelTag.String(), Unit: "unit-wordpress-0"},
		// authorization on unit tag fails
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := s.uniter.EnterScope(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestEnterScope(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relKey := relationtesting.GenNewKey(c, relTag.Id())
	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	addr := "x.x.x.x"
	unitName := coreunit.Name(s.wordpressUnitTag.Id())
	settings := map[string]string{"ingress-address": addr}
	s.expectEnterScope(relUUID, unitName, settings, nil)

	s.networkService.EXPECT().GetUnitRelationNetwork(gomock.Any(), unitName, relKey).Return(domainnetwork.UnitNetwork{
		EndpointName:     "mysql",
		IngressAddresses: []string{addr},
	}, nil)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.EnterScope(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, tc.DeepEquals, emptyErrorResults)
}

// TestEnterScopeReturnsPotentialRelationUnitNotValid tests that if EnterScope
// returns PotentialRelationUnitNotValid the facade method still returns no
// error.
func (s *uniterRelationSuite) TestEnterScopeReturnsPotentialRelationUnitNotValid(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relUUID := relationtesting.GenRelationUUID(c)
	relKey := relationtesting.GenNewKey(c, relTag.Id())
	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	addr := "x.x.x.x"
	unitName := coreunit.Name(s.wordpressUnitTag.Id())
	settings := map[string]string{"ingress-address": addr}
	s.expectEnterScope(relUUID, unitName, settings,
		relationerrors.PotentialRelationUnitNotValid)
	s.networkService.EXPECT().GetUnitRelationNetwork(gomock.Any(), unitName, relKey).Return(domainnetwork.UnitNetwork{
		EndpointName:     "mysql",
		IngressAddresses: []string{addr},
	}, nil)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()},
	}}
	result, err := s.uniter.EnterScope(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	emptyErrorResults := params.ErrorResults{Results: []params.ErrorResult{{}}}
	c.Assert(result, tc.DeepEquals, emptyErrorResults)
}

// TestLeaveScopeFails tests for unauthorized errors, unit tag
// validation, and ensures the method works in bulk.
func (s *uniterRelationSuite) TestLeaveScopeFails(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	failRelTag := names.NewRelationTag("postgresql:database wordpress:mysql")
	s.expectGetRelationUUIDByKey(relationtesting.GenNewKey(c, failRelTag.Id()), "",
		relationerrors.RelationNotFound)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// Not the authorized unit
		{Relation: "relation-42", Unit: "unit-foo-0"},
		// Invalid relation tag
		{Relation: "relation-42", Unit: s.wordpressUnitTag.String()},
		// Relation key not found
		{Relation: failRelTag.String(), Unit: s.wordpressUnitTag.String()},
		// Invalid unit tag
		{Relation: relTag.String(), Unit: "application-wordpress"},
	}}
	result, err := s.uniter.LeaveScope(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `"application-wordpress" is not a valid unit tag`}},
		},
	})
}

func (s *uniterRelationSuite) TestWatchRelationUnits(c *tc.C) {
	// arrange
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	relUUID := relationtesting.GenRelationUUID(c)
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")
	relKey, err := corerelation.ParseKeyFromTagString(relTag.String())
	c.Assert(err, tc.ErrorIsNil)
	s.expectGetRelationUUIDByKey(relKey, relUUID, nil)
	watcherID := "watch1"
	unitUUIDs := []coreunit.UUID{
		unittesting.GenUnitUUID(c),
		unittesting.GenUnitUUID(c),
	}
	appUUIDs := []coreapplication.ID{
		applicationtesting.GenApplicationUUID(c),
	}

	unitName := coreunit.Name(s.wordpressUnitTag.Id())
	watchedUUID := unittesting.GenUnitUUID(c)
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(watchedUUID, nil)

	// Changes and expected results should match.
	changes := relation.RelationUnitsChange{
		Changed: map[coreunit.Name]int64{
			"wordpress/0": 42,
		},
		AppChanged: map[string]int64{
			"wordpress": 47,
		},
		Departed: []coreunit.Name{"mysql/0"},
	}
	s.expectWatchRelatedUnitsChange(watchedUUID, relUUID, unitUUIDs, appUUIDs, watcherID, changes)

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: s.wordpressUnitTag.String()}},
	}
	result, err := s.uniter.WatchRelationUnits(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)

	expectedResult := params.RelationUnitsWatchResults{Results: []params.RelationUnitsWatchResult{
		{
			RelationUnitsWatcherId: watcherID,
			Changes: params.RelationUnitsChange{
				Changed: map[string]params.UnitSettings{
					"wordpress/0": {Version: 42},
				},
				AppChanged: map[string]int64{
					"wordpress": 47,
				},
				Departed: []string{"mysql/0"},
			},
		},
	}}
	c.Assert(result, tc.DeepEquals, expectedResult)
}

// TestWatchRelationUnitsFails tests for unauthorized errors, unit tag
// validation, and ensures the method works in bulk.
func (s *uniterRelationSuite) TestWatchRelationUnitsFails(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	relTag := names.NewRelationTag("mysql:database wordpress:mysql")

	// act
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		// Not the authorized unit
		{Relation: "relation-42", Unit: "unit-foo-0"},
		// Invalid relation tag
		{Relation: "relation-42", Unit: s.wordpressUnitTag.String()},
		// Invalid unit tag
		{Relation: relTag.String(), Unit: "application-wordpress"},
	}}
	result, err := s.uniter.WatchRelationUnits(c.Context(), args)

	// assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.RelationUnitsWatchResults{
		Results: []params.RelationUnitsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `"relation-42" is not a valid relation tag`}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterRelationSuite) TestWatchUnitRelations(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	unitUUID := unittesting.GenUnitUUID(c)
	watcherID := "watcher-id"
	relationKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relationChanges := make(chan []string, 1)
	change := []string{relationKey.String()}
	relationChanges <- change
	watch := watchertest.NewMockStringsWatcher(relationChanges)
	s.expectGetUnitUUID(s.wordpressUnitTag.Id(), unitUUID, nil)
	s.expectWatchLifeSuspendedStatus(unitUUID, watch, nil)
	s.expectWatcherRegistry(watcherID, watch, nil)

	// Act
	results, err := s.uniter.WatchUnitRelations(c.Context(),
		params.Entities{
			Entities: []params.Entity{
				{Tag: s.wordpressUnitTag.String()},
			}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{
				StringsWatcherId: watcherID,
				Changes:          change,
				Error:            nil,
			},
		},
	})
}

func (s *uniterRelationSuite) TestWatchUnitRelationsErrUnauthorized(c *tc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	args := params.Entities{Entities: []params.Entity{
		// Bad unit tag.
		{Tag: "application"},
		// Not the authorized unit
		{Tag: "unit-mysql-4"},
	}}

	// Act
	results, err := s.uniter.WatchUnitRelations(c.Context(), args)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

}

func (s *uniterRelationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	unitAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.wordpressUnitTag.Id()
		}, nil
	}

	appAuthFunc := func(ctx context.Context) (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag.Id() == s.wordpressAppTag.Id()
		}, nil
	}

	authorizer := &apiservertesting.FakeAuthorizer{
		Tag:        s.authTag,
		Controller: true,
	}

	s.uniter = &UniterAPI{
		modelUUID:         model.UUID(coretesting.ModelTag.Id()),
		modelType:         model.IAAS,
		accessApplication: appAuthFunc,
		accessUnit:        unitAuthFunc,
		auth:              authorizer,
		clock:             testclock.NewClock(time.Now()),
		logger:            loggertesting.WrapCheckLog(c),

		applicationService: s.applicationService,
		networkService:     s.networkService,
		relationService:    s.relationService,
		statusService:      s.statusService,
		watcherRegistry:    s.watcherRegistry,
	}

	c.Cleanup(func() {
		s.applicationService = nil
		s.networkService = nil
		s.relationService = nil
		s.statusService = nil
		s.watcherRegistry = nil

	})
	return ctrl
}

func (s *uniterRelationSuite) expectGetRelationUUIDByKey(key corerelation.Key, relUUID corerelation.UUID, err error) {
	s.relationService.EXPECT().GetRelationUUIDByKey(gomock.Any(), key).Return(relUUID, err)
}

func (s *uniterRelationSuite) expectGetRelationDetails(c *tc.C, relUUID corerelation.UUID, relID int, relTag names.RelationTag) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relUUID,
		ID:   relID,
		Key:  relationtesting.GenNewKey(c, relTag.Id()),
		Endpoints: []relation.Endpoint{
			{
				ApplicationName: "wordpress",
				Relation: charm.Relation{
					Name:      "database",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
			{
				ApplicationName: "mysql",
				Relation: charm.Relation{
					Name:      "mysql",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}, nil)
}

func (s *uniterRelationSuite) expectGetRelationDetailsNotFound(relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{}, relationerrors.RelationNotFound)
}

func (s *uniterRelationSuite) expectGetRelationDetailsUnexpectedAppName(c *tc.C, relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationDetails(gomock.Any(), relUUID).Return(relation.RelationDetails{
		Life: life.Alive,
		UUID: relationtesting.GenRelationUUID(c),
		ID:   101,
		Endpoints: []relation.Endpoint{
			{
				ApplicationName: "failure-application",
				Relation: charm.Relation{
					Name:      "database",
					Role:      charm.RoleRequirer,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
			{
				ApplicationName: "mysql",
				Relation: charm.Relation{
					Name:      "mysql",
					Role:      charm.RoleProvider,
					Interface: "mysql",
					Scope:     charm.ScopeGlobal,
				},
			},
		},
	}, nil)
}

func (s *uniterRelationSuite) expectGetApplicationIDByName(appName string, id coreapplication.ID) {
	s.applicationService.EXPECT().GetApplicationIDByName(gomock.Any(), appName).Return(id, nil)
}

func (s *uniterRelationSuite) expectGetRelationApplicationSettingsWithLeader(unitName coreunit.Name, uuid corerelation.UUID, id coreapplication.ID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationApplicationSettingsWithLeader(gomock.Any(), unitName, uuid, id).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetRelationApplicationSettings(uuid corerelation.UUID, id coreapplication.ID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationApplicationSettings(gomock.Any(), uuid, id).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnit(relUUID corerelation.UUID, uuid corerelation.UnitUUID, unitTagID string) {
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, coreunit.Name(unitTagID)).Return(uuid, nil)
}

func (s *uniterRelationSuite) expectGetRelationUnitSettings(uuid corerelation.UnitUUID, settings map[string]string) {
	s.relationService.EXPECT().GetRelationUnitSettings(gomock.Any(), uuid).Return(settings, nil)
}

func (s *uniterRelationSuite) expectGetUnitUUID(name string, unitUUID coreunit.UUID, err error) {
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name(name)).Return(unitUUID, err)
}

func (s *uniterRelationSuite) expectedGetRelationsStatusForUnit(c *tc.C, uuid coreunit.UUID, input []params.RelationUnitStatus) {
	expectedStatuses := make([]relation.RelationUnitStatus, len(input))
	for i, in := range input {
		// The caller created the tag, programing error if this fails.
		tag, _ := names.ParseRelationTag(in.RelationTag)
		expectedStatuses[i] = relation.RelationUnitStatus{
			Key:       relationtesting.GenNewKey(c, tag.Id()),
			InScope:   in.InScope,
			Suspended: in.Suspended,
		}
	}
	s.relationService.EXPECT().GetRelationsStatusForUnit(gomock.Any(), uuid).Return(expectedStatuses, nil)
}

func (s *uniterRelationSuite) expectGetRelationUUIDByID(relID int, relUUID corerelation.UUID, err error) {
	s.relationService.EXPECT().GetRelationUUIDByID(gomock.Any(), relID).Return(relUUID, err)
}

func (s *uniterRelationSuite) expectSetRelationStatus(unitName string, relUUID corerelation.UUID, relStatus status.StatusInfo) {
	name, _ := coreunit.NewName(unitName)
	s.statusService.EXPECT().SetRelationStatus(gomock.Any(), name, relUUID, relStatus).Return(nil)
}

func (s *uniterRelationSuite) expectEnterScope(uuid corerelation.UUID, name coreunit.Name, settings map[string]string, err error) {
	s.relationService.EXPECT().EnterScope(gomock.Any(), uuid, name, settings, gomock.Any()).Return(err)
}

func (s *uniterRelationSuite) expectWatchLifeSuspendedStatus(unitUUID coreunit.UUID, watch watcher.StringsWatcher, err error) {
	s.relationService.EXPECT().WatchLifeSuspendedStatus(gomock.Any(), unitUUID).Return(watch, err)
}

func (s *uniterRelationSuite) expectWatcherRegistry(watchID string, watch *watchertest.MockStringsWatcher, err error) {
	s.watcherRegistry.EXPECT().Register(gomock.Any(), watch).Return(watchID, err).AnyTimes()
}

func (s *uniterRelationSuite) expectWatchRelatedUnitsChange(
	watchedUnitUUID coreunit.UUID,
	relUUID corerelation.UUID,
	unitUUIDs []coreunit.UUID,
	appUUIDS []coreapplication.ID,
	watcherID string,
	changes relation.RelationUnitsChange,
) {
	channel := make(chan []string, 1)
	mockWatcher := watchertest.NewMockStringsWatcher(channel)
	channel <- append(transform.Slice(unitUUIDs, encodeUnitFromUUID), transform.Slice(appUUIDS, encodeAppFromUUID)...)
	close(channel)

	s.relationService.EXPECT().WatchRelatedUnits(gomock.Any(), watchedUnitUUID, relUUID).Return(mockWatcher, nil)
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return(watcherID, nil)
	s.relationService.EXPECT().GetRelationUnitChanges(gomock.Any(), unitUUIDs, appUUIDS).Return(changes, nil)
}

func encodeUnitFromUUID(uuid coreunit.UUID) string {
	return relation.EncodeUnitUUID(uuid.String())
}

func encodeAppFromUUID(uuid coreapplication.ID) string {
	return relation.EncodeApplicationUUID(uuid.String())
}

type commitHookChangesSuite struct {
	testhelpers.IsolationSuite

	applicationService *MockApplicationService
	networkService     *MockNetworkService
	relationService    *MockRelationService

	uniter *UniterAPI
}

func TestCommitHookChangesSuite(t *testing.T) {
	tc.Run(t, &commitHookChangesSuite{})
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettings(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitTag := names.NewUnitTag("wordpress/0")
	relTag := names.NewRelationTag("wordpress:db mysql:db")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	appSettings := map[string]string{"wanda": "firebaugh", "deleteme": ""}
	unitSettings := map[string]string{"wanda": "firebaugh", "deleteme": ""}
	relKey := relationtesting.GenNewKey(c, relTag.Id())
	s.expectGetRelationUUIDByKey(relKey, relUUID)
	s.expectGetRelationUnit(relUUID, relUnitUUID, unitTag.Id())
	s.expectedSetRelationApplicationAndUnitSettings(coreunit.Name(unitTag.Id()), relUnitUUID, appSettings, unitSettings)

	canAccess := func(tag names.Tag) bool {
		return true
	}
	arg := params.RelationUnitSettings{
		Relation:            relTag.String(),
		Unit:                unitTag.String(),
		Settings:            unitSettings,
		ApplicationSettings: appSettings,
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(c.Context(), arg, canAccess)

	// assert
	c.Assert(err, tc.IsNil)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsBadUnitTag(c *tc.C) {
	// arrange
	arg := params.RelationUnitSettings{
		Unit: "machine-9",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(c.Context(), arg, nil)

	// assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsFailCanAccess(c *tc.C) {
	// arrange
	canAccess := func(tag names.Tag) bool {
		return false
	}
	arg := params.RelationUnitSettings{
		Unit: "unit-failauth-2",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(c.Context(), arg, canAccess)

	// assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) TestUpdateUnitAndApplicationSettingsBadRelationTag(c *tc.C) {
	// arrange
	canAccess := func(tag names.Tag) bool {
		return true
	}
	arg := params.RelationUnitSettings{
		Unit:     "unit-wordpress-2",
		Relation: "failme",
	}

	// act
	err := s.uniter.updateUnitAndApplicationSettings(c.Context(), arg, canAccess)

	// assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworks(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey1 := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relKey2 := relationtesting.GenNewKey(c, "wordpress:web nginx:web")
	relUUID1 := relationtesting.GenRelationUUID(c)
	relUUID2 := relationtesting.GenRelationUUID(c)
	relUnitUUID1 := relationtesting.GenRelationUnitUUID(c)
	relUnitUUID2 := relationtesting.GenRelationUnitUUID(c)

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey1, InScope: true},
		{Key: relKey2, InScope: true},
	}, nil)

	// For relation 1
	s.expectGetRelationUUIDByKey(relKey1, relUUID1)
	s.expectGetRelationUnit(relUUID1, relUnitUUID1, string(unitName))
	s.expectGetUnitRelationNetworkWithEgress(unitName, relKey1, "10.0.0.1", "192.168.0.0/24")
	s.expectedSetRelationApplicationAndUnitSettings(unitName, relUnitUUID1, nil, map[string]string{
		"ingress-address": "10.0.0.1",
		"egress-subnets":  "192.168.0.0/24",
	})

	// For relation 2
	s.expectGetRelationUUIDByKey(relKey2, relUUID2)
	s.expectGetRelationUnit(relUUID2, relUnitUUID2, string(unitName))
	s.expectGetUnitRelationNetwork(unitName, relKey2, "10.0.0.2")
	s.expectedSetRelationApplicationAndUnitSettings(unitName, relUnitUUID2, nil, map[string]string{
		"ingress-address": "10.0.0.2",
	})

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.IsNil)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksGetUnitUUIDError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	expectedErr := internalerrors.New("unit not found")

	// Set up expectations
	s.expectGetUnitUUID(unitName, coreunit.UUID(""), expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `getting UUID of unit "wordpress/0": unit not found`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksGetRelationsStatusError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	expectedErr := internalerrors.New("failed to get relations")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, nil, expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `getting relations for unit "wordpress/0": failed to get relations`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksGetRelationUUIDError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	expectedErr := internalerrors.New("relation not found")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey, InScope: true},
	}, nil)
	// Mock the error return
	s.relationService.EXPECT().GetRelationUUIDByKey(gomock.Any(), relKey).Return(corerelation.UUID(""), expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `getting relation UUID: relation not found`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksGetRelationUnitError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relUUID := relationtesting.GenRelationUUID(c)
	expectedErr := internalerrors.New("relation unit not found")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey, InScope: true},
	}, nil)
	s.expectGetRelationUUIDByKey(relKey, relUUID)
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, unitName).Return(corerelation.UnitUUID(""), expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `getting relation uni UUIDt: relation unit not found`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksGetUnitRelationNetworkError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	expectedErr := internalerrors.New("network not found")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey, InScope: true},
	}, nil)
	s.expectGetRelationUUIDByKey(relKey, relUUID)
	s.expectGetRelationUnit(relUUID, relUnitUUID, string(unitName))
	s.expectGetUnitRelationNetworkError(unitName, relKey, expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `getting relation network: network not found`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksSetRelationSettingsError(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relUUID := relationtesting.GenRelationUUID(c)
	relUnitUUID := relationtesting.GenRelationUnitUUID(c)
	expectedErr := internalerrors.New("failed to set settings")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey, InScope: true},
	}, nil)
	s.expectGetRelationUUIDByKey(relKey, relUUID)
	s.expectGetRelationUnit(relUUID, relUnitUUID, string(unitName))
	s.expectGetUnitRelationNetwork(unitName, relKey, "10.0.0.1")
	s.relationService.EXPECT().SetRelationApplicationAndUnitSettings(
		gomock.Any(),
		unitName,
		relUnitUUID,
		nil,
		map[string]string{"ingress-address": "10.0.0.1"},
	).Return(expectedErr)

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.NotNil)
	c.Assert(err.Error(), tc.Matches, `setting relation application and unit settings: failed to set settings`)
}

func (s *commitHookChangesSuite) TestSetUnitRelationNetworksSkipsRelationsNotInScope(c *tc.C) {
	// arrange
	defer s.setupMocks(c).Finish()
	unitName := coreunit.Name("wordpress/0")
	unitUUID := unittesting.GenUnitUUID(c)
	relKey1 := relationtesting.GenNewKey(c, "wordpress:db mysql:db")
	relKey2 := relationtesting.GenNewKey(c, "wordpress:web nginx:web")

	// Set up expectations
	s.expectGetUnitUUID(unitName, unitUUID, nil)
	s.expectGetRelationsStatusForUnit(unitUUID, []relation.RelationUnitStatus{
		{Key: relKey1, InScope: false}, // This relation is not in scope and should be skipped
		{Key: relKey2, InScope: true},
	}, nil)

	// Only relation 2 should be processed
	relUUID2 := relationtesting.GenRelationUUID(c)
	relUnitUUID2 := relationtesting.GenRelationUnitUUID(c)
	s.expectGetRelationUUIDByKey(relKey2, relUUID2)
	s.expectGetRelationUnit(relUUID2, relUnitUUID2, string(unitName))
	s.expectGetUnitRelationNetwork(unitName, relKey2, "10.0.0.2")
	s.expectedSetRelationApplicationAndUnitSettings(unitName, relUnitUUID2, nil, map[string]string{
		"ingress-address": "10.0.0.2",
	})

	// act
	err := s.uniter.setUnitRelationNetworks(c.Context(), unitName)

	// assert
	c.Assert(err, tc.IsNil)
}

func (s *commitHookChangesSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.relationService = NewMockRelationService(ctrl)
	s.networkService = NewMockNetworkService(ctrl)

	s.uniter = &UniterAPI{
		logger: loggertesting.WrapCheckLog(c),

		applicationService: s.applicationService,
		networkService:     s.networkService,
		relationService:    s.relationService,
	}

	c.Cleanup(func() {
		s.applicationService = nil
		s.networkService = nil
		s.relationService = nil
		s.uniter = nil
	})

	return ctrl
}

func (s *commitHookChangesSuite) expectGetRelationUUIDByKey(key corerelation.Key, relUUID corerelation.UUID) {
	s.relationService.EXPECT().GetRelationUUIDByKey(gomock.Any(), key).Return(relUUID, nil)
}

func (s *commitHookChangesSuite) expectGetRelationUnit(relUUID corerelation.UUID, uuid corerelation.UnitUUID, unitTagID string) {
	s.relationService.EXPECT().GetRelationUnit(gomock.Any(), relUUID, coreunit.Name(unitTagID)).Return(uuid, nil)
}

func (s *commitHookChangesSuite) expectedSetRelationApplicationAndUnitSettings(unitName coreunit.Name, uuid corerelation.UnitUUID, appSettings, unitSettings map[string]string) {
	s.relationService.EXPECT().SetRelationApplicationAndUnitSettings(gomock.Any(), unitName, uuid, appSettings, unitSettings).Return(nil)
}

func (s *commitHookChangesSuite) expectGetUnitRelationNetwork(unitName coreunit.Name, key corerelation.Key,
	ingress string) {
	s.networkService.EXPECT().GetUnitRelationNetwork(gomock.Any(), unitName, key).Return(domainnetwork.UnitNetwork{
		IngressAddresses: []string{ingress},
	}, nil)
}

func (s *commitHookChangesSuite) expectGetUnitRelationNetworkWithEgress(unitName coreunit.Name, key corerelation.Key,
	ingress, egress string) {
	s.networkService.EXPECT().GetUnitRelationNetwork(gomock.Any(), unitName, key).Return(domainnetwork.UnitNetwork{
		IngressAddresses: []string{ingress},
		EgressSubnets:    []string{egress},
	}, nil)
}

func (s *commitHookChangesSuite) expectGetUnitRelationNetworkError(unitName coreunit.Name, key corerelation.Key, err error) {
	s.networkService.EXPECT().GetUnitRelationNetwork(gomock.Any(), unitName, key).Return(domainnetwork.UnitNetwork{}, err)
}

func (s *commitHookChangesSuite) expectGetUnitUUID(unitName coreunit.Name, unitUUID coreunit.UUID, err error) {
	s.applicationService.EXPECT().GetUnitUUID(gomock.Any(), unitName).Return(unitUUID, err)
}

func (s *commitHookChangesSuite) expectGetRelationsStatusForUnit(unitUUID coreunit.UUID, relations []relation.RelationUnitStatus, err error) {
	s.relationService.EXPECT().GetRelationsStatusForUnit(gomock.Any(), unitUUID).Return(relations, err)
}

type apiAddresserSuite struct {
	watcherRegistry    *MockWatcherRegistry
	apiAddressAccessor *MockAPIAddressAccessor
}

func TestAPIAddresserSuite(t *testing.T) {
	tc.Run(t, &apiAddresserSuite{})
}

// TestAPIAddresses ensures that the APIAddresser is wired up within the uniter.
// Functionality of the APIAddresser is tested in apiserver/common.
func (s *apiAddresserSuite) TestAPIAddresses(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.apiAddressAccessor.EXPECT().GetAllAPIAddressesForAgents(gomock.Any()).Return([]string{"0.1.2.3:1234"}, nil)

	uniter := &UniterAPI{
		APIAddresser: common.NewAPIAddresser(s.apiAddressAccessor, s.watcherRegistry),
	}

	// Act
	result, err := uniter.APIAddresses(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *apiAddresserSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.apiAddressAccessor = NewMockAPIAddressAccessor(ctrl)
	s.watcherRegistry = NewMockWatcherRegistry(ctrl)

	c.Cleanup(func() {
		s.apiAddressAccessor = nil
		s.watcherRegistry = nil
	})
	return ctrl
}
