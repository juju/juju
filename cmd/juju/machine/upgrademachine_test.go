// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/cmd/juju/machine/mocks"
	"github.com/juju/juju/rpc/params"
)

type UpgradeMachineSuite struct {
	testing.IsolationSuite

	statusExpectation   *statusExpectation
	prepareExpectation  *upgradeMachinePrepareExpectation
	completeExpectation *upgradeMachineCompleteExpectation
}

var _ = gc.Suite(&UpgradeMachineSuite{})

func (s *UpgradeMachineSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.statusExpectation = &statusExpectation{
		status: &params.FullStatus{
			Machines: map[string]params.MachineStatus{
				"1": {Id: "1"},
				"2": {
					Id: "2",
					Containers: map[string]params.MachineStatus{
						"2/lxd/0": {Id: "2/lxd/0"},
					},
				},
			},
			Applications: map[string]params.ApplicationStatus{
				"foo": {
					Units: map[string]params.UnitStatus{
						"foo/1": {
							Machine: "1",
							Subordinates: map[string]params.UnitStatus{
								"sub/1": {},
							},
						},
						"foo/2": {
							Machine: "2/lxd/0",
							Subordinates: map[string]params.UnitStatus{
								"sub/2": {},
							},
						},
					},
				},
			},
		},
	}
	s.prepareExpectation = &upgradeMachinePrepareExpectation{gomock.Any(), gomock.Any(), gomock.Any()}
	s.completeExpectation = &upgradeMachineCompleteExpectation{gomock.Any()}

	// TODO: remove this patch once we removed all the old series from tests in current package.
	s.PatchValue(&machine.SupportedJujuSeries,
		func(time.Time, string, string) (set.Strings, error) {
			return set.NewStrings(
				"centos7", "centos9", "genericlinux",
				"jammy", "focal", "bionic", "xenial",
			), nil
		},
	)
}

const (
	machineArg   = "1"
	containerArg = "2/lxd/0"

	channelArg = "20.04/stable"
	baseArg    = "ubuntu@20.04"
)

func (s *UpgradeMachineSuite) runUpgradeMachineCommand(c *gc.C, args ...string) error {
	_, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", args...)
	return err
}

func (s *UpgradeMachineSuite) ctxWithConfirmation(c *gc.C, confirmation string) *cmd.Context {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stderr = &stderr
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin
	stdin.WriteString(confirmation)

	return ctx
}

