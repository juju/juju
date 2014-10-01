// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/set"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
)

var logger = loggo.GetLogger("juju.apiserver.runcmd")

func init() {
	common.RegisterStandardFacade("RunCommand", 1, NewRunCommandAPI)
}

// RunCommand defines the methods on the runcmd API end point.
type RunCommand interface {
	Run(args params.RunParamsV1) (params.RunResults, error)
	RunOnAllMachines(run params.RunParamsV1) (params.RunResults, error)
}

// RunCommandAPI implements the run command interface and is the concrete
// implementation of the api end point.
type RunCommandAPI struct {
	state      *state.State
	authorizer common.Authorizer
	resources  *common.Resources
}

var _ RunCommand = (*RunCommandAPI)(nil)

// RemoteExec extends the standard ssh.ExecParams by providing the machine and
// perhaps the unit ids.  These are then returned in the params.RunResult return
// values.
type RemoteExec struct {
	ssh.ExecParams
	MachineId string
	UnitId    string
}

// NewRunCommandAPI returns an initialized RunCommandAPI
func NewRunCommandAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*RunCommandAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &RunCommandAPI{
		state:      st,
		authorizer: authorizer,
		resources:  resources,
	}, nil
}

func (api *RunCommandAPI) Run(runCmd params.RunParamsV1) (results params.RunResults, err error) {
	var remoteParams []*RemoteExec
	var quotedCommands = utils.ShQuote(runCmd.Commands)

	tags, err := api.expandTargets(runCmd.Targets)
	if err != nil {
		return results, errors.Trace(err)
	}

	for _, tag := range tags {
		var execParam *RemoteExec
		command := "juju-run"

		kind := tag.Kind()
		switch kind {
		case names.MachineTagKind:
			machine, err := api.state.Machine(tag.Id())
			if err != nil {
				return results, errors.Trace(err)
			}
			command += fmt.Sprintf(" --no-context %s", quotedCommands)
			execParam = remoteParamsForMachine(machine, command, runCmd.Timeout)
		case names.UnitTagKind:
			if runCmd.Context != nil {
				relation, err := api.getRelation(api.state, tag, runCmd.Context)
				if err != nil {
					return results, errors.Annotate(err, "--relation")
				}

				if relation.isValidRelation() {
					command += fmt.Sprintf(" --relation %d", relation.Id)
				}

				if relation.isValidRemoteUnit() {
					command += fmt.Sprintf(" --remote-unit %s", relation.RemoteUnit)
				}
			}

			machine, err := api.machineFromUnitTag(tag)
			if err != nil {
				return results, errors.Trace(err)
			}

			command += fmt.Sprintf(" %s %s", tag.Id(), quotedCommands)
			execParam = remoteParamsForMachine(machine, command, runCmd.Timeout)
			execParam.UnitId = tag.Id()
		}
		remoteParams = append(remoteParams, execParam)
	}
	return ParallelExecute(getDataDir(api), remoteParams), nil
}

// RunOnAllMachines attempts to run the specified command on all the machines.
func (api *RunCommandAPI) RunOnAllMachines(run params.RunParamsV1) (params.RunResults, error) {
	machines, err := api.state.AllMachines()
	if err != nil {
		return params.RunResults{}, err
	}
	var remoteParams []*RemoteExec
	quotedCommands := utils.ShQuote(run.Commands)
	command := fmt.Sprintf("juju-run --no-context %s", quotedCommands)
	for _, machine := range machines {
		remoteParams = append(remoteParams, remoteParamsForMachine(machine, command, run.Timeout))
	}
	return ParallelExecute(getDataDir(api), remoteParams), nil
}

// ParallelExecute executes all of the requests defined in the params,
// using the system identity stored in the dataDir.
func ParallelExecute(dataDir string, runParams []*RemoteExec) params.RunResults {
	logger.Debugf("exec %#v", runParams)
	var outstanding sync.WaitGroup
	var lock sync.Mutex
	var result []params.RunResult
	identity := filepath.Join(dataDir, agent.SystemIdentity)
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

func getDataDir(api *RunCommandAPI) string {
	dataResource, ok := api.resources.Get("dataDir").(common.StringResource)
	if !ok {
		return ""
	}
	return dataResource.String()
}

// machineFromUnitTag attempts to find the state.Machine for a given tag.
func (api *RunCommandAPI) machineFromUnitTag(tag names.Tag) (*state.Machine, error) {
	unit, err := api.state.Unit(tag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineId, err := unit.AssignedMachineId()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return api.state.Machine(machineId)
}

// expandTargets filters the list of targets to a unique set.
// This includes expanding and filtering the duplicate targets from
// different services. The result is a list of names.Tag.
func (api *RunCommandAPI) expandTargets(targets []string) ([]names.Tag, error) {
	tagSet := set.NewTags()
	for _, target := range targets {
		tag, err := names.ParseTag(target)
		if err != nil {
			return nil, err
		}
		kind := tag.Kind()
		switch kind {
		case names.MachineTagKind, names.UnitTagKind:
			tagSet.Add(tag)
		case names.ServiceTagKind:
			service, err := api.state.Service(tag.Id())
			if err != nil {
				return nil, errors.Trace(err)
			}
			units, err := service.AllUnits()
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, unit := range units {
				tagSet.Add(unit.Tag())
			}
		}
	}
	return tagSet.Values(), nil
}

// remoteParamsForMachine returns a filled in RemoteExec instance
// based on the machine, command and timeout params.  If the machine
// does not have an internal address, the Host is empty. This is caught
// by the function that actually tries to execute the command.
func remoteParamsForMachine(machine *state.Machine, command string, timeout time.Duration) *RemoteExec {
	// magic boolean parameters are bad :-(
	address := network.SelectInternalAddress(machine.Addresses(), false)
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

type commandRelation struct {
	Id         int
	RemoteUnit string
}

func (c commandRelation) isValidRelation() bool {
	return c.Id > -1
}

func (c commandRelation) isValidRemoteUnit() bool {
	return len(c.RemoteUnit) > 0
}

// getRelation takes a RunContext and turns the string representations of a Relation
// and RemoteUnit in to an actual state.Relation Id (relatioin)
// and state.Unit Name (remoteUnit).
func (api *RunCommandAPI) getRelation(state *state.State, tag names.Tag, context *params.RunContext) (commandRelation, error) {
	result := commandRelation{Id: -1, RemoteUnit: ""}

	if len(context.Relation) > 0 {
		endpoints, err := api.state.InferEndpoints(names.UnitService(tag.Id()), context.Relation)
		if err != nil {
			return result, errors.Trace(err)
		}

		relation, err := api.state.EndpointsRelation(endpoints...)
		if err != nil {
			return result, errors.Errorf("no relation %s", context.Relation)
		}
		result.Id = relation.Id()
	}

	if len(context.RemoteUnit) > 0 {
		remoteUnit, err := api.state.Unit(context.RemoteUnit)
		if err != nil {
			return result, errors.Errorf("%s is not a valid remote-unit", context.RemoteUnit)
		}
		result.RemoteUnit = remoteUnit.Name()
	}
	return result, nil
}

// MachineOrder is used to provide the api to sort the results by the machine
// id.
type MachineOrder []params.RunResult

func (a MachineOrder) Len() int           { return len(a) }
func (a MachineOrder) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a MachineOrder) Less(i, j int) bool { return a[i].MachineId < a[j].MachineId }
