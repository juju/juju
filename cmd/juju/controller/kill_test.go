// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apicontroller "github.com/juju/juju/api/controller/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/cmd/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	coretesting "github.com/juju/juju/internal/testing"
)

type KillSuite struct {
	baseDestroySuite

	clock *testclock.Clock
}

func TestKillSuite(t *testing.T) {
	tc.Run(t, &KillSuite{})
}

func (s *KillSuite) SetUpTest(c *tc.C) {
	s.baseDestroySuite.SetUpTest(c)
	s.clock = testclock.NewClock(time.Now())
}

func (s *KillSuite) runKillCommand(c *tc.C, args ...string) (*cmd.Context, error) {
	return cmdtesting.RunCommand(c, s.newKillCommand(), args...)
}

func (s *KillSuite) newKillCommand() cmd.Command {
	clock := s.clock
	if clock == nil {
		clock = testclock.NewClock(time.Now())
	}
	return controller.NewKillCommandForTest(
		s.api, s.store, s.apierror, s.controllerModelConfigAPI, clock, nil,
		environs.Destroy,
	)
}

func (s *KillSuite) TestKillNoControllerNameError(c *tc.C) {
	_, err := s.runKillCommand(c)
	c.Assert(err, tc.ErrorMatches, "no controller specified")
}

func (s *KillSuite) TestKillBadFlags(c *tc.C) {
	_, err := s.runKillCommand(c, "-n")
	c.Assert(err, tc.ErrorMatches, "option provided but not defined: -n")
}

func (s *KillSuite) TestKillDurationFlags(c *tc.C) {
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
			c.Check(err, tc.ErrorIsNil)
			c.Check(controller.KillTimeout(wrapped), tc.Equals, test.expected)
		} else {
			c.Check(err, tc.ErrorMatches, test.err)
		}
	}
}

func (s *KillSuite) TestKillWaitForModels_AllGood(c *tc.C) {
	s.resetAPIModels(c)
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	err = controller.KillWaitForModels(wrapped, ctx, s.api, test1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "All models reclaimed, cleaning up controller machines\n")
}

func (s *KillSuite) TestKillWaitForModels_ActuallyWaits(c *tc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Qualifier:          "prod",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, tc.ErrorIsNil)

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
		Qualifier:          "prod",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.removeModel(test2UUID)
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 1 machine\n" +
		"All models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_WaitsForControllerMachines(c *tc.C) {
	s.api.allModels = nil
	s.api.envStatus = map[string]base.ModelStatus{}
	s.addModel("controller", base.ModelStatus{
		UUID:               test1UUID,
		Life:               life.Alive,
		Qualifier:          "prod",
		TotalMachineCount:  3,
		HostedMachineCount: 2,
	})
	s.clock.Advance(5 * time.Second)

	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, tc.ErrorIsNil)

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
		Qualifier:          "prod",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.setModelStatus(base.ModelStatus{
		UUID:               test1UUID,
		Life:               life.Dying,
		Qualifier:          "prod",
		HostedMachineCount: 0,
	})
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting for 0 model, 2 machines\n" +
		"Waiting for 0 model, 1 machine\n" +
		"All models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_TimeoutResetsWithChange(c *tc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Qualifier:          "prod",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=20s"})
	c.Assert(err, tc.ErrorIsNil)

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
		Qualifier:          "prod",
		HostedMachineCount: 1,
	})
	s.clock.Advance(5 * time.Second)

	s.syncClockAlarm(c)
	s.removeModel(test2UUID)
	s.clock.Advance(5 * time.Second)

	select {
	case err := <-result:
		c.Assert(err, tc.ErrorIsNil)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 20s\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 15s\n" +
		"Waiting for 1 model, 1 machine, will kill machines directly in 20s\n" +
		"All models reclaimed, cleaning up controller machines\n"

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expect)
}

func (s *KillSuite) TestKillWaitForModels_TimeoutWithNoChange(c *tc.C) {
	s.resetAPIModels(c)
	s.addModel("model-1", base.ModelStatus{
		UUID:               test2UUID,
		Life:               life.Dying,
		Qualifier:          "prod",
		HostedMachineCount: 2,
		ApplicationCount:   2,
	})
	wrapped := s.newKillCommand()
	err := cmdtesting.InitCommand(wrapped, []string{"test1", "--timeout=1m"})
	c.Assert(err, tc.ErrorIsNil)

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
		c.Assert(err, tc.ErrorMatches, "timed out")
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for result")
	}
	expect := "" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 25s\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 20s\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 15s\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 10s\n" +
		"Waiting for 1 model, 2 machines, 2 applications, will kill machines directly in 5s\n"

	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, expect)
}

