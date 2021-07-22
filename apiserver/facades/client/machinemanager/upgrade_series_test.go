// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type UpgradeSeriesSuiteValidate struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpgradeSeriesSuiteValidate{})

func (s *UpgradeSeriesSuiteValidate) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	applications := []Application{application}

	unit := NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("foo/0"))
	units := []Unit{unit}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, "focal", false).Return(nil)
	validator.EXPECT().ValidateMachine(machine).Return(nil)
	validator.EXPECT().ValidateUnits(units).Return(nil)

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

func (s *UpgradeSeriesSuiteValidate) TestValidateWithValidateSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(errors.New("boom"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

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

func (s *UpgradeSeriesSuiteValidate) TestValidateApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	applications := []Application{application}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(nil)
	validator.EXPECT().ValidateApplications(applications, "focal", false).Return(errors.New("boom"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

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

type UpgradeSeriesSuitePrepare struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&UpgradeSeriesSuitePrepare{})

func (s UpgradeSeriesSuitePrepare) TestPrepare(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	applications := []Application{application}

	unit := NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []Unit{unit}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, "focal")
	machine.EXPECT().Series().Return("bionic").Times(2)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, `started upgrade series from "bionic" to "focal"`)

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)
	state.EXPECT().ApplicationsFromMachine(machine).Return(applications, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0")
	validator.EXPECT().ValidateApplications(applications, "focal", false)
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := NewMockAuthorizer(ctrl)

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareWithRollback(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []Unit{unit}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, "focal")
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().RemoveUpgradeSeriesLock()

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(errors.New("bad"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := NewMockAuthorizer(ctrl)

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "focal", false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareWithRollbackError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := NewMockUnit(ctrl)
	unit.EXPECT().UnitTag().Return(names.NewUnitTag("app/0"))

	units := []Unit{unit}

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Units().Return(units, nil)
	machine.EXPECT().CreateUpgradeSeriesLock([]string{"app/0"}, "focal")
	machine.EXPECT().Series().Return("bionic")
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().RemoveUpgradeSeriesLock().Return(errors.New("boom"))

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateSeries("focal", "bionic", "machine-0").Return(errors.New("bad"))
	validator.EXPECT().ValidateMachine(machine).Return(nil)

	authorizer := NewMockAuthorizer(ctrl)

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "focal", false)
	c.Assert(err, gc.ErrorMatches, `boom occurred while cleaning up from: bad`)
}

func (s UpgradeSeriesSuitePrepare) TestPrepareValidationFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)

	state := NewMockUpgradeSeriesState(ctrl)
	state.EXPECT().MachineFromTag("machine-0").Return(machine, nil)

	validator := NewMockUpgradeSeriesValidator(ctrl)
	validator.EXPECT().ValidateMachine(machine).Return(errors.New("bad"))

	authorizer := NewMockAuthorizer(ctrl)

	api := NewUpgradeSeriesAPI(state, validator, authorizer)
	err := api.Prepare("machine-0", "focal", false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

type ValidatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ValidatorSuite{})

func (s ValidatorSuite) TestValidateApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	localApp := NewMockApplication(ctrl)
	localApp.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source: corecharm.Local.String(),
	})
	storeApp := NewMockApplication(ctrl)
	storeApp.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source: corecharm.CharmStore.String(),
	})
	charmhubApp := NewMockApplication(ctrl)
	charmhubApp.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Source: corecharm.CharmHub.String(),
	})
	applications := []Application{
		localApp,
		storeApp,
		charmhubApp,
	}

	localValidator := NewMockUpgradeSeriesValidator(ctrl)
	localValidator.EXPECT().ValidateApplications([]Application{localApp, storeApp}, "focal", false)
	removeValidator := NewMockUpgradeSeriesValidator(ctrl)
	removeValidator.EXPECT().ValidateApplications([]Application{charmhubApp}, "focal", false)

	validator := upgradeSeriesValidator{
		localValidator:  localValidator,
		removeValidator: removeValidator,
	}

	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateApplicationsWithNoOrigin(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(nil)
	applications := []Application{application}

	localValidator := NewMockUpgradeSeriesValidator(ctrl)
	localValidator.EXPECT().ValidateApplications(applications, "focal", false)
	removeValidator := NewMockUpgradeSeriesValidator(ctrl)
	removeValidator.EXPECT().ValidateApplications([]Application(nil), "focal", false)

	validator := upgradeSeriesValidator{
		localValidator:  localValidator,
		removeValidator: removeValidator,
	}

	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateMachine(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(false, nil)

	validator := upgradeSeriesValidator{}

	err := validator.ValidateMachine(machine)
	c.Assert(err, jc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateMachineIsManager(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Tag().Return(names.NewMachineTag("0"))
	machine.EXPECT().IsManager().Return(true)

	validator := upgradeSeriesValidator{}

	err := validator.ValidateMachine(machine)
	c.Assert(err, gc.ErrorMatches, `machine-0 is a controller and cannot be targeted for series upgrade`)
}

func (s ValidatorSuite) TestValidateMachineIsLockedForSeriesUpgrade(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	machine := NewMockMachine(ctrl)
	machine.EXPECT().Id().Return("0")
	machine.EXPECT().IsManager().Return(false)
	machine.EXPECT().IsLockedForSeriesUpgrade().Return(true, nil)
	machine.EXPECT().UpgradeSeriesStatus().Return(model.UpgradeSeriesPrepareRunning, nil)

	validator := upgradeSeriesValidator{}

	err := validator.ValidateMachine(machine)
	c.Assert(err, gc.ErrorMatches, `upgrade series lock found for "0"; series upgrade is in the "prepare running" state`)
}

func (s ValidatorSuite) TestValidateUnits(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := NewMockUnit(ctrl)
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Idle}, nil)
	unit.EXPECT().Status().Return(status.StatusInfo{Status: status.Active}, nil)
	units := []Unit{unit}

	validator := upgradeSeriesValidator{}

	err := validator.ValidateUnits(units)
	c.Assert(err, jc.ErrorIsNil)
}

func (s ValidatorSuite) TestValidateUnitsNotIdle(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := NewMockUnit(ctrl)
	unit.EXPECT().Name().Return("foo/0")
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Blocked}, nil)
	units := []Unit{unit}

	validator := upgradeSeriesValidator{}

	err := validator.ValidateUnits(units)
	c.Assert(err, gc.ErrorMatches, `unit foo/0 is not ready to start a series upgrade; its agent status is: "blocked" `)
}

