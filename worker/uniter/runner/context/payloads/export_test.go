// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"github.com/juju/juju/v2/core/payloads"
)

func ContextPayloads(ctx *PayloadsHookContext) map[string]payloads.Payload {
	return ctx.payloads
}