func (s *KillSuite) resetAPIModels(c *tc.C) {
	s.api.allModels = nil
	s.api.envStatus = map[string]base.ModelStatus{}
	s.addModel("controller", base.ModelStatus{
		UUID:              test1UUID,
		Life:              life.Alive,
		Qualifier:         "prod",
		TotalMachineCount: 1,
	})
}

func (s *KillSuite) addModel(name string, status base.ModelStatus) {
	s.api.allModels = append(s.api.allModels, base.UserModel{
		Name:      name,
		UUID:      status.UUID,
		Qualifier: status.Qualifier,
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

func (s *KillSuite) syncClockAlarm(c *tc.C) {
	select {
	case <-s.clock.Alarms():
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for test clock After call")
	}
}

func (s *KillSuite) TestKillUnknownArgument(c *tc.C) {
	_, err := s.runKillCommand(c, "model", "whoops")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["whoops"\]`)
}

func (s *KillSuite) TestKillUnknownController(c *tc.C) {
	_, err := s.runKillCommand(c, "foo")
	c.Assert(err, tc.ErrorMatches, `controller foo not found`)
}

func (s *KillSuite) TestKillCannotConnectToAPISucceeds(c *tc.C) {
	s.api, s.apierror = nil, errors.New("connection refused")
	ctx, err := s.runKillCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "Unable to open API: connection refused")
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillWithAPIConnection(c *tc.C) {
	_, err := s.runKillCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	s.api.CheckCallNames(c, "DestroyController", "AllModels", "ModelStatus", "Close")
	destroyStorage := true
	s.api.CheckCall(c, 0, "DestroyController", apicontroller.DestroyControllerParams{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithoutAPIConnection(c *tc.C) {
	s.api, s.apierror = nil, errors.New("connection refused")
	_, err := s.runKillCommand(c, "test3", "--no-prompt")
	c.Assert(err, tc.ErrorMatches,
		"getting controller environ: unable to get bootstrap information from client store or API",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillEnvironmentGetFailsWithAPIConnection(c *tc.C) {
	s.controllerModelConfigAPI.SetErrors(errors.NotFoundf(`controller "test3"`))
	_, err := s.runKillCommand(c, "test3", "--no-prompt")
	c.Assert(err, tc.ErrorMatches,
		"getting controller environ: getting model config from API: controller \"test3\" not found",
	)
	checkControllerExistsInStore(c, "test3", s.store)
}

func (s *KillSuite) TestKillDestroysControllerWithAPIError(c *tc.C) {
	s.api.SetErrors(errors.New("some destroy error"))
	ctx, err := s.runKillCommand(c, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "Unable to destroy controller through the API: some destroy error\nDestroying through provider")
	checkControllerRemovedFromStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCommandConfirmation(c *tc.C) {
	var stdin, stdout, stderr bytes.Buffer
	ctx, err := cmd.DefaultContext()
	c.Assert(err, tc.ErrorIsNil)
	ctx.Stdout = &stdout
	ctx.Stdin = &stdin
	ctx.Stderr = &stderr

	// Ensure confirmation is requested if "--no-prompt" is not specified.
	stdin.WriteString("n")
	errc := cmdtesting.RunCommandWithContext(ctx, s.newKillCommand(), "test1")
	select {
	case err := <-errc:
		c.Check(err, tc.ErrorMatches, "controller destruction: aborted")
	case <-time.After(coretesting.LongWait):
		c.Fatalf("command took too long")
	}
	//testLog := c.GetTestLog()
	//c.Check(testLog, tc.Matches, "(.|\n)*WARNING.*test1(.|\n)*")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillCommandControllerAlias(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, s.newKillCommand(), "test1", "--no-prompt")
	c.Assert(err, tc.ErrorIsNil)
	checkControllerRemovedFromStore(c, "test1:test1", s.store)
}

func (s *KillSuite) TestKillAPIPermErrFails(c *tc.C) {
	testDialer := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		return nil, apiservererrors.ErrPerm
	}
	cmd := controller.NewKillCommandForTest(nil, s.store, nil,
		s.controllerModelConfigAPI, clock.WallClock, testDialer,
		environs.Destroy,
	)
	_, err := cmdtesting.RunCommand(c, cmd, "test1", "--no-prompt")
	c.Assert(err, tc.ErrorMatches, "cannot destroy controller: permission denied")
	checkControllerExistsInStore(c, "test1", s.store)
}

func (s *KillSuite) TestKillEarlyAPIConnectionTimeout(c *tc.C) {
	clock := &mockClock{}

	stop := make(chan struct{})
	defer close(stop)
	testDialer := func(context.Context, *api.Info, api.DialOpts) (api.Connection, error) {
		<-stop
		return nil, errors.New("kill command waited too long")
	}

	cmd := controller.NewKillCommandForTest(nil, s.store, nil,
		s.controllerModelConfigAPI, clock, testDialer,
		environs.Destroy,
	)
	ctx, err := cmdtesting.RunCommand(c, cmd, "test1", "--no-prompt")
	c.Check(err, tc.ErrorIsNil)
	c.Check(cmdtesting.Stderr(ctx), tc.Contains, "Unable to open API: open connection timed out")
	checkControllerRemovedFromStore(c, "test1", s.store)
	// Check that we were actually told to wait for 10s.
	c.Assert(clock.wait, tc.Equals, 10*time.Second)
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

func (s *KillSuite) TestControllerStatus(c *tc.C) {
	s.api.allModels = []base.UserModel{
		{Name: "admin",
			UUID:      "123",
			Qualifier: "prod",
		}, {Name: "env1",
			UUID:      "456",
			Qualifier: "staging",
		}, {Name: "env2",
			UUID:      "789",
			Qualifier: "testing",
		},
	}

	s.api.envStatus = make(map[string]base.ModelStatus)
	for _, env := range s.api.allModels {
		s.api.envStatus[env.UUID] = base.ModelStatus{
			UUID:               env.UUID,
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
			Qualifier:          env.Qualifier,
		}
	}

	environmentStatus, err := controller.NewData(c.Context(), s.api, "123")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(environmentStatus.Controller.HostedModelCount, tc.Equals, 2)
	c.Assert(environmentStatus.Controller.HostedMachineCount, tc.Equals, 6)
	c.Assert(environmentStatus.Controller.ApplicationCount, tc.Equals, 3)
	c.Assert(environmentStatus.Models, tc.HasLen, 2)

	for i, expected := range []struct {
		Qualifier          string
		Name               string
		Life               life.Value
		HostedMachineCount int
		ApplicationCount   int
	}{
		{
			Qualifier:          "staging",
			Name:               "env1",
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
		}, {
			Qualifier:          "testing",
			Name:               "env2",
			Life:               life.Dying,
			HostedMachineCount: 2,
			ApplicationCount:   1,
		},
	} {
		c.Assert(environmentStatus.Models[i].Qualifier, tc.Equals, expected.Qualifier)
		c.Assert(environmentStatus.Models[i].Name, tc.Equals, expected.Name)
		c.Assert(environmentStatus.Models[i].Life, tc.Equals, expected.Life)
		c.Assert(environmentStatus.Models[i].HostedMachineCount, tc.Equals, expected.HostedMachineCount)
		c.Assert(environmentStatus.Models[i].ApplicationCount, tc.Equals, expected.ApplicationCount)
	}

}

func (s *KillSuite) TestFmtControllerStatus(c *tc.C) {
	data := controller.CtrData{
		UUID:               "uuid",
		HostedModelCount:   3,
		HostedMachineCount: 20,
		ApplicationCount:   8,
	}
	out := controller.FmtCtrStatus(data)
	c.Assert(out, tc.Equals, "Waiting for 3 models, 20 machines, 8 applications")
}

func (s *KillSuite) TestFmtEnvironStatus(c *tc.C) {
	data := controller.ModelData{
		"uuid",
		"prod",
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
	c.Assert(out, tc.Equals, "\tprod/envname (dying), 8 machines, 1 application, 2 volumes, 1 filesystem")
}
