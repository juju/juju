// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/controller"
	cmdtesting "github.com/juju/juju/cmd/testing"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type KillSuite struct {
	DestroySuite
}

var _ = gc.Suite(&KillSuite{})

func (s *KillSuite) runKillCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return testing.RunCommand(c, s.newKillCommand(), args...)
}

func (s *KillSuite) newKillCommand() cmd.Command {
	return controller.NewKillCommandForTest(
		s.api, s.clientapi, s.apierror, &mockClock{}, nil)
}

func (s *KillSuite) TestKillNoControllerNameError(c *gc.C) {
	_, err := s.runKillCommand(c)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *KillSuite) TestKillBadFlags(c *gc.C) {
	_, err := s.runKillCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *KillSuite) TestKillUnknownArgument(c *gc.C) {
	_, err := s.runKillCommand(c, "environment", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *KillSuite) TestKillUnknownController(c *gc.C) {
	_, err := s.runKillCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read controller info: environment "foo" not found`)
}

func (s *KillSuite) TestKillNonControllerEnvFails(c *gc.C) {
	_, err := s.runKillCommand(c, "test2")
	c.Assert(err, gc.ErrorMatches, "\"test2\" is not a controller; use juju environment destroy to destroy it")
}

func (s *KillSuite) TestKillCannotConnectToAPISucceeds(c *gc.C) {
	s.apierror = errors.New("connection refused")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to open API: connection refused")
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillWithAPIConnection(c *gc.C) {
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithoutAPIConnection(c *gc.C) {
	s.apierror = errors.New("connection refused")
	s.api.err = errors.NotFoundf(`controller "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: unable to get bootstrap information from API")
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithAPIConnection(c *gc.C) {
	s.api.err = errors.NotFoundf(`controller "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: controller \"test3\" not found")
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{Message: "DestroyController", Code: params.CodeNotImplemented}
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestClientKillDestroysControllerWithAPIError(c *gc.C) {
	s.api.err = &params.Error{Message: "DestroyController", Code: params.CodeNotImplemented}
	s.clientapi.err = errors.New("some destroy error")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to destroy controller through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillDestroysControllerWithAPIError(c *gc.C) {
	s.api.err = errors.New("some destroy error")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to destroy controller through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.api.destroyAll, jc.IsTrue)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCommandConfirmation(c *gc.C) {
	var stdin, stdout bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, jc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin

	// Ensure confirmation is requested if "-y" is not specified.
	stdin.WriteString("n")
	_, errc := cmdtesting.RunCommand(ctx, s.newKillCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillAPIPermErrFails(c *gc.C) {
	testDialer := func(sysName string) (api.Connection, error) {
		return nil, common.ErrPerm
	}
	cmd := controller.NewKillCommandForTest(nil, nil, nil, clock.WallClock, testDialer)
	_, err := testing.RunCommand(c, cmd, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy controller: permission denied")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEarlyAPIConnectionTimeout(c *gc.C) {
	clock := &mockClock{}

	stop := make(chan struct{})
	defer close(stop)
	testDialer := func(sysName string) (api.Connection, error) {
		<-stop
		return nil, errors.New("kill command waited too long")
	}

	cmd := controller.NewKillCommandForTest(nil, nil, nil, clock, testDialer)
	ctx, err := testing.RunCommand(c, cmd, "test1", "-y")
	c.Check(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to open API: open connection timed out")
	checkControllerRemovedFromStore(c, "test1", s.store)
	// Check that we were actually told to wait for 10s.
	c.Assert(clock.wait, gc.Equals, 10*time.Second)
}

// mockClock will panic if anything but After is called
type mockClock struct {
	clock.Clock
	wait time.Duration
}

func (m *mockClock) After(duration time.Duration) <-chan time.Time {
	m.wait = duration
	return time.After(time.Millisecond)
}

func (s *KillSuite) TestControllerStatus(c *gc.C) {
	s.api.allEnvs = []base.UserEnvironment{
		{Name: "admin",
			UUID:  "123",
			Owner: names.NewUserTag("admin").String(),
		}, {Name: "env1",
			UUID:  "456",
			Owner: names.NewUserTag("bob").String(),
		}, {Name: "env2",
			UUID:  "789",
			Owner: names.NewUserTag("jo").String(),
		},
	}

	s.api.envStatus = make(map[string]base.EnvironmentStatus)
	for _, env := range s.api.allEnvs {
		owner, err := names.ParseUserTag(env.Owner)
		c.Assert(err, jc.ErrorIsNil)
		s.api.envStatus[env.UUID] = base.EnvironmentStatus{
			UUID:               env.UUID,
			Life:               params.Dying,
			HostedMachineCount: 2,
			ServiceCount:       1,
			Owner:              owner.Canonical(),
		}
	}

	ctrStatus, envsStatus, err := controller.NewData(s.api, "123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctrStatus.HostedEnvCount, gc.Equals, 2)
	c.Assert(ctrStatus.HostedMachineCount, gc.Equals, 6)
	c.Assert(ctrStatus.ServiceCount, gc.Equals, 3)
	c.Assert(envsStatus, gc.HasLen, 2)

	for i, expected := range []struct {
		Owner              string
		Name               string
		Life               params.Life
		HostedMachineCount int
		ServiceCount       int
	}{
		{
			Owner:              "bob@local",
			Name:               "env1",
			Life:               params.Dying,
			HostedMachineCount: 2,
			ServiceCount:       1,
		}, {
			Owner:              "jo@local",
			Name:               "env2",
			Life:               params.Dying,
			HostedMachineCount: 2,
			ServiceCount:       1,
		},
	} {
		c.Assert(envsStatus[i].Owner, gc.Equals, expected.Owner)
		c.Assert(envsStatus[i].Name, gc.Equals, expected.Name)
		c.Assert(envsStatus[i].Life, gc.Equals, expected.Life)
		c.Assert(envsStatus[i].HostedMachineCount, gc.Equals, expected.HostedMachineCount)
		c.Assert(envsStatus[i].ServiceCount, gc.Equals, expected.ServiceCount)
	}

}

func (s *KillSuite) TestFmtControllerStatus(c *gc.C) {
	data := controller.CtrData{
		3,
		20,
		8,
	}
	out := controller.FmtCtrStatus(data)
	c.Assert(out, gc.Equals, "Waiting on 3 environments, 20 machines, 8 services")
}

func (s *KillSuite) TestFmtEnvironStatus(c *gc.C) {
	data := controller.EnvData{
		"owner@local",
		"envname",
		params.Dying,
		8,
		1,
	}

	out := controller.FmtEnvStatus(data)
	c.Assert(out, gc.Equals, "owner@local/envname (dying), 8 machines, 1 service")
}
