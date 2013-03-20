package log

import (
	"fmt"
	"sync"
)

type Logger interface {
	Output(calldepth int, s string) error
}

// nilLogger discards any output sent to its Output method.
type nilLogger struct{}

func (nilLogger) Output(int, string) error { return nil }

var (
	target struct {
		sync.Mutex
		logger Logger
	}
	Debug bool

	// NilLogger is the default log target.
	NilLogger = nilLogger{}
)

func init() {
	SetTarget(NilLogger)
}

// Target returns the current log target.
func Target() Logger {
	target.Lock()
	defer target.Unlock()
	return target.logger
}

// SetTarget sets the logging target and returns the
// previous value of the logging target.
func SetTarget(logger Logger) (prev Logger) {
	target.Lock()
	defer target.Unlock()
	prev = target.logger
	target.logger = logger
	return
}

// Errorf logs a message using the ERROR priority.
func Errorf(format string, a ...interface{}) (err error) {
	return Target().Output(2, "ERROR: "+fmt.Sprintf(format, a...))
}

// Warningf logs a message using the WARNING priority.
func Warningf(format string, a ...interface{}) (err error) {
	return Target().Output(2, "WARNING: "+fmt.Sprintf(format, a...))
}

// Noticef logs a message using the NOTICE priority.
func Noticef(format string, a ...interface{}) (err error) {
	return Target().Output(2, "NOTICE: "+fmt.Sprintf(format, a...))
}

// Infof logs a message using the INFO priority.
func Infof(format string, a ...interface{}) (err error) {
	return Target().Output(2, "INFO: "+fmt.Sprintf(format, a...))
}

// Debugf logs a message using the DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	if Debug {
		return Target().Output(2, "DEBUG: "+fmt.Sprintf(format, a...))
	}
	return nil
}
