// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/juju/workload"
)

func AddPayload(ctx *Context, id string, pl workload.Payload) {
	if _, ok := ctx.payloads[id]; !ok {
		ctx.payloads[id] = pl
	} else {
		ctx.updates[pl.FullID()] = pl
	}
}

func AddPayloads(ctx *Context, payloads ...workload.Payload) {
	for _, pl := range payloads {
		AddPayload(ctx, pl.FullID(), pl)
	}
}
