// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/cmdtest"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	_ "github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
)

type KillSuite struct {
	baseDestroySuite

	clock *testclock.Clock
}

var _ = gc.Suite(&KillSuite{})

func (s *KillSuite) SetUpTest(c *gc.C) {
	s.baseDestroySuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
}

func (s *KillSuite) runKillCommand(c *gc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newKillCommand(), args...)
}

func (s *KillSuite) newKillCommand() cmd.Command {
	clock := s.clock
	if clock == nil {
		clock = testclock.NewClock(time.Now())
	}
	return controller.NewKillCommandForTest(
		s.api, s.clientapi, s.store, s.apierror, clock, nil,
		func() (controller.CredentialAPI, error) { return s.controllerCredentialAPI, nil },
		environs.Destroy,
	)
}

func (s *KillSuite) TestKillNoControllerNameError(c *gc.C) {
	_, err := s.runKillCommand(c)
	c.Assert(err, gc.ErrorMatches, "no controller specified")
}

func (s *KillSuite) TestKillBadFlags(c *gc.C) {
	_, err := s.runKillCommand(c, "-n")
	c.Assert(err, gc.ErrorMatches, "option provided but not defined: -n")
}

func (s *KillSuite) TestKillDurationFlags(c *gc.C) {
	for i, test := range []struct {
		args     []string
		expected time.Duration
		err      string
	}{
		{
			expected: 5 * time.Minute,
		}, {
			args:     []string{"-t", "2m"},
			expected: 2 * time.Minute,
		}, {
			args:     []string{"--timeout", "2m"},
			expected: 2 * time.Minute,
		}, {
			args:     []string{"-t", "0"},
			expected: 0,
		},
	} {
		c.Logf("duration test %d", i)
		wrapped := s.newKillCommand()
		args := append([]string{"test1"}, test.args...)
		err := cmdtesting.InitCommand(wrapped, args)
		if test.err == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(controller.KillTimeout(wrapped), gc.Equals, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.err)
		}
	}
}

func (s *KillSuite) TestKillWaitForModels_AllGood(c *gc.C) {
	s.resetAPIModels(c)
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	err = controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "All hosted models reclaimed, cleaning up controller machines\n")
}

