// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/juju/juju/core/paths"
)

// NewFileWriter returns an io.WriteCloser that will write log messages to disk.
func NewFileWriter(logPath string, maxSize, maxBackups int) (io.WriteCloser, error) {
	if err := paths.PrimeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming fails.
		logger.Warningf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}
	return &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSize, // MB
		MaxBackups: maxBackups,
		Compress:   true,
	}, nil
}
