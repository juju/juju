// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system_test

import (
	"bytes"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/system"
	cmdtesting "github.com/juju/juju/cmd/testing"
	"github.com/juju/juju/juju"
	_ "github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/testing"
)

type KillSuite struct {
	DestroySuite
}

var _ = gc.Suite(&KillSuite{})

func (s *KillSuite) SetUpTest(c *gc.C) {
	s.DestroySuite.SetUpTest(c)
}

func (s *KillSuite) runKillCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	cmd := system.NewKillCommand(
		s.api,
		s.clientapi,
		s.apierror,
		func(name string) (api.Connection, error) {
			return juju.NewAPIFromName(name, nil)
		})
	return testing.RunCommand(c, cmd, args...)
}

func (s *KillSuite) newKillCommand() cmd.Command {
	return system.NewKillCommand(
		s.api,
		s.clientapi,
		s.apierror,
		func(name string) (api.Connection, error) {
			return juju.NewAPIFromName(name, nil)
		})
}

func (s *KillSuite) TestKillNoSystemNameError(c *gc.C) {
	_, err := s.runKillCommand(c)
	c.Assert(err, gc.ErrorMatches, "no system specified")
}

func (s *KillSuite) TestKillBadFlags(c *gc.C) {
	_, err := s.runKillCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "flag provided but not defined: -n")
}

func (s *KillSuite) TestKillUnknownArgument(c *gc.C) {
	_, err := s.runKillCommand(c, "environment", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *KillSuite) TestKillUnknownSystem(c *gc.C) {
	_, err := s.runKillCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `cannot read system info: environment "foo" not found`)
}

func (s *KillSuite) TestKillNonSystemEnvFails(c *gc.C) {
	_, err := s.runKillCommand(c, "test2")
	c.Assert(err, gc.ErrorMatches, "\"test2\" is not a system; use juju environment destroy to destroy it")
}

func (s *KillSuite) TestKillCannotConnectToAPISucceeds(c *gc.C) {
	s.apierror = errors.New("connection refused")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to open API: connection refused")
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillWithAPIConnection(c *gc.C) {
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.destroyAll, jc.IsTrue)
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)

	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithoutAPIConnection(c *gc.C) {
	s.apierror = errors.New("connection refused")
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: unable to get bootstrap information from API")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithAPIConnection(c *gc.C) {
	s.api.err = errors.NotFoundf(`system "test3"`)
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot obtain bootstrap information: system \"test3\" not found")
	checkSystemExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillFallsBackToClient(c *gc.C) {
	s.api.err = &params.Error{Message: "DestroySystem", Code: params.CodeNotImplemented}
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestClientKillDestroysSystemWithAPIError(c *gc.C) {
	s.api.err = &params.Error{Message: "DestroySystem", Code: params.CodeNotImplemented}
	s.clientapi.err = errors.New("some destroy error")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to destroy system through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.clientapi.destroycalled, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillDestroysSystemWithAPIError(c *gc.C) {
	s.api.err = errors.New("some destroy error")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testing.Stderr(ctx), jc.Contains, "Unable to destroy system through the API: some destroy error.  Destroying through provider.")
	c.Assert(s.api.destroyAll, jc.IsTrue)
	checkSystemRemovedFromStore(c, "test1", s.store)
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
		c.Check(err, gc.ErrorMatches, "system destruction aborted")
	case <-time.After(testing.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(testing.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillAPIPermErrFails(c *gc.C) {
	testDialer := func(sysName string) (api.Connection, error) {
		return nil, common.ErrPerm
	}

	cmd := system.NewKillCommand(nil, nil, nil, testDialer)
	_, err := testing.RunCommand(c, cmd, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy system: permission denied")
	checkSystemExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEarlyAPIConnectionTimeout(c *gc.C) {
	stop := make(chan struct{})
	defer close(stop)
	testDialer := func(sysName string) (api.Connection, error) {
		<-stop
		return nil, errors.New("kill command waited too long")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		cmd := system.NewKillCommand(nil, nil, nil, testDialer)
		ctx, err := testing.RunCommand(c, cmd, "test1", "-y")
		c.Check(err, jc.ErrorIsNil)
		c.Check(testing.Stderr(ctx), jc.Contains, "Unable to open API: connection to state server timed out")
		c.Check(s.api.destroyAll, jc.IsFalse)
		checkSystemRemovedFromStore(c, "test1", s.store)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Minute):
		c.Fatalf("Kill command waited too long to open the API")
	}
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

	ctrStatus, envsStatus, err := system.NewData(s.api, "123")
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
	data := system.CtrData{
		3,
		20,
		8,
	}
	out := system.FmtCtrStatus(data)
	c.Assert(out, gc.Equals, "Waiting on 3 environments, 20 machines, 8 services")
}

func (s *KillSuite) TestFmtEnvironStatus(c *gc.C) {
	data := system.EnvData{
		"owner@local",
		"envname",
		params.Dying,
		8,
		1,
	}

	out := system.FmtEnvStatus(data)
	c.Assert(out, gc.Equals, "owner@local/envname (dying), 8 machines, 1 service")
}
