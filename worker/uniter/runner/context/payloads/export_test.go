// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"github.com/juju/juju/payload"
)

func ContextPayloads(ctx *PayloadsHookContext) map[string]payload.Payload {
	return ctx.payloads
}
