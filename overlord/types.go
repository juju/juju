// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"context"

	"github.com/juju/juju/overlord/logstate"
)

type LogManager interface {
	StateManager
	AppendLines(context.Context, []logstate.Line) error
}
