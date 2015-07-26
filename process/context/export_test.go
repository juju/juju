// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/process"
)

func SetComponent(cmd cmd.Command, compCtx Component) {
	switch cmd := cmd.(type) {
	case *ProcRegistrationCommand:
		cmd.compCtx = compCtx
	case *ProcInfoCommand:
		cmd.compCtx = compCtx
	}
	// TODO(ericsnow) Add ProcLaunchCommand here.
}

func AddProc(ctx *Context, id string, original *process.Info) {
	var proc *process.Info
	if original != nil {
		if id != original.ID() {
			panic("ID mismatch")
		}
		info := *original
		proc = &info
	}
	if _, ok := ctx.processes[id]; !ok {
		ctx.processes[id] = proc
	} else {
		if proc == nil {
			panic("update can't be nil")
		}
		info := *proc
		ctx.updates[info.ID()] = &info
	}
}

func AddProcs(ctx *Context, procs ...*process.Info) {
	for _, proc := range procs {
		AddProc(ctx, proc.Name, proc)
	}
}

func GetCmdInfo(cmd cmd.Command) *process.Info {
	switch cmd := cmd.(type) {
	case *ProcRegistrationCommand:
		return cmd.info
	default:
		return nil
	}
}
