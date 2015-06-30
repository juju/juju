// Copyright 2015 Canonical Ltd.
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
	"github.com/juju/juju/process/plugin"
)

var logger = loggo.GetLogger("juju.process.api.server")

// API serves workload process-specific API methods.
type API struct {
	st State
}

// State is an interface that exposes functionality this package needs to wrap
// in an API.
type State interface {
	RegisterProcess(unit names.UnitTag, info process.Info) error
	ListProcesses(unit names.UnitTag, ids []string) ([]process.Info, error)
	SetProcessStatus(unit names.UnitTag, id string, status string) error
	UnregisterProcess(unit names.UnitTag, id string) error
}

// NewAPI creates a new instance of the Process API facade.
func NewAPI(st State, authorizer common.Authorizer) (API, error) {
	if !authorizer.AuthUnitAgent() {
		return API{}, errors.Trace(common.ErrPerm)
	}
	return API{st: st}, nil
}

// RegisterProcess registers a workload process in state.
func (a API) RegisterProcess(args api.RegisterProcessArgs) error {
	info := api2Proc(args.ProcessInfo)
	unit := names.NewUnitTag(info.Name)

	return errors.Trace(a.st.RegisterProcess(unit, info))
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (a *API) ListProcesses(args api.ListProcessesArgs) ([]api.ProcessInfo, error) {
	infos, err := a.st.ListProcesses(names.NewUnitTag(args.UnitTag), args.IDs)
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
func (a *API) SetProcessStatus(args api.SetProcessStatusArgs) error {
	unit := names.NewUnitTag(args.UnitTag)
	return errors.Trace(a.st.SetProcessStatus(unit, args.ID, args.Status.Status))
}

// UnregisterProcess marks the identified process as unregistered.
func (a *API) UnregisterProcess(args api.UnregisterProcessArgs) error {
	unit := names.NewUnitTag(args.UnitTag)
	return errors.Trace(a.st.UnregisterProcess(unit, args.ID))
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
			Ports:       api2charmPorts(p.Process.Ports),
			Volumes:     api2charmVolumes(p.Process.Volumes),
			EnvVars:     p.Process.EnvVars,
		},
		Status: process.Status(p.Status),
		Details: plugin.ProcDetails{
			ID: p.Details.ID,
			ProcStatus: plugin.ProcStatus{
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
			Ports:       charm2apiPorts(p.Process.Ports),
			Volumes:     charm2apiVolumes(p.Process.Volumes),
			EnvVars:     p.Process.EnvVars,
		},
		Status: int(p.Status),
		Details: api.ProcDetails{
			ID: p.Details.ID,
			ProcStatus: api.ProcStatus{
				Status: p.Details.Status,
			},
		},
	}
}

func api2charmPorts(ports []api.ProcessPort) []charm.ProcessPort {
	ret := make([]charm.ProcessPort, len(ports))
	for i, p := range ports {
		ret[i] = charm.ProcessPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

func api2charmVolumes(vols []api.ProcessVolume) []charm.ProcessVolume {
	ret := make([]charm.ProcessVolume, len(vols))
	for i, v := range vols {
		ret[i] = charm.ProcessVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}

func charm2apiPorts(ports []charm.ProcessPort) []api.ProcessPort {
	ret := make([]api.ProcessPort, len(ports))
	for i, p := range ports {
		ret[i] = api.ProcessPort{
			Internal: p.Internal,
			External: p.External,
			Endpoint: p.Endpoint,
		}
	}
	return ret
}

func charm2apiVolumes(vols []charm.ProcessVolume) []api.ProcessVolume {
	ret := make([]api.ProcessVolume, len(vols))
	for i, v := range vols {
		ret[i] = api.ProcessVolume{
			ExternalMount: v.ExternalMount,
			InternalMount: v.InternalMount,
			Mode:          v.Mode,
			Name:          v.Name,
		}
	}
	return ret
}
