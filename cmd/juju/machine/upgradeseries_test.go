// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"bytes"
	"errors"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/machine/mocks"
	"github.com/juju/juju/testing/gomockmatchers"
)

type UpgradeSeriesSuite struct {
}

var _ = gc.Suite(&UpgradeSeriesSuite{})

const machineArg = "1"
const seriesArg = "xenial"

func (s *UpgradeSeriesSuite) SetUpTest(c *gc.C) {}

func runUpgradeSeriesCommand(c *gc.C, args ...string) error {
	err := runUpgradeSeriesCommandWithConfirmation(c, "y", args...)
	return err
}

func runUpgradeSeriesCommandWithConfirmation(c *gc.C, confirmation string, args ...string) error {
	mockController := gomock.NewController(c)
	mockUpgradeSeriesAPI := mocks.NewMockUpgradeMachineSeriesAPI(mockController)
	// Stub for CLI arg testing
	mockUpgradeSeriesAPI.EXPECT().UpgradeSeriesPrepare(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	return runUpgradeSeriesCommandGivenMock(c, mockUpgradeSeriesAPI, confirmation, args...)
}

func runUpgradeSeriesCommandGivenMock(c *gc.C, mockUpgradeSeriesAPI *mocks.MockUpgradeMachineSeriesAPI, confirmation string, args ...string) error {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stderr = &stderr
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin
	stdin.WriteString(confirmation)

	com := machine.NewUpgradeSeriesCommandForTest(mockUpgradeSeriesAPI)

	err = cmdtesting.InitCommand(com, args)
	if err != nil {
		return err
	}

	err = com.Run(ctx)
	if err != nil {
		return err
	}

	if stderr.String() != "" {
		return errors.New(stderr.String())
	}

	return nil
}

func (s *UpgradeSeriesSuite) TestPrepareCommand(c *gc.C) {
	mockController := gomock.NewController(c)
	defer mockController.Finish()
	mockUpgradeSeriesAPI := mocks.NewMockUpgradeMachineSeriesAPI(mockController)
	mockUpgradeSeriesAPI.EXPECT().UpgradeSeriesPrepare(machineArg, seriesArg, gomockmatcher.OfTypeBool(false))

	err := runUpgradeSeriesCommandGivenMock(c, mockUpgradeSeriesAPI, "y", machine.PrepareCommand, machineArg, seriesArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestPrepareCommandShouldAcceptForceOption(c *gc.C) {
	mockController := gomock.NewController(c)
	defer mockController.Finish()
	mockUpgradeSeriesAPI := mocks.NewMockUpgradeMachineSeriesAPI(mockController)
	mockUpgradeSeriesAPI.EXPECT().UpgradeSeriesPrepare(machineArg, seriesArg, gomockmatcher.OfTypeBool(true))

	err := runUpgradeSeriesCommandGivenMock(c, mockUpgradeSeriesAPI, "y", machine.PrepareCommand, machineArg, seriesArg, "--force")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestPrepareCommandShouldAbortOnFailedConfirmation(c *gc.C) {
	err := runUpgradeSeriesCommandWithConfirmation(c, "n", machine.PrepareCommand, machineArg, seriesArg)
	c.Assert(err, gc.ErrorMatches, "upgrade series: aborted")
}

func (s *UpgradeSeriesSuite) TestUpgradeCommandShouldNotAcceptInvalidPrepCommands(c *gc.C) {
	invalidPrepCommand := "actuate"
	err := runUpgradeSeriesCommand(c, invalidPrepCommand, machineArg, seriesArg)
	c.Assert(err, gc.ErrorMatches, ".* \"actuate\" is an invalid upgrade-series command")
}

func (s *UpgradeSeriesSuite) TestUpgradeCommandShouldNotAcceptInvalidMachineArgs(c *gc.C) {
	invalidMachineArg := "machine5"
	err := runUpgradeSeriesCommand(c, machine.PrepareCommand, invalidMachineArg, seriesArg)
	c.Assert(err, gc.ErrorMatches, "\"machine5\" is an invalid machine name")
}

func (s *UpgradeSeriesSuite) TestPrepareCommandShouldOnlyAcceptSupportedSeries(c *gc.C) {
	BadSeries := "Combative Caribou"
	err := runUpgradeSeriesCommand(c, machine.PrepareCommand, machineArg, BadSeries)
	c.Assert(err, gc.ErrorMatches, ".* is an unsupported series")
}

func (s *UpgradeSeriesSuite) TestPrepareCommandShouldSupportSeriesRegardlessOfCase(c *gc.C) {
	capitalizedCaseXenial := "Xenial"
	err := runUpgradeSeriesCommand(c, machine.PrepareCommand, machineArg, capitalizedCaseXenial)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestCompleteCommand(c *gc.C) {
	mockController := gomock.NewController(c)
	defer mockController.Finish()
	mockUpgradeSeriesAPI := mocks.NewMockUpgradeMachineSeriesAPI(mockController)
	mockUpgradeSeriesAPI.EXPECT().UpgradeSeriesComplete(machineArg)

	err := runUpgradeSeriesCommandGivenMock(c, mockUpgradeSeriesAPI, "y", machine.CompleteCommand, machineArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeSeriesSuite) TestCompleteCommandDoesNotAcceptSeries(c *gc.C) {
	err := runUpgradeSeriesCommand(c, machine.CompleteCommand, machineArg, seriesArg)
	c.Assert(err, gc.ErrorMatches, "wrong number of arguments")
}