func (s *KillSuite) TestKillWaitForModels_ActuallyWaits(c *gc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	result := make(chan error)
	go func() {
		err := controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
		result <- err
	}()

	s.syncClockAlarm(c)
	s.setModelStatus(base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.removeModel(test2UUID)
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 1 machine\n" +
		"All hosted models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_WaitsForControllerMachines(c *gc.C) {
	s.api.allModels = nil
	s.api.envStatus = map[string]base.ModelStatus{}
	s.addModel("controller", base.ModelStatus{
		UUID:               test1UUID,
		Life:               life.Alive,
		Owner:              "admin",
		TotalMachineCount:  3,
		HostedMachineCount: 2,
	})
	s.clock.Advance(5 * time.Second)

	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	result := make(chan error)
	go func() {
		err := controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
		result <- err
	}()

	s.syncClockAlarm(c)
	s.setModelStatus(base.ModelStatus{
		UUID:               test1UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.setModelStatus(base.ModelStatus{
		UUID:               test1UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 0,
	})
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting on 0 model, 2 machines\n" +
		"Waiting on 0 model, 1 machine\n" +
		"All hosted models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_TimeoutResetsWithChange(c *gc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=20s"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	result := make(chan error)
	go func() {
		err := controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
		result <- err
	}()

	s.syncClockAlarm(c)
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.setModelStatus(base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.removeModel(test2UUID)
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, jc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 20s\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 15s\n" +
		"Waiting on 1 model, 1 machine, will kill machines directly in 20s\n" +
		"All hosted models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_TimeoutWithNoChange(c *gc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Owner:              "admin",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	result := make(chan error)
	go func() {
		err := controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
		result <- err
	}()

	for i := 0; i < 12; i++ {
		s.syncClockAlarm(c)
		s.clock.Advance(5 * time.Second)
	}

	select {
	case err := <-result:
		c.Assert(err, gc.ErrorMatches, "timed out")
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 25s\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 20s\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 15s\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 10s\n" +
		"Waiting on 1 model, 2 machines, 2 applications, will kill machines directly in 5s\n"

	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, expect)
}

func (s *KillSuite) resetAPIModels(c *gc.C) {
	s.api.allModels = nil
	s.api.envStatus = map[string]base.ModelStatus{}
	s.addModel("controller", base.ModelStatus{
		UUID:              test1UUID,
		Life:              life.Alive,
		Owner:             "admin",
		TotalMachineCount: 1,
	})
}

func (s *KillSuite) addModel(name string, status base.ModelStatus) {
	s.api.allModels = append(s.api.allModels, base.UserModel{
		Name:  name,
		UUID:  status.UUID,
		Owner: status.Owner,
	})
	s.api.envStatus[status.UUID] = status
}

func (s *KillSuite) setModelStatus(status base.ModelStatus) {
	s.api.envStatus[status.UUID] = status
}

func (s *KillSuite) removeModel(uuid string) {
	for i, v := range s.api.allModels {
		if v.UUID == uuid {
			s.api.allModels = append(s.api.allModels[:i], s.api.allModels[i+1:]...)
			break
		}
	}
	delete(s.api.envStatus, uuid)
}

func (s *KillSuite) syncClockAlarm(c *gc.C) {
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for test clock After call")
	}
}

func (s *KillSuite) TestKillUnknownArgument(c *gc.C) {
	_, err := s.runKillCommand(c, "model", "whoops")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *KillSuite) TestKillUnknownController(c *gc.C) {
	_, err := s.runKillCommand(c, "foo")
	c.Assert(err, gc.ErrorMatches, `controller foo not found`)
}

func (s *KillSuite) TestKillCannotConnectToAPISucceeds(c *gc.C) {
	s.api, s.apierror = nil, errors.New("connection refused")
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), jc.Contains, "Unable to open API: connection refused")
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillWithAPIConnection(c *gc.C) {
	_, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "DestroyController", "AllModels", "ModelStatus", "Close")
	destroyStorage := true
	s.api.CheckCall(c, 0, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	c.Assert(s.clientapi.destroycalled, jc.IsFalse)
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithoutAPIConnection(c *gc.C) {
	s.api, s.apierror = nil, errors.New("connection refused")
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches,
		"getting controller environ: unable to get bootstrap information from client store or API",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithAPIConnection(c *gc.C) {
	s.api.SetErrors(errors.NotFoundf(`controller "test3"`))
	_, err := s.runKillCommand(c, "test3", "-y")
	c.Assert(err, gc.ErrorMatches,
		"getting controller environ: getting model config from API: controller \"test3\" not found",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillDestroysControllerWithAPIError(c *gc.C) {
	s.api.SetErrors(errors.New("some destroy error"))
	ctx, err := s.runKillCommand(c, "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), jc.Contains, "Unable to destroy controller through the API: some destroy error\nDestroying through provider")
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
	_, errc := cmdtest.RunCommandWithDummyProvider(ctx, s.newKillCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, gc.ErrorMatches, "controller destruction aborted")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("command took too long")
	}
	c.Check(cmdtesting.Stdout(ctx), gc.Matches, "WARNING!.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCommandControllerAlias(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, s.newKillCommand(), "test1", "-y")
	c.Assert(err, jc.ErrorIsNil)
	checkControllerRemovedFromStore(c, "test1:test1", s.store)
}

func (s *KillSuite) TestKillAPIPermErrFails(c *gc.C) {
	testDialer := func(*api.Info, api.DialOpts) (api.Connection, error) {
		return nil, common.ErrPerm
	}
	cmd := controller.NewKillCommandForTest(nil, nil, s.store, nil, clock.WallClock, testDialer,
		func() (controller.CredentialAPI, error) { return s.controllerCredentialAPI, nil },
		environs.Destroy,
	)
	_, err := cmdtesting.RunCommand(c, cmd, "test1", "-y")
	c.Assert(err, gc.ErrorMatches, "cannot destroy controller: permission denied")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEarlyAPIConnectionTimeout(c *gc.C) {
	clock := &mockClock{}

	stop := make(chan struct{})
	defer close(stop)
	testDialer := func(*api.Info, api.DialOpts) (api.Connection, error) {
		<-stop
		return nil, errors.New("kill command waited too long")
	}

	cmd := controller.NewKillCommandForTest(nil, nil, s.store, nil, clock, testDialer,
		func() (controller.CredentialAPI, error) { return s.controllerCredentialAPI, nil },
		environs.Destroy,
	)
	ctx, err := cmdtesting.RunCommand(c, cmd, "test1", "-y")
	c.Check(err, jc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), jc.Contains, "Unable to open API: open connection timed out")
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
	s.api.allModels = []base.UserModel{
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

	s.api.envStatus = make(map[string]base.ModelStatus)
	for _, env := range s.api.allModels {
		owner, err := names.ParseUserTag(env.Owner)
		c.Assert(err, jc.ErrorIsNil)
		s.api.envStatus[env.UUID] = base.ModelStatus{
			UUID:               env.UUID,
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
			Owner:              owner.Id(),
		}
	}

	ctrStatus, envsStatus, err := controller.NewData(s.api, "123")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ctrStatus.HostedModelCount, gc.Equals, 2)
	c.Assert(ctrStatus.HostedMachineCount, gc.Equals, 6)
	c.Assert(ctrStatus.ApplicationCount, gc.Equals, 3)
	c.Assert(envsStatus, gc.HasLen, 2)

	for i, expected := range []struct {
		Owner              string
		Name               string
		Life               life.Value
		HostedMachineCount int
		ApplicationCount   int
	}{
		{
			Owner:              "bob",
			Name:               "env1",
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
		}, {
			Owner:              "jo",
			Name:               "env2",
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
		},
	} {
		c.Assert(envsStatus[i].Owner, gc.Equals, expected.Owner)
		c.Assert(envsStatus[i].Name, gc.Equals, expected.Name)
		c.Assert(envsStatus[i].Life, gc.Equals, expected.Life)
		c.Assert(envsStatus[i].HostedMachineCount, gc.Equals, expected.HostedMachineCount)
		c.Assert(envsStatus[i].ApplicationCount, gc.Equals, expected.ApplicationCount)
	}

}

func (s *KillSuite) TestFmtControllerStatus(c *gc.C) {
	data := controller.CtrData{
		UUID:               "uuid",
		HostedModelCount:   3,
		HostedMachineCount: 20,
		ApplicationCount:   8,
	}
	out := controller.FmtCtrStatus(data)
	c.Assert(out, gc.Equals, "Waiting on 3 models, 20 machines, 8 applications")
}

func (s *KillSuite) TestFmtEnvironStatus(c *gc.C) {
	data := controller.ModelData{
		"uuid",
		"owner",
		"envname",
		life.Dying,
		8,
		1,
		2,
		1,
		0,
		0,
	}

	out := controller.FmtModelStatus(data)
	c.Assert(out, gc.Equals, "\towner/envname (dying), 8 machines, 1 application, 2 volumes, 1 filesystem")
}
