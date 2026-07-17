// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/lumberjack/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logsink"
)

const (
	batchSize     = 512
	flushInterval = 2 * time.Second
)

// PrimeLogSink sets up the logging sink for the controller app.
// If maxSizeMB or maxBackups is zero, compiled-in defaults from
// controller.DefaultAgentLogfileMaxSize and
// controller.DefaultAgentLogfileMaxBackups are used.
func PrimeLogSink(logDir string, maxSizeMB, maxBackups int) (*logsink.LogSink, error) {
	if maxSizeMB == 0 {
		maxSizeMB = controller.DefaultAgentLogfileMaxSize
	}
	if maxBackups == 0 {
		maxBackups = controller.DefaultAgentLogfileMaxBackups
	}

	path := filepath.Join(logDir, "logsink.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.LogfilePermission)
	if err != nil {
		return nil, errors.Errorf("unable to create log sink file %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, errors.Errorf("unable to close log sink file %q: %w", path, err)
	}

	logger := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		Compress:   true,
	}

	return logsink.NewLogSink(logger, batchSize, flushInterval, clock.WallClock), nil
}
