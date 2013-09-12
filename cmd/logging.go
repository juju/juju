// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
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
	ShowLog bool
	Config  string
}

// AddFlags adds appropriate flags to f.
func (l *Log) AddFlags(f *gnuflag.FlagSet) {
	f.StringVar(&l.Path, "log-file", "", "path to write log to")
	// TODO(thumper): rename verbose to --show-log
	f.BoolVar(&l.Verbose, "v", false, "if set, log additional messages")
	f.BoolVar(&l.Verbose, "verbose", false, "if set, log additional messages")
	f.BoolVar(&l.Debug, "debug", false, "if set, log debugging messages")
	f.StringVar(&l.Config, "log-config", "", "specify log levels for modules")
	f.BoolVar(&l.ShowLog, "show-log", false, "if set, write the log file to stderr")
}

// Start starts logging using the given Context.
func (l *Log) Start(ctx *Context) error {
	if l.Path != "" {
		path := ctx.AbsPath(l.Path)
		target, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		writer := loggo.NewSimpleWriter(target, &loggo.DefaultFormatter{})
		err = loggo.RegisterWriter("logfile", writer, loggo.TRACE)
		if err != nil {
			return err
		}
	}
	level := loggo.WARNING
	if l.Verbose {
		ctx.Stdout.Write([]byte("verbose is deprecated with the current meaning, use show-log\n"))
		l.ShowLog = true
	}
	if l.ShowLog {
		level = loggo.INFO
	}
	if l.Debug {
		l.ShowLog = true
		level = loggo.DEBUG
	}

	if l.ShowLog {
		// We replace the default writer to use ctx.Stderr rather than os.Stderr.
		writer := loggo.NewSimpleWriter(ctx.Stderr, &loggo.DefaultFormatter{})
		_, err := loggo.ReplaceDefaultWriter(writer)
		if err != nil {
			return err
		}
	} else {
		loggo.RemoveWriter("default")
	}
	// Set the level on the root logger.
	loggo.GetLogger("").SetLogLevel(level)
	// Override the logging config with specified logging config.
	loggo.ConfigureLoggers(l.Config)
	return nil
}
