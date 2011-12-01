package log

import "fmt"

type Logger interface {
	Output(calldepth int, s string) error
}

var (
	Target Logger
	Debug  bool
)

// Printf logs the formatted message onto the Target Logger.
func Printf(format string, v ...interface{}) {
	if Target != nil {
		Target.Output(2, "JUJU "+fmt.Sprintf(format, v...))
	}
}

// Debugf logs the formatted message onto the Target Logger
// if Debug is true.
func Debugf(format string, v ...interface{}) {
	if Debug && Target != nil {
		Target.Output(2, "JUJU:DEBUG "+fmt.Sprintf(format, v...))
	}
}
