package juju

import "fmt"

type Logger interface {
	Output(calldepth int, s string) error
}

var globalLogger Logger
var globalDebug bool

const (
	logPrefix = "JUJU "
	dbgPrefix = "JUJU:DEBUG "
)

// Specify the *log.Logger object where log messages should be sent to.
func SetLogger(logger Logger) {
	globalLogger = logger
}

// Enable the delivery of debug messages to the logger.  Only meaningful
// if a logger is also set.
func SetDebug(debug bool) {
	globalDebug = debug
}

// Logf logs the formatted message onto the Logger set via SetLogger.
func Logf(format string, v ...interface{}) {
	if globalLogger != nil {
		globalLogger.Output(2, logPrefix+fmt.Sprintf(format, v...))
	}
}

// Debugf logs the formatted message onto the Logger set via SetLogger,
// as long as debugging was enabled with SetDebug.
func Debugf(format string, v ...interface{}) {
	if globalDebug && globalLogger != nil {
		globalLogger.Output(2, dbgPrefix+fmt.Sprintf(format, v...))
	}
}
