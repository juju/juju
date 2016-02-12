// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client_test

import (
	"fmt"
	"sync"
	"time"

	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/exec"
	"github.com/juju/utils/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/client"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type runSuite struct {
	baseSuite
}

var _ = gc.Suite(&runSuite{})

func (s *runSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *runSuite) addMachineWithAddress(c *gc.C, address string) *state.Machine {
	machine := s.addMachine(c)
	machine.SetProviderAddresses(network.NewAddress(address))
	return machine
}

func (s *runSuite) TestRemoteParamsForMachinePopulates(c *gc.C) {
	machine := s.addMachine(c)
	result := client.RemoteParamsForMachine(machine, "command", time.Minute)
	c.Assert(result.Command, gc.Equals, "command")
	c.Assert(result.Timeout, gc.Equals, time.Minute)
	c.Assert(result.MachineId, gc.Equals, machine.Id())
	// Now an empty host isn't particularly useful, but the machine doesn't
	// have an address to use.
	c.Assert(machine.Addresses(), gc.HasLen, 0)
	c.Assert(result.Host, gc.Equals, "")
}

func (s *runSuite) TestRemoteParamsForMachinePopulatesWithAddress(c *gc.C) {
	machine := s.addMachineWithAddress(c, "10.3.2.1")

	result := client.RemoteParamsForMachine(machine, "command", time.Minute)
	c.Assert(result.Command, gc.Equals, "command")
	c.Assert(result.Timeout, gc.Equals, time.Minute)
	c.Assert(result.MachineId, gc.Equals, machine.Id())
	c.Assert(result.Host, gc.Equals, "ubuntu@10.3.2.1")
}

func (s *runSuite) addUnit(c *gc.C, service *state.Service) *state.Unit {
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	mId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(mId)
	c.Assert(err, jc.ErrorIsNil)
	machine.SetProviderAddresses(network.NewAddress("10.3.2.1"))
	return unit
}

func (s *runSuite) TestGetAllUnitNames(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	owner := s.AdminUserTag(c)
	magic, err := s.State.AddService(state.AddServiceArgs{Name: "magic", Owner: owner.String(), Charm: charm})
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	notAssigned, err := s.State.AddService(state.AddServiceArgs{Name: "not-assigned", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	_, err = notAssigned.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddService(state.AddServiceArgs{Name: "no-units", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)

	wordpress, err := s.State.AddService(state.AddServiceArgs{Name: "wordpress", Owner: owner.String(), Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	wordpress0 := s.addUnit(c, wordpress)
	_, err = s.State.AddService(state.AddServiceArgs{Name: "logging", Owner: owner.String(), Charm: s.AddTestingCharm(c, "logging")})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		message  string
		expected []string
		units    []string
		services []string
		error    string
	}{{
		message: "no units, expected nil slice",
	}, {
		message: "asking for a unit that isn't there",
		units:   []string{"foo/0"},
		error:   `unit "foo/0" not found`,
	}, {
		message:  "asking for a service that isn't there",
		services: []string{"foo"},
		error:    `service "foo" not found`,
	}, {
		message:  "service with no units is not really an error",
		services: []string{"no-units"},
	}, {
		message:  "A service with units not assigned is an error",
		services: []string{"not-assigned"},
		error:    `unit "not-assigned/0" is not assigned to a machine`,
	}, {
		message:  "A service with units",
		services: []string{"magic"},
		expected: []string{"magic/0", "magic/1"},
	}, {
		message:  "Asking for just a unit",
		units:    []string{"magic/0"},
		expected: []string{"magic/0"},
	}, {
		message:  "Asking for just a subordinate unit",
		units:    []string{"logging/0"},
		expected: []string{"logging/0"},
	}, {
		message:  "Asking for a unit, and the service",
		services: []string{"magic"},
		units:    []string{"magic/0"},
		expected: []string{"magic/0", "magic/1"},
	}} {
		c.Logf("%v: %s", i, test.message)
		result, err := client.GetAllUnitNames(s.State, test.units, test.services)
		if test.error == "" {
			c.Check(err, jc.ErrorIsNil)
			var units []string
			for _, unit := range result {
				units = append(units, unit.Name())
			}
			c.Check(units, jc.SameContents, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.error)
		}
	}
}

func (s *runSuite) mockSSH(c *gc.C, cmd string) {
	gitjujutesting.PatchExecutable(c, s, "ssh", cmd)
	gitjujutesting.PatchExecutable(c, s, "scp", cmd)
	client, _ := ssh.NewOpenSSHClient()
	s.PatchValue(&ssh.DefaultClient, client)
}

func (s *runSuite) TestParallelExecuteErrorsOnBlankHost(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Command: "foo",
				Timeout: testing.LongWait,
			},
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "missing host address")
}

func (s *runSuite) TestParallelExecuteAddsIdentity(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Host:    "localhost",
				Command: "foo",
				Timeout: testing.LongWait,
			},
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "")
	c.Assert(string(result.Stderr), jc.Contains, "system-identity")
}

func (s *runSuite) TestParallelExecuteCopiesAcrossMachineAndUnit(c *gc.C) {
	s.mockSSH(c, echoInputShowArgs)

	params := []*client.RemoteExec{
		{
			ExecParams: ssh.ExecParams{
				Host:    "localhost",
				Command: "foo",
				Timeout: testing.LongWait,
			},
			MachineId: "machine-id",
			UnitId:    "unit-id",
		},
	}

	runResults := client.ParallelExecute("/some/dir", params)
	c.Assert(runResults.Results, gc.HasLen, 1)
	result := runResults.Results[0]
	c.Assert(result.Error, gc.Equals, "")
	c.Assert(result.MachineId, gc.Equals, "machine-id")
	c.Assert(result.UnitId, gc.Equals, "unit-id")
}

func (s *runSuite) TestRunOnAllMachines(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")
	s.addMachineWithAddress(c, "10.3.2.2")
	s.addMachineWithAddress(c, "10.3.2.3")

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()
	results, err := client.RunOnAllMachines("hostname", testing.LongWait)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)

	var expectedResults []params.RunResult
	for i := 0; i < 3; i++ {
		expectedResults = append(expectedResults,
			params.RunResult{
				ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[0])},

				MachineId: fmt.Sprint(i),
			})
	}

	c.Check(results, jc.DeepEquals, expectedResults)
	c.Check(string(results[0].Stdout), gc.Equals, expectedCommand[0])
	c.Check(string(results[1].Stdout), gc.Equals, expectedCommand[0])
	c.Check(string(results[2].Stdout), gc.Equals, expectedCommand[0])
}

