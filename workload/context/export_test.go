// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/juju/workload"
)

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
