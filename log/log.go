package log

import "fmt"
import "io"
import "os"
import stdlog "log"

type Logger interface {
	Output(calldepth int, s string) error
}

type LogInfo interface {
    Logfile() string
    Verbose() bool
}

var (
	Target Logger
	Debug  bool
)

func Init(i LogInfo) error {
    var target io.Writer = os.Stderr
    if i.Logfile() != "" {
        logfile, err := os.OpenFile(i.Logfile(), os.O_WRONLY|os.O_APPEND, 0644)
        if err != nil {
            return err
        }
        target = io.MultiWriter(logfile, os.Stderr)
    }
    Target = stdlog.New(target, "", 0)
    Debug = i.Verbose()
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
