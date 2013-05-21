// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	return l.Logger.Output(calldepth, s)
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
		l.Logger = stdlog.New(target, "", stdlog.LstdFlags)
		log.SetTarget(l)
	}
	return
}