func (s *runSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &rpc.RequestError{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) TestBlockRunOnAllMachines(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")
	s.addMachineWithAddress(c, "10.3.2.2")
	s.addMachineWithAddress(c, "10.3.2.3")

	s.mockSSH(c, echoInput)

	// block all changes
	s.BlockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := s.APIState.Client().RunOnAllMachines("hostname", testing.LongWait)
	s.AssertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestRunMachineAndService(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	charm := s.AddTestingCharm(c, "dummy")
	owner := s.Factory.MakeUser(c, nil).Tag()
	magic, err := s.State.AddService(state.AddServiceArgs{Name: "magic", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()
	results, err := client.Run(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Machines: []string{"0"},
			Services: []string{"magic"},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 3)

	expectedResults := []params.RunResult{
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[0])},
			MachineId:    "0",
		},
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[1])},
			MachineId:    "1",
			UnitId:       "magic/0",
		},
		{
			ExecResponse: exec.ExecResponse{Stdout: []byte(expectedCommand[2])},
			MachineId:    "2",
			UnitId:       "magic/1",
		},
	}

	c.Assert(results, jc.DeepEquals, expectedResults)
}

func (s *runSuite) TestBlockRunMachineAndService(c *gc.C) {
	// Make three machines.
	s.addMachineWithAddress(c, "10.3.2.1")

	charm := s.AddTestingCharm(c, "dummy")
	owner := s.Factory.MakeUser(c, nil).Tag()
	magic, err := s.State.AddService(state.AddServiceArgs{Name: "magic", Owner: owner.String(), Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.mockSSH(c, echoInput)

	// hmm... this seems to be going through the api client, and from there
	// through to the apiserver implementation. Not ideal, but it is how the
	// other client tests are written.
	client := s.APIState.Client()

	// block all changes
	s.BlockAllChanges(c, "TestBlockRunMachineAndService")
	_, err = client.Run(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
			Machines: []string{"0"},
			Services: []string{"magic"},
		})
	s.AssertBlocked(c, err, "TestBlockRunMachineAndService")
}

func (s *runSuite) TestStartSerialWaitParallel(c *gc.C) {
	st := starter{serialChecker{c: c, block: make(chan struct{})}}
	w := waiter{concurrentChecker{c: c, block: make(chan struct{})}}
	count := 4
	w.started.Add(count)
	st.finished.Add(count)
	args := make([]*client.RemoteExec, count)
	for i := range args {
		args[i] = &client.RemoteExec{
			ExecParams: ssh.ExecParams{Timeout: testing.LongWait},
		}
	}

	results := &params.RunResults{}
	results.Results = make([]params.RunResult, count)

	// run this in a goroutine so we can futz with things asynchronously.
	go client.StartSerialWaitParallel(args, results, st.start, w.wait)

	// ok, so we give the function some time to run... if we are running start
	// in goroutines, this would give them time to do their thing.
	<-time.After(testing.ShortWait)

	// if start is being run synchronously, then only one of the start functions
	// should have been called by now.
	c.Assert(st.count, gc.Equals, 1)

	// good, now let's unblock start and let'em fly
	st.unblock()

	// wait for the start functions to complete
	select {
	case <-st.waitFinish():
		// good, all start functions called.
	case <-time.After(testing.ShortWait):
		c.Fatalf("timed out waiting for start functions to be called.")
	}

	// wait for the wait functions to run their startups
	select {
	case <-w.waitStarted():
		// good, all start functions called.
	case <-time.After(testing.ShortWait):
		c.Fatalf("Timed out waiting for start functions to be called. Start functions probably not called as goroutines.")
	}
	w.unblock()
}

type starter struct {
	serialChecker
}

func (s *starter) start(_ ssh.ExecParams) (*ssh.RunningCmd, error) {
	s.called()
	return nil, nil
}

type waiter struct {
	concurrentChecker
}

func (w *waiter) wait(wg *sync.WaitGroup, _ *ssh.RunningCmd, _ *params.RunResult, _ <-chan struct{}) {
	defer wg.Done()
	w.called()
}

// serialChecker is a type that lets us check that a function is called
// serially, not concurrently.
type serialChecker struct {
	c        *gc.C
	mu       sync.Mutex
	block    chan struct{}
	count    int
	finished sync.WaitGroup
}

// unblock unblocks the called method.
func (s *serialChecker) unblock() {
	close(s.block)
}

// called registers tha this function has been called.  It will block until
// unblock is called, or will timeout after a LongWait.
func (s *serialChecker) called() {
	defer s.finished.Done()
	// make sure we serialize access to the struct
	s.mu.Lock()
	// log that we've been called
	s.count++
	s.mu.Unlock()
	select {
	case <-s.block:
	case <-time.After(testing.LongWait):
		s.c.Fatalf("time out waiting for unblock")
	}
}

// waitFinish waits on the finished waitgroup, and closes the returned channel
// when it is.
func (s *serialChecker) waitFinish() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		s.finished.Wait()
		close(done)
	}()
	return done
}

// concurrentChecker is a type to allow us to check that the a function is being
// called concurrently, not serially.
type concurrentChecker struct {
	c       *gc.C
	block   chan struct{}
	started sync.WaitGroup
}

// unblock unblocks the functions currently blocked by this type.
func (c *concurrentChecker) unblock() {
	close(c.block)
}

// waitStarted waits on the started waitgroup, and closes the returned channel
// when it is.
func (c *concurrentChecker) waitStarted() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		c.started.Wait()
		close(done)
	}()
	return done
}

// wait is the function we pass to startSerialWaitParallel. It gives each
// goroutine an index, so we can keep track of which ones were run when, and
// blocks until released by the unblock function.
func (c *concurrentChecker) called() {
	// signal that we've started
	c.started.Done()

	// now we block on the block channel, waiting to be unblocked.  This way, if
	// we don't get the started waitgroup finished, we know these functions
	// weren't run concurrently.
	select {
	case <-c.block:
		// good, all functions were run and we got unblocked.
	case <-time.After(testing.LongWait):
		c.c.Fatalf("timed out waiting to unblock")
	}
}
