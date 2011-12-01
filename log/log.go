package log

import "fmt"

type Logger interface {
	Output(calldepth int, s string) error
}

var (
	GlobalLogger Logger
	Debug        bool
)

const (
	logPrefix = "JUJU "
	dbgPrefix = "JUJU:DEBUG "
)

// Logf logs the formatted message onto the Logger set via SetLogger.
func Logf(format string, v ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Output(2, logPrefix+fmt.Sprintf(format, v...))
	}
}

// Debugf logs the formatted message onto the Logger set via SetLogger,
// as long as debugging was enabled with SetDebug.
func Debugf(format string, v ...interface{}) {
	if Debug && GlobalLogger != nil {
		GlobalLogger.Output(2, dbgPrefix+fmt.Sprintf(format, v...))
	}
}
