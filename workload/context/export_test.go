// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/workload"
)

func SetComponent(cmd cmd.Command, compCtx Component) {
	switch cmd := cmd.(type) {
	case *WorkloadTrackCommand:
		cmd.compCtx = compCtx
	case *WorkloadInfoCommand:
		cmd.compCtx = compCtx
	}
	// TODO(ericsnow) Add WorkloadLaunchCommand here.
}

func AddWorkload(ctx *Context, id string, info workload.Info) {
	if _, ok := ctx.workloads[id]; !ok {
		ctx.workloads[id] = info
	} else {
		ctx.updates[info.ID()] = info
	}
}

func AddWorkloads(ctx *Context, workloads ...workload.Info) {
	for _, wl := range workloads {
		AddWorkload(ctx, wl.Name, wl)
	}
}

func GetCmdInfo(cmd cmd.Command) *workload.Info {
	switch cmd := cmd.(type) {
	case *WorkloadTrackCommand:
		return cmd.info
	case *WorkloadLaunchCommand:
		return cmd.info
	default:
		return nil
	}
}

var ArgNameOrId = idArg
