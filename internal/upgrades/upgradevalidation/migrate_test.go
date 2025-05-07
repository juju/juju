// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradevalidation_test

import (
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/upgrades/upgradevalidation"
	"github.com/juju/juju/internal/upgrades/upgradevalidation/mocks"
	"github.com/juju/juju/state"
)

func makeBases(os string, vers []string) []state.Base {
	bases := make([]state.Base, len(vers))
	for i, vers := range vers {
		bases[i] = state.Base{OS: os, Channel: vers}
	}
	return bases
}

var _ = tc.Suite(&migrateSuite{})

type migrateSuite struct {
	testhelpers.IsolationSuite

	st           *mocks.MockState
	agentService *mocks.MockModelAgentService
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju3(c *tc.C) {
	defer s.setupMocks(c).Finish()

	validators := upgradevalidation.ValidatorsForModelMigrationSource()

	checker := upgradevalidation.NewModelUpgradeCheck(s.st, "test-model", s.agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}

func (s *migrateSuite) TestValidatorsForModelMigrationSourceJuju31(c *tc.C) {
	defer s.setupMocks(c).Finish()

	validators := upgradevalidation.ValidatorsForModelMigrationSource()

	checker := upgradevalidation.NewModelUpgradeCheck(s.st, "test-model", s.agentService, validators...)
	blockers, err := checker.Validate()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(blockers, tc.IsNil)
}

func (s *migrateSuite) initializeMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.st = mocks.NewMockState(ctrl)
	s.agentService = mocks.NewMockModelAgentService(ctrl)
	return ctrl
}

func (s *migrateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.initializeMocks(c)

	s.PatchValue(&upgradevalidation.SupportedJujuBases, func() []base.Base {
		return transform.Slice([]string{"ubuntu@24.04", "ubuntu@22.04", "ubuntu@20.04"}, base.MustParseBaseFromString)
	})

	// - check if the model has win machines;
	s.st.EXPECT().MachineCountForBase(makeBases("ubuntu", []string{"24.04/stable", "22.04/stable", "20.04/stable"})).Return(nil, nil)
	s.st.EXPECT().AllMachinesCount().Return(0, nil)

	return ctrl
}
