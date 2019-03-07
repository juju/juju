// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apiinstancemutater "github.com/juju/juju/api/instancemutater"
	apimocks "github.com/juju/juju/api/instancemutater/mocks"
	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
)

type mutaterSuite struct {
	jujutesting.BaseSuite

	tag names.MachineTag

	logger  *mocks.MockLogger
	machine *apimocks.MockMutaterMachine
}

var _ = gc.Suite(&mutaterSuite{})

func (s *mutaterSuite) SetUpTest(c *gc.C) {
	s.tag = names.NewMachineTag("2")
	s.BaseSuite.SetUpTest(c)
}

func (s *mutaterSuite) TestUnitsChanged(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	unitNames := []string{"ubuntu/0"}
	info := apiinstancemutater.ProfileInfo{
		Changes:         false,
		CurrentProfiles: []string{"juju-default-lxd-profile-0"},
	}

	mExp := s.machine.EXPECT()
	mExp.CharmProfilingInfo(unitNames).Return(info, nil)

	c.Assert(instancemutater.UnitsChanged(s.logger, s.machine, unitNames), jc.ErrorIsNil)
}

func (s *mutaterSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.logger = mocks.NewMockLogger(ctrl)
	lExp := s.logger.EXPECT()
	lExp.Tracef(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	s.machine = apimocks.NewMockMutaterMachine(ctrl)
	s.machine.EXPECT().Tag().Return(s.tag).AnyTimes()

	return ctrl
}
