package log

import (
	"fmt"
)

type Logger interface {
	Output(calldepth int, s string) error
}

var (
	Target Logger
	Debug  bool
)

// Errorf logs a message using the ERROR priority.
func Errorf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "ERROR: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Warningf logs a message using the WARNING priority.
func Warningf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "WARNING: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Noticef logs a message using the NOTICE priority.
func Noticef(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "NOTICE: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Infof logs a message using the INFO priority.
func Infof(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "INFO: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Debugf logs a message using the DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	if Debug && Target != nil {
		return Target.Output(2, "DEBUG: "+fmt.Sprintf(format, a...))
	}
	return nil
}
