package cmd

import (
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/log"
	stdlog "log"
	"os"
	"strings"
)

// Log supplies the necessary functionality for Commands that wish to set up
// logging.
type Log struct {
	Prefix  string
	Path    string
	Verbose bool
	Debug   bool
	log.Logger
}

// AddFlags adds appropriate flags to f.
func (l *Log) AddFlags(f *gnuflag.FlagSet) {
	f.StringVar(&l.Path, "log-file", "", "path to write log to")
	f.BoolVar(&l.Verbose, "v", false, "if set, log additional messages")
	f.BoolVar(&l.Verbose, "verbose", false, "if set, log additional messages")
	f.BoolVar(&l.Debug, "debug", false, "if set, log debugging messages")
}

func (l *Log) Output(calldepth int, s string) error {
	// split the log line between the LEVEL and the message
	output := strings.SplitN(s, " ", 2)
	// recombine it inserting our prefix between the LEVEL and the message
	output = []string{output[0], l.Prefix, output[1]}
	return l.Logger.Output(calldepth, strings.Join(output, " "))
}

// Start starts logging using the given Context.
func (l *Log) Start(ctx *Context) (err error) {
	log.Debug = l.Debug
	var target io.Writer
	if l.Path != "" {
		path := ctx.AbsPath(l.Path)
		target, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return
		}
	} else if l.Verbose || l.Debug {
		target = ctx.Stderr
	}
	if target != nil {
		l.Prefix = "JUJU:" + l.Prefix
		l.Logger = stdlog.New(target, "", stdlog.LstdFlags)
		log.SetTarget(l)
	}
	return
}
