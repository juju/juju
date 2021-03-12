// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
)

type UpgradeSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpgradeSeriesSuite{})

func (s *UpgradeSeriesSuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	applications := []Application{application}

	unit := NewMockUnit(ctrl)
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Idle}, nil)
	unit.EXPECT().Status().Return(status.StatusInfo{Status: status.Active}, nil)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("foo/0"))
	units := []Unit{unit}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Units().Return(units, nil)

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, "focal", false).Return(nil)

	authorizer := NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []ValidationEntity{
		{Tag: "machine-0", Series: "focal"},
	}

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, []ValidationResult{
		{UnitNames: []string{"foo/0"}},
	})
}

func (s *UpgradeSeriesSuite) TestValidateIsManager(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(true)

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)

	authorizer := NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []ValidationEntity{
		{Tag: "machine-0", Series: "focal"},
	}

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, `machine-0 is a controller and cannot be targeted for series upgrade`)
}

func (s *UpgradeSeriesSuite) TestValidateIsLockedForSeriesUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(true, nil)
	machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesError, nil)
	machine.EXPECT().Id().Return("machine-0")

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)

	authorizer := NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []ValidationEntity{
		{Tag: "machine-0", Series: "focal"},
	}

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, `upgrade series lock found for "machine-0"; series upgrade is in the "error" state`)
}

func (s *UpgradeSeriesSuite) TestValidateWithValidateSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)
	machine.EXPECT().Series().Return("bionic")

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(errors.New("boom"))

	authorizer := NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []ValidationEntity{
		{Tag: "machine-0", Series: "focal"},
	}

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, `boom`)
}

func (s *UpgradeSeriesSuite) TestValidateApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	applications := []Application{application}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)
	machine.EXPECT().Series().Return("bionic")

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, "focal", false).Return(errors.New("boom"))

	authorizer := NewMockAuthorizer(ctrl)
	authorizer.EXPECT().CanRead().Return(nil)

	entities := []ValidationEntity{
		{Tag: "machine-0", Series: "focal"},
	}

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	result, err := api.Validate(entities)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result[0].Error, gc.ErrorMatches, `boom`)
}
