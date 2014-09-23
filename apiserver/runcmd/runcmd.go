// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils"
	"github.com/juju/utils/set"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/utils/ssh"
)

var logger = loggo.GetLogger("juju.apiserver.runcmd")

func init() {
	common.RegisterStandardFacade("RunCommand", 0, NewRunCommandAPI)
}

// RunCommand defines the methods on the runcmd API end point.
type RunCommand interface {
	Run(args []RunCommands) (params.RunResults, error)
}

// RunCommands holds the information for a `juju run` command.
type RunCommands struct {
	Commands string
	Targets  []string
	Context  *RunContext
	Timeout  time.Duration
}

// RunContext holds the information for a `juju-run` command
// that was provided the --relation option.
type RunContext struct {
	Relation   string
	RemoteUnit string
}

// RunCommandAPI implements the run command interface and is the concrete
// implementation of the api end point.
type RunCommandAPI struct {
	state      *state.State
	authorizer common.Authorizer
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
	}, nil
}

func (api *RunCommandAPI) Run(runCommands []RunCommands) (results params.RunResults, err error) {
	for _, runCmd := range runCommands {
		var params []*RemoteExec
		var quotedCommands = utils.ShQuote(runCmd.Commands)

		command := "juju-run"

		tags, err := api.expandTargets(runCmd.Targets)
		if err != nil {
			return results, errors.Trace(err)
		}

		for _, tag := range tags {
			var execParam *RemoteExec

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
					relation, err := api.getRelation(api.state, runCmd.Context)
					if err != nil {
						return results, errors.Trace(err)
					}
					command += fmt.Sprintf(" --relation %d", relation.Id)
					command += fmt.Sprintf(" --remote-unit %s", relation.RemoteUnit)
				}

				machine, err := api.machineFromUnitTag(tag)
				if err != nil {
					return results, errors.Trace(err)
				}

				command += fmt.Sprintf(" %s %s", tag.Id(), quotedCommands)
				execParam = remoteParamsForMachine(machine, command, runCmd.Timeout)
				execParam.UnitId = tag.Id()
			}
			params = append(params, execParam)
		}
	}
	return results, nil
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
	tagSet := set.Tags{}
	for _, target := range targets {
		tag, err := names.ParseTag(target)
		if err != nil {
			return nil, errors.Trace(err)
		}

		kind := tag.Kind()
		switch kind {
		case names.MachineTagKind:
		case names.UnitTagKind:
			tagSet.Add(tag)
		case names.ServiceTagKind:
			service, err := api.state.Service(tag.String())
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

// getRelation takes a RunContext and turns the string representations of a Relation
// and RemoteUnit in to an actual state.Relation Id (relatioin)
// and state.Unit Name (remoteUnit).
func (api *RunCommandAPI) getRelation(state *state.State, context *RunContext) (commandRelation, error) {
	var empty commandRelation

	endpoints, err := api.state.InferEndpoints(context.Relation)
	if err != nil {
		return empty, errors.Trace(err)
	}

	relation, err := api.state.EndpointsRelation(endpoints...)
	if err != nil {
		return empty, errors.Trace(err)
	}

	remoteUnit, err := api.state.Unit(context.RemoteUnit)
	if err != nil {
		return empty, errors.Trace(err)
	}

	return commandRelation{
		Id:         relation.Id(),
		RemoteUnit: remoteUnit.Name(),
	}, nil
}