func (s *UpgradeMachineSuite) runUpgradeMachineCommandWithConfirmation(
	c *gc.C, confirmation string, args ...string,
) (*cmd.Context, error) {
	ctx := s.ctxWithConfirmation(c, confirmation)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockStatusAPI := mocks.NewMockStatusAPI(ctrl)
	mockUpgradeMachineAPI := mocks.NewMockUpgradeMachineAPI(ctrl)

	uExp := mockUpgradeMachineAPI.EXPECT()
	prep := s.prepareExpectation
	uExp.UpgradeSeriesPrepare(prep.machineArg, prep.channelArg, prep.force).AnyTimes()
	uExp.UpgradeSeriesComplete(s.completeExpectation.machineNumber).AnyTimes()

	mockStatusAPI.EXPECT().Status(gomock.Nil()).AnyTimes().Return(s.statusExpectation.status, nil)

	com := machine.NewUpgradeMachineCommandForTest(mockStatusAPI, mockUpgradeMachineAPI)

	err := cmdtesting.InitCommand(com, args)
	if err != nil {
		return nil, err
	}
	err = com.Run(ctx)
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (s *UpgradeMachineSuite) TestPrepareCommandMachines(c *gc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{machineArg, channelArg, gomock.Eq(false)}
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandContainers(c *gc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{containerArg, channelArg, gomock.Eq(false)}
	err := s.runUpgradeMachineCommand(c, containerArg, machine.PrepareCommand, baseArg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestTooFewArgs(c *gc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg)
	c.Assert(err, gc.ErrorMatches, "wrong number of arguments")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptForceOption(c *gc.C) {
	s.prepareExpectation = &upgradeMachinePrepareExpectation{machineArg, channelArg, gomock.Eq(true)}
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "--force")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAbortOnFailedConfirmation(c *gc.C) {
	_, err := s.runUpgradeMachineCommandWithConfirmation(c, "n", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, gc.ErrorMatches, "upgrade machine: aborted")
}

func (s *UpgradeMachineSuite) TestUpgradeCommandShouldNotAcceptInvalidPrepCommands(c *gc.C) {
	invalidPrepCommand := "actuate"
	err := s.runUpgradeMachineCommand(c, machineArg, invalidPrepCommand, baseArg)
	c.Assert(err, gc.ErrorMatches,
		".* \"actuate\" is an invalid upgrade-machine command; valid commands are: prepare, complete.")
}

func (s *UpgradeMachineSuite) TestUpgradeCommandShouldNotAcceptInvalidMachineArgs(c *gc.C) {
	invalidMachineArg := "machine5"
	err := s.runUpgradeMachineCommand(c, invalidMachineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, gc.ErrorMatches, "\"machine5\" is an invalid machine name")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldOnlyAcceptSupportedSeries(c *gc.C) {
	badSeries := "Combative Caribou"
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, badSeries)
	c.Assert(err, gc.ErrorMatches, ".* is an unsupported series")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldSupportSeriesRegardlessOfCase(c *gc.C) {
	capitalizedCaseXenial := "Xenial"
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, capitalizedCaseXenial)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestCompleteCommand(c *gc.C) {
	s.completeExpectation.machineNumber = machineArg
	err := s.runUpgradeMachineCommand(c, machineArg, machine.CompleteCommand)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestCompleteCommandDoesNotAcceptSeries(c *gc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.CompleteCommand, baseArg)
	c.Assert(err, gc.ErrorMatches, "wrong number of arguments")
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYes(c *gc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "--yes")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYesAbbreviation(c *gc.C) {
	err := s.runUpgradeMachineCommand(c, machineArg, machine.PrepareCommand, baseArg, "-y")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldPromptUserForConfirmation(c *gc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctx.Stderr.(*bytes.Buffer).String(), gc.Matches, `(?s).*Continue \[y\/N\]\? .*`)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldIndicateOnlySubordinatesOnMachine(c *gc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "y", machineArg, machine.PrepareCommand, baseArg)
	c.Assert(err, jc.ErrorIsNil)

	out := ctx.Stdout.(*bytes.Buffer).String()
	c.Check(strings.Contains(out, "sub/1"), jc.IsTrue)
	c.Check(strings.Contains(out, "sub/2"), jc.IsFalse)
}

func (s *UpgradeMachineSuite) TestPrepareCommandShouldAcceptYesFlagAndNotPrompt(c *gc.C) {
	ctx, err := s.runUpgradeMachineCommandWithConfirmation(c, "n", machineArg, machine.PrepareCommand, baseArg, "-y")
	c.Assert(err, jc.ErrorIsNil)

	//There is no confirmation message since the `-y/--yes` flag is being used to avoid the prompt.
	confirmationMessage := ""

	finishedMessage := fmt.Sprintf(machine.UpgradeMachinePrepareFinishedMessage, machineArg)
	displayedMessage := strings.Join([]string{confirmationMessage, finishedMessage}, "") + "\n"
	out := ctx.Stderr.(*bytes.Buffer).String()
	c.Assert(out, gc.Equals, displayedMessage)
	c.Assert(out, jc.Contains, fmt.Sprintf("juju upgrade-machine %s complete", machineArg))
}

type statusExpectation struct {
	status interface{}
}

type upgradeMachinePrepareExpectation struct {
	machineArg, channelArg, force interface{}
}

type upgradeMachineCompleteExpectation struct {
	machineNumber interface{}
}
