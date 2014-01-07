// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"launchpad.net/juju-core/environs/cloudinit"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils/ssh"
)

func (c *Client) Run(params api.RunParams) (api.RunResults, error) {
	return api.RunResults{}, fmt.Errorf("TODO")
}

func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) (api.RunResults, error) {
	machines, err := c.api.state.AllMachines()
	if err != nil {
		return api.RunResults{}, err
	}
	identity := filepath.Join(c.api.agentConfig.DataDir(), cloudinit.SystemIdentity)
	var params []RemoteExec
	for _, machine := range machines {
		address := instance.SelectInternalAddress(machine.Addresses(), false)
		params = append(params, RemoteExec{
			ExecParams: ssh.ExecParams{
				IdentityFile: identity,
				Host:         address,
				Command:      commands,
				Timeout:      timeout,
			},
			MachineId: machine.Id(),
		})
	}
	return ParallelExecute(params), nil
}

type RemoteExec struct {
	ssh.ExecParams
	MachineId string
	UnitId    string
}

func ParallelExecute(params []RemoteExec) api.RunResults {
	var outstanding sync.WaitGroup
	var lock sync.Mutex
	var result []api.RunResult
	for _, param := range params {
		outstanding.Add(1)
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
	return api.RunResult{result}
}
