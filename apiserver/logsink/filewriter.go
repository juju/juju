// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"io"

	"github.com/juju/lumberjack/v2"

	"github.com/juju/juju/core/paths"
)

// NewFileWriter returns an io.WriteCloser that will write log messages to disk.
func NewFileWriter(logPath string, maxSizeMB, maxBackups int) (io.WriteCloser, error) {
	if err := paths.PrimeLogFile(logPath); err != nil {
		// This isn't a fatal error so log and continue if priming fails.
		logger.Warningf("Unable to prime %s (proceeding anyway): %v", logPath, err)
	}
	ljLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		Compress:   true,
	}
	logger.Debugf("created rotating log file %q with max size %d MB and max backups %d",
		ljLogger.Filename, ljLogger.MaxSize, ljLogger.MaxBackups)
	return ljLogger, nil
}
