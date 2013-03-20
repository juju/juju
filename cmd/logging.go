package cmd

import (
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/log"
	stdlog "log"
	"os"
)

// Log supplies the necessary functionality for Commands that wish to set up
// logging.
type Log struct {
	Prefix  string
	Path    string
	Verbose bool
	Debug   bool
}

// AddFlags adds appropriate flags to f.
func (c *Log) AddFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.Path, "log-file", "", "path to write log to")
	f.BoolVar(&c.Verbose, "v", false, "if set, log additional messages")
	f.BoolVar(&c.Verbose, "verbose", false, "if set, log additional messages")
	f.BoolVar(&c.Debug, "debug", false, "if set, log debugging messages")
}

// Start starts logging using the given Context.
func (c *Log) Start(ctx *Context) (err error) {
	log.Debug = c.Debug
	log.Target = nil
	var target io.Writer
	prefix := "JUJU:" + c.Prefix
	if c.Path != "" {
		path := ctx.AbsPath(c.Path)
		target, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return
		}
	} else if c.Verbose || c.Debug {
		target = ctx.Stderr
	}
	if target != nil {
		log.Target = stdlog.New(target, prefix+": ", stdlog.LstdFlags)
	}
	return
}
