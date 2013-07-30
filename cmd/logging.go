// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"io"
	"os"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"
)

// Log supplies the necessary functionality for Commands that wish to set up
// logging.
type Log struct {
	Path    string
	Verbose bool
	Debug   bool
	Config  string
}

// AddFlags adds appropriate flags to f.
func (l *Log) AddFlags(f *gnuflag.FlagSet) {
	f.StringVar(&l.Path, "log-file", "", "path to write log to")
	// TODO(thumper): rename verbose to --show-log
	f.BoolVar(&l.Verbose, "v", false, "if set, log additional messages")
	f.BoolVar(&l.Verbose, "verbose", false, "if set, log additional messages")
	f.BoolVar(&l.Debug, "debug", false, "if set, log debugging messages")
	defaultLogConfig := os.Getenv("JUJU_LOGGING_CONFIG")
	f.StringVar(&l.Config, "log-config", defaultLogConfig, "specify log levels for modules")
}

// Start starts logging using the given Context.
func (l *Log) Start(ctx *Context) (err error) {
	var target io.Writer
	if l.Path != "" {
		path := ctx.AbsPath(l.Path)
		target, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
	} else if l.Verbose || l.Debug {
		target = ctx.Stderr
	}

	if target != nil {
		writer := loggo.NewSimpleWriter(target, &loggo.DefaultFormatter{})
		_, err = loggo.ReplaceDefaultWriter(writer)
		if err != nil {
			return err
		}
	} else {
		loggo.RemoveWriter("default")
	}
	if l.Verbose || l.Debug {
		level := loggo.INFO
		if l.Debug {
			level = loggo.DEBUG
		}
		// Set the level on the root logger.
		loggo.GetLogger("").SetLogLevel(level)
	}
	loggo.ConfigureLoggers(l.Config)
	return nil
}
