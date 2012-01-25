package log

import (
	"fmt"
	"io"
	stdlog "log"
	"os"
)

var stderr io.Writer = os.Stderr

type Logger interface {
	Output(calldepth int, s string) error
}

var (
	Target Logger
	Debug  bool
)

// SetFile sets Target such that log functions will always write to os.Stderr
// and optionally (ie if path is not empty) a log file.
func SetFile(path string) error {
	var target io.Writer = stderr
	if path != "" {
		logfile, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		target = io.MultiWriter(logfile, stderr)
	}
	Target = stdlog.New(target, "", 0)
	return nil
}

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
