// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"github.com/juju/lumberjack/v2"

	"github.com/juju/juju/agent"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logsink"
)

const (
	batchSize     = 512
	flushInterval = 2 * time.Second
)

// PrimeLogSink sets up the logging sink for the agent.
func PrimeLogSink(cfg agent.Config) (*logsink.LogSink, error) {
	path := filepath.Join(cfg.LogDir(), "logsink.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.LogfilePermission)
	if err != nil {
		return nil, errors.Errorf("unable to open log file %q: %w", path, err)
	}
	if err := paths.SetSyslogOwner(path); err != nil {
		return nil, errors.Errorf("unable to set syslog owner on %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return nil, errors.Errorf("unable to close log file %q: %w", path, err)
	}

	logger := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    cfg.AgentLogfileMaxSizeMB(),
		MaxBackups: cfg.AgentLogfileMaxBackups(),
		Compress:   true,
	}

	return logsink.NewLogSink(logger, batchSize, flushInterval, clock.WallClock), nil
}

type TagWriter struct {
	LogSink   corelogger.LogSink
	Tag       string
	ModelUUID string
}

func (w TagWriter) Write(entry loggo.Entry) {
	var location string
	if entry.Filename != "" {
		location = entry.Filename + ":" + strconv.Itoa(entry.Line)
	}

	w.LogSink.Log([]corelogger.LogRecord{{
		Time:      entry.Timestamp,
		Module:    entry.Module,
		Entity:    w.Tag,
		Location:  location,
		Level:     corelogger.Level(entry.Level),
		Message:   entry.Message,
		Labels:    entry.Labels,
		ModelUUID: w.ModelUUID,
	}})
}