func (s ValidatorSuite) TestValidateUnitsInErrorState(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	unit := NewMockUnit(ctrl)
	unit.EXPECT().Name().Return("foo/0")
	unit.EXPECT().AgentStatus().Return(status.StatusInfo{Status: status.Idle}, nil)
	unit.EXPECT().Status().Return(status.StatusInfo{Status: status.Error}, nil)
	units := []Unit{unit}

	validator := upgradeSeriesValidator{}

	err := validator.ValidateUnits(units)
	c.Assert(err, gc.ErrorMatches, `unit foo/0 is not ready to start a series upgrade; its status is: "error" `)
}

type StateValidatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&StateValidatorSuite{})

func (s StateValidatorSuite) TestValidateApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"focal", "bionic"}}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []Application{application}

	validator := stateSeriesValidator{}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationsWithFallbackSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	url := charm.MustParseURL("cs:focal/foo-1")

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()
	ch.EXPECT().URL().Return(url)

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []Application{application}

	validator := stateSeriesValidator{}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s StateValidatorSuite) TestValidateApplicationsWithUnsupportedSeries(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"xenial", "bionic"}}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()
	ch.EXPECT().String().Return("cs:foo-1")

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []Application{application}

	validator := stateSeriesValidator{}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, gc.ErrorMatches, `series "focal" not supported by charm "cs:foo-1", supported series are: xenial, bionic`)
}

func (s StateValidatorSuite) TestValidateApplicationsWithUnsupportedSeriesWithForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	ch := NewMockCharm(ctrl)
	ch.EXPECT().Meta().Return(&charm.Meta{Series: []string{"xenial", "bionic"}}).MinTimes(2)
	ch.EXPECT().Manifest().Return(nil).AnyTimes()

	application := NewMockApplication(ctrl)
	application.EXPECT().Charm().Return(ch, false, nil)

	applications := []Application{application}

	validator := stateSeriesValidator{}
	err := validator.ValidateApplications(applications, "focal", true)
	c.Assert(err, jc.ErrorIsNil)
}

type CharmhubValidatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CharmhubValidatorSuite{})

func (s CharmhubValidatorSuite) TestValidateApplications(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	applications := []Application{application}

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, jc.ErrorIsNil)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithNoRevision(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{})
	application.EXPECT().Name().Return("foo")

	applications := []Application{application}

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, gc.ErrorMatches, `no revision found for application "foo"`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithClientRefreshError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{},
	}, errors.Errorf("bad"))

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	applications := []Application{application}

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, gc.ErrorMatches, `bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithRefreshError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	applications := []Application{application}

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplications(applications, "focal", false)
	c.Assert(err, gc.ErrorMatches, `unable to locate application with series focal: bad`)
}

func (s CharmhubValidatorSuite) TestValidateApplicationsWithRefreshErrorAndForce(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	client := NewMockCharmhubClient(ctrl)
	client.EXPECT().Refresh(gomock.Any(), gomock.Any()).Return([]transport.RefreshResponse{
		{Error: &transport.APIError{
			Message: "bad",
		}},
	}, nil)

	revision := 1

	application := NewMockApplication(ctrl)
	application.EXPECT().CharmOrigin().Return(&state.CharmOrigin{
		Revision: &revision,
		Platform: &state.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Series:       "bionic",
		},
	})

	applications := []Application{application}

	validator := charmhubSeriesValidator{
		client: client,
	}
	err := validator.ValidateApplications(applications, "focal", true)
	c.Assert(err, jc.ErrorIsNil)
}
