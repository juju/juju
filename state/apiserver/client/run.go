// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
	"path/filepath"
	"sort"
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
	execParams := &RemoteExec{
		ExecParams: ssh.ExecParams{
			Command: command,
			Timeout: timeout,
		},
		MachineId: machine.Id(),
	}
	if address != "" {
		execParams.Host = fmt.Sprintf("ubuntu@%s", address)
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

func (c *Client) Run(run api.RunParams) (results api.RunResults, err error) {
	units, err := getAllUnitNames(c.api.state, run.Units, run.Services)
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
		machine, err := c.api.state.Machine(machineId)
		if err != nil {
			return results, err
		}
		command := fmt.Sprintf("juju-run %s %s", unit.Name(), quotedCommands)
		execParam := remoteParamsForMachine(machine, command, run.Timeout)
		execParam.UnitId = unit.Name()
		params = append(params, execParam)
	}
	for _, machineId := range run.Machines {
		machine, err := c.api.state.Machine(machineId)
		if err != nil {
			return results, err
		}
		execParam := remoteParamsForMachine(machine, run.Commands, run.Timeout)
		params = append(params, execParam)
	}
	return ParallelExecute(c.api.agentConfig.DataDir(), params), nil
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

// ParallelExecute executes all of the requests defined in the params,
// using the system identity stored in the dataDir.
func ParallelExecute(dataDir string, params []*RemoteExec) api.RunResults {
	logger.Debugf("exec %#v", params)
	var outstanding sync.WaitGroup
	var lock sync.Mutex
	var result []api.RunResult
	identity := filepath.Join(dataDir, cloudinit.SystemIdentity)
	for _, param := range params {
		outstanding.Add(1)
		logger.Debugf("exec on %s: %#v", param.MachineId, *param)
		param.IdentityFile = identity
		go func(param *RemoteExec) {
			response, err := ssh.ExecuteCommandOnMachine(param.ExecParams)
			logger.Debugf("reponse from %s: %v (err:%v)", param.MachineId, response, err)
			execResponse := api.RunResult{
				RemoteResponse: response,
				MachineId:      param.MachineId,
				UnitId:         param.UnitId,
			}
			if err != nil {
				execResponse.Error = fmt.Sprint(err)
			}

			lock.Lock()
			defer lock.Unlock()
			result = append(result, execResponse)
			outstanding.Done()
		}(param)
	}

	outstanding.Wait()
	sort.Sort(MachineOrder(result))
	return api.RunResults{result}
}

type MachineOrder []api.RunResult

func (a MachineOrder) Len() int           { return len(a) }
func (a MachineOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MachineOrder) Less(i, j int) bool { return a[i].MachineId < a[j].MachineId }
