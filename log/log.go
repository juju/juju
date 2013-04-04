package log

import (
	"fmt"
	"sync"
)

type Logger interface {
	Output(calldepth int, s string) error
}

var (
	target struct {
		sync.Mutex
		logger Logger
	}
	Debug bool
)

// Target returns the current log target.
func Target() Logger {
	target.Lock()
	defer target.Unlock()
	return target.logger
}

// SetTarget sets the logging target and returns its
// previous value.
func SetTarget(logger Logger) (prev Logger) {
	target.Lock()
	defer target.Unlock()
	prev = target.logger
	target.logger = logger
	return
}

func logf(format string, a ...interface{}) error {
	if target := Target(); target != nil {
		const calldepth = 3 // magic
		return target.Output(calldepth, fmt.Sprintf(format, a...))
	}
	return nil
}

// Errorf logs a message using the ERROR priority.
func Errorf(format string, a ...interface{}) error {
	return logf("ERROR "+format, a...)
}

// Warningf logs a message using the WARNING priority.
func Warningf(format string, a ...interface{}) error {
	return logf("WARNING "+format, a...)
}

// Noticef logs a message using the NOTICE priority.
func Noticef(format string, a ...interface{}) error {
	return logf("NOTICE "+format, a...)
}

// Infof logs a message using the INFO priority.
func Infof(format string, a ...interface{}) error {
	return logf("INFO "+format, a...)
}

// Debugf logs a message using the DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	if Debug {
		return logf("DEBUG "+format, a...)
	}
	return nil
}
