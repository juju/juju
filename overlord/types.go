package overlord

import (
	"context"

	"github.com/juju/juju/overlord/logstate"
)

type LogManager interface {
	StateManager
	AppendLines(context.Context, []logstate.Line) error
}
