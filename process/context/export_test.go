// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

func SetComponent(cmd cmd.Command, compCtx jujuc.ContextComponent) {
	switch cmd := cmd.(type) {
	case *RegisterCommand:
		cmd.compCtx = compCtx
	}
}
