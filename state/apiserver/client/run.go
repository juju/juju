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
	"launchpad.net/juju-core/state/api/params"
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

// Run the commands specified on the machines identified through the
// list of machines, units and services.
func (c *Client) Run(run params.RunParams) (results params.RunResults, err error) {
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
		command := `[ -f "$HOME/.juju-proxy" ] && . "$HOME/.juju-proxy"`
		command += fmt.Sprintf("\njuju-run --no-context %s", quotedCommands)
		execParam := remoteParamsForMachine(machine, command, run.Timeout)
		params = append(params, execParam)
	}
	return ParallelExecute(c.api.dataDir, params), nil
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (c *Client) RunOnAllMachines(run params.RunParams) (params.RunResults, error) {
	machines, err := c.api.state.AllMachines()
	if err != nil {
		return params.RunResults{}, err
	}
	var params []*RemoteExec
	quotedCommands := utils.ShQuote(run.Commands)
	command := fmt.Sprintf("juju-run --no-context %s", quotedCommands)
	for _, machine := range machines {
		params = append(params, remoteParamsForMachine(machine, command, run.Timeout))
	}
	return ParallelExecute(c.api.dataDir, params), nil
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
func ParallelExecute(dataDir string, runParams []*RemoteExec) params.RunResults {
	logger.Debugf("exec %#v", runParams)
	var outstanding sync.WaitGroup
	var lock sync.Mutex
	var result []params.RunResult
	identity := filepath.Join(dataDir, cloudinit.SystemIdentity)
	for _, param := range runParams {
		outstanding.Add(1)
		logger.Debugf("exec on %s: %#v", param.MachineId, *param)
		param.IdentityFile = identity
		go func(param *RemoteExec) {
			response, err := ssh.ExecuteCommandOnMachine(param.ExecParams)
			logger.Debugf("reponse from %s: %v (err:%v)", param.MachineId, response, err)
			execResponse := params.RunResult{
				ExecResponse: response,
				MachineId:    param.MachineId,
				UnitId:       param.UnitId,
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
	return params.RunResults{result}
}

// MachineOrder is used to provide the api to sort the results by the machine
// id.
type MachineOrder []params.RunResult

func (a MachineOrder) Len() int           { return len(a) }
func (a MachineOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MachineOrder) Less(i, j int) bool { return a[i].MachineId < a[j].MachineId }
