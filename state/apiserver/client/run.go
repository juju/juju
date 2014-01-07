// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
	"path/filepath"
	"sync"
	"time"

	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils/set"
	"launchpad.net/juju-core/utils/ssh"
)

// remoteParamsForMachine returns a filled in RemoteExec instance
// based on the machine, command and timeout params.  If the machine
// does not have an internal address, the Host is empty. This is caught
// by the function that actually tries to execute the command.
func remoteParamsForMachine(machine *state.Machine, command string, timeout time.Duration) *RemoteExec {
	// magic boolean parameters are bad :-(
	address := instance.SelectInternalAddress(machine.Addresses(), false)
	return &RemoteExec{
		ExecParams: ssh.ExecParams{
			Host:    address,
			Command: command,
			Timeout: timeout,
		},
		MachineId: machine.Id(),
	}
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
		// We only operate on principal units, and only thise that have an
		// assigned machines.
		if unit.IsPrincipal() {
			if _, err := unit.AssignedMachineId(); err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("%s is not a principal unit", unit)
		}
		result = append(result, unit)
	}
	return result, nil
}

func (c *Client) Run(params api.RunParams) (results api.RunResults, err error) {
	units, err := getAllUnitNames(c.api.state, params.Units, params.Services)
	if err != nil {
		return results, err
	}
	// We want to create a RemoteExec for each unit and each machine.
	// If we have both a unit and a machine request, we run it twice,
	// once for the unit inside the exec context using juju-run, and
	// the other outside the context just using bash.
	var execParams []*RemoteExec
	var quotedCommand = utils.ShQuote(params.Commands)
	for _, unit := range units {
		// We know that the unit is both a principal unit, and that it has an
		// assigned machine.
		machineId, _ := unit.AssignedMachineId()
		machine, err := c.api.state.Machine(machineId)
		if err != nil {
			return results, err
		}
		command := fmt.Sprintf("juju-run %s %s", unit.Name(), quotedCommand)
		execParam := remoteParamsForMachine(machine, command, params.Timeout)
		execParam.UnitId = unit.Name()
		execParams = append(execParams, execParam)
	}
	for _, machineId := range params.Machines {
		machine, err := c.api.state.Machine(machineId)
		if err != nil {
			return results, err
		}
		execParam := remoteParamsForMachine(machine, params.Commands, params.Timeout)
		execParams = append(execParams, execParam)
	}
	return ParallelExecute(c.api.agentConfig.DataDir(), execParams), nil
}

func (c *Client) RunOnAllMachines(run api.RunParams) (api.RunResults, error) {
	machines, err := c.api.state.AllMachines()
	if err != nil {
		return api.RunResults{}, err
	}
	var params []*RemoteExec
	for _, machine := range machines {
		params = append(params, remoteParamsForMachine(machine, run.Commands, run.Timeout))
	}
	return ParallelExecute(c.api.agentConfig.DataDir(), params), nil
}

type RemoteExec struct {
	ssh.ExecParams
	MachineId string
	UnitId    string
}

func ParallelExecute(dataDir string, params []*RemoteExec) api.RunResults {
	var outstanding sync.WaitGroup
	var lock sync.Mutex
	var result []api.RunResult
	identity := filepath.Join(dataDir, cloudinit.SystemIdentity)
	for _, param := range params {
		outstanding.Add(1)
		param.IdentityFile = identity
		go func() {
			response, err := ssh.ExecuteCommandOnMachine(param.ExecParams)

			execResponse := api.RunResult{
				RemoteResponse: response,
				MachineId:      param.MachineId,
				UnitId:         param.UnitId,
				Error:          err,
			}

			lock.Lock()
			defer lock.Unlock()
			result = append(result, execResponse)
			outstanding.Done()
		}()
	}

	outstanding.Wait()
	return api.RunResults{result}
}
