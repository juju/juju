package overlord

import "github.com/juju/juju/overlord/logstate"

type LogManager interface {
	StateManager
	AppendLines([]logstate.Line) error
}
