// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/set"
	"github.com/juju/utils/ssh"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
)

// remoteParamsForMachine returns a filled in RemoteExec instance
// based on the machine, command and timeout params.  If the machine
// does not have an internal address, the Host is empty. This is caught
// by the function that actually tries to execute the command.
func remoteParamsForMachine(machine *state.Machine, command string, timeout time.Duration) *RemoteExec {
	// magic boolean parameters are bad :-(
	address, ok := network.SelectInternalAddress(machine.Addresses(), false)
	execParams := &RemoteExec{
		ExecParams: ssh.ExecParams{
			Command: command,
			Timeout: timeout,
		},
		MachineId: machine.Id(),
	}
	if ok {
		execParams.Host = fmt.Sprintf("ubuntu@%s", address.Value)
	}
	return execParams
}

// getAllUnitNames returns a sequence of valid Unit objects from state. If any
// of the service names or unit names are not found, an error is returned.
func getAllUnitNames(st *state.State, units, services []string) (result []*state.Unit, err error) {
	unitsSet := set.NewStrings(units...)
	for _, name := range services {
		service, err := st.Service(name)
		if err != nil {
			return nil, err
		}
		units, err := service.AllUnits()
		if err != nil {
			return nil, err
		}
		for _, unit := range units {
			unitsSet.Add(unit.Name())
		}
	}
	for _, unitName := range unitsSet.Values() {
		unit, err := st.Unit(unitName)
		if err != nil {
			return nil, err
		}
		// We only operate on units that have an assigned machine.
		if _, err := unit.AssignedMachineId(); err != nil {
			return nil, err
		}
		result = append(result, unit)
	}
	return result, nil
}

func (c *Client) getDataDir() string {
	dataResource, ok := c.api.resources.Get("dataDir").(common.StringResource)
	if !ok {
		return ""
	}
	return dataResource.String()
}

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (c *Client) Run(run params.RunParams) (results params.RunResults, err error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.RunResults{}, errors.Trace(err)
	}
	units, err := getAllUnitNames(c.api.state(), run.Units, run.Services)
	if err != nil {
		return results, err
	}
	// We want to create a RemoteExec for each unit and each machine.
	// If we have both a unit and a machine request, we run it twice,
	// once for the unit inside the exec context using juju-run, and
	// the other outside the context just using bash.
	var params []*RemoteExec
	var quotedCommands = utils.ShQuote(run.Commands)
	for _, unit := range units {
		// We know that the unit is both a principal unit, and that it has an
		// assigned machine.
		machineId, _ := unit.AssignedMachineId()
		machine, err := c.api.stateAccessor.Machine(machineId)
		if err != nil {
			return results, err
		}
		command := fmt.Sprintf("juju-run %s %s", unit.Name(), quotedCommands)
		execParam := remoteParamsForMachine(machine, command, run.Timeout)
		execParam.UnitId = unit.Name()
		params = append(params, execParam)
	}
	for _, machineId := range run.Machines {
		machine, err := c.api.stateAccessor.Machine(machineId)
		if err != nil {
			return results, err
		}
		command := fmt.Sprintf("juju-run --no-context %s", quotedCommands)
		execParam := remoteParamsForMachine(machine, command, run.Timeout)
		params = append(params, execParam)
	}
	return ParallelExecute(c.getDataDir(), params), nil
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (c *Client) RunOnAllMachines(run params.RunParams) (params.RunResults, error) {
	if err := c.check.ChangeAllowed(); err != nil {
		return params.RunResults{}, errors.Trace(err)
	}
	machines, err := c.api.stateAccessor.AllMachines()
	if err != nil {
		return params.RunResults{}, err
	}
	var params []*RemoteExec
	quotedCommands := utils.ShQuote(run.Commands)
	command := fmt.Sprintf("juju-run --no-context %s", quotedCommands)
	for _, machine := range machines {
		params = append(params, remoteParamsForMachine(machine, command, run.Timeout))
	}
	return ParallelExecute(c.getDataDir(), params), nil
}

// RemoteExec extends the standard ssh.ExecParams by providing the machine and
// perhaps the unit ids.  These are then returned in the params.RunResult return
// values.
type RemoteExec struct {
	ssh.ExecParams
	MachineId string
	UnitId    string
}

// ParallelExecute executes all of the requests defined in the params,
// using the system identity stored in the dataDir.
func ParallelExecute(dataDir string, args []*RemoteExec) params.RunResults {
	var results params.RunResults
	results.Results = make([]params.RunResult, len(args), len(args))

	identity := filepath.Join(dataDir, agent.SystemIdentity)
	for i, arg := range args {
		arg.ExecParams.IdentityFile = identity

		results.Results[i] = params.RunResult{
			MachineId: arg.MachineId,
			UnitId:    arg.UnitId,
		}
	}

	startSerialWaitParallel(args, &results, ssh.StartCommandOnMachine, waitOnCommand)

	// TODO(ericsnow) lp:1517076
	// Why do we sort these? Shouldn't we keep them
	// in the same order that they were requested?
	sort.Sort(MachineOrder(results.Results))
	return results
}

// startSerialWaitParallel start a command for each RemoteExec, one at
// a time and then waits for all the results asynchronously.
//
// We do this because ssh.StartCommandOnMachine() relies on os/exec.Cmd,
// which in turn relies on fork+exec. That means every copy of the
// command we run will require a separate fork. This can be a problem
// for controllers with low resources or in environments with many
// machines.
//
// Note that when the start operation completes, the memory of the
// forked process will have already been replaced with that of the
// exec'ed command. This is a relatively quick operation relative to
// the wait operation. That is why start-serially-and-wait-in-parallel
// is a viable approach.
func startSerialWaitParallel(
	args []*RemoteExec,
	results *params.RunResults,
	start func(params ssh.ExecParams) (*ssh.RunningCmd, error),
	wait func(wg *sync.WaitGroup, cmd *ssh.RunningCmd, result *params.RunResult, cancel <-chan struct{}),
) {
	var wg sync.WaitGroup
	for i, arg := range args {
		logger.Debugf("exec on %s: %#v", arg.MachineId, *arg)

		cancel := make(chan struct{})
		go func(d time.Duration) {
			<-clock.WallClock.After(d)
			close(cancel)
		}(arg.Timeout)

		// Start the commands serially...
		cmd, err := start(arg.ExecParams)
		if err != nil {
			results.Results[i].Error = err.Error()
			continue
		}

		wg.Add(1)
		// ...but wait for them in parallel.
		go wait(&wg, cmd, &results.Results[i], cancel)
	}
	wg.Wait()
}

func waitOnCommand(wg *sync.WaitGroup, cmd *ssh.RunningCmd, result *params.RunResult, cancel <-chan struct{}) {
	defer wg.Done()
	response, err := cmd.WaitWithCancel(cancel)
	logger.Debugf("response from %s: %#v (err:%v)", result.MachineId, response, err)
	result.ExecResponse = response
	if err != nil {
		result.Error = err.Error()
	}
}

// MachineOrder is used to provide the api to sort the results by the machine
// id.
type MachineOrder []params.RunResult

func (a MachineOrder) Len() int           { return len(a) }
func (a MachineOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MachineOrder) Less(i, j int) bool { return a[i].MachineId < a[j].MachineId }
