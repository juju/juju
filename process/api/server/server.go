// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Process", 0, NewAPI)
}

var logger = loggo.GetLogger("juju.process.api.server")

// API serves workload process-specific API methods.
type API struct {
	state *state.State
}

// NewAPI creates a new instance of the Process API facade.
func NewAPI(st *state.State, _ *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthUnitAgent() {
		return nil, errors.Trace(common.ErrPerm)
	}
	return &API{st}, nil
}

// RegisterProcess registers a workload process in state.
func (a *API) RegisterProcess(args api.RegisterProcessArgs) error {
	info := api2Proc(args.ProcessInfo)
	unit := names.NewUnitTag(info.Name)

	// return a.st.RegisterProcess(unit, info)
	return nil
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (a *API) ListProcesses(args api.ListProcessesArgs) ([]api.ProcessInfo, error) {
	var infos []process.Info
	var err error
	// infos, err := a.st.ListProcesses(names.NewUnitTag(args.tag), args.ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rets := make([]api.ProcessInfo, len(infos))
	for i, info := range infos {
		rets[i] = proc2api(info)
	}
	return rets, nil
}

// SetProcessStatus sets the raw status of a workload process.
func (a *API) SetProcessStatus(unit names.UnitTag, id string, status process.Status) error {
}

// UnregisterProcess marks the identified process as unregistered.
func (a *API) UnregisterProcess(unit names.UnitTag, id string) error {
	ps := newUnitProcesses(st, unit, nil)
	if err := ps.Unregister(id); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func api2Proc(p api.ProcessInfo) process.Info {
	return process.Info{
		Process: charm.Process{
			Name:        p.Process.Name,
			Description: p.Process.Description,
			Type:        p.Process.Type,
			TypeOptions: p.Process.TypeOptions,
			Command:     p.Process.Command,
			Image:       p.Process.Image,
			Ports:       p.Process.Ports,
			Volumes:     p.Process.Volumes,
			EnvVars:     p.Process.EnvVars,
		},
		Status: process.Status(p.Status),
		Details: process.ProcDetails{
			ID: p.Details.ID,
			ProcStatus: process.ProcStatus{
				Status: p.Details.Status,
			},
		},
	}
}

func proc2api(p process.Info) api.ProcessInfo {
	return api.ProcessInfo{
		Process: api.Process{
			Name:        p.Process.Name,
			Description: p.Process.Description,
			Type:        p.Process.Type,
			TypeOptions: p.Process.TypeOptions,
			Command:     p.Process.Command,
			Image:       p.Process.Image,
			Ports:       p.Process.Ports,
			Volumes:     p.Process.Volumes,
			EnvVars:     p.Process.EnvVars,
		},
		Status: api.Status(p.Status),
		Details: api.ProcDetails{
			ID: p.Details.ID,
			ProcStatus: api.ProcStatus{
				Status: p.Details.Status,
			},
		},
	}
}
