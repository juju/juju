// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshprovisioner_test

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/manual"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	"github.com/juju/juju/environs/manual/sshprovisioner/mocks"
)

type removerSuite struct {
	testing.IsolationSuite

	host string

	mockCommand *mocks.MockCommandExec
	mockRunner  *mocks.MockCommandRunner
}

var _ = gc.Suite(&removerSuite{})

func (s *removerSuite) TestRemoveMachineWithNoUbuntuUser(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUbuntuLogin(errors.New("nope"))

	args := manual.RemoveMachineArgs{
		Host:        s.host,
		CommandExec: s.mockCommand,
	}
	err := sshprovisioner.RemoveMachine(args)
	c.Assert(err, gc.ErrorMatches, "unable to login to remove machine on .*")
}

func (s *removerSuite) TestRemoveMachineWhenProvisionError(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUbuntuLogin(nil)
	s.expectProvisionError(errors.New("nope"))

	args := manual.RemoveMachineArgs{
		Host:        s.host,
		CommandExec: s.mockCommand,
	}
	err := sshprovisioner.RemoveMachine(args)
	c.Assert(err, gc.ErrorMatches, "error checking if provisioned: nope")
}

func (s *removerSuite) TestRemoveMachineWhenNotProvisioned(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUbuntuLogin(nil)
	s.expectNotProvision()

	args := manual.RemoveMachineArgs{
		Host:        s.host,
		CommandExec: s.mockCommand,
	}
	err := sshprovisioner.RemoveMachine(args)
	c.Assert(err, gc.ErrorMatches, "machine not provisioned")
}

func (s *removerSuite) TestRemoveMachine(c *gc.C) {
	ctrl := s.setup(c)
	defer ctrl.Finish()

	s.expectUbuntuLogin(nil)
	s.expectProvisioned()
	s.expectTearDown()

	args := manual.RemoveMachineArgs{
		Host:        s.host,
		CommandExec: s.mockCommand,
	}
	err := sshprovisioner.RemoveMachine(args)
	c.Assert(err, gc.IsNil)
}

func (s *removerSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.host = "10.10.0.3"

	s.mockCommand = mocks.NewMockCommandExec(ctrl)
	s.mockRunner = mocks.NewMockCommandRunner(ctrl)

	return ctrl
}

func (s *removerSuite) expectUbuntuLogin(err error) {
	s.mockCommand.EXPECT().Command(fmt.Sprintf("ubuntu@%s", s.host), []string{"sudo", "-n", "true"}).Return(s.mockRunner)
	s.mockRunner.EXPECT().Run().Return(err)
}

func (s *removerSuite) expectNotProvision() {
	s.mockCommand.EXPECT().Command(fmt.Sprintf("ubuntu@%s", s.host), []string{"/bin/bash"}).Return(s.mockRunner)
	rExp := s.mockRunner.EXPECT()
	rExp.SetStdout(gomock.Any()).SetArg(0, *bytes.NewBuffer([]byte{}))
	rExp.SetStderr(gomock.Any())
	rExp.SetStdin(gomock.Any())
	rExp.Run()
}

func (s *removerSuite) expectProvisionError(err error) {
	s.mockCommand.EXPECT().Command(fmt.Sprintf("ubuntu@%s", s.host), []string{"/bin/bash"}).Return(s.mockRunner)
	rExp := s.mockRunner.EXPECT()
	rExp.SetStdout(gomock.Any())
	rExp.SetStderr(gomock.Any())
	rExp.SetStdin(gomock.Any())
	rExp.Run().Return(err)
}

func (s *removerSuite) expectProvisioned() {
	s.mockCommand.EXPECT().Command(fmt.Sprintf("ubuntu@%s", s.host), []string{"/bin/bash"}).Return(s.mockRunner)
	rExp := s.mockRunner.EXPECT()
	rExp.SetStdout(gomock.Any()).SetArg(0, *bytes.NewBuffer([]byte("juju")))
	rExp.SetStderr(gomock.Any())
	rExp.SetStdin(gomock.Any())
	rExp.Run()
}

func (s *removerSuite) expectTearDown() {
	s.mockCommand.EXPECT().Command(fmt.Sprintf("ubuntu@%s", s.host), []string{"sudo", "/bin/bash"}).Return(s.mockRunner)
	rExp := s.mockRunner.EXPECT()
	rExp.SetStdout(gomock.Any())
	rExp.SetStderr(gomock.Any())
	rExp.SetStdin(strings.NewReader(sshprovisioner.TearDownScript(false)))
	rExp.Run().Return(nil)
}
