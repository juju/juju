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

// Emerg logs a message using the LOG_EMERG priority.
func Emergencyf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "EMERGENCY: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Alert logs a message using the LOG_ALERT priority.
func Alertf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "ALERT: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Crit logs a message using the LOG_CRIT priority.
func Criticalf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "CRITICAL: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Err logs a message using the LOG_ERR priority.
func Errorf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "ERROR: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Warning logs a message using the LOG_WARNING priority.
func Warningf(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "WARNING: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Notice logs a message using the LOG_NOTICE priority.
func Noticef(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "NOTICE: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Info logs a message using the LOG_INFO priority.
func Infof(format string, a ...interface{}) (err error) {
	if Target != nil {
		return Target.Output(2, "INFO: "+fmt.Sprintf(format, a...))
	}
	return nil
}

// Debug logs a message using the LOG_DEBUG priority.
func Debugf(format string, a ...interface{}) (err error) {
	if Debug && Target != nil {
		return Target.Output(2, "DEBUG: "+fmt.Sprintf(format, a...))
	}
	return nil
}
